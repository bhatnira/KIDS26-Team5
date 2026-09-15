package types

// LLMConfigAddDto is the request body for adding or updating a user LLM config.
type LLMConfigAddDto struct {
	Name               string  `json:"name" binding:"required"`
	Provider           string  `json:"provider" binding:"required"`
	APIKey             string  `json:"api_key" binding:"required"`
	Model              string  `json:"model" binding:"required"`
	BaseURL            string  `json:"base_url"`
	MaxTokens          int     `json:"max_tokens"`
	Temperature        float64 `json:"temperature"`
	ThinkingEnabled    bool    `json:"thinking_enabled"`
	ThinkingEffort     string  `json:"thinking_effort"`
	IsDefault          bool    `json:"is_default"`
	InsecureSkipVerify bool    `json:"insecure_skip_verify"`
}

// LLMConfigResponse is the response shape for a single config (API key is masked).
type LLMConfigResponse struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Provider           string  `json:"provider"`
	APIKey             string  `json:"api_key"` // masked
	Model              string  `json:"model"`
	BaseURL            string  `json:"base_url,omitempty"`
	MaxTokens          int     `json:"max_tokens"`
	Temperature        float64 `json:"temperature"`
	ThinkingEnabled    bool    `json:"thinking_enabled"`
	ThinkingEffort     string  `json:"thinking_effort,omitempty"`
	IsDefault          bool    `json:"is_default"`
	InsecureSkipVerify bool    `json:"insecure_skip_verify"`
}

// LLMTestConnectionDto is the request body for testing an LLM connection.
type LLMTestConnectionDto struct {
	Provider           string  `json:"provider" binding:"required"`
	APIKey             string  `json:"api_key" binding:"required"`
	Model              string  `json:"model" binding:"required"`
	BaseURL            string  `json:"base_url"`
	MaxTokens          int     `json:"max_tokens"`
	Temperature        float64 `json:"temperature"`
	ThinkingEnabled    bool    `json:"thinking_enabled"`
	ThinkingEffort     string  `json:"thinking_effort"`
	InsecureSkipVerify bool    `json:"insecure_skip_verify"`
}
