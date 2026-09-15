// Package agent is the service layer for the AI agent chat surface.
// It owns conversation CRUD on top of the framework session.Service, the
// SSE-based streaming endpoint that drives the agent runner, and CRUD for
// agent settings (workspace, MCP, skills) + artifact listing.
package agent

import (
	"context"
	"errors"

	agentmod "antelope/internal/modules/agent"
	"antelope/internal/modules/setting"
	"antelope/internal/modules/sse"
	"antelope/internal/modules/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"trpc.group/trpc-go/trpc-agent-go/session"
)

// Service is the public contract consumed by HTTP handlers.
type Service interface {
	// Chat surface.
	CreateConversation(ctx context.Context, userID uint, title string) (gin.H, error)
	ListConversations(ctx context.Context, userID uint, limit, offset int) (gin.H, error)
	GetConversation(ctx context.Context, userID uint, sessionID string) (gin.H, error)
	DeleteConversation(ctx context.Context, userID uint, sessionID string) error
	SendMessageStream(c *gin.Context, userID uint, sessionID, content string, attachments []agentmod.AttachmentRef)

	// Workspace settings.
	GetWorkspaceConfig(ctx context.Context, userID uint) (gin.H, error)
	UpdateWorkspaceConfig(ctx context.Context, userID uint, dto WorkspaceConfigDto) (gin.H, error)

	// MCP settings.
	ListMCPConfigs(ctx context.Context, userID uint) (gin.H, error)
	AddMCPConfig(ctx context.Context, userID uint, dto MCPConfigDto) (gin.H, error)
	UpdateMCPConfig(ctx context.Context, userID, id uint, dto MCPConfigDto) (gin.H, error)
	DeleteMCPConfig(ctx context.Context, userID, id uint) error

	// Skills (read-only: the built-in library ships with the binary; user
	// upload comes with a later step).
	ListSkills(ctx context.Context, userID uint, q SkillQuery) (gin.H, error)
	GetSkill(ctx context.Context, userID uint, name string) (gin.H, error)

	// Artifacts.
	ListArtifacts(ctx context.Context, userID uint, sessionID string) (gin.H, error)
	GetArtifactDownloadURL(ctx context.Context, userID uint, sessionID, name string, version int) (gin.H, error)
}

// Deps bundles every infrastructure dependency the service needs.
type Deps struct {
	Agent   *agentmod.Manager
	DB      *gorm.DB
	Storage *storage.ClientManager
	SSE     *sse.Manager
	Cfg     setting.AgentConfig
}

type service struct {
	agent   *agentmod.Manager
	db      *gorm.DB
	storage *storage.ClientManager
	sse     *sse.Manager
	cfg     setting.AgentConfig
	session session.Service
}

// NewService validates the deps and returns a ready Service.
func NewService(d Deps) Service {
	if d.Agent == nil || d.DB == nil || d.Storage == nil || d.SSE == nil {
		panic("services/agent: agent, db, storage, and sse are required")
	}
	if d.Cfg.AppName == "" {
		d.Cfg.AppName = agentmod.AppName
	}
	return &service{
		agent:   d.Agent,
		db:      d.DB,
		storage: d.Storage,
		sse:     d.SSE,
		cfg:     d.Cfg,
		session: d.Agent.Session(),
	}
}

// ── shared helpers ──────────────────────────────────────────────────────────

var errUserIDRequired = errors.New("user id is required")
