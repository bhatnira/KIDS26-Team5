package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"antelope/internal/modules/auth/jwt"
	"antelope/internal/modules/auth/session"
	"antelope/internal/modules/log"
	"antelope/internal/modules/misc"
	setting2 "antelope/internal/modules/setting"
	"antelope/models"
	"antelope/pkg/apperr"
	"antelope/pkg/response"
	"antelope/pkg/secretbox"
	"antelope/pkg/types"
	"antelope/services/auth/source/ldap"
	"antelope/services/auth/source/local"
	"antelope/services/auth/source/oidc"

	"github.com/gin-gonic/gin"
	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// clockSkewTolerance is subtracted from NotBefore to accept tokens from peers
// whose clocks are slightly behind the issuer.
const clockSkewTolerance = 5 * time.Second

// Service is the combined interface for authentication and provider management.
type Service interface {
	// Login operations
	LocalLogin(c *gin.Context, dto types.LoginDto) (gin.H, error)
	LdapLogin(c *gin.Context, dto types.LdapLoginDto) (gin.H, error)
	// StartOidc begins an OIDC login: it mints a one-time, cryptographically
	// random state nonce (+ PKCE verifier), stores it in Redis, and returns the
	// provider authorization URL to redirect the browser to.
	StartOidc(c *gin.Context, providerName string) (gin.H, error)
	OidcCallback(c *gin.Context, dto types.OidcCallbackDto) (gin.H, error)
	RefreshToken(token string) (gin.H, error)
	Logout(c *gin.Context) error

	// IssueAPIKeyToken mints a long-lived access token for use as a personal API
	// key. It reuses the access-token signing path so the resulting token is
	// accepted by the standard auth middleware. Returns the signed token, its
	// JTI (for revocation), and absolute expiry.
	IssueAPIKeyToken(base BaseClaims, ttl time.Duration) (token, jti string, expiresAt time.Time, err error)

	// Auth provider management
	GetEnabledProviders() (gin.H, error)
	GetAllProviders() (gin.H, error)
	AddProvider(dto types.AuthProviderAddDto) (gin.H, error)
	UpdateProvider(dto types.AuthProviderUpdateDto) error
	DeleteProvider(id uint) error
	TestProvider(id uint) (gin.H, error)
}

type authService struct {
	db           *gorm.DB
	jwtCfg       setting2.JwtConfig
	production   bool
	sessionStore session.Store
	rdb          redis.UniversalClient
	box          *secretbox.Box // optional at-rest encryption for provider secrets (nil = plaintext)
}

// NewService constructs an AuthService. box may be nil, in which case provider
// secrets (LDAP bind password, OIDC client secret) are persisted as plaintext.
func NewService(db *gorm.DB, jwtCfg setting2.JwtConfig, production bool, sessionStore session.Store, rdb redis.UniversalClient, box *secretbox.Box) Service {
	return &authService{db: db, jwtCfg: jwtCfg, production: production, sessionStore: sessionStore, rdb: rdb, box: box}
}

const (
	// oidcStateTTL bounds how long a started OIDC login may sit before the
	// callback must arrive. Short enough to limit replay, long enough for a
	// human to authenticate at the IdP.
	oidcStateTTL    = 10 * time.Minute
	oidcStatePrefix = "oidc:state:"
)

// oidcStateEntry is the value stored in Redis under the one-time state nonce.
type oidcStateEntry struct {
	Provider string `json:"provider"`
	Verifier string `json:"verifier"`
}

func oidcStateKey(state string) string { return oidcStatePrefix + state }

// randToken returns n random bytes as a URL-safe base64 string.
func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ── Login operations ──────────────────────────────────────────────────────────

