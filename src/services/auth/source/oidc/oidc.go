package oidc

import (
	"context"
	"errors"

	"antelope/internal/modules/setting"
	"antelope/services/auth/source"

	goidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var (
	ErrIssuerURLNotSpecified    = errors.New("IssuerURL is not specified")
	ErrClientIDNotSpecified     = errors.New("ClientID is not specified")
	ErrClientSecretNotSpecified = errors.New("ClientSecret is not specified")
	ErrRedirectURLNotSpecified  = errors.New("RedirectURL is not specified")
	ErrFailedToExchangeToken    = errors.New("failed to exchange token")
	ErrNoIDTokenField           = errors.New("no id_token field in oauth2 token")
	ErrFailedToVerifyIDToken    = errors.New("failed to verify ID Token")
	ErrNoRightClaim             = errors.New("there is no right claim in ID Token (it should have any of these preferred_username, email, nickname)")
	ErrOidcConfigNotConfigured  = errors.New("config is not configured")
)

// compile-time interface assertion
var _ source.Provider[setting.OidcConfig, string] = (*OidcProvider)(nil)

// OidcProvider authenticates users via an OpenID Connect provider.
type OidcProvider struct {
	config        *setting.OidcConfig
	oauthConfig   *oauth2.Config
	oauthVerifier *goidc.IDTokenVerifier

	// pkceVerifier, when set, enables PKCE: GetCode sends the S256 challenge and
	// Verify replays the verifier on the token exchange. It must be persisted
	// between the start and callback requests (it is stored in Redis alongside
	// the one-time state nonce).
	pkceVerifier string
}

func NewOidcProvider() *OidcProvider {
	return &OidcProvider{}
}

// GeneratePKCEVerifier returns a new random PKCE code verifier. Keeping the
// oauth2 dependency here means callers never import oauth2 directly.
func GeneratePKCEVerifier() string {
	return oauth2.GenerateVerifier()
}

// SetPKCEVerifier sets the PKCE code verifier used by GetCode/Verify.
func (p *OidcProvider) SetPKCEVerifier(verifier string) {
	p.pkceVerifier = verifier
}

func (p *OidcProvider) Configured() bool {
	return p.config != nil
}

func (p *OidcProvider) SetConfig(cfg setting.OidcConfig) error {
	if cfg.IssuerURL == "" {
		return ErrIssuerURLNotSpecified
	}
	if cfg.ClientID == "" {
		return ErrClientIDNotSpecified
	}
	if cfg.ClientSecret == "" {
		return ErrClientSecretNotSpecified
	}
	if cfg.RedirectURL == "" {
		return ErrRedirectURLNotSpecified
	}
	p.config = &cfg
	return nil
}

func (p *OidcProvider) DeleteConfig() {
	p.config = nil
	p.oauthConfig = nil
	p.oauthVerifier = nil
}

// Connect is a no-op for OIDC (initialization is lazy via initialize()).
func (p *OidcProvider) Connect(_ setting.OidcConfig) error { return nil }

// GetCode returns the authorization URL for starting the OIDC flow. When a PKCE
// verifier is set it appends the S256 code challenge.
func (p *OidcProvider) GetCode(state string) (string, error) {
	if err := p.initialize(); err != nil {
		return "", err
	}
	var opts []oauth2.AuthCodeOption
	if p.pkceVerifier != "" {
		opts = append(opts, oauth2.S256ChallengeOption(p.pkceVerifier))
	}
	return p.oauthConfig.AuthCodeURL(state, opts...), nil
}

// Verify exchanges an authorization code for tokens and returns the user's identity.
func (p *OidcProvider) Verify(code string) (string, error) {
	if err := p.initialize(); err != nil {
		return "", err
	}
	var opts []oauth2.AuthCodeOption
	if p.pkceVerifier != "" {
		opts = append(opts, oauth2.VerifierOption(p.pkceVerifier))
	}
	oauthToken, errExchange := p.oauthConfig.Exchange(context.Background(), code, opts...)
	if errExchange != nil {
		return "", errors.Join(ErrFailedToExchangeToken, errExchange)
	}

	rawIDToken, ok := oauthToken.Extra("id_token").(string)
	if !ok {
		return "", ErrNoIDTokenField
	}

	idToken, errVerify := p.oauthVerifier.Verify(context.Background(), rawIDToken)
	if errVerify != nil {
		return "", errors.Join(ErrFailedToVerifyIDToken, errVerify)
	}

	// See https://openid.net/specs/openid-connect-core-1_0.html#ScopeClaims
	var claims struct {
		Username string `json:"preferred_username"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", err
	}
	if claims.Username != "" {
		return claims.Username, nil
	}
	if claims.Email != "" {
		return claims.Email, nil
	}
	if claims.Nickname != "" {
		return claims.Nickname, nil
	}
	return "", ErrNoRightClaim
}

func (p *OidcProvider) initialize() error {
	if p.config == nil {
		return ErrOidcConfigNotConfigured
	}
	provider, err := goidc.NewProvider(context.Background(), p.config.IssuerURL)
	if err != nil {
		return err
	}

	p.oauthConfig = &oauth2.Config{
		ClientID:     p.config.ClientID,
		ClientSecret: p.config.ClientSecret,
		RedirectURL:  p.config.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{goidc.ScopeOpenID, "profile", "email"},
	}
	p.oauthVerifier = provider.Verifier(&goidc.Config{ClientID: p.config.ClientID})
	return nil
}
