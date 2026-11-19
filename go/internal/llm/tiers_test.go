package llm

import (
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// A one-provider setup must keep behaving exactly as before: the authoring tier follows the play tier when
// DM_AI_HEAVY_PROVIDER is unset.
func TestHeavyTierFallsBackToThePlayTier(t *testing.T) {
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: "http://192.168.0.111:11434", AILightModel: "llama3.1:8b"}

	backend, err := resolveChatBackend(cfg, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.name != lmStudioBackendName || backend.baseURL != "http://192.168.0.111:11434" {
		t.Fatalf("authoring should follow the play provider, got %+v", backend)
	}
	if backend.model != "llama3.1:8b" {
		t.Fatalf("authoring should inherit the play model, got %q", backend.model)
	}
}

// The whole point of the tier: play turns on a local model, the long documents on a cloud one, from one process.
func TestHeavyProviderAndModelOverrideOnlyTheAuthoringTier(t *testing.T) {
	cfg := &config.Config{
		AILightProvider: config.ProviderLMStudio,
		AILightURL:      "http://192.168.0.111:11434",
		AILightModel:    "llama3.1:8b",
		AIHeavyProvider: config.ProviderDeepSeek,
		AIHeavyModel:    "deepseek-chat",
		DeepSeekAPIKey:  "sk-deepseek",
	}

	playName, playURL, playModel, err := BackendFor(cfg, false)
	if err != nil {
		t.Fatalf("play tier: %v", err)
	}
	if playName != lmStudioBackendName || playURL != "http://192.168.0.111:11434" || playModel != "llama3.1:8b" {
		t.Fatalf("play tier should stay local, got %s %s %s", playName, playURL, playModel)
	}

	heavyName, heavyURL, heavyModel, err := BackendFor(cfg, true)
	if err != nil {
		t.Fatalf("authoring tier: %v", err)
	}
	if heavyName != deepSeekBackendName || heavyURL != "https://api.deepseek.com" || heavyModel != "deepseek-chat" {
		t.Fatalf("authoring tier should be DeepSeek, got %s %s %s", heavyName, heavyURL, heavyModel)
	}
}

// Only the model has to change to author on a bigger model of the same provider.
func TestHeavyModelAloneKeepsThePlayProvider(t *testing.T) {
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: "http://127.0.0.1:11434", AILightModel: "llama3.1:8b", AIHeavyModel: "gpt-oss:latest"}

	backend, err := resolveChatBackend(cfg, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.name != lmStudioBackendName {
		t.Fatalf("expected the play provider, got %+v", backend)
	}
	if backend.model != "gpt-oss:latest" {
		t.Fatalf("expected the authoring model override, got %q", backend.model)
	}
}

// A typo has to name the setting that holds it, or the operator fixes the light provider and stays broken.
func TestUnknownHeavyProviderNamesItsOwnSetting(t *testing.T) {
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AIHeavyProvider: "anthropic"}

	_, err := resolveChatBackend(cfg, true)
	if err == nil {
		t.Fatal("expected an error for a provider the server does not serve")
	}
	if !strings.Contains(err.Error(), "DM_AI_HEAVY_PROVIDER") || !strings.Contains(err.Error(), "anthropic") {
		t.Fatalf("the error should name the setting and the rejected value, got %v", err)
	}
}

// The heavy flag on a request is what routes it; an authoring call must not reach the play endpoint.
func TestHeavyRequestIsSentToTheAuthoringBackend(t *testing.T) {
	playServer := newChatServer(t)
	heavyServer := newChatServer(t)
	pointProviderAt(t, config.ProviderDeepSeek, heavyServer.URL)
	cfg := &config.Config{
		AILightProvider: config.ProviderLMStudio,
		AILightURL:      playServer.URL,
		AILightModel:    "llama3.1:8b",
		AIHeavyProvider: config.ProviderDeepSeek,
		AIHeavyModel:    "deepseek-chat",
		DeepSeekAPIKey:  "sk-deepseek",
	}

	if _, err := generate(t, cfg, map[string]interface{}{"heavy": true}); err != nil {
		t.Fatalf("authoring call failed: %v", err)
	}
	if heavyServer.body["model"] != "deepseek-chat" {
		t.Fatalf("authoring call should use the heavy model, got %v", heavyServer.body["model"])
	}
	if heavyServer.authorization != "Bearer sk-deepseek" {
		t.Fatalf("authoring call should use the heavy key, got %q", heavyServer.authorization)
	}
	if playServer.body != nil {
		t.Fatalf("authoring call reached the play endpoint: %+v", playServer.body)
	}

	if _, err := generate(t, cfg, nil); err != nil {
		t.Fatalf("play call failed: %v", err)
	}
	if playServer.body["model"] != "llama3.1:8b" {
		t.Fatalf("play call should use the play model, got %v", playServer.body["model"])
	}
}

