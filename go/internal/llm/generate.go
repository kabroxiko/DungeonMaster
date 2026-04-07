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

// GenerateResponse performs chat-completions against OpenAI or LM Studio.
func GenerateResponse(ctx context.Context, cfg *config.Config, input map[string]interface{}, opts map[string]interface{}) (string, error) {
	model := strings.TrimSpace(pickStr(opts, "model", ""))
	if model == "" {
		if cfg.UseLMStudio {
			model = strings.TrimSpace(cfg.LMStudioModel)
			if model == "" {
				return "", fmt.Errorf("LM Studio is enabled (DM_USE_LM_STUDIO): set DM_LM_STUDIO_MODEL to a loaded model id exactly as shown in LM Studio (e.g. mistralai/mistral-nemo-instruct-2407@q4_k_m); OpenAI names like gpt-3.5-turbo are rejected by the local server")
			}
		} else {
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
		map[bool]string{false: "openai", true: "lm_studio"}[cfg.UseLMStudio],
		model, maxTok, temp, gameID, len(messages))
	debugLogRequestMessages(cfg, messages)

	var out string
	var err error
	if cfg.UseLMStudio {
		out, err = lmStudioChat(ctx, cfg, messages, model, maxTok, temp, opts)
	} else {
		out, err = openAIChat(ctx, cfg, messages, model, maxTok, temp, opts)
	}
	if err != nil {
		debugLog(cfg, "error: %v", err)
		return "", err
	}
	debugLog(cfg, "ok gameId=%q out_len=%d", gameID, len(out))
	debugLogLongLines(cfg, "llm-debug out|", out)
	return out, nil
}

func openAIChat(ctx context.Context, cfg *config.Config, messages []interface{}, model string, maxTok int, temp float64, opts map[string]interface{}) (string, error) {
	key := cfg.OpenAIAPIKey
	if key == "" {
		return "", fmt.Errorf("DM_OPENAI_API_KEY is not set")
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
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
		return "", fmt.Errorf("openai: %s: %s", resp.Status, string(raw))
	}
	var data struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	if len(data.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices")
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
