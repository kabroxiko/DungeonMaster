package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deckofdmthings/gmai/internal/config"
)

func debugLog(cfg *config.Config, format string, args ...interface{}) {
	if cfg == nil || !cfg.DebugLLM {
		return
	}
	log.Printf("llm-debug "+format, args...)
}

func debugLogLongLines(cfg *config.Config, linePrefix string, s string) {
	if cfg == nil || !cfg.DebugLLM {
		return
	}
	s = strings.TrimSpace(s)
	if s == "" {
		log.Printf("%s <empty>", linePrefix)
		return
	}
	// Split on newlines first (readable YAML/logs), then chunk by runes so UTF-8 is never split mid-rune.
	const maxRunes = 4000
	for _, line := range strings.Split(s, "\n") {
		runes := []rune(line)
		for len(runes) > 0 {
			n := maxRunes
			if n > len(runes) {
				n = len(runes)
			}
			log.Printf("%s %s", linePrefix, string(runes[:n]))
			runes = runes[n:]
		}
	}
}

func debugLogRequestMessages(cfg *config.Config, messages []interface{}) {
	if cfg == nil || !cfg.DebugLLM {
		return
	}
	for i, m := range messages {
		mm, _ := m.(map[string]interface{})
		if mm == nil {
			log.Printf("llm-debug request msg[%d] role=? bytes=0", i)
			continue
		}
		role, _ := mm["role"].(string)
		content := ""
		switch c := mm["content"].(type) {
		case string:
			content = c
		default:
			content = fmt.Sprint(mm["content"])
		}
		log.Printf("llm-debug request msg[%d] role=%s bytes=%d", i, role, len(content))
		debugLogLongLines(cfg, "llm-debug request msg["+strconv.Itoa(i)+"]|", content)
	}
}

// GenerateResponse performs chat-completions against OpenAI, DeepSeek, or LM Studio.
func GenerateResponse(ctx context.Context, cfg *config.Config, input map[string]interface{}, opts map[string]interface{}) (string, error) {
	backend, err := resolveChatBackend(cfg)
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(pickStr(opts, "model", ""))
	if model == "" {
		switch backend.name {
		case "lm_studio":
			model = strings.TrimSpace(cfg.LMStudioModel)
			if model == "" {
				return "", fmt.Errorf("LM Studio is enabled (DM_USE_LM_STUDIO): set DM_LM_STUDIO_MODEL to a loaded model id exactly as shown in LM Studio (e.g. mistralai/mistral-nemo-instruct-2407@q4_k_m); OpenAI names like gpt-3.5-turbo are rejected by the local server")
			}
		case "deepseek":
			model = strings.TrimSpace(cfg.DeepSeekModel)
			if model == "" {
				model = "deepseek-chat"
			}
		default:
			model = strings.TrimSpace(cfg.OpenAIModel)
			if model == "" {
				model = "gpt-3.5-turbo"
			}
		}
	}
	messages, _ := input["messages"].([]interface{})
	if messages == nil {
		prompt, _ := input["prompt"].(string)
		messages = []interface{}{map[string]interface{}{"role": "user", "content": prompt}}
	}
	maxTok := 500
	if v, ok := opts["max_tokens"].(int); ok {
		maxTok = v
	} else if v, ok := opts["max_tokens"].(float64); ok {
		maxTok = int(v)
	}
	temp := 1.0
	if v, ok := opts["temperature"].(float64); ok {
		temp = v
	}

	gameID := pickStr(opts, "gameId", "")
	debugLog(cfg, "call backend=%s model=%s max_tokens=%d temp=%g gameId=%q messages=%d",
		backend.name, model, maxTok, temp, gameID, len(messages))
	debugLogRequestMessages(cfg, messages)

	var out string
	var err2 error
	if cfg.UseLMStudio {
		out, err2 = lmStudioChat(ctx, cfg, messages, model, maxTok, temp, opts)
	} else {
		out, err2 = openAICompatibleChat(ctx, backend, messages, model, maxTok, temp, opts)
	}
	if err2 != nil {
		debugLog(cfg, "error: %v", err2)
		return "", err2
	}
	debugLog(cfg, "ok gameId=%q out_len=%d", gameID, len(out))
	debugLogLongLines(cfg, "llm-debug out|", out)
	return out, nil
}

// chatBackend is the endpoint chat completions are sent to. OpenAI and DeepSeek speak the same protocol, so they
// differ only in where the request goes, which key authorises it, and the name that appears in errors.
type chatBackend struct {
	name    string
	baseURL string
	apiKey  string
	keyEnv  string
}