func (s *authService) LocalLogin(c *gin.Context, dto types.LoginDto) (gin.H, error) {
	provider := local.NewLocalProvider(s.db)
	user, err := provider.VerifyAndLoad(local.LocalSubject{Email: dto.Email, Password: dto.Password})
	if err != nil {
		switch {
		case errors.Is(err, local.ErrUserNotFound):
			return nil, apperr.Unauthorized(response.UserNotExist)
		case errors.Is(err, local.ErrUserDisabled):
			return nil, apperr.Forbidden("account is disabled")
		default:
			return nil, apperr.Unauthorized(response.EmailOrPasswordError)
		}
	}
	return s.buildTokenPair(c, BaseClaims{ID: user.ID, Email: user.Email, Role: user.Role})
}

func (s *authService) LdapLogin(c *gin.Context, dto types.LdapLoginDto) (gin.H, error) {
	var provider models.AuthProvider
	if err := s.db.Where("name = ? AND type = ? AND enabled = ?",
		dto.Provider, models.AuthProviderLDAP, true).First(&provider).Error; err != nil {
		return nil, apperr.Unauthorized("LDAP provider not found or disabled")
	}

	bindPass, err := s.decryptSecret(provider.LdapBindPass)
	if err != nil {
		log.L().Error("decrypt LDAP bind password", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	ldapCfg := setting2.LdapConfig{
		Url: provider.LdapURL, BindDN: provider.LdapBindDN, BindPass: bindPass,
		BaseDN: provider.LdapBaseDN, Filter: provider.LdapFilter,
	}
	if provider.LdapTLSCert != "" {
		ldapCfg.Tls = &setting2.LdapTlsConfig{CaCert: provider.LdapTLSCert}
	}

	lp := ldap.NewLdapProvider()
	if err := lp.SetConfig(ldapCfg); err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	username, err := lp.Verify(ldap.LdapSubject{Username: dto.Username, Password: dto.Password})
	if err != nil {
		log.L().Warn("LDAP auth failed", zap.String("username", dto.Username), zap.Error(err))
		return nil, apperr.Unauthorized("Invalid LDAP credentials")
	}

	user, err := s.findOrCreateExternalUser(username, provider.Name, models.AuthSourceLDAP)
	if err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	return s.buildTokenPair(c, BaseClaims{ID: user.ID, Email: user.Email, Role: user.Role})
}

// StartOidc mints a one-time random state nonce + PKCE verifier, persists them
// in Redis keyed by the nonce, and returns the provider authorization URL.
func (s *authService) StartOidc(c *gin.Context, providerName string) (gin.H, error) {
	var provider models.AuthProvider
	if err := s.db.Where("name = ? AND type = ? AND enabled = ?",
		providerName, models.AuthProviderOIDC, true).First(&provider).Error; err != nil {
		return nil, apperr.Unauthorized("OIDC provider not found or disabled")
	}

	clientSecret, err := s.decryptSecret(provider.OidcClientSecret)
	if err != nil {
		log.L().Error("decrypt OIDC client secret", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	op := oidc.NewOidcProvider()
	if err := op.SetConfig(setting2.OidcConfig{
		IssuerURL: provider.OidcIssuerURL, ClientID: provider.OidcClientID,
		ClientSecret: clientSecret, RedirectURL: provider.OidcRedirectURL,
	}); err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}

	// Cryptographically-random, single-use state binds the callback to this
	// login attempt (defeats login-CSRF / account-fixation); PKCE protects the
	// authorization code in transit.
	state, err := randToken(32)
	if err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	verifier := oidc.GeneratePKCEVerifier()
	op.SetPKCEVerifier(verifier)

	entry, err := json.Marshal(oidcStateEntry{Provider: provider.Name, Verifier: verifier})
	if err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	if err := s.rdb.Set(c.Request.Context(), oidcStateKey(state), entry, oidcStateTTL).Err(); err != nil {
		log.L().Error("failed to persist OIDC state", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}

	authURL, err := op.GetCode(state)
	if err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	return gin.H{"auth_url": authURL}, nil
}

func (s *authService) OidcCallback(c *gin.Context, dto types.OidcCallbackDto) (gin.H, error) {
	ctx := c.Request.Context()
	key := oidcStateKey(dto.State)

	// Atomically fetch-and-delete the one-time state. GETDEL closes the
	// check-then-act window that a separate GET+DEL leaves open: with multiple
	// pods, two concurrent callbacks carrying the same state could otherwise both
	// read it before either deleted it, and both pass this guard. A missing key
	// (redis.Nil) means it was never issued by us, already consumed, or expired —
	// reject (CSRF / login-fixation guard). Requires Redis >= 6.2.
	raw, err := s.rdb.GetDel(ctx, key).Result()
	if err != nil {
		return nil, apperr.BadRequest(response.FailCode, "invalid or expired OIDC state parameter")
	}

	var entry oidcStateEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}

	var provider models.AuthProvider
	if err := s.db.Where("name = ? AND type = ? AND enabled = ?",
		entry.Provider, models.AuthProviderOIDC, true).First(&provider).Error; err != nil {
		return nil, apperr.Unauthorized("OIDC provider not found or disabled")
	}

	clientSecret, err := s.decryptSecret(provider.OidcClientSecret)
	if err != nil {
		log.L().Error("decrypt OIDC client secret", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	op := oidc.NewOidcProvider()
	if err := op.SetConfig(setting2.OidcConfig{
		IssuerURL: provider.OidcIssuerURL, ClientID: provider.OidcClientID,
		ClientSecret: clientSecret, RedirectURL: provider.OidcRedirectURL,
	}); err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	op.SetPKCEVerifier(entry.Verifier)

	username, err := op.Verify(dto.Code)
	if err != nil {
		return nil, apperr.Unauthorized("OIDC authentication failed")
	}
	user, err := s.findOrCreateExternalUser(username, provider.Name, models.AuthSourceOIDC)
	if err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	return s.buildTokenPair(c, BaseClaims{ID: user.ID, Email: user.Email, Role: user.Role})
}

func (s *authService) RefreshToken(refreshToken string) (gin.H, error) {
	j := jwt.NewJWT(s.jwtCfg.AccessSigningKey, s.jwtCfg.RefreshSigningKey)
	claims, err := jwt.ParseRefresh[CustomClaims, *CustomClaims](j, refreshToken)
	if err != nil {
		return nil, apperr.Unauthorized(err.Error())
	}
	if claims.TokenType != "refresh" {
		return nil, apperr.Unauthorized(response.TokenInvalid)
	}

	var user models.User
	if err := s.db.First(&user, claims.BaseClaims.ID).Error; err != nil {
		return nil, apperr.Unauthorized(response.UserNotExist)
	}
	if user.Status == 0 {
		return nil, apperr.Forbidden("account is disabled")
	}

	accessToken, newRefresh, accessClaims, refreshClaims, err := s.loginToken(BaseClaims{
		ID: user.ID, Email: user.Email, Role: user.Role,
	})
	if err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	return gin.H{
		"accessToken":      accessToken,
		"refreshToken":     newRefresh,
		"expiresIn":        int(accessClaims.ExpiresAt.Unix() - accessClaims.NotBefore.Unix()),
		"accessExpiresAt":  accessClaims.ExpiresAt.Unix() * 1000,
		"refreshExpiresAt": refreshClaims.ExpiresAt.Unix() * 1000,
	}, nil
}

func (s *authService) Logout(c *gin.Context) error {
	if raw, exists := c.Get("claims"); exists {
		if claims, ok := raw.(*CustomClaims); ok && claims.JTI != "" {
			ttl := time.Until(claims.RegisteredClaims.ExpiresAt.Time)
			if ttl > 0 {
				if err := s.sessionStore.Revoke(c.Request.Context(), claims.JTI, ttl); err != nil {
					log.L().Warn("failed to revoke token JTI", zap.String("jti", claims.JTI), zap.Error(err))
				}
			}
		}
	}
	s.clearRefreshCookie(c)
	return nil
}

// ── Provider management ───────────────────────────────────────────────────────

func (s *authService) GetEnabledProviders() (gin.H, error) {
	var providers []models.AuthProvider
	if err := s.db.Where("enabled = ?", true).Order("priority ASC").Find(&providers).Error; err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	// The OIDC auth URL is no longer pre-baked here: it must carry a per-attempt,
	// one-time state nonce, so the frontend fetches it from GET
	// /auth/oidc/:provider/start (StartOidc) when the user initiates login.
	infos := make([]types.AuthProvidersInfoDto, 0, len(providers))
	for _, p := range providers {
		infos = append(infos, types.AuthProvidersInfoDto{
			Name: p.Name, Type: p.Type, DisplayName: p.DisplayName,
		})
	}
	return gin.H{"providers": infos}, nil
}

func (s *authService) GetAllProviders() (gin.H, error) {
	var providers []models.AuthProvider
	if err := s.db.Order("priority ASC").Find(&providers).Error; err != nil {
		return nil, apperr.ServerError(response.SystemError)
	}
	dtos := make([]types.AuthProviderDto, len(providers))
	for i, p := range providers {
		dtos[i] = types.AuthProviderDto{
			ID: p.ID, Name: p.Name, Type: p.Type, Enabled: p.Enabled,
			DisplayName: p.DisplayName, Priority: p.Priority,
			LdapURL: p.LdapURL, LdapBindDN: p.LdapBindDN, LdapBaseDN: p.LdapBaseDN, LdapFilter: p.LdapFilter,
			OidcIssuerURL: p.OidcIssuerURL, OidcClientID: p.OidcClientID, OidcRedirectURL: p.OidcRedirectURL,
		}
	}
	return gin.H{"providers": dtos}, nil
}

func (s *authService) AddProvider(dto types.AuthProviderAddDto) (gin.H, error) {
	var existing models.AuthProvider
	if err := s.db.Where("name = ?", dto.Name).First(&existing).Error; err == nil {
		return nil, apperr.CheckFail(response.CheckFailCode, "provider with this name already exists")
	}
	if err := validateProviderConfig(dto); err != nil {
		return nil, apperr.CheckFail(response.CheckFailCode, err.Error())
	}
	provider := dto.ToModel()
	var err error
	if provider.LdapBindPass, err = s.encryptSecret(provider.LdapBindPass); err != nil {
		log.L().Error("encrypt LDAP bind password", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	if provider.OidcClientSecret, err = s.encryptSecret(provider.OidcClientSecret); err != nil {
		log.L().Error("encrypt OIDC client secret", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	if err := s.db.Create(&provider).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// Lost a concurrent-create race; the unique index on name held.
			return nil, apperr.CheckFail(response.CheckFailCode, "provider with this name already exists")
		}
		return nil, apperr.ServerError(response.SystemError)
	}
	return gin.H{"id": provider.ID}, nil
}

func (s *authService) UpdateProvider(dto types.AuthProviderUpdateDto) error {
	var existing models.AuthProvider
	if err := s.db.First(&existing, dto.ID).Error; err != nil {
		return apperr.NotFound("provider not found")
	}
	updates := map[string]any{
		"name": dto.Name, "type": dto.Type, "enabled": dto.Enabled,
		"display_name": dto.DisplayName, "priority": dto.Priority,
		"ldap_url": dto.LdapURL, "ldap_bind_dn": dto.LdapBindDN,
		"ldap_base_dn": dto.LdapBaseDN, "ldap_filter": dto.LdapFilter,
		"ldap_tls_cert":   dto.LdapTLSCert,
		"oidc_issuer_url": dto.OidcIssuerURL, "oidc_client_id": dto.OidcClientID,
		"oidc_redirect_url": dto.OidcRedirectURL,
	}
	// Only overwrite a secret when a non-empty value is supplied, and store it
	// encrypted at rest (mirrors the storage/LLM managers).
	if dto.LdapBindPass != "" {
		enc, err := s.encryptSecret(dto.LdapBindPass)
		if err != nil {
			log.L().Error("encrypt LDAP bind password", zap.Error(err))
			return apperr.ServerError(response.SystemError)
		}
		updates["ldap_bind_pass"] = enc
	}
	if dto.OidcClientSecret != "" {
		enc, err := s.encryptSecret(dto.OidcClientSecret)
		if err != nil {
			log.L().Error("encrypt OIDC client secret", zap.Error(err))
			return apperr.ServerError(response.SystemError)
		}
		updates["oidc_client_secret"] = enc
	}
	if err := s.db.Model(&existing).Updates(updates).Error; err != nil {
		return apperr.ServerError(response.SystemError)
	}
	return nil
}

func (s *authService) DeleteProvider(id uint) error {
	result := s.db.Unscoped().Delete(&models.AuthProvider{}, id)
	if result.Error != nil {
		return apperr.ServerError(response.SystemError)
	}
	if result.RowsAffected == 0 {
		return apperr.NotFound("provider not found")
	}
	return nil
}

func (s *authService) TestProvider(id uint) (gin.H, error) {
	var provider models.AuthProvider
	if err := s.db.First(&provider, id).Error; err != nil {
		return nil, apperr.NotFound("provider not found")
	}
	switch provider.Type {
	case models.AuthProviderLDAP:
		bindPass, err := s.decryptSecret(provider.LdapBindPass)
		if err != nil {
			log.L().Error("decrypt LDAP bind password", zap.Error(err))
			return nil, apperr.ServerError(response.SystemError)
		}
		lp := ldap.NewLdapProvider()
		ldapCfg := setting2.LdapConfig{
			Url: provider.LdapURL, BindDN: provider.LdapBindDN, BindPass: bindPass,
			BaseDN: provider.LdapBaseDN, Filter: provider.LdapFilter,
		}
		if provider.LdapTLSCert != "" {
			ldapCfg.Tls = &setting2.LdapTlsConfig{CaCert: provider.LdapTLSCert}
		}
		if err := lp.Connect(ldapCfg); err != nil {
			return nil, apperr.CheckFail(response.CheckFailCode, fmt.Sprintf("LDAP connection failed: %v", err))
		}
	case models.AuthProviderOIDC:
		clientSecret, err := s.decryptSecret(provider.OidcClientSecret)
		if err != nil {
			log.L().Error("decrypt OIDC client secret", zap.Error(err))
			return nil, apperr.ServerError(response.SystemError)
		}
		op := oidc.NewOidcProvider()
		if err := op.SetConfig(setting2.OidcConfig{
			IssuerURL: provider.OidcIssuerURL, ClientID: provider.OidcClientID,
			ClientSecret: clientSecret, RedirectURL: provider.OidcRedirectURL,
		}); err != nil {
			return nil, apperr.CheckFail(response.CheckFailCode, fmt.Sprintf("OIDC config invalid: %v", err))
		}
	}
	return gin.H{"status": "connection successful"}, nil
}

// IssueAPIKeyToken mints a single long-lived access token (no refresh token, no
// cookie). It mirrors the access-token half of loginToken with a caller-supplied
// TTL.
func (s *authService) IssueAPIKeyToken(base BaseClaims, ttl time.Duration) (string, string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)
	jti := uuid.NewString()

	// Stamp the user's current token epoch so a later role/status change or
	// password reset can force-revoke this API key. Best-effort: a Redis blip
	// here just means epoch 0 (the token stays valid until the first bump).
	epoch, _ := s.sessionStore.CurrentEpoch(context.Background(), base.ID)

	claims := CustomClaims{
		BaseClaims: base, TokenType: "access", JTI: jti, Epoch: epoch,
		RegisteredClaims: gojwt.RegisteredClaims{
			Audience:  gojwt.ClaimStrings{"Antelope"},
			NotBefore: gojwt.NewNumericDate(now.Add(-clockSkewTolerance)),
			ExpiresAt: gojwt.NewNumericDate(expiresAt),
			Issuer:    s.jwtCfg.Issuer,
		},
	}

	j := jwt.NewJWT(s.jwtCfg.AccessSigningKey, s.jwtCfg.RefreshSigningKey)
	token, err := j.SignAccess(claims)
	if err != nil {
		log.L().Error("api key token generation failed", zap.Error(err))
		return "", "", time.Time{}, err
	}
	return token, jti, expiresAt, nil
}

// ── Token helpers (private) ───────────────────────────────────────────────────

func (s *authService) loginToken(base BaseClaims) (accessToken, refreshToken string, accessClaims, refreshClaims gojwt.RegisteredClaims, err error) {
	accessExpire, err := misc.ParseDuration(s.jwtCfg.AccessExpiresTime)
	if err != nil {
		return accessToken, refreshToken, accessClaims, refreshClaims, err
	}
	refreshExpire, err := misc.ParseDuration(s.jwtCfg.RefreshExpiresTime)
	if err != nil {
		return accessToken, refreshToken, accessClaims, refreshClaims, err
	}
	now := time.Now()
	audience := gojwt.ClaimStrings{"Antelope"}

	// Stamp the current token epoch so role/status changes (or a password reset)
	// can force-revoke this session. Best-effort on a Redis error (epoch 0).
	epoch, _ := s.sessionStore.CurrentEpoch(context.Background(), base.ID)

	accClaims := CustomClaims{
		BaseClaims: base, TokenType: "access", JTI: uuid.NewString(), Epoch: epoch,
		RegisteredClaims: gojwt.RegisteredClaims{
			Audience:  audience,
			NotBefore: gojwt.NewNumericDate(now.Add(-clockSkewTolerance)),
			ExpiresAt: gojwt.NewNumericDate(now.Add(accessExpire)),
			Issuer:    s.jwtCfg.Issuer,
		},
	}
	refClaims := CustomClaims{
		BaseClaims: base, TokenType: "refresh", JTI: uuid.NewString(), Epoch: epoch,
		RegisteredClaims: gojwt.RegisteredClaims{
			Audience:  audience,
			NotBefore: gojwt.NewNumericDate(now.Add(-clockSkewTolerance)),
			ExpiresAt: gojwt.NewNumericDate(now.Add(refreshExpire)),
			Issuer:    s.jwtCfg.Issuer,
		},
	}

	j := jwt.NewJWT(s.jwtCfg.AccessSigningKey, s.jwtCfg.RefreshSigningKey)
	accessToken, err = j.SignAccess(accClaims)
	if err != nil {
		return accessToken, refreshToken, accessClaims, refreshClaims, err
	}
	refreshToken, err = j.SignRefresh(refClaims)
	accessClaims = accClaims.RegisteredClaims
	refreshClaims = refClaims.RegisteredClaims
	return accessToken, refreshToken, accessClaims, refreshClaims, err
}

func (s *authService) buildTokenPair(c *gin.Context, base BaseClaims) (gin.H, error) {
	access, refresh, accClaims, refClaims, err := s.loginToken(base)
	if err != nil {
		log.L().Error("token generation failed", zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	ttl := time.Until(refClaims.ExpiresAt.Time)
	s.setRefreshCookie(c, refresh, ttl)

	// Enrich userInfo with display fields the frontend needs (avatar initial,
	// profile prefill). Best-effort: token issuance must not fail if this lookup does.
	var profile models.User
	s.db.Select("name", "department", "auth_source").First(&profile, base.ID)

	return gin.H{
		"accessToken":      access,
		"refreshToken":     refresh,
		"expiresIn":        int(accClaims.ExpiresAt.Unix() - accClaims.NotBefore.Unix()),
		"accessExpiresAt":  accClaims.ExpiresAt.Unix() * 1000,
		"refreshExpiresAt": refClaims.ExpiresAt.Unix() * 1000,
		"userInfo": gin.H{
			"id":          base.ID,
			"email":       base.Email,
			"role":        base.Role,
			"name":        profile.Name,
			"department":  profile.Department,
			"auth_source": profile.AuthSource,
		},
	}, nil
}

func (s *authService) setRefreshCookie(c *gin.Context, token string, ttl time.Duration) {
	host := requestHost(c)
	maxAge := int(ttl.Seconds())
	if net.ParseIP(host) != nil {
		c.SetCookie("refreshToken", token, maxAge, "/", "", s.production, true)
	} else {
		c.SetCookie("refreshToken", token, maxAge, "/", host, s.production, true)
	}
}

func (s *authService) clearRefreshCookie(c *gin.Context) {
	host := requestHost(c)
	if net.ParseIP(host) != nil {
		c.SetCookie("refreshToken", "", -1, "/", "", s.production, true)
	} else {
		c.SetCookie("refreshToken", "", -1, "/", host, s.production, true)
	}
}

func requestHost(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.Host)
	if err != nil {
		return c.Request.Host
	}
	return host
}

// ── Helper: find or create external user ─────────────────────────────────────

// findOrCreateExternalUser looks up an external user by (external_id, auth_provider)
// and creates one if not found.
//
// CONC-3: The original implementation had a TOCTOU race: two concurrent logins
// for the same new user both saw "not found" and both tried to INSERT, causing one
// to fail with a duplicate-key error surfaced as a generic 500. The fix re-checks
// inside a transaction so only one INSERT succeeds; the other reads the freshly
// committed row.
func (s *authService) findOrCreateExternalUser(username, providerName string, authSource models.AuthSource) (*models.User, error) {
	var user models.User

	// Fast path: user already exists — no transaction needed.
	if err := s.db.Where("external_id = ? AND auth_provider = ?", username, providerName).First(&user).Error; err == nil {
		return &user, nil
	}

	// Slow path: upsert inside a serializable transaction to prevent duplicate inserts.
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Re-check inside the transaction to close the TOCTOU window.
		if err := tx.Where("external_id = ? AND auth_provider = ?", username, providerName).First(&user).Error; err == nil {
			return nil
		}

		if strings.Contains(username, "@") {
			if err := tx.Where("email = ?", username).First(&user).Error; err == nil {
				user.AuthSource = authSource
				user.AuthProvider = providerName
				user.ExternalID = username
				return tx.Save(&user).Error
			}
		}

		emailAddr := username
		if !strings.Contains(username, "@") {
			emailAddr = username + "@external"
		}
		user = models.User{
			Name: username, Email: emailAddr,
			AuthSource: authSource, AuthProvider: providerName, ExternalID: username,
			Status: 1, Role: "user",
		}
		return tx.Create(&user).Error
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// validateProviderConfig validates provider-specific configuration.
func validateProviderConfig(in types.AuthProviderAddDto) error {
	switch in.Type {
	case models.AuthProviderLDAP:
		if in.LdapURL == "" {
			return errors.New("LDAP URL is required")
		}
		if in.LdapBindDN == "" {
			return errors.New("LDAP Bind DN is required")
		}
		if in.LdapBindPass == "" {
			return errors.New("LDAP Bind Password is required")
		}
		if in.LdapBaseDN == "" {
			return errors.New("LDAP Base DN is required")
		}
		if in.LdapFilter == "" {
			return errors.New("LDAP Filter is required")
		}
	case models.AuthProviderOIDC:
		if in.OidcIssuerURL == "" {
			return errors.New("OIDC Issuer URL is required")
		}
		if in.OidcClientID == "" {
			return errors.New("OIDC Client ID is required")
		}
		if in.OidcClientSecret == "" {
			return errors.New("OIDC Client Secret is required")
		}
		if in.OidcRedirectURL == "" {
			return errors.New("OIDC Redirect URL is required")
		}
	case models.AuthProviderBasic:
		// no extra validation
	default:
		return errors.New("unknown provider type")
	}
	return nil
}
