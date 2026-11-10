package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// chatServer records what a provider endpoint received and answers like an OpenAI-compatible service.
type chatServer struct {
	*httptest.Server
	path          string
	authorization string
	body          map[string]interface{}
}

func newChatServer(t *testing.T) *chatServer {
	t.Helper()
	cs := &chatServer{}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.path = r.URL.Path
		cs.authorization = r.Header.Get("Authorization")
		cs.body = nil
		_ = json.NewDecoder(r.Body).Decode(&cs.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"the bridge is out"}}]}`))
	}))
	t.Cleanup(cs.Close)
	return cs
}

func generate(t *testing.T, cfg *config.Config, opts map[string]interface{}) (string, error) {
	t.Helper()
	input := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "what is west of the bridge?"},
		},
	}
	if opts == nil {
		opts = map[string]interface{}{}
	}
	return GenerateResponse(context.Background(), cfg, input, opts)
}

func TestDeepSeekUsesItsOwnEndpointModelAndKey(t *testing.T) {
	srv := newChatServer(t)
	cfg := &config.Config{UseDeepSeek: true, DeepSeekURL: srv.URL, DeepSeekAPIKey: "sk-deepseek", DeepSeekModel: "deepseek-reasoner"}

	out, err := generate(t, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "the bridge is out" {
		t.Fatalf("unexpected content: %q", out)
	}
	if srv.path != "/v1/chat/completions" {
		t.Fatalf("expected the OpenAI-compatible path, got %q", srv.path)
	}
	if srv.authorization != "Bearer sk-deepseek" {
		t.Fatalf("expected the DeepSeek key, got %q", srv.authorization)
	}
	if srv.body["model"] != "deepseek-reasoner" {
		t.Fatalf("expected the configured model, got %v", srv.body["model"])
	}
}

func TestDeepSeekDefaultsToChatModel(t *testing.T) {
	srv := newChatServer(t)
	cfg := &config.Config{UseDeepSeek: true, DeepSeekURL: srv.URL, DeepSeekAPIKey: "k"}

	if _, err := generate(t, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.body["model"] != "deepseek-chat" {
		t.Fatalf("expected deepseek-chat by default, got %v", srv.body["model"])
	}
}

func TestDeepSeekToleratesABaseURLThatAlreadyEndsInV1(t *testing.T) {
	srv := newChatServer(t)
	cfg := &config.Config{UseDeepSeek: true, DeepSeekURL: srv.URL + "/v1", DeepSeekAPIKey: "k"}

	if _, err := generate(t, cfg, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.path != "/v1/chat/completions" {
		t.Fatalf("a /v1 base URL must not be doubled, got %q", srv.path)
	}
}

func TestDeepSeekKeepsJSONModeForTheDMEnvelope(t *testing.T) {
	srv := newChatServer(t)
	cfg := &config.Config{UseDeepSeek: true, DeepSeekURL: srv.URL, DeepSeekAPIKey: "k"}

	// dm.go asks for json_object; DeepSeek supports it, so dropping it would let the model wrap its reply in prose.
	opts := map[string]interface{}{"response_format": map[string]interface{}{"type": "json_object"}}
	if _, err := generate(t, cfg, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rf, _ := srv.body["response_format"].(map[string]interface{})
	if rf["type"] != "json_object" {
		t.Fatalf("expected json_object to be forwarded, got %v", srv.body["response_format"])
	}
}

func TestDeepSeekReportsItsOwnMissingKey(t *testing.T) {
	cfg := &config.Config{UseDeepSeek: true, DeepSeekURL: "http://127.0.0.1:1", DeepSeekAPIKey: ""}

	_, err := generate(t, cfg, nil)
	if err == nil {
		t.Fatal("expected an error when the DeepSeek key is missing")
	}
	if !strings.Contains(err.Error(), "DM_DEEPSEEK_API_KEY") {
		t.Fatalf("the error should name the setting that fixes it, got %v", err)
	}
}

func TestDeepSeekSurfacesProviderErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Authentication Fails"}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{UseDeepSeek: true, DeepSeekURL: srv.URL, DeepSeekAPIKey: "bad"}
	_, err := generate(t, cfg, nil)
	if err == nil {
		t.Fatal("expected an error for a rejected key")
	}
	if !strings.Contains(err.Error(), "deepseek") || !strings.Contains(err.Error(), "Authentication Fails") {
		t.Fatalf("the error should name the provider and keep the message, got %v", err)
	}
}

func TestEnablingTwoHostedBackendsIsRejected(t *testing.T) {
	cfg := &config.Config{UseLMStudio: true, UseDeepSeek: true, DeepSeekAPIKey: "k"}

	_, err := generate(t, cfg, nil)
	if err == nil {
		t.Fatal("expected an error when two backends are enabled at once")
	}
	if !strings.Contains(err.Error(), "DM_USE_LM_STUDIO") || !strings.Contains(err.Error(), "DM_USE_DEEPSEEK") {
		t.Fatalf("the error should name both settings, got %v", err)
	}
}

func TestProviderDefaultsAreUnchanged(t *testing.T) {
	openai, err := resolveChatBackend(&config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if openai.name != "openai" || openai.baseURL != "https://api.openai.com" || openai.keyEnv != "DM_OPENAI_API_KEY" {
		t.Fatalf("OpenAI remains the default backend, got %+v", openai)
	}

	deepseek, err := resolveChatBackend(&config.Config{UseDeepSeek: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deepseek.baseURL != "https://api.deepseek.com" || deepseek.keyEnv != "DM_DEEPSEEK_API_KEY" {
		t.Fatalf("unexpected DeepSeek defaults: %+v", deepseek)
	}

	local, err := resolveChatBackend(&config.Config{UseLMStudio: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if local.baseURL != "http://localhost:1234" {
		t.Fatalf("LM Studio default URL changed: %+v", local)
	}
}
