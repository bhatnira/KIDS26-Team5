package setting

type JwtConfig struct {
	AccessSigningKey   string `mapstructure:"access-signing-key" json:"access-signing-key" yaml:"access-signing-key"`
	AccessExpiresTime  string `mapstructure:"access-expires-time" json:"access-expires-time" yaml:"access-expires-time"`
	RefreshSigningKey  string `mapstructure:"refresh-signing-key" json:"refresh-signing-key" yaml:"refresh-signing-key"`
	RefreshExpiresTime string `mapstructure:"refresh-expires-time" json:"refresh-expires-time" yaml:"refresh-expires-time"`
	Issuer             string `mapstructure:"issuer" json:"issuer" yaml:"issuer"`
}
