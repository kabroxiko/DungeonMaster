package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// toolServer answers with a scripted sequence of assistant messages and records what it was sent.
type toolServer struct {
	mu       sync.Mutex
	requests []map[string]interface{}
	replies  []string
	status   int
}

func newToolServer(t *testing.T, replies ...string) (*toolServer, string) {
	t.Helper()
	ts := &toolServer{replies: replies}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		ts.mu.Lock()
		index := len(ts.requests)
		ts.requests = append(ts.requests, body)
		status := ts.status
		ts.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"Unrecognized request argument: tools"}}`))
			return
		}
		if index >= len(ts.replies) {
			index = len(ts.replies) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(ts.replies[index]))
	}))
	t.Cleanup(srv.Close)
	return ts, srv.URL
}

func testTool() []Tool {
	return []Tool{{
		Name:        "roll",
		Description: "roll one action",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"actor": map[string]interface{}{"type": "string"}},
		},
	}}
}

// The turn this exists for: the model asks for a die, gets the number, and answers with it in hand.
func TestToolCallIsExecutedBeforeTheAnswer(t *testing.T) {
	ts, url := newToolServer(t,
		`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"roll","arguments":"{\"actor\":\"Bram\"}"}}]}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"narration\":\"Bram golpea.\"}"}}]}`,
	)
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: url, AILightModel: "local/model"}

	var seen []ToolCall
	out, err := GenerateWithTools(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "ataco"}},
	}, map[string]interface{}{"max_tokens": 500}, testTool(), func(call ToolCall) ToolResult {
		seen = append(seen, call)
		return ToolResult{CallID: call.ID, Content: "d20(9)+4 = 13 vs AC 12 -> hit"}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != `{"narration":"Bram golpea."}` {
		t.Fatalf("expected the model's answer, got %q", out)
	}
	if len(seen) != 1 || seen[0].Name != "roll" || seen[0].Arguments["actor"] != "Bram" {
		t.Fatalf("the roll was not executed as asked: %+v", seen)
	}
	// The second request has to carry the result, or the answer was written without it.
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.requests) != 2 {
		t.Fatalf("expected two requests, got %d", len(ts.requests))
	}
	msgs, _ := ts.requests[1]["messages"].([]interface{})
	last, _ := msgs[len(msgs)-1].(map[string]interface{})
	if last["role"] != "tool" || !strings.Contains(last["content"].(string), "13 vs AC 12") {
		t.Fatalf("the tool result was not handed back: %v", last)
	}
	if _, sent := ts.requests[0]["tools"]; !sent {
		t.Fatal("the first request did not offer tools")
	}
}

// A host that does not know the field must say so once, and never be asked again.
func TestToolsRefusalIsRemembered(t *testing.T) {
	ts, url := newToolServer(t, `{"choices":[{"finish_reason":"stop","message":{"content":"hola"}}]}`)
	ts.status = 400
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: url, AILightModel: "local/model"}

	_, err := GenerateWithTools(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "ataco"}},
	}, nil, testTool(), func(call ToolCall) ToolResult { return ToolResult{CallID: call.ID} })
	if err != ErrToolsUnsupported {
		t.Fatalf("expected ErrToolsUnsupported, got %v", err)
	}
	if !ToolsUnsupported(url) {
		t.Fatal("the endpoint should be remembered as refusing tools")
	}
	if _, err := GenerateWithTools(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "ataco"}},
	}, nil, testTool(), func(call ToolCall) ToolResult { return ToolResult{CallID: call.ID} }); err != ErrToolsUnsupported {
		t.Fatalf("a refused endpoint should not be asked again, got %v", err)
	}
}

// A model that keeps calling must not hold the table's turn open forever.
func TestToolLoopIsCapped(t *testing.T) {
	loop := `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"roll","arguments":"{}"}}]}}]}`
	_, url := newToolServer(t, loop)
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: url, AILightModel: "local/model"}

	_, err := GenerateWithTools(context.Background(), cfg, map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "ataco"}},
	}, nil, testTool(), func(call ToolCall) ToolResult { return ToolResult{CallID: call.ID, Content: "ok"} })
	if err == nil || !strings.Contains(err.Error(), "kept calling tools") {
		t.Fatalf("expected a capped loop error, got %v", err)
	}
}

// Small models sometimes send the argument object as a JSON string inside a string.
func TestToolArgumentsSurviveDoubleEncoding(t *testing.T) {
	parsed, ok := parseToolArguments(`"{\"actor\":\"Bram\",\"modifier\":5}"`)
	if !ok || parsed["actor"] != "Bram" {
		t.Fatalf("double-encoded arguments should still be read: %v %v", parsed, ok)
	}
}
