package config

import "testing"

// DM_AI_PROVIDER is one name, so the accepted spellings have to fold onto it and anything else must survive unchanged
// so the backend resolver can reject it by name.
func TestNormalizeAIProvider(t *testing.T) {
	cases := map[string]string{
		"":              ProviderOpenAI,
		"openai":        ProviderOpenAI,
		"  OpenAI ":     ProviderOpenAI,
		"deepseek":      ProviderDeepSeek,
		"DEEPSEEK":      ProviderDeepSeek,
		"lmstudio":      ProviderLMStudio,
		"lm studio":     ProviderLMStudio,
		"lm_studio":     ProviderLMStudio,
		"lm-studio":     ProviderLMStudio,
		"  LM-Studio  ": ProviderLMStudio,
		"gemini":        "gemini",
	}
	for raw, want := range cases {
		if got := NormalizeAIProvider(raw); got != want {
			t.Fatalf("NormalizeAIProvider(%q) = %q, want %q", raw, got, want)
		}
	}
}
