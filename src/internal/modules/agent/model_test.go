package agent

import (
	"testing"

	"antelope/internal/modules/llmconfig"

	"trpc.group/trpc-go/trpc-agent-go/model"
)

func TestApplyUserGenerationConfig(t *testing.T) {
	t.Run("tokens and temperature set only when positive", func(t *testing.T) {
		var gc model.GenerationConfig
		ApplyUserGenerationConfig(&gc, &llmconfig.UserLLMConfig{MaxTokens: 4096, Temperature: 0.7})
		if gc.MaxTokens == nil || *gc.MaxTokens != 4096 {
			t.Fatalf("MaxTokens = %v, want 4096", gc.MaxTokens)
		}
		if gc.Temperature == nil || *gc.Temperature != 0.7 {
			t.Fatalf("Temperature = %v, want 0.7", gc.Temperature)
		}

		var zero model.GenerationConfig
		ApplyUserGenerationConfig(&zero, &llmconfig.UserLLMConfig{MaxTokens: 0, Temperature: 0})
		if zero.MaxTokens != nil {
			t.Errorf("MaxTokens should stay nil for 0, got %v", *zero.MaxTokens)
		}
		if zero.Temperature != nil {
			t.Errorf("Temperature should stay nil for 0, got %v", *zero.Temperature)
		}
	})

	t.Run("thinking disabled leaves thinking fields nil", func(t *testing.T) {
		var gc model.GenerationConfig
		ApplyUserGenerationConfig(&gc, &llmconfig.UserLLMConfig{Provider: "anthropic", ThinkingEnabled: false, ThinkingEffort: "high"})
		if gc.ThinkingEnabled != nil {
			t.Errorf("ThinkingEnabled should be nil when disabled, got %v", *gc.ThinkingEnabled)
		}
		if gc.ReasoningEffort != nil {
			t.Errorf("ReasoningEffort should be nil when thinking disabled, got %v", *gc.ReasoningEffort)
		}
	})

	t.Run("anthropic thinking sets enabled + effort", func(t *testing.T) {
		var gc model.GenerationConfig
		ApplyUserGenerationConfig(&gc, &llmconfig.UserLLMConfig{Provider: "anthropic", ThinkingEnabled: true, ThinkingEffort: "xhigh"})
		if gc.ThinkingEnabled == nil || !*gc.ThinkingEnabled {
			t.Fatalf("ThinkingEnabled = %v, want true", gc.ThinkingEnabled)
		}
		if gc.ReasoningEffort == nil || *gc.ReasoningEffort != "xhigh" {
			t.Fatalf("ReasoningEffort = %v, want xhigh", gc.ReasoningEffort)
		}
	})

	t.Run("openai thinking sets only reasoning_effort, never thinking_enabled", func(t *testing.T) {
		for _, provider := range []string{"openai", "openai-compatible", ""} {
			var gc model.GenerationConfig
			ApplyUserGenerationConfig(&gc, &llmconfig.UserLLMConfig{Provider: provider, ThinkingEnabled: true, ThinkingEffort: "medium"})
			// Setting ThinkingEnabled would inject an unsupported `thinking_enabled`
			// body field that the OpenAI API rejects (HTTP 400).
			if gc.ThinkingEnabled != nil {
				t.Errorf("provider %q: ThinkingEnabled must stay nil, got %v", provider, *gc.ThinkingEnabled)
			}
			if gc.ReasoningEffort == nil || *gc.ReasoningEffort != "medium" {
				t.Errorf("provider %q: ReasoningEffort = %v, want medium", provider, gc.ReasoningEffort)
			}
		}
	})

	t.Run("anthropic thinking without effort omits reasoning effort", func(t *testing.T) {
		var gc model.GenerationConfig
		ApplyUserGenerationConfig(&gc, &llmconfig.UserLLMConfig{Provider: "anthropic", ThinkingEnabled: true, ThinkingEffort: ""})
		if gc.ThinkingEnabled == nil || !*gc.ThinkingEnabled {
			t.Fatalf("ThinkingEnabled = %v, want true", gc.ThinkingEnabled)
		}
		if gc.ReasoningEffort != nil {
			t.Errorf("ReasoningEffort should be nil when effort empty, got %v", *gc.ReasoningEffort)
		}
	})
}

func TestBuildModelOpenAICompatibleAlias(t *testing.T) {
	// openai-compatible must resolve like openai rather than erroring as an
	// unsupported provider.
	mdl, err := BuildModel(t.Context(), &llmconfig.UserLLMConfig{
		Provider: "openai-compatible",
		Model:    "gpt-4o",
		APIKey:   "sk-test",
		BaseURL:  "https://example.invalid/v1",
	})
	if err != nil {
		t.Fatalf("BuildModel(openai-compatible) error = %v, want nil", err)
	}
	if mdl == nil {
		t.Fatal("BuildModel(openai-compatible) returned nil model")
	}
}
