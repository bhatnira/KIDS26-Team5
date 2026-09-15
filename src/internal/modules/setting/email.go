package setting

type EmailConfig struct {
	Host     string `mapstructure:"host" json:"host" yaml:"host"`
	Port     int    `mapstructure:"port" json:"port" yaml:"port"`
	From     string `mapstructure:"from" json:"from" yaml:"from"`
	Nickname string `mapstructure:"nickname" json:"nickname" yaml:"nickname"`
	Username string `mapstructure:"username" json:"username" yaml:"username"`
	Password string `mapstructure:"password" json:"password" yaml:"password"`
	// AuthType controls the SMTP authentication mechanism.
	// Accepted values: "plain" (default), "login", "crammd5", "none".
	// NOTE: quote this in YAML — an unquoted `no`/`none` is otherwise parsed
	// as a boolean by YAML and silently falls back to "plain".
	AuthType string `mapstructure:"auth-type" json:"auth-type" yaml:"auth-type"`
	// TLSPolicy controls how TLS is negotiated with the SMTP server.
	// Accepted values: "opportunistic" (default), "mandatory", "disabled".
	TLSPolicy string `mapstructure:"tls-policy" json:"tls-policy" yaml:"tls-policy"`
}
