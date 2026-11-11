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
