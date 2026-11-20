package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/deckofdmthings/gmai/internal/config"
)

// Tool is one function the model may call mid-turn: a name, what it is for, and a JSON-schema for its arguments.
type Tool struct {
	Name        string                 `json:"-"`
	Description string                 `json:"-"`
	Parameters  map[string]interface{} `json:"-"`
}

func (t Tool) schema() map[string]interface{} {
	params := t.Parameters
	if params == nil {
		params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  params,
		},
	}
}

// ToolCall is one request from the model for a function to be executed.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
	// RawArguments is what the model sent, kept so the transcript can be replayed to the server byte for byte.
	RawArguments string
}

// ToolResult is what the caller hands back for one call.
type ToolResult struct {
	CallID  string
	Content string
}

// ToolRunner executes one tool call. It runs on the server, so the number it returns is the number the table sees.
type ToolRunner func(call ToolCall) ToolResult

// ErrToolsUnsupported reports that this endpoint refused the tools field, so the caller should fall back to a path that
// does not need it. It is a capability answer, not a failure of the turn.
var ErrToolsUnsupported = fmt.Errorf("this endpoint does not accept tools")

// noTools remembers, per endpoint, that the tools field was refused, so the fallback is chosen once instead of asked
// again on every turn.
var noTools sync.Map

// ToolsUnsupported reports whether this endpoint already refused tools.
func ToolsUnsupported(baseURL string) bool {
	_, refused := noTools.Load(baseURL)
	return refused
}

const maxToolRounds = 6

// GenerateWithTools runs one turn as a conversation with the server's functions.
//
// The model may answer directly, or ask for dice, read the encounter, open one - and then answer with the results in
// hand. That ordering is the point: a model that rolls its own dice invents them and then explains the invented number,
// while a model that asks for a roll has already been told the truth before it writes a word of narration.
func GenerateWithTools(ctx context.Context, cfg *config.Config, input map[string]interface{}, opts map[string]interface{}, tools []Tool, run ToolRunner) (string, error) {
	if len(tools) == 0 || run == nil {
		return "", fmt.Errorf("GenerateWithTools needs at least one tool")
	}
	heavy, _ := opts["heavy"].(bool)
	backend, err := resolveChatBackend(cfg, heavy)
	if err != nil {
		return "", err
	}
	if ToolsUnsupported(backend.baseURL) {
		return "", ErrToolsUnsupported
	}
	model := strings.TrimSpace(pickStr(opts, "model", ""))
	if model == "" {
		model = backend.model
	}
	messages := toMessageList(input)
	maxTok := intOption(opts, "max_tokens", 800)
	temp := floatOption(opts, "temperature", 0.8)
	gameID := pickStr(opts, "gameId", "")
	effort := reasoningEffortFor(opts, backend.baseURL, "")
	rf := responseFormatFor(opts, backend.baseURL)

	schemas := make([]interface{}, 0, len(tools))
	for _, tool := range tools {
		schemas = append(schemas, tool.schema())
	}

	for round := 0; round < maxToolRounds; round++ {
		msg, refused, err := chatMessage(ctx, backend, messages, model, maxTok, temp, effort, rf, schemas)
		if err != nil {
			// An endpoint written as https that speaks http is repaired here exactly as on the plain path.
			if downgraded, repaired := rememberPlainHTTP(backend.baseURL, err); repaired {
				backend.baseURL = downgraded
				msg, refused, err = chatMessage(ctx, backend, messages, model, maxTok, temp, effort, rf, schemas)
			}
			if err != nil {
				return "", err
			}
		}
		if refused {
			noTools.Store(backend.baseURL, struct{}{})
			return "", ErrToolsUnsupported
		}
		calls := parseToolCalls(msg)
		if len(calls) == 0 {
			out := stripHarmonyChannels(normalizeAssistantContent(msg["content"]))
			if strings.TrimSpace(out) == "" {
				out = stripHarmonyChannels(coerceAssistantOutputToString(msg["content"]))
			}
			debugLog(cfg, "tools: answered without a call gameId=%q rounds=%d out_len=%d", gameID, round, len(out))
			return out, nil
		}
		debugLog(cfg, "tools: %d call(s) gameId=%q round=%d", len(calls), gameID, round)

		messages = append(messages, assistantToolCallMessage(msg, calls))
		for _, call := range calls {
			result := run(call)
			debugLogLongLines(cfg, "tools result|", call.Name+" "+result.Content)
			messages = append(messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": call.ID,
				"content":      result.Content,
			})
		}
	}
	// The loop is capped so a model that keeps calling cannot hold the table's turn open forever.
	return "", fmt.Errorf("the model kept calling tools for %d rounds without answering", maxToolRounds)
}

