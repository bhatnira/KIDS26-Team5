package ldap

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"antelope/internal/modules/setting"
	"antelope/services/auth/source"

	gldap "github.com/go-ldap/ldap/v3"
	"go.uber.org/zap"
)

var (
	ErrUrlNotSpecified         = errors.New("url is not specified")
	ErrBindDNNotSpecified      = errors.New("BindDN is not specified")
	ErrBindPassNotSpecified    = errors.New("BindPass is not specified")
	ErrBaseDNNotSpecified      = errors.New("BaseDN is not specified")
	ErrFilterNotSpecified      = errors.New("filter is not specified")
	ErrSchemeNotLdaps          = errors.New("scheme is not ldaps")
	ErrLdapConfigNotConfigured = errors.New("config is not configured")
	ErrUserNotFoundOrTooMany   = errors.New("user isn't found or too many entries returned")
	ErrFailedToAppendCACert    = errors.New("failed to append CA cert")
)

// LdapSubject holds the credentials forwarded to the LDAP provider.
type LdapSubject struct {
	Username string
	Password string
}

// compile-time interface assertion
var _ source.Provider[setting.LdapConfig, LdapSubject] = (*LdapProvider)(nil)

// LdapProvider authenticates users against an LDAP/LDAPS directory.
type LdapProvider struct {
	config *setting.LdapConfig
}

func NewLdapProvider() *LdapProvider {
	return &LdapProvider{}
}

func (p *LdapProvider) Configured() bool {
	return p.config != nil
}

func (p *LdapProvider) SetConfig(cfg setting.LdapConfig) error {
	if cfg.Url == "" {
		return ErrUrlNotSpecified
	}
	if cfg.BindDN == "" {
		return ErrBindDNNotSpecified
	}
	if cfg.BindPass == "" {
		return ErrBindPassNotSpecified
	}
	if cfg.BaseDN == "" {
		return ErrBaseDNNotSpecified
	}
	if cfg.Filter == "" {
		return ErrFilterNotSpecified
	}
	if cfg.Tls != nil && !strings.HasPrefix(cfg.Url, "ldaps://") {
		return ErrSchemeNotLdaps
	}
	p.config = &cfg
	return nil
}

func (p *LdapProvider) DeleteConfig() {
	p.config = nil
}

func (p *LdapProvider) Connect(cfg setting.LdapConfig) error {
	conn, err := p.getConnection(cfg)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Verify authenticates the subject and returns the username on success.
func (p *LdapProvider) Verify(subject LdapSubject) (string, error) {
	if !p.Configured() {
		return "", ErrLdapConfigNotConfigured
	}
	conn, err := p.getConnection(*p.config)
	defer func(conn *gldap.Conn) {
		if conn == nil {
			return
		}
		if err := conn.Close(); err != nil {
			zap.L().Error("failed to close ldap connection", zap.Error(err))
		}
	}(conn)
	if err != nil {
		return "", err
	}

	searchRequest := gldap.NewSearchRequest(
		p.config.BaseDN,
		gldap.ScopeWholeSubtree, gldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(p.config.Filter, gldap.EscapeFilter(subject.Username)),
		[]string{"dn"}, nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return "", err
	}
	if len(result.Entries) != 1 {
		return "", ErrUserNotFoundOrTooMany
	}

	userDN := result.Entries[0].DN
	if err := conn.Bind(userDN, subject.Password); err != nil {
		return "", fmt.Errorf("invalid credentials: %w", err)
	}

	return subject.Username, nil
}

func (p *LdapProvider) getConnection(cfg setting.LdapConfig) (*gldap.Conn, error) {
	dialer := &net.Dialer{Timeout: 3 * time.Second}

	tlsConfig := &tls.Config{}
	if cfg.Tls != nil && cfg.Tls.CaCert != "" {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(cfg.Tls.CaCert)); !ok {
			return nil, ErrFailedToAppendCACert
		}
		tlsConfig.RootCAs = certPool
	} else {
		tlsConfig.InsecureSkipVerify = true
	}

	options := []gldap.DialOpt{
		gldap.DialWithDialer(dialer),
		gldap.DialWithTLSConfig(tlsConfig),
	}

	conn, err := gldap.DialURL(cfg.Url, options...)
	if err != nil {
		zap.L().Error("failed to connect to ldap server", zap.Error(err))
		return conn, err
	}
	conn.SetTimeout(3 * time.Second)
	if err = conn.Bind(cfg.BindDN, cfg.BindPass); err != nil {
		zap.L().Error("failed to bind to ldap server", zap.Error(err))
		return conn, err
	}

	return conn, nil
}
