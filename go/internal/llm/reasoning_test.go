package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// effortProbe records the reasoning_effort of every request it receives and answers with the given bodies in order,
// repeating the last one when the calls run past them.
type effortProbe struct {
	mu     sync.Mutex
	url    string
	effort []interface{}
	seen   []bool
	bodies []string
}

func newEffortProbe(t *testing.T, bodies ...string) *effortProbe {
	t.Helper()
	p := &effortProbe{bodies: bodies}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		p.mu.Lock()
		value, present := body["reasoning_effort"]
		p.effort = append(p.effort, value)
		p.seen = append(p.seen, present)
		index := len(p.effort) - 1
		p.mu.Unlock()
		if index >= len(p.bodies) {
			index = len(p.bodies) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(p.bodies[index]))
	}))
	t.Cleanup(srv.Close)
	p.url = srv.URL
	return p
}

func (p *effortProbe) calls() ([]interface{}, []bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]interface{}{}, p.effort...), append([]bool{}, p.seen...)
}

const emptyReplyBody = `{"choices":[{"finish_reason":"stop","message":{"content":""}}]}`

const answerBody = `{"choices":[{"finish_reason":"stop","message":{"content":"Bram hits the goblin."}}]}`

// A reasoning model can spend the whole answer budget thinking and end its turn with nothing to show. Repeating the
// same request asks for the same thing again; asking for a lower effort is what gets a visible answer.
func TestEmptyReplyIsRetriedWithLowReasoningEffort(t *testing.T) {
	probe := newEffortProbe(t, emptyReplyBody, answerBody)
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: probe.url, AILightModel: "openai/gpt-oss-20b"}

	out, err := generate(t, cfg, map[string]interface{}{"max_tokens": 1200})
	if err != nil || out != "Bram hits the goblin." {
		t.Fatalf("expected the second reply, got %q / %v", out, err)
	}
	effort, seen := probe.calls()
	if len(effort) != 2 {
		t.Fatalf("expected two calls, saw %d", len(effort))
	}
	if seen[0] {
		t.Fatalf("the first call should not force an effort, it sent %v", effort[0])
	}
	if effort[1] != "low" {
		t.Fatalf("the retry should ask for low effort, sent %v", effort[1])
	}
}

// The knob is not universal: a host that does not know the field answers 400. The answer matters more than the field,
// so the call is repeated without it and the endpoint is remembered.
func TestEndpointThatRejectsReasoningEffortIsAskedWithoutIt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, present := body["reasoning_effort"]; present {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"unknown field reasoning_effort"}`))
			return
		}
		// The first plain call comes back empty, which is what makes the retry add the field this host refuses.
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = w.Write([]byte(emptyReplyBody))
			return
		}
		_, _ = w.Write([]byte(answerBody))
	}))
	defer srv.Close()

	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: srv.URL, AILightModel: "local/model"}
	out, err := generate(t, cfg, map[string]interface{}{"max_tokens": 1200})
	if err != nil || out != "Bram hits the goblin." {
		t.Fatalf("expected an answer once the field was dropped, got %q / %v", out, err)
	}
	if _, refused := noReasoningEffort.Load(srv.URL); !refused {
		t.Fatalf("the endpoint should be remembered as refusing reasoning_effort")
	}
}

// A caller that asks for an effort gets it on the first call, before any retry decides anything.
func TestExplicitReasoningEffortIsSentOnTheFirstCall(t *testing.T) {
	probe := newEffortProbe(t, answerBody)
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: probe.url, AILightModel: "local/model"}

	if _, err := generate(t, cfg, map[string]interface{}{"max_tokens": 800, "reasoning_effort": "high"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	effort, _ := probe.calls()
	if len(effort) != 1 || effort[0] != "high" {
		t.Fatalf("expected one call with high effort, got %v", effort)
	}
}

// The hosted path takes the same knob and drops it the same way, so a provider that rejects it still answers.
func TestHostedProviderThatRejectsReasoningEffortIsAskedWithoutIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, present := body["reasoning_effort"]; present {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Unrecognized request argument: reasoning_effort"}}`))
			return
		}
		_, _ = w.Write([]byte(answerBody))
	}))
	defer srv.Close()
	pointProviderAt(t, config.ProviderDeepSeek, srv.URL)

	cfg := &config.Config{AILightProvider: config.ProviderDeepSeek, DeepSeekAPIKey: "sk-test", AILightModel: "deepseek-chat"}
	out, err := generate(t, cfg, map[string]interface{}{"max_tokens": 800, "reasoning_effort": "low"})
	if err != nil || out != "Bram hits the goblin." {
		t.Fatalf("expected an answer once the field was dropped, got %q / %v", out, err)
	}
}
