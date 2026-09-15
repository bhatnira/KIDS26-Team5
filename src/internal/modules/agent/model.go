package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"antelope/internal/modules/llmconfig"

	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/anthropic"
	"trpc.group/trpc-go/trpc-agent-go/model/gemini"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
)

// BuildModel constructs a framework model.Model from a user's resolved
// LLM config. Mirrors antelope-agent's BuildModel (ported verbatim):
// DeepSeek runs through the OpenAI client with a different base URL +
// variant; Gemini takes its credentials from env vars (GEMINI_API_KEY /
// GOOGLE_API_KEY) rather than via a functional option.
func BuildModel(ctx context.Context, cfg *llmconfig.UserLLMConfig) (model.Model, error) {
	if cfg == nil {
		return nil, errors.New("nil user llm config")
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	name := strings.TrimSpace(cfg.Model)
	if name == "" {
		return nil, errors.New("model name is required")
	}
	apiKey := cfg.APIKey
	baseURL := strings.TrimSpace(cfg.BaseURL)

	switch provider {
	case "anthropic":
		opts := []anthropic.Option{}
		if apiKey != "" {
			opts = append(opts, anthropic.WithAPIKey(apiKey))
		}
		if baseURL != "" {
			opts = append(opts, anthropic.WithBaseURL(baseURL))
		}
		return anthropic.New(name, opts...), nil

	case "openai", "openai-compatible", "":
		opts := []openai.Option{openai.WithVariant(openai.VariantOpenAI)}
		if apiKey != "" {
			opts = append(opts, openai.WithAPIKey(apiKey))
		}
		if baseURL != "" {
			opts = append(opts, openai.WithBaseURL(baseURL))
		}
		return openai.New(name, opts...), nil

	case "deepseek":
		opts := []openai.Option{openai.WithVariant(openai.VariantDeepSeek)}
		if apiKey != "" {
			opts = append(opts, openai.WithAPIKey(apiKey))
		}
		if baseURL == "" {
			baseURL = "https://api.deepseek.com"
		}
		opts = append(opts, openai.WithBaseURL(baseURL))
		return openai.New(name, opts...), nil

	case "gemini":
		// gemini reads its API key from GEMINI_API_KEY (or GOOGLE_API_KEY) via
		// the underlying genai client; no functional option is exposed for it.
		_ = apiKey
		return gemini.New(ctx, name)

	default:
		return nil, fmt.Errorf("unsupported provider %q (expected anthropic|openai|openai-compatible|deepseek|gemini)", provider)
	}
}

// ApplyUserGenerationConfig fills the generation parameters carried by a user's
// LLM config onto genCfg. It is the single source of truth shared by the agent
// factory (live chat) and the connection-test path so both honour the same
// rules. Following the framework's pointer-when-set idiom, a field is only set
// when the user supplied a meaningful value:
//
//   - MaxTokens / Temperature: only when > 0, so a stored 0 falls back to the
//     provider default rather than forcing fully-deterministic / zero output.
//   - Thinking: only when the user enabled it, and mapped per provider (see
//     below) because the trpc-agent-go adapters translate the thinking knobs
//     into provider-specific request fields.
//
// Thinking parameter mapping (matches trpc-agent-go v1.10.0 adapters):
//
//   - OpenAI / openai-compatible: thinking is controlled ONLY by the standard
//     `reasoning_effort` field (valid on reasoning models). Setting
//     ThinkingEnabled here would make the OpenAI adapter inject an unsupported
//     `thinking_enabled` body field that api.openai.com rejects with HTTP 400,
//     so we deliberately leave ThinkingEnabled unset.
//   - Anthropic: the adapter's applyThinkingConfig is a no-op unless
//     ThinkingEnabled is set, so we set it to enable adaptive thinking and pass
//     the effort through ReasoningEffort (low|medium|high|xhigh|max).
//   - DeepSeek: ThinkingEnabled maps to a valid {"thinking":{"type":…}} object,
//     so both knobs are safe.
func ApplyUserGenerationConfig(genCfg *model.GenerationConfig, cfg *llmconfig.UserLLMConfig) {
	if cfg == nil {
		return
	}
	if cfg.MaxTokens > 0 {
		maxTokens := cfg.MaxTokens
		genCfg.MaxTokens = &maxTokens
	}
	if cfg.Temperature > 0 {
		temperature := cfg.Temperature
		genCfg.Temperature = &temperature
	}
	if !cfg.ThinkingEnabled {
		return
	}

	effort := strings.TrimSpace(cfg.ThinkingEffort)
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "anthropic", "deepseek":
		// These adapters require the explicit toggle to turn thinking on.
		enabled := true
		genCfg.ThinkingEnabled = &enabled
		if effort != "" {
			genCfg.ReasoningEffort = &effort
		}
	default:
		// OpenAI / openai-compatible (and anything else routed through the
		// OpenAI client): use only the standard reasoning_effort field.
		if effort != "" {
			genCfg.ReasoningEffort = &effort
		}
	}
}
