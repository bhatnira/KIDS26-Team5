package setting

type LdapConfig struct {
	Url      string         `json:"url" yaml:"url"`
	BindDN   string         `json:"bindDN" yaml:"bindDN"`
	BindPass string         `json:"bindPass" yaml:"bindPass"`
	BaseDN   string         `json:"baseDN" yaml:"baseDN"`
	Filter   string         `json:"filter" yaml:"filter"`
	Tls      *LdapTlsConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
}

type LdapTlsConfig struct {
	CaCert string `json:"caCert" yaml:"caCert"`
}
