package config

import "testing"

// DM_AI_PROVIDER is one name, so the accepted spellings have to fold onto it and anything else must survive unchanged
// so the backend resolver can reject it by name.
func TestNormalizeAIProvider(t *testing.T) {
	cases := map[string]string{
		"":                 ProviderOpenAI,
		"openai":           ProviderOpenAI,
		"  OpenAI ":        ProviderOpenAI,
		"deepseek":         ProviderDeepSeek,
		"DEEPSEEK":         ProviderDeepSeek,
		"gemini":           ProviderGemini,
		"GEMINI":           ProviderGemini,
		"google":           ProviderGemini,
		"Google AI Studio": ProviderGemini,
		"google-ai-studio": ProviderGemini,
		"ai_studio":        ProviderGemini,
		"lmstudio":         ProviderLMStudio,
		"lm studio":        ProviderLMStudio,
		"lm_studio":        ProviderLMStudio,
		"lm-studio":        ProviderLMStudio,
		"  LM-Studio  ":    ProviderLMStudio,
		"anthropic":        "anthropic",
	}
	for raw, want := range cases {
		if got := NormalizeAIProvider(raw); got != want {
			t.Fatalf("NormalizeAIProvider(%q) = %q, want %q", raw, got, want)
		}
	}
}

// The heavy tier's "not configured" has to survive Load as an empty string: folding it onto OpenAI would send
// authoring to the cloud on a local-only install.
func TestNormalizeOptionalProviderKeepsEmptyEmpty(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"   ":          "",
		"deepseek":     ProviderDeepSeek,
		"  LM-Studio ": ProviderLMStudio,
		"anthropic":    "anthropic",
	}
	for raw, want := range cases {
		if got := normalizeOptionalProvider(raw); got != want {
			t.Fatalf("normalizeOptionalProvider(%q) = %q, want %q", raw, got, want)
		}
	}
	t.Setenv("DM_AI_HEAVY_PROVIDER", "")
	if cfg := Load(); cfg.AIHeavyProvider != "" {
		t.Fatalf("an unset heavy provider must stay empty, got %q", cfg.AIHeavyProvider)
	}
	t.Setenv("DM_AI_HEAVY_PROVIDER", "deepseek")
	t.Setenv("DM_AI_HEAVY_MODEL", "deepseek-chat")
	cfg := Load()
	if cfg.AIHeavyProvider != ProviderDeepSeek || cfg.AIHeavyModel != "deepseek-chat" {
		t.Fatalf("heavy tier not read from the environment: %q %q", cfg.AIHeavyProvider, cfg.AIHeavyModel)
	}
}
