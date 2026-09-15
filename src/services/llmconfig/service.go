package llmconfig

import (
	"context"
	"net/url"
	"strings"
	"time"

	"antelope/internal/modules/agent"
	llmcfgmod "antelope/internal/modules/llmconfig"
	"antelope/internal/modules/log"
	"antelope/pkg/apperr"
	"antelope/pkg/response"
	"antelope/pkg/types"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

// Service provides CRUD for per-user LLM configurations.
type Service interface {
	GetConfigs(userID uint) (gin.H, error)
	AddConfig(userID uint, dto types.LLMConfigAddDto) (gin.H, error)
	UpdateConfig(userID uint, configID string, dto types.LLMConfigAddDto) (gin.H, error)
	DeleteConfig(userID uint, configID string) error
	TestConnection(dto types.LLMTestConnectionDto) (gin.H, error)
}

type llmConfigService struct {
	manager *llmcfgmod.Manager
}

func NewService(manager *llmcfgmod.Manager) Service {
	return &llmConfigService{manager: manager}
}

func (s *llmConfigService) GetConfigs(userID uint) (gin.H, error) {
	configs, err := s.manager.GetConfigs(userID)
	if err != nil {
		log.L().Error("failed to get llm configs", zap.Uint("userID", userID), zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}

	resp := make([]types.LLMConfigResponse, len(configs))
	for i, c := range configs {
		resp[i] = toResponse(c)
	}
	return gin.H{"configs": resp}, nil
}

func (s *llmConfigService) AddConfig(userID uint, dto types.LLMConfigAddDto) (gin.H, error) {
	cfg := llmcfgmod.UserLLMConfig{
		Name:               dto.Name,
		Provider:           dto.Provider,
		APIKey:             dto.APIKey,
		Model:              dto.Model,
		BaseURL:            normalizeBaseURL(dto.BaseURL),
		MaxTokens:          dto.MaxTokens,
		Temperature:        dto.Temperature,
		ThinkingEnabled:    dto.ThinkingEnabled,
		ThinkingEffort:     dto.ThinkingEffort,
		IsDefault:          dto.IsDefault,
		InsecureSkipVerify: dto.InsecureSkipVerify,
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}

	created, err := s.manager.AddConfig(userID, cfg)
	if err != nil {
		log.L().Error("failed to add llm config", zap.Uint("userID", userID), zap.Error(err))
		return nil, apperr.ServerError(response.SystemError)
	}
	return gin.H{"config": toResponse(created)}, nil
}

func (s *llmConfigService) UpdateConfig(userID uint, configID string, dto types.LLMConfigAddDto) (gin.H, error) {
	updated := llmcfgmod.UserLLMConfig{
		Name:               dto.Name,
		Provider:           dto.Provider,
		APIKey:             dto.APIKey,
		Model:              dto.Model,
		BaseURL:            normalizeBaseURL(dto.BaseURL),
		MaxTokens:          dto.MaxTokens,
		Temperature:        dto.Temperature,
		ThinkingEnabled:    dto.ThinkingEnabled,
		ThinkingEffort:     dto.ThinkingEffort,
		IsDefault:          dto.IsDefault,
		InsecureSkipVerify: dto.InsecureSkipVerify,
	}
	if updated.MaxTokens <= 0 {
		updated.MaxTokens = 4096
	}

	// If API key is fully masked, keep the existing one
	if isMasked(dto.APIKey) {
		existing, err := s.manager.GetConfig(userID, configID)
		if err != nil || existing == nil {
			return nil, apperr.NotFound("config not found")
		}
		updated.APIKey = existing.APIKey
	}

	if err := s.manager.UpdateConfig(userID, configID, updated); err != nil {
		log.L().Error("failed to update llm config", zap.Uint("userID", userID), zap.String("configID", configID), zap.Error(err))
		return nil, apperr.NotFound("config not found")
	}

	configs, _ := s.manager.GetConfigs(userID)
	resp := make([]types.LLMConfigResponse, len(configs))
	for i, c := range configs {
		resp[i] = toResponse(c)
	}
	return gin.H{"configs": resp}, nil
}

func (s *llmConfigService) DeleteConfig(userID uint, configID string) error {
	if err := s.manager.DeleteConfig(userID, configID); err != nil {
		log.L().Error("failed to delete llm config", zap.Uint("userID", userID), zap.String("configID", configID), zap.Error(err))
		return apperr.NotFound("config not found")
	}
	return nil
}

func (s *llmConfigService) TestConnection(dto types.LLMTestConnectionDto) (gin.H, error) {
	cfg := &llmcfgmod.UserLLMConfig{
		Provider:        dto.Provider,
		APIKey:          dto.APIKey,
		Model:           dto.Model,
		BaseURL:         normalizeBaseURL(dto.BaseURL),
		MaxTokens:       dto.MaxTokens,
		Temperature:     dto.Temperature,
		ThinkingEnabled: dto.ThinkingEnabled,
		ThinkingEffort:  dto.ThinkingEffort,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mdl, err := agent.BuildModel(ctx, cfg)
	if err != nil {
		return nil, apperr.CheckFail(response.CheckFailCode, "Failed to initialize model: "+err.Error())
	}
	req := &model.Request{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: "Say 'ok' in one word."},
		},
	}
	// Exercise the same generation params (incl. thinking) the live chat would
	// use, so a misconfigured thinking effort surfaces at test time.
	agent.ApplyUserGenerationConfig(&req.GenerationConfig, cfg)
	respCh, err := mdl.GenerateContent(ctx, req)
	if err != nil {
		return nil, apperr.CheckFail(response.CheckFailCode, "Connection test failed: "+err.Error())
	}
	// Drain channel — the first non-nil error response indicates failure.
	for resp := range respCh {
		if resp == nil {
			continue
		}
		if resp.Error != nil {
			return nil, apperr.CheckFail(response.CheckFailCode, "Connection test failed: "+resp.Error.Message)
		}
		if resp.Done {
			break
		}
	}
	return gin.H{"success": true}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toResponse(c llmcfgmod.UserLLMConfig) types.LLMConfigResponse {
	return types.LLMConfigResponse{
		ID:                 c.ID,
		Name:               c.Name,
		Provider:           c.Provider,
		APIKey:             maskAPIKey(c.APIKey),
		Model:              c.Model,
		BaseURL:            c.BaseURL,
		MaxTokens:          c.MaxTokens,
		Temperature:        c.Temperature,
		ThinkingEnabled:    c.ThinkingEnabled,
		ThinkingEffort:     c.ThinkingEffort,
		IsDefault:          c.IsDefault,
		InsecureSkipVerify: c.InsecureSkipVerify,
	}
}

// normalizeBaseURL ensures the URL ends with a path component so the go-openai
// library resolves "/chat/completions" correctly.  If the caller supplies a
// bare host (e.g. "https://my-llm-host") we append "/v1" to match the default
// OpenAI convention ("https://api.openai.com/v1").
func normalizeBaseURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" || u.Path == "/" {
		return strings.TrimRight(raw, "/") + "/v1"
	}
	return raw
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

func isMasked(s string) bool {
	return strings.Contains(s, "****")
}
