package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// A Google AI Studio key authorises the generativelanguage endpoint, whose compatibility layer sits under
// /v1beta/openai. The route must be built from the configured base rather than the OpenAI /v1 convention, or every
// call would 404.
func TestGeminiUsesItsOwnEndpointModelAndKey(t *testing.T) {
	srv := newChatServer(t)
	cfg := &config.Config{AIProvider: config.ProviderGemini, GeminiURL: srv.URL, GeminiAPIKey: "AIza-key", GeminiModel: "gemini-2.5-pro"}

	out, err := generate(t, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "the bridge is out" {
		t.Fatalf("unexpected content: %q", out)
	}
	if srv.path != "/chat/completions" {
		t.Fatalf("expected the route appended to Gemini's base, got %q", srv.path)
	}
	if srv.authorization != "Bearer AIza-key" {
		t.Fatalf("expected the Gemini key, got %q", srv.authorization)
	}
	if srv.body["model"] != "gemini-2.5-pro" {
		t.Fatalf("expected the configured model, got %v", srv.body["model"])
	}
}

func TestGeminiDefaultsToAIStudioEndpointAndModel(t *testing.T) {
	backend, err := resolveChatBackend(&config.Config{AIProvider: config.ProviderGemini})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.baseURL != DefaultGeminiBaseURL {
		t.Fatalf("expected the AI Studio base URL by default, got %q", backend.baseURL)
	}
	if backend.chatURL() != DefaultGeminiBaseURL+"/chat/completions" {
		t.Fatalf("expected the chat route under /v1beta/openai, got %q", backend.chatURL())
	}
	if backend.keyEnv != "DM_GEMINI_API_KEY" {
		t.Fatalf("the error text must name the setting that fixes a missing key, got %q", backend.keyEnv)
	}

	model, err := defaultModelFor(&config.Config{}, backend)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "gemini-2.5-flash" {
		t.Fatalf("expected a plain chat model by default, got %q", model)
	}
}

// The operator may paste the whole URL from the docs, including the route. Appending it again would 404.
func TestGeminiBaseMayAlreadyNameTheRoute(t *testing.T) {
	cfg := &config.Config{AIProvider: config.ProviderGemini, GeminiURL: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"}

	backend, err := resolveChatBackend(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Count(backend.chatURL(), "/chat/completions"); got != 1 {
		t.Fatalf("expected the route exactly once, got %q", backend.chatURL())
	}
}

func TestGeminiReportsItsOwnMissingKey(t *testing.T) {
	cfg := &config.Config{AIProvider: config.ProviderGemini, GeminiURL: "http://127.0.0.1:1", GeminiAPIKey: ""}

	_, err := generate(t, cfg, nil)
	if err == nil {
		t.Fatal("expected an error when the Gemini key is missing")
	}
	if !strings.Contains(err.Error(), "DM_GEMINI_API_KEY") {
		t.Fatalf("the error should name the setting that fixes it, got %v", err)
	}
}

func TestGeminiSurfacesProviderErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"API key not valid. Please pass a valid API key."}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AIProvider: config.ProviderGemini, GeminiURL: srv.URL, GeminiAPIKey: "bad"}
	_, err := generate(t, cfg, nil)
	if err == nil {
		t.Fatal("expected an error for a rejected key")
	}
	if !strings.Contains(err.Error(), "gemini") || !strings.Contains(err.Error(), "API key not valid") {
		t.Fatalf("the error should name the provider and keep the message, got %v", err)
	}
}

// dm.go asks for json_object; Gemini's compatibility layer honours it, and dropping it would let the model wrap the DM
// envelope in prose that the parser rejects.
func TestGeminiKeepsJSONModeForTheDMEnvelope(t *testing.T) {
	srv := newChatServer(t)
	cfg := &config.Config{AIProvider: config.ProviderGemini, GeminiURL: srv.URL, GeminiAPIKey: "k"}

	opts := map[string]interface{}{"response_format": map[string]interface{}{"type": "json_object"}}
	if _, err := generate(t, cfg, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rf, _ := srv.body["response_format"].(map[string]interface{})
	if rf["type"] != "json_object" {
		t.Fatalf("expected json_object to be forwarded, got %v", srv.body["response_format"])
	}
}
