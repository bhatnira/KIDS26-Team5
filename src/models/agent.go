package models

import "gorm.io/gorm"

// MCPConfig stores a per-user (or global) MCP server connection.
// UserID == nil marks a globally-shared config administered by super users.
type MCPConfig struct {
	gorm.Model

	UserID         *uint  `gorm:"index;comment:'owner user id; null = global'" json:"user_id,omitempty"`
	Name           string `gorm:"type:varchar(128);not null;comment:'display name'" json:"name"`
	Transport      string `gorm:"type:varchar(16);not null;comment:'stdio | sse | streamable'" json:"transport"`
	ServerURL      string `gorm:"type:varchar(512);comment:'http endpoint for sse/streamable'" json:"server_url,omitempty"`
	HeadersJSON    string `gorm:"type:text;comment:'json map of auth/extra headers'" json:"headers_json,omitempty"`
	Command        string `gorm:"type:varchar(256);comment:'binary to spawn for stdio'" json:"command,omitempty"`
	ArgsJSON       string `gorm:"type:text;comment:'json array of args for stdio'" json:"args_json,omitempty"`
	TimeoutSeconds int    `gorm:"default:30;comment:'connect/request timeout'" json:"timeout_seconds"`
	Enabled        bool   `gorm:"default:true;comment:'whether the agent should load this server'" json:"enabled"`
	Description    string `gorm:"type:varchar(512);comment:'human description'" json:"description,omitempty"`
}

// AgentSkill catalogues a per-user (or global) skill bundle stored in object storage.
// UserID == nil marks a built-in/shared skill bundle visible to every user.
type AgentSkill struct {
	gorm.Model

	UserID      *uint  `gorm:"index;comment:'owner user id; null = global'" json:"user_id,omitempty"`
	Name        string `gorm:"type:varchar(128);not null;comment:'skill identifier (matches SKILL.md frontmatter)'" json:"name"`
	Domain      string `gorm:"type:varchar(64);comment:'skill domain (singlecell, literature, ...)'" json:"domain,omitempty"`
	Version     string `gorm:"type:varchar(32);comment:'skill version'" json:"version,omitempty"`
	Description string `gorm:"type:varchar(512);comment:'short description from SKILL.md'" json:"description,omitempty"`
	S3Bucket    string `gorm:"type:varchar(128);comment:'bucket holding the skill bundle (user skills only)'" json:"s3_bucket,omitempty"`
	S3Prefix    string `gorm:"type:varchar(512);comment:'object prefix within bucket'" json:"s3_prefix,omitempty"`
	SizeBytes   int64  `gorm:"comment:'bundle size in bytes'" json:"size_bytes,omitempty"`
	SHA256      string `gorm:"type:varchar(64);comment:'bundle content hash for change detection'" json:"sha256,omitempty"`
}

// AgentWorkspaceConfig holds a user's agent runtime preferences: which
// bucket the agent uses as its workspace and the Daytona credentials that
// back their chats' code execution.
type AgentWorkspaceConfig struct {
	gorm.Model

	UserID uint   `gorm:"uniqueIndex;not null;comment:'owner user id'" json:"user_id"`
	Bucket string `gorm:"type:varchar(128);not null;comment:'workspace bucket in user storage'" json:"bucket"`

	// Daytona credentials. APIURL is optional — the executor falls back to
	// DAYTONA_API_URL env when blank, or to the SDK default otherwise.
	//
	// DaytonaAPIKey is encrypted at rest (AES-256-GCM via pkg/secretbox) when
	// ANTELOPE_SYSTEM_ENCRYPT_KEY is set, mirroring the per-user storage/LLM
	// secrets; DaytonaKeyEncrypted records whether the stored value is
	// ciphertext so reads decrypt unambiguously and pre-encryption rows migrate
	// in place on their next save.
	DaytonaAPIKey       string `gorm:"type:text;comment:'user-supplied Daytona API key (encrypted at rest when a key is configured)'" json:"-"`
	DaytonaKeyEncrypted bool   `gorm:"not null;default:false;comment:'true when DaytonaAPIKey holds ciphertext'" json:"-"`
	DaytonaAPIURL       string `gorm:"type:varchar(256);comment:'Daytona API base URL (optional)'" json:"daytona_api_url,omitempty"`
}