// assistantToolCallMessage echoes the model's own call back into the transcript, which is what the protocol expects
// before the tool results are read.
func assistantToolCallMessage(msg map[string]interface{}, calls []ToolCall) map[string]interface{} {
	rawCalls, _ := msg["tool_calls"].([]interface{})
	if len(rawCalls) == 0 {
		rawCalls = make([]interface{}, 0, len(calls))
		for _, call := range calls {
			rawCalls = append(rawCalls, map[string]interface{}{
				"id":   call.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      call.Name,
					"arguments": call.RawArguments,
				},
			})
		}
	}
	return map[string]interface{}{
		"role":       "assistant",
		"content":    msg["content"],
		"tool_calls": rawCalls,
	}
}

func parseToolCalls(msg map[string]interface{}) []ToolCall {
	raw, _ := msg["tool_calls"].([]interface{})
	if len(raw) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		fn, _ := m["function"].(map[string]interface{})
		name := strings.TrimSpace(fmt.Sprint(fn["name"]))
		if name == "" {
			continue
		}
		call := ToolCall{ID: fmt.Sprint(m["id"]), Name: name}
		if call.ID == "" || call.ID == "<nil>" {
			call.ID = fmt.Sprintf("call_%d", i)
		}
		switch args := fn["arguments"].(type) {
		case string:
			call.RawArguments = args
			parsed, ok := parseToolArguments(args)
			if ok {
				call.Arguments = parsed
			}
		case map[string]interface{}:
			call.Arguments = args
			if b, err := json.Marshal(args); err == nil {
				call.RawArguments = string(b)
			}
		}
		if call.RawArguments == "" {
			call.RawArguments = "{}"
		}
		if call.Arguments == nil {
			call.Arguments = map[string]interface{}{}
		}
		out = append(out, call)
	}
	return out
}

// parseToolArguments reads the argument object, which small models sometimes send as a JSON string inside a string.
func parseToolArguments(raw string) (map[string]interface{}, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]interface{}{}, true
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
		return obj, true
	}
	// A doubly-encoded object: {"{\"a\":1}"} or "{\"a\":1}" with the quotes left on.
	if unquoted, err := strconvUnquote(trimmed); err == nil && unquoted != trimmed {
		if err := json.Unmarshal([]byte(unquoted), &obj); err == nil {
			return obj, true
		}
	}
	return nil, false
}

func strconvUnquote(s string) (string, error) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s, nil
	}
	var out string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return s, err
	}
	return out, nil
}

// chatMessage posts one chat completion and hands back the assistant message as it arrived, tool calls included.
func chatMessage(ctx context.Context, backend chatBackend, messages []interface{}, model string, maxTok int, temp float64, effort string, rf map[string]interface{}, tools []interface{}) (map[string]interface{}, bool, error) {
	body := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  maxTok,
		"temperature": temp,
		"tools":       tools,
	}
	if rf != nil {
		body["response_format"] = rf
	}
	if effort != "" {
		body["reasoning_effort"] = effort
	}
	b, _ := json.Marshal(body)
	url := backend.chatURL()
	if backend.name == lmStudioBackendName {
		url = strings.TrimRight(backend.baseURL, "/") + "/v1/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if backend.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+backend.apiKey)
	}
	resp, err := LongHTTPClient().Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		lower := strings.ToLower(string(raw))
		// A host that does not take the field says so in its 4xx body: that is a capability answer, not a broken turn.
		if strings.Contains(lower, "tool") && (strings.Contains(lower, "unsupported") || strings.Contains(lower, "not supported") ||
			strings.Contains(lower, "unknown") || strings.Contains(lower, "unrecognized") || strings.Contains(lower, "invalid")) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("%s: %s: %s", backend.name, resp.Status, string(raw))
	}
	var data struct {
		Choices []struct {
			FinishReason string                 `json:"finish_reason"`
			Message      map[string]interface{} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, false, err
	}
	if len(data.Choices) == 0 {
		return nil, false, fmt.Errorf("%s: empty choices", backend.name)
	}
	if strings.EqualFold(data.Choices[0].FinishReason, "length") {
		return nil, false, truncatedReplyError(backend.name, maxTok)
	}
	msg := data.Choices[0].Message
	if msg == nil {
		msg = map[string]interface{}{}
	}
	return msg, false, nil
}

func toMessageList(input map[string]interface{}) []interface{} {
	if messages, ok := input["messages"].([]interface{}); ok && messages != nil {
		return messages
	}
	prompt, _ := input["prompt"].(string)
	return []interface{}{map[string]interface{}{"role": "user", "content": prompt}}
}

func intOption(opts map[string]interface{}, key string, fallback int) int {
	switch v := opts[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return fallback
}

func floatOption(opts map[string]interface{}, key string, fallback float64) float64 {
	if v, ok := opts[key].(float64); ok {
		return v
	}
	return fallback
}
