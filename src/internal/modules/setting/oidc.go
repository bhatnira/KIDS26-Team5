package setting

type OidcConfig struct {
	IssuerURL    string `json:"issuerUrl" yaml:"issuerUrl"`
	ClientID     string `json:"clientId" yaml:"clientId"`
	ClientSecret string `json:"clientSecret" yaml:"clientSecret"`
	RedirectURL  string `json:"redirectUrl" yaml:"redirectUrl"`
}
