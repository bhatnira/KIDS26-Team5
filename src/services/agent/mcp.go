package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"antelope/internal/modules/log"
	"antelope/models"
	"antelope/pkg/apperr"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MCPConfigDto is the request shape for create/update of an MCP config.
type MCPConfigDto struct {
	Name           string            `json:"name"`
	Transport      string            `json:"transport"`
	ServerURL      string            `json:"server_url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Enabled        *bool             `json:"enabled,omitempty"`
	Description    string            `json:"description,omitempty"`
}

// ListMCPConfigs returns the user's MCP configs plus any globally-shared
// ones (UserID == NULL). Sensitive header values stay as-is — the same
// JWT-protected route serves them back to the same user.
func (s *service) ListMCPConfigs(ctx context.Context, userID uint) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}
	var configs []models.MCPConfig
	err := s.db.WithContext(ctx).
		Where("user_id = ? OR user_id IS NULL", userID).
		Order("user_id IS NULL DESC, id ASC").
		Find(&configs).Error
	if err != nil {
		log.L().Error("list mcp configs failed", zap.Error(err))
		return nil, apperr.ServerError("failed to list MCP configs")
	}
	items := make([]gin.H, 0, len(configs))
	for _, c := range configs {
		items = append(items, mcpConfigView(c, userID))
	}
	return gin.H{"items": items}, nil
}

// AddMCPConfig stores a new user-scoped MCP config and evicts the user's
// MCP pool cache so the next chat turn rebuilds with the new server.
func (s *service) AddMCPConfig(ctx context.Context, userID uint, dto MCPConfigDto) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}
	if err := validateMCPDto(dto); err != nil {
		return nil, err
	}
	cfg := models.MCPConfig{
		UserID:         pUint(userID),
		Name:           strings.TrimSpace(dto.Name),
		Transport:      strings.ToLower(strings.TrimSpace(dto.Transport)),
		ServerURL:      strings.TrimSpace(dto.ServerURL),
		HeadersJSON:    mustJSON(dto.Headers),
		Command:        strings.TrimSpace(dto.Command),
		ArgsJSON:       mustJSON(dto.Args),
		TimeoutSeconds: dto.TimeoutSeconds,
		Enabled:        deref(dto.Enabled, true),
		Description:    strings.TrimSpace(dto.Description),
	}
	if err := s.db.WithContext(ctx).Create(&cfg).Error; err != nil {
		log.L().Error("create mcp config failed", zap.Error(err))
		return nil, apperr.ServerError("failed to save MCP config")
	}
	s.agent.MCP().Invalidate(userID)
	return mcpConfigView(cfg, userID), nil
}

// UpdateMCPConfig updates a user-owned config. Global rows (UserID NULL)
// are admin-managed and cannot be edited through this endpoint.
func (s *service) UpdateMCPConfig(ctx context.Context, userID, id uint, dto MCPConfigDto) (gin.H, error) {
	if userID == 0 {
		return nil, errUserIDRequired
	}
	if err := validateMCPDto(dto); err != nil {
		return nil, err
	}

	var cfg models.MCPConfig
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NotFound("MCP config not found")
		}
		log.L().Error("load mcp config failed", zap.Error(err))
		return nil, apperr.ServerError("failed to load MCP config")
	}

	cfg.Name = strings.TrimSpace(dto.Name)
	cfg.Transport = strings.ToLower(strings.TrimSpace(dto.Transport))
	cfg.ServerURL = strings.TrimSpace(dto.ServerURL)
	cfg.HeadersJSON = mustJSON(dto.Headers)
	cfg.Command = strings.TrimSpace(dto.Command)
	cfg.ArgsJSON = mustJSON(dto.Args)
	cfg.TimeoutSeconds = dto.TimeoutSeconds
	cfg.Enabled = deref(dto.Enabled, cfg.Enabled)
	cfg.Description = strings.TrimSpace(dto.Description)

	if err := s.db.WithContext(ctx).Save(&cfg).Error; err != nil {
		log.L().Error("save mcp config failed", zap.Error(err))
		return nil, apperr.ServerError("failed to save MCP config")
	}
	s.agent.MCP().Invalidate(userID)
	return mcpConfigView(cfg, userID), nil
}

// DeleteMCPConfig removes a user-owned config. Global rows are immutable.
func (s *service) DeleteMCPConfig(ctx context.Context, userID, id uint) error {
	if userID == 0 {
		return errUserIDRequired
	}
	res := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.MCPConfig{})
	if res.Error != nil {
		log.L().Error("delete mcp config failed", zap.Error(res.Error))
		return apperr.ServerError("failed to delete MCP config")
	}
	if res.RowsAffected == 0 {
		return apperr.NotFound("MCP config not found")
	}
	s.agent.MCP().Invalidate(userID)
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func validateMCPDto(dto MCPConfigDto) error {
	if strings.TrimSpace(dto.Name) == "" {
		return apperr.BadRequest(400, "name is required")
	}
	switch strings.ToLower(strings.TrimSpace(dto.Transport)) {
	case "stdio", "sse", "streamable":
	default:
		return apperr.BadRequest(400, "transport must be one of stdio | sse | streamable")
	}
	return nil
}

func mcpConfigView(c models.MCPConfig, userID uint) gin.H {
	var headers map[string]string
	_ = json.Unmarshal([]byte(c.HeadersJSON), &headers)
	var args []string
	_ = json.Unmarshal([]byte(c.ArgsJSON), &args)

	editable := c.UserID != nil && *c.UserID == userID
	if !editable {
		// Global/admin-managed rows (UserID == nil) are listed to every user,
		// but their header values carry auth secrets (Authorization: Bearer …,
		// X-Api-Key: …). Expose only the header names so the UI can show which
		// headers exist without disclosing another tenant's credentials. The
		// agent runtime reads headers straight from the DB model, so masking
		// the view does not affect live server connections.
		headers = maskHeaderValues(headers)
	}

	return gin.H{
		"id":              c.ID,
		"name":            c.Name,
		"transport":       c.Transport,
		"server_url":      c.ServerURL,
		"headers":         headers,
		"command":         c.Command,
		"args":            args,
		"timeout_seconds": c.TimeoutSeconds,
		"enabled":         c.Enabled,
		"description":     c.Description,
		"is_global":       c.UserID == nil,
		"editable":        editable,
		"created_at":      c.CreatedAt,
		"updated_at":      c.UpdatedAt,
	}
}

// maskHeaderValues replaces every header value with a fixed placeholder while
// preserving the keys. Returns nil for an empty/nil map so the JSON field
// stays absent rather than emitting an empty object.
func maskHeaderValues(h map[string]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	masked := make(map[string]string, len(h))
	for k := range h {
		masked[k] = "***"
	}
	return masked
}

func mustJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func pUint(u uint) *uint { return &u }

func deref(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}