// resolveChatBackend picks the configured provider, and refuses ambiguous configuration rather than guessing which
// of two enabled backends the operator meant.
func resolveChatBackend(cfg *config.Config) (chatBackend, error) {
	if cfg == nil {
		return chatBackend{}, fmt.Errorf("no configuration")
	}
	if cfg.UseLMStudio && cfg.UseDeepSeek {
		return chatBackend{}, fmt.Errorf("DM_USE_LM_STUDIO and DM_USE_DEEPSEEK are both set; enable only one of them")
	}
	switch {
	case cfg.UseLMStudio:
		base := strings.TrimRight(cfg.LMStudioURL, "/")
		if base == "" {
			base = "http://localhost:1234"
		}
		return chatBackend{name: "lm_studio", baseURL: base}, nil
	case cfg.UseDeepSeek:
		base := strings.TrimRight(cfg.DeepSeekURL, "/")
		if base == "" {
			base = "https://api.deepseek.com"
		}
		return chatBackend{name: "deepseek", baseURL: base, apiKey: cfg.DeepSeekAPIKey, keyEnv: "DM_DEEPSEEK_API_KEY"}, nil
	default:
		return chatBackend{name: "openai", baseURL: "https://api.openai.com", apiKey: cfg.OpenAIAPIKey, keyEnv: "DM_OPENAI_API_KEY"}, nil
	}
}

// chatCompletionsURL accepts a base URL with or without the /v1 suffix, which providers document inconsistently.
func chatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func openAICompatibleChat(ctx context.Context, backend chatBackend, messages []interface{}, model string, maxTok int, temp float64, opts map[string]interface{}) (string, error) {
	key := backend.apiKey
	if key == "" {
		return "", fmt.Errorf("%s is not set", backend.keyEnv)
	}
	body := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  maxTok,
		"temperature": temp,
	}
	if rf, ok := opts["response_format"]; ok {
		body["response_format"] = rf
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL(backend.baseURL), bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := LongHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s: %s: %s", backend.name, resp.Status, string(raw))
	}
	var data struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	if len(data.Choices) == 0 {
		return "", fmt.Errorf("%s: empty choices", backend.name)
	}
	// A reply that stopped on the token budget is half a document. Returning it makes the failure surface later as
	// an unreadable parse, so say what actually happened instead.
	if strings.EqualFold(data.Choices[0].FinishReason, "length") {
		return "", fmt.Errorf("%s: reply hit max_tokens (%d) before finishing, so it was cut off; reasoning models spend part of that budget on hidden tokens, so raise max_tokens or use a model that does not reason", backend.name, maxTok)
	}
	out := normalizeAssistantContent(data.Choices[0].Message.Content)
	if out != "" {
		return out, nil
	}
	return coerceAssistantOutputToString(data.Choices[0].Message.Content), nil
}

func lmStudioChat(ctx context.Context, cfg *config.Config, messages []interface{}, model string, maxTok int, temp float64, opts map[string]interface{}) (string, error) {
	base := strings.TrimRight(cfg.LMStudioURL, "/")
	if base == "" {
		base = "http://localhost:1234"
	}
	body := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  maxTok,
		"temperature": temp,
	}
	if rf, ok := opts["response_format"]; ok {
		if m, ok := rf.(map[string]interface{}); ok {
			if t, _ := m["type"].(string); t == "json_schema" || t == "text" {
				body["response_format"] = rf
			}
		}
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := LongHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return lmStudioNative(ctx, cfg, messages, model, temp)
	}
	var data struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return lmStudioNative(ctx, cfg, messages, model, temp)
	}
	if len(data.Choices) == 0 {
		return lmStudioNative(ctx, cfg, messages, model, temp)
	}
	out := normalizeAssistantContent(data.Choices[0].Message.Content)
	if out != "" {
		return out, nil
	}
	return coerceAssistantOutputToString(data.Choices[0].Message.Content), nil
}

func lmStudioNative(ctx context.Context, cfg *config.Config, messages []interface{}, model string, temp float64) (string, error) {
	base := strings.TrimRight(cfg.LMStudioURL, "/")
	prompt := messagesToPrompt(messages)
	body := map[string]interface{}{
		"model":       model,
		"input":       prompt,
		"temperature": temp,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/chat", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := LongHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("lm studio: %s", string(raw))
	}
	var data map[string]interface{}
	_ = json.Unmarshal(raw, &data)
	candidates := []string{"output", "result", "generated_text"}
	for _, k := range candidates {
		if v, ok := data[k]; ok {
			if s := coerceAssistantOutputToString(v); s != "" {
				return s, nil
			}
		}
	}
	return "", fmt.Errorf("lm studio: no text in response")
}

func messagesToPrompt(messages []interface{}) string {
	var parts []string
	for _, m := range messages {
		mm, _ := m.(map[string]interface{})
		if mm == nil {
			continue
		}
		role, _ := mm["role"].(string)
		c := normalizeAssistantContent(mm["content"])
		parts = append(parts, strings.ToUpper(role)+": "+c)
	}
	return strings.Join(parts, "\n\n")
}

func pickStr(m map[string]interface{}, key, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

// LongHTTPClient is used for LLM requests (avoid default client short timeouts behind proxies).
func LongHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Minute}
}
