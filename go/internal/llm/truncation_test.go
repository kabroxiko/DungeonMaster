package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// A reply that stopped on the token budget is half a document. Handing it back as if it were complete makes the
// caller fail later on an unreadable parse, which is exactly how a reasoning model broke campaign generation.
func TestTruncatedReplyIsReportedInsteadOfReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"creativeSeed:\n  titleMood: \"angustia adminis"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AIProvider: config.ProviderDeepSeek, DeepSeekURL: srv.URL, DeepSeekAPIKey: "k"}
	out, err := GenerateResponse(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "go"}},
	}, map[string]interface{}{"max_tokens": 900})
	if err == nil {
		t.Fatalf("expected an error, got partial text %q", out)
	}
	if out != "" {
		t.Fatalf("partial text must not be returned, got %q", out)
	}
	for _, want := range []string{"max_tokens", "900", "deepseek"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should mention %q, got %v", want, err)
		}
	}
}

func TestCompleteReplyStillReturns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"fine"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AIProvider: config.ProviderDeepSeek, DeepSeekURL: srv.URL, DeepSeekAPIKey: "k"}
	out, err := GenerateResponse(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "go"}},
	}, nil)
	if err != nil || out != "fine" {
		t.Fatalf("expected the text, got %q / %v", out, err)
	}
}

// LM Studio returns the same partial document on the same budget, and its native fallback would re-run the whole
// generation, so the cut-off has to be reported here too instead of surfacing as an unreadable parse.
func TestLMStudioTruncatedReplyIsReportedInsteadOfReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"creativeSeed:\n  titleMood: \"angu"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AIProvider: config.ProviderLMStudio, LMStudioURL: srv.URL, LMStudioModel: "local/model"}
	out, err := GenerateResponse(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "go"}},
	}, map[string]interface{}{"max_tokens": 700})
	if err == nil {
		t.Fatalf("expected an error, got partial text %q", out)
	}
	if out != "" {
		t.Fatalf("partial text must not be returned, got %q", out)
	}
	for _, want := range []string{"max_tokens", "700", "lm_studio"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should mention %q, got %v", want, err)
		}
	}
}