// Two LM Studio/Ollama boxes are both "lmstudio", so the authoring tier has to be able to name its own machine.
func TestHeavyTierCanLiveOnAnotherLocalHost(t *testing.T) {
	playServer := newChatServer(t)
	heavyServer := newChatServer(t)
	cfg := &config.Config{
		AILightProvider: config.ProviderLMStudio,
		AILightURL:      playServer.URL,
		AILightModel:    "llama3.1:8b",
		AIHeavyURL:      heavyServer.URL,
		AIHeavyModel:    "gpt-oss:latest",
	}

	if _, err := generate(t, cfg, map[string]interface{}{"heavy": true}); err != nil {
		t.Fatalf("authoring call failed: %v", err)
	}
	if heavyServer.body["model"] != "gpt-oss:latest" {
		t.Fatalf("authoring call should use the heavy model, got %v", heavyServer.body["model"])
	}
	if heavyServer.path != "/v1/chat/completions" {
		t.Fatalf("authoring call should use the heavy host, path %q", heavyServer.path)
	}
	if playServer.body != nil {
		t.Fatalf("authoring call reached the play host: %+v", playServer.body)
	}

	if _, err := generate(t, cfg, nil); err != nil {
		t.Fatalf("play call failed: %v", err)
	}
	if playServer.body["model"] != "llama3.1:8b" {
		t.Fatalf("play call should stay on the play host, got %v", playServer.body["model"])
	}
	if heavyServer.path == "" {
		t.Fatal("play call never reached its host")
	}
}

// The override also has to move the key, so a second cloud account (or a key-protected proxy) can author.
func TestHeavyTierCanUseItsOwnKey(t *testing.T) {
	playServer := newChatServer(t)
	heavyServer := newChatServer(t)
	pointProviderAt(t, config.ProviderDeepSeek, heavyServer.URL)
	cfg := &config.Config{
		AILightProvider: config.ProviderLMStudio,
		AILightURL:      playServer.URL,
		AILightModel:    "llama3.1:8b",
		AIHeavyProvider: config.ProviderDeepSeek,
		AIHeavyModel:    "deepseek-chat",
		AIHeavyAPIKey:   "sk-authoring",
		DeepSeekAPIKey:  "sk-play",
	}

	if _, err := generate(t, cfg, map[string]interface{}{"heavy": true}); err != nil {
		t.Fatalf("authoring call failed: %v", err)
	}
	if heavyServer.authorization != "Bearer sk-authoring" {
		t.Fatalf("authoring call should use DM_AI_HEAVY_API_KEY, got %q", heavyServer.authorization)
	}
}

// Gemini's route is derived from its own base: the heavy role reaches the same endpoint without naming one, and the
// route under /v1beta/openai is built from the provider, not from a variable.
func TestHeavyGeminiSharesTheProvidersEndpoint(t *testing.T) {
	cfg := &config.Config{
		AILightProvider: config.ProviderGemini,
		GeminiAPIKey:    "AIza-play",
		AIHeavyProvider: config.ProviderGemini,
	}

	backend, err := resolveChatBackend(cfg, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.chatURL() != providerEndpoints.gemini+"/chat/completions" {
		t.Fatalf("heavy chat route not built from the provider: %q", backend.chatURL())
	}
	if backend.apiKey != "AIza-play" {
		t.Fatalf("the heavy tier should fall back to the provider key, got %q", backend.apiKey)
	}
}
