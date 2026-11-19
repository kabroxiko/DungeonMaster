package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// The DM turn asks for json_object on every call, and an Ollama host enforces it as a grammar. The local path used to
// drop the field, which left the model free to answer in prose and the envelope parser to reject it.
func TestLocalBackendForwardsJsonObjectMode(t *testing.T) {
	srv := newChatServer(t)
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: srv.URL, AILightModel: "llama3.1:8b"}

	if _, err := generate(t, cfg, map[string]interface{}{
		"response_format": map[string]interface{}{"type": "json_object"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rf, ok := srv.body["response_format"].(map[string]interface{})
	if !ok {
		t.Fatalf("response_format was dropped: %+v", srv.body)
	}
	if rf["type"] != "json_object" {
		t.Fatalf("wrong response_format forwarded: %+v", rf)
	}
}

// An LM Studio build that refuses the field must keep working: the call is retried without it and the endpoint is not
// asked again, so no other provider pays for this.
func TestLocalBackendRetriesWithoutJsonModeWhenRefused(t *testing.T) {
	var calls int32
	var sawField int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		atomic.AddInt32(&calls, 1)
		// Refuse the field itself, the way an older LM Studio would; a request without it is served normally.
		if _, ok := body["response_format"]; ok {
			atomic.StoreInt32(&sawField, 1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Unrecognized request argument supplied: response_format"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"narration\":\"ok\"}"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: srv.URL, AILightModel: "some-local-model"}
	opts := map[string]interface{}{"response_format": map[string]interface{}{"type": "json_object"}}

	out, err := generate(t, cfg, opts)
	if err != nil {
		t.Fatalf("a provider without json_object support failed: %v", err)
	}
	if out == "" {
		t.Fatal("expected the retried reply")
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected one rejected attempt and one retry, got %d calls", calls)
	}

	// The endpoint is remembered: a later turn must not send the field at all.
	atomic.StoreInt32(&calls, 0)
	atomic.StoreInt32(&sawField, 0)
	if _, err := generate(t, cfg, opts); err != nil {
		t.Fatalf("later call failed: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("the endpoint should not be asked twice again, got %d calls", calls)
	}
	if atomic.LoadInt32(&sawField) != 0 {
		t.Fatal("response_format was sent again after the endpoint refused it")
	}
}
