package types

import (
	"antelope/models"
)

// AuthProviderDto represents an auth provider for API responses
type AuthProviderDto struct {
	ID          uint                    `json:"id"`
	Name        string                  `json:"name"`
	Type        models.AuthProviderType `json:"type"`
	Enabled     bool                    `json:"enabled"`
	DisplayName string                  `json:"display_name"`
	Priority    int                     `json:"priority"`

	// LDAP fields (sensitive data excluded)
	LdapURL    string `json:"ldap_url,omitempty"`
	LdapBindDN string `json:"ldap_bind_dn,omitempty"`
	LdapBaseDN string `json:"ldap_base_dn,omitempty"`
	LdapFilter string `json:"ldap_filter,omitempty"`

	// OIDC fields (sensitive data excluded)
	OidcIssuerURL   string `json:"oidc_issuer_url,omitempty"`
	OidcClientID    string `json:"oidc_client_id,omitempty"`
	OidcRedirectURL string `json:"oidc_redirect_url,omitempty"`
}

// AuthProviderAddDto for creating a new auth provider
type AuthProviderAddDto struct {
	Name        string                  `json:"name" binding:"required,nonblank"`
	Type        models.AuthProviderType `json:"type" binding:"required"`
	Enabled     bool                    `json:"enabled"`
	DisplayName string                  `json:"display_name"`
	Priority    int                     `json:"priority"`

	// LDAP fields
	LdapURL      string `json:"ldap_url"`
	LdapBindDN   string `json:"ldap_bind_dn"`
	LdapBindPass string `json:"ldap_bind_pass"`
	LdapBaseDN   string `json:"ldap_base_dn"`
	LdapFilter   string `json:"ldap_filter"`
	LdapTLSCert  string `json:"ldap_tls_cert"`

	// OIDC fields
	OidcIssuerURL    string `json:"oidc_issuer_url"`
	OidcClientID     string `json:"oidc_client_id"`
	OidcClientSecret string `json:"oidc_client_secret"`
	OidcRedirectURL  string `json:"oidc_redirect_url"`
}

// ToModel converts DTO to model
func (d *AuthProviderAddDto) ToModel() models.AuthProvider {
	return models.AuthProvider{
		Name:             d.Name,
		Type:             d.Type,
		Enabled:          d.Enabled,
		DisplayName:      d.DisplayName,
		Priority:         d.Priority,
		LdapURL:          d.LdapURL,
		LdapBindDN:       d.LdapBindDN,
		LdapBindPass:     d.LdapBindPass,
		LdapBaseDN:       d.LdapBaseDN,
		LdapFilter:       d.LdapFilter,
		LdapTLSCert:      d.LdapTLSCert,
		OidcIssuerURL:    d.OidcIssuerURL,
		OidcClientID:     d.OidcClientID,
		OidcClientSecret: d.OidcClientSecret,
		OidcRedirectURL:  d.OidcRedirectURL,
	}
}

// AuthProviderUpdateDto for updating an auth provider
type AuthProviderUpdateDto struct {
	ID          uint                    `json:"id" binding:"required"`
	Name        string                  `json:"name" binding:"required,nonblank"`
	Type        models.AuthProviderType `json:"type" binding:"required"`
	Enabled     bool                    `json:"enabled"`
	DisplayName string                  `json:"display_name"`
	Priority    int                     `json:"priority"`

	// LDAP fields
	LdapURL      string `json:"ldap_url"`
	LdapBindDN   string `json:"ldap_bind_dn"`
	LdapBindPass string `json:"ldap_bind_pass"` // Only update if not empty
	LdapBaseDN   string `json:"ldap_base_dn"`
	LdapFilter   string `json:"ldap_filter"`
	LdapTLSCert  string `json:"ldap_tls_cert"`

	// OIDC fields
	OidcIssuerURL    string `json:"oidc_issuer_url"`
	OidcClientID     string `json:"oidc_client_id"`
	OidcClientSecret string `json:"oidc_client_secret"` // Only update if not empty
	OidcRedirectURL  string `json:"oidc_redirect_url"`
}

// LdapLoginDto for LDAP login request
type LdapLoginDto struct {
	Username string `json:"username" binding:"required,nonblank"`
	Password string `json:"password" binding:"required,nonblank"`
	Provider string `json:"provider" binding:"required,nonblank"` // Provider name
}

// OidcCallbackDto for OIDC callback. The provider is resolved server-side from
// the one-time state nonce (stored in Redis by StartOidc), so the client only
// needs to return the code and state it received from the IdP.
type OidcCallbackDto struct {
	Code  string `json:"code" binding:"required,nonblank"`
	State string `json:"state" binding:"required,nonblank"`
}

// AuthProvidersInfoDto for public auth providers info
type AuthProvidersInfoDto struct {
	Name        string                  `json:"name"`
	Type        models.AuthProviderType `json:"type"`
	DisplayName string                  `json:"display_name"`
	// For OIDC, include the auth URL
	AuthURL string `json:"auth_url,omitempty"`
}
