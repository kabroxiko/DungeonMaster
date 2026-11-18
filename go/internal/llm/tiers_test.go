package llm

import (
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// A one-provider setup must keep behaving exactly as before: the authoring tier follows the play tier when
// DM_AI_HEAVY_PROVIDER is unset.
func TestHeavyTierFallsBackToThePlayTier(t *testing.T) {
	cfg := &config.Config{AIProvider: config.ProviderLMStudio, LMStudioURL: "http://192.168.0.111:11434", LMStudioModel: "llama3.1:8b"}

	backend, err := resolveChatBackend(cfg, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.name != lmStudioBackendName || backend.baseURL != "http://192.168.0.111:11434" {
		t.Fatalf("authoring should follow the play provider, got %+v", backend)
	}
	model, err := defaultModelFor(cfg, backend, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "llama3.1:8b" {
		t.Fatalf("authoring should inherit the play model, got %q", model)
	}
}

// The whole point of the tier: play turns on a local model, the long documents on a cloud one, from one process.
func TestHeavyProviderAndModelOverrideOnlyTheAuthoringTier(t *testing.T) {
	cfg := &config.Config{
		AIProvider:      config.ProviderLMStudio,
		LMStudioURL:     "http://192.168.0.111:11434",
		LMStudioModel:   "llama3.1:8b",
		AIHeavyProvider: config.ProviderDeepSeek,
		AIHeavyModel:    "deepseek-chat",
		DeepSeekURL:     "https://api.deepseek.com",
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
	cfg := &config.Config{AIProvider: config.ProviderLMStudio, LMStudioURL: "http://127.0.0.1:11434", LMStudioModel: "llama3.1:8b", AIHeavyModel: "gpt-oss:latest"}

	backend, err := resolveChatBackend(cfg, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.name != lmStudioBackendName {
		t.Fatalf("expected the play provider, got %+v", backend)
	}
	model, err := defaultModelFor(cfg, backend, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "gpt-oss:latest" {
		t.Fatalf("expected the authoring model override, got %q", model)
	}
}

// A typo has to name the setting that holds it, or the operator fixes DM_AI_PROVIDER and stays broken.
func TestUnknownHeavyProviderNamesItsOwnSetting(t *testing.T) {
	cfg := &config.Config{AIProvider: config.ProviderLMStudio, AIHeavyProvider: "anthropic"}

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
	cfg := &config.Config{
		AIProvider:      config.ProviderLMStudio,
		LMStudioURL:     playServer.URL,
		LMStudioModel:   "llama3.1:8b",
		AIHeavyProvider: config.ProviderDeepSeek,
		AIHeavyModel:    "deepseek-chat",
		DeepSeekURL:     heavyServer.URL,
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
		AIProvider:    config.ProviderLMStudio,
		LMStudioURL:   playServer.URL,
		LMStudioModel: "llama3.1:8b",
		AIHeavyURL:    heavyServer.URL,
		AIHeavyModel:  "gpt-oss:latest",
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
	cfg := &config.Config{
		AIProvider:      config.ProviderLMStudio,
		LMStudioURL:     playServer.URL,
		LMStudioModel:   "llama3.1:8b",
		AIHeavyProvider: config.ProviderDeepSeek,
		AIHeavyURL:      heavyServer.URL,
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

// Gemini derives its chat route from its base, so an overridden base must move the route too.
func TestHeavyGeminiURLRebuildsItsChatRoute(t *testing.T) {
	cfg := &config.Config{
		AIProvider:      config.ProviderGemini,
		GeminiAPIKey:    "AIza-play",
		AIHeavyProvider: config.ProviderGemini,
		AIHeavyURL:      "https://gemini-proxy.internal/v1beta/openai",
	}

	backend, err := resolveChatBackend(cfg, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.chatURL() != "https://gemini-proxy.internal/v1beta/openai/chat/completions" {
		t.Fatalf("heavy chat route not rebuilt from the override: %q", backend.chatURL())
	}
}
