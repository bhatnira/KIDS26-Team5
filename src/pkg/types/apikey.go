package types

// APIKeyGenerateDto carries a request to generate a personal API key.
// ExpireDays is the lifetime in days; the frontend offers presets and a custom value.
type APIKeyGenerateDto struct {
	Name       string `json:"name" binding:"required,nonblank"`
	ExpireDays int    `json:"expire_days" binding:"required,min=1,max=365"`
}
