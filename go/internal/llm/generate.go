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

// GenerateResponse performs chat-completions against OpenAI, DeepSeek, Google AI Studio (Gemini), or LM Studio.
func GenerateResponse(ctx context.Context, cfg *config.Config, input map[string]interface{}, opts map[string]interface{}) (string, error) {
	backend, err := resolveChatBackend(cfg)
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(pickStr(opts, "model", ""))
	if model == "" {
		model, err = defaultModelFor(cfg, backend)
		if err != nil {
			return "", err
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
	if backend.name == lmStudioBackendName {
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

// chatBackend is the endpoint chat completions are sent to. OpenAI, DeepSeek and Gemini's compatibility layer speak
// the same protocol, so they differ only in where the request goes, which key authorises it, and the name that appears
// in errors.
type chatBackend struct {
	name    string
	baseURL string
	// endpoint, when set, is the full chat-completions URL to POST to. Gemini serves its compatibility layer under
	// /v1beta/openai rather than /v1, so it sets this instead of relying on the base URL convention below.
	endpoint string
	apiKey   string
	keyEnv   string
}

const (
	lmStudioBackendName = "lm_studio"
	deepSeekBackendName = "deepseek"
	geminiBackendName   = "gemini"
	openAIBackendName   = "openai"
	// DefaultGeminiBaseURL is the base of Gemini's OpenAI-compatible surface, the one the AI Studio key unlocks. It is
	// the whole base including /v1beta/openai; only the chat-completions route gets appended to it.
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
)

// chatURL is where a chat-completions request is POSTed for this backend.
func (b chatBackend) chatURL() string {
	if b.endpoint != "" {
		return b.endpoint
	}
	return chatCompletionsURL(b.baseURL)
}

// resolveChatBackend picks the provider named by DM_AI_PROVIDER. It is one name, so there is no combination to guess
// between; an unknown name is an error rather than a silent fallback to a provider the operator did not choose.
func resolveChatBackend(cfg *config.Config) (chatBackend, error) {
	if cfg == nil {
		return chatBackend{}, fmt.Errorf("no configuration")
	}
	switch config.NormalizeAIProvider(cfg.AIProvider) {
	case config.ProviderLMStudio:
		base := strings.TrimRight(cfg.LMStudioURL, "/")
		if base == "" {
			base = "http://localhost:1234"
		}
		return chatBackend{name: lmStudioBackendName, baseURL: base}, nil
	case config.ProviderDeepSeek:
		base := strings.TrimRight(cfg.DeepSeekURL, "/")
		if base == "" {
			base = "https://api.deepseek.com"
		}
		return chatBackend{name: deepSeekBackendName, baseURL: base, apiKey: cfg.DeepSeekAPIKey, keyEnv: "DM_DEEPSEEK_API_KEY"}, nil
	case config.ProviderGemini:
		base := strings.TrimRight(cfg.GeminiURL, "/")
		if base == "" {
			base = DefaultGeminiBaseURL
		}
		return chatBackend{name: geminiBackendName, baseURL: base, endpoint: geminiChatURL(base), apiKey: cfg.GeminiAPIKey, keyEnv: "DM_GEMINI_API_KEY"}, nil
	case config.ProviderOpenAI:
		return chatBackend{name: openAIBackendName, baseURL: "https://api.openai.com", apiKey: cfg.OpenAIAPIKey, keyEnv: "DM_OPENAI_API_KEY"}, nil
	default:
		return chatBackend{}, fmt.Errorf("unknown DM_AI_PROVIDER %q: use %s", cfg.AIProvider, strings.Join(config.AIProviderOptions, ", "))
	}
}

// defaultModelFor returns the model a backend runs when a call does not name one.
func defaultModelFor(cfg *config.Config, backend chatBackend) (string, error) {
	switch backend.name {
	case lmStudioBackendName:
		model := strings.TrimSpace(cfg.LMStudioModel)
		if model == "" {
			return "", fmt.Errorf("DM_AI_PROVIDER=lmstudio needs DM_LM_STUDIO_MODEL set to a loaded model id exactly as shown in LM Studio (e.g. mistralai/mistral-nemo-instruct-2407@q4_k_m); OpenAI names like gpt-3.5-turbo are rejected by the local server")
		}
		return model, nil
	case deepSeekBackendName:
		model := strings.TrimSpace(cfg.DeepSeekModel)
		if model == "" {
			model = "deepseek-chat"
		}
		return model, nil
	case geminiBackendName:
		model := strings.TrimSpace(cfg.GeminiModel)
		if model == "" {
			model = "gemini-2.5-flash"
		}
		return model, nil
	default:
		model := strings.TrimSpace(cfg.OpenAIModel)
		if model == "" {
			model = "gpt-3.5-turbo"
		}
		return model, nil
	}
}

// BackendFor reports the provider, endpoint and model this configuration resolves to. Startup logs it, and it makes a
// misconfigured DM_AI_PROVIDER or missing model id fail at boot instead of on the first player action.
func BackendFor(cfg *config.Config) (name, baseURL, model string, err error) {
	backend, err := resolveChatBackend(cfg)
	if err != nil {
		return "", "", "", err
	}
	model, err = defaultModelFor(cfg, backend)
	if err != nil {
		return backend.name, backend.baseURL, "", err
	}
	return backend.name, backend.baseURL, model, nil
}

// chatCompletionsURL accepts a base URL with or without the /v1 suffix, which providers document inconsistently.
func chatCompletionsURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

// geminiChatURL appends the chat-completions route to Gemini's OpenAI-compatible base. The /v1 convention above cannot
// be reused: Gemini's compatibility layer lives under /v1beta/openai, so the base already includes it and only the route
// is appended. A base that names the route already is left alone, mirroring how chatCompletionsURL tolerates /v1.
func geminiChatURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultGeminiBaseURL
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.chatURL(), bytes.NewReader(b))
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
