package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// gpt-oss and other harmony models write their turn as channels, and a server that does not split them hands the whole
// transcript back as content. The analysis is the model talking to itself; the answer is what follows the final channel.
func TestHarmonyChannelsAreUnwrappedToTheAnswer(t *testing.T) {
	raw := "<|channel|>analysis<|message|>Need random. Provide YAML.<|end|><|start|>assistant<|channel|>final<|message|>" +
		"creativeSeed:\n  titleMood: \"melancólica nostalgia\"<|end|>"
	got := stripHarmonyChannels(raw)
	if got != "creativeSeed:\n  titleMood: \"melancólica nostalgia\"" {
		t.Fatalf("expected only the final answer, got %q", got)
	}

	if plain := stripHarmonyChannels("location: Cae\n"); plain != "location: Cae\n" {
		t.Fatalf("text without control tokens must be untouched, got %q", plain)
	}

	// A reply that never reached its final channel carries no answer at all, so the caller retries instead of parsing
	// the model's reasoning as if it were the document it asked for.
	if reasoningOnly := stripHarmonyChannels("<|channel|>analysis<|message|>still thinking<|end|>"); reasoningOnly != "" {
		t.Fatalf("an analysis-only reply has no answer, got %q", reasoningOnly)
	}
}

// An empty reply is usually a reasoning model that ended its turn without a visible answer, and asking again often
// works. It must not surface as a stage that failed on nothing.
func TestEmptyReplyIsAskedOnceMore(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":""}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"titleMood: grim"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AIProvider: config.ProviderLMStudio, LMStudioURL: srv.URL, LMStudioModel: "local/model"}
	out, err := GenerateResponse(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "go"}},
	}, map[string]interface{}{"max_tokens": 3000})
	if err != nil || out != "titleMood: grim" {
		t.Fatalf("expected the second reply, got %q / %v", out, err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected exactly one retry, saw %d calls", got)
	}
}

// When both attempts come back empty, the error has to name the real cause so the table is not told "AI response empty".
func TestEmptyReplyTwiceIsAnActionableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":""}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AIProvider: config.ProviderLMStudio, LMStudioURL: srv.URL, LMStudioModel: "local/model"}
	out, err := GenerateResponse(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "go"}},
	}, map[string]interface{}{"max_tokens": 3000})
	if err == nil {
		t.Fatalf("expected an error, got %q", out)
	}
	for _, want := range []string{"no text", "max_tokens"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should mention %q, got %v", want, err)
		}
	}
}
