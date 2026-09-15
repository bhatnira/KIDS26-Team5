package types

// UserStorageConfigDto represents the user's storage configuration
type UserStorageConfigDto struct {
	Host               string `json:"host" binding:"required,nonblank"`
	Port               int    `json:"port" binding:"required,min=1,max=65535"`
	AccessKey          string `json:"access_key" binding:"required,nonblank"`
	SecretKey          string `json:"secret_key" binding:"required,nonblank"`
	UseSSL             bool   `json:"use_ssl"`
	Region             string `json:"region"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
}

// UserStorageConfigResponse represents the response for user storage config
// Note: SecretKey is masked for security
type UserStorageConfigResponse struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	AccessKey          string `json:"access_key"`
	SecretKey          string `json:"secret_key"` // Will be masked
	UseSSL             bool   `json:"use_ssl"`
	Region             string `json:"region"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	Configured         bool   `json:"configured"`
}

// TestStorageConnectionDto is the request to test storage connection
type TestStorageConnectionDto struct {
	Host               string `json:"host" binding:"required,nonblank"`
	Port               int    `json:"port" binding:"required,min=1,max=65535"`
	AccessKey          string `json:"access_key" binding:"required,nonblank"`
	SecretKey          string `json:"secret_key" binding:"required,nonblank"`
	UseSSL             bool   `json:"use_ssl"`
	Region             string `json:"region"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
}
