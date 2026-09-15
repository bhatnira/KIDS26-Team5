package models

import (
	"gorm.io/gorm"
)

// AuthProviderType defines the type of authentication provider
type AuthProviderType string

const (
	AuthProviderBasic AuthProviderType = "basic"
	AuthProviderLDAP  AuthProviderType = "ldap"
	AuthProviderOIDC  AuthProviderType = "oidc"
)

// AuthProvider stores authentication provider configurations in the database
type AuthProvider struct {
	gorm.Model
	Name        string           `gorm:"type:varchar(64);uniqueIndex;not null" json:"name"`
	Type        AuthProviderType `gorm:"type:varchar(20);not null" json:"type"`
	Enabled     bool             `gorm:"default:false" json:"enabled"`
	DisplayName string           `gorm:"type:varchar(128)" json:"display_name"`
	Priority    int              `gorm:"default:0" json:"priority"` // Lower number = higher priority

	// LDAP-specific fields (stored as JSON or separate columns)
	LdapURL      string `gorm:"type:varchar(256)" json:"ldap_url,omitempty"`
	LdapBindDN   string `gorm:"type:varchar(256)" json:"ldap_bind_dn,omitempty"`
	LdapBindPass string `gorm:"type:varchar(256)" json:"-"` // secret: never serialise to clients
	LdapBaseDN   string `gorm:"type:varchar(256)" json:"ldap_base_dn,omitempty"`
	LdapFilter   string `gorm:"type:varchar(512)" json:"ldap_filter,omitempty"`
	LdapTLSCert  string `gorm:"type:text" json:"ldap_tls_cert,omitempty"`

	// OIDC-specific fields
	OidcIssuerURL    string `gorm:"type:varchar(512)" json:"oidc_issuer_url,omitempty"`
	OidcClientID     string `gorm:"type:varchar(256)" json:"oidc_client_id,omitempty"`
	OidcClientSecret string `gorm:"type:varchar(256)" json:"-"` // secret: never serialise to clients
	OidcRedirectURL  string `gorm:"type:varchar(512)" json:"oidc_redirect_url,omitempty"`
}

// TableName specifies the table name for AuthProvider
func (AuthProvider) TableName() string {
	return "auth_providers"
}

// IsLDAP checks if this is an LDAP provider
func (p *AuthProvider) IsLDAP() bool {
	return p.Type == AuthProviderLDAP
}

// IsOIDC checks if this is an OIDC provider
func (p *AuthProvider) IsOIDC() bool {
	return p.Type == AuthProviderOIDC
}

// IsBasic checks if this is a basic auth provider
func (p *AuthProvider) IsBasic() bool {
	return p.Type == AuthProviderBasic
}
