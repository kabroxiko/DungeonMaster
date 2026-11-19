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
	"sync"
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
// opts["heavy"] marks an authoring call (campaign, character sheet, opening adventure), which is what sends it to the
// heavy tier; play turns leave it unset and use the play provider.
func GenerateResponse(ctx context.Context, cfg *config.Config, input map[string]interface{}, opts map[string]interface{}) (string, error) {
	heavy, _ := opts["heavy"].(bool)
	backend, err := resolveChatBackend(cfg, heavy)
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(pickStr(opts, "model", ""))
	if model == "" {
		model = backend.model
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

	// A reasoning model answers in two parts: hidden analysis first, then the visible turn, both out of the same
	// max_tokens budget. One that thinks too long ends its turn with nothing to show, so the retry asks for less
	// analysis instead of repeating the same request verbatim.
	effort := reasoningEffortFor(opts, backend.baseURL, "")
	retryEffort := "low"
	if _, refused := noReasoningEffort.Load(backend.baseURL); refused {
		retryEffort = ""
	}
	ask := func(effort string) (string, error) {
		if backend.name == lmStudioBackendName {
			return lmStudioChat(ctx, backend, messages, model, maxTok, temp, opts, effort)
		}
		return openAICompatibleChat(ctx, backend, messages, model, maxTok, temp, opts, effort)
	}
	out, err2 := ask(effort)
	if err2 == nil && strings.TrimSpace(out) == "" {
		debugLog(cfg, "empty reply for gameId=%q; asking once more with reasoning_effort=%q", gameID, retryEffort)
		out, err2 = ask(retryEffort)
	}
	if err2 != nil {
		debugLog(cfg, "error: %v", err2)
		return "", err2
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("%s: the model returned no text at all (a reply spent entirely on reasoning is the usual cause); raise max_tokens or use a model that answers directly", backend.name)
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
	// model is what this tier runs when a call does not name one.
	model string
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

// providerEndpoints is where each hosted provider lives. A provider's endpoint is a property of the provider, so no
// configuration variable can move it: the only role that takes a URL is lmstudio, which stands for "an
// OpenAI-compatible server at an address I name". The struct is a variable only so tests can point a provider at a
// local fake server; the server never changes it.
var providerEndpoints = struct {
	openAI   string
	deepSeek string
	gemini   string
}{
	openAI:   "https://api.openai.com",
	deepSeek: "https://api.deepseek.com",
	gemini:   DefaultGeminiBaseURL,
}

// chatURL is where a chat-completions request is POSTed for this backend.
func (b chatBackend) chatURL() string {
	if b.endpoint != "" {
		return b.endpoint
	}
	return chatCompletionsURL(b.baseURL)
}

// resolveChatBackend resolves the endpoint a role runs on. Play turns use the light tier (DM_AI_LIGHT_*); authoring
// calls use the heavy tier, which follows the light one unless DM_AI_HEAVY_PROVIDER names another provider.
//
// A role is a provider, a model and a key. Only lmstudio also takes a URL, because it is the one provider whose address
// is not part of its name; openai, deepseek and gemini live where they live, so a URL set for them is a configuration
// error rather than a value silently ignored.
func resolveChatBackend(cfg *config.Config, heavy bool) (chatBackend, error) {
	if cfg == nil {
		return chatBackend{}, fmt.Errorf("no configuration")
	}

	provider := config.NormalizeAIProvider(cfg.AILightProvider)
	rawURL, model, key := cfg.AILightURL, cfg.AILightModel, cfg.AILightAPIKey
	urlVar, modelVar, keyVar := "DM_AI_LIGHT_URL", "DM_AI_LIGHT_MODEL", "DM_AI_LIGHT_API_KEY"
	if heavy {
		urlVar, modelVar, keyVar = "DM_AI_HEAVY_URL", "DM_AI_HEAVY_MODEL", "DM_AI_HEAVY_API_KEY"
		if other := strings.TrimSpace(cfg.AIHeavyProvider); other != "" {
			// Another provider: its own endpoint, model and key. Nothing is inherited from the light tier, which is what
			// keeps "deepseek" from inheriting the local server's address.
			provider = config.NormalizeAIProvider(other)
			rawURL, model, key = cfg.AIHeavyURL, cfg.AIHeavyModel, cfg.AIHeavyAPIKey
		} else {
			// Same provider and host as the light tier; only what the heavy variables name changes.
			if v := strings.TrimSpace(cfg.AIHeavyURL); v != "" {
				rawURL = v
			}
			if v := strings.TrimSpace(cfg.AIHeavyModel); v != "" {
				model = v
			}
			if v := strings.TrimSpace(cfg.AIHeavyAPIKey); v != "" {
				key = v
			}
		}
	}
	return backendForProvider(cfg, provider, rawURL, model, key, backendSelectors{url: urlVar, model: modelVar, key: keyVar, role: selectorRole(heavy)})
}

// backendSelectors names the variables of one role, so every error points at the line the operator has to edit.
type backendSelectors struct {
	role  string
	url   string
	model string
	key   string
}

func selectorRole(heavy bool) string {
	if heavy {
		return "DM_AI_HEAVY"
	}
	return "DM_AI_LIGHT"
}

// backendForProvider builds the backend for one provider. The endpoint comes from the provider itself for the hosted
// APIs and from the role for lmstudio; anything else is refused with the variable that caused it.
func backendForProvider(cfg *config.Config, provider, rawURL, model, apiKey string, sel backendSelectors) (chatBackend, error) {
	base := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	model = strings.TrimSpace(model)
	apiKey = strings.TrimSpace(apiKey)

	// A hosted provider has one address. Accepting an override here is exactly how "deepseek" ended up pointing at the
	// LM Studio box someone had configured a minute earlier.
	fixedURL := func(endpoint string) error {
		if base == "" {
			return nil
		}
		return fmt.Errorf("%s is not used with %s: its endpoint is %s. Clear it, or name lmstudio to reach an OpenAI-compatible server at an address you choose",
			sel.url, provider, endpoint)
	}

	switch provider {
	case config.ProviderLMStudio:
		if base == "" {
			return chatBackend{}, fmt.Errorf("%s is required with %s_PROVIDER=lmstudio: point it at the machine running the OpenAI-compatible server (Ollama or LM Studio)", sel.url, sel.role)
		}
		if model == "" {
			return chatBackend{}, fmt.Errorf("%s_PROVIDER=lmstudio needs %s set to a model id that server has loaded (e.g. llama3.1:8b-instruct-q4_K_M)", sel.role, sel.model)
		}
		return chatBackend{name: lmStudioBackendName, baseURL: base, model: model, apiKey: apiKey, keyEnv: sel.key}, nil
	case config.ProviderDeepSeek:
		if err := fixedURL(providerEndpoints.deepSeek); err != nil {
			return chatBackend{}, err
		}
		if model == "" {
			model = "deepseek-chat"
		}
		if apiKey == "" {
			apiKey = cfg.DeepSeekAPIKey
		}
		return chatBackend{name: deepSeekBackendName, baseURL: providerEndpoints.deepSeek, model: model, apiKey: apiKey, keyEnv: keyEnvFor(sel, apiKey, cfg.DeepSeekAPIKey, "DM_DEEPSEEK_API_KEY")}, nil
	case config.ProviderGemini:
		if err := fixedURL(providerEndpoints.gemini); err != nil {
			return chatBackend{}, err
		}
		if model == "" {
			model = "gemini-2.5-flash"
		}
		if apiKey == "" {
			apiKey = cfg.GeminiAPIKey
		}
		// Gemini's compatibility layer lives under /v1beta/openai rather than /v1, so the route is derived from the base.
		return chatBackend{name: geminiBackendName, baseURL: providerEndpoints.gemini, endpoint: geminiChatURL(providerEndpoints.gemini), model: model, apiKey: apiKey, keyEnv: keyEnvFor(sel, apiKey, cfg.GeminiAPIKey, "DM_GEMINI_API_KEY")}, nil
	case config.ProviderOpenAI:
		if err := fixedURL(providerEndpoints.openAI); err != nil {
			return chatBackend{}, err
		}
		if model == "" {
			model = "gpt-3.5-turbo"
		}
		if apiKey == "" {
			apiKey = cfg.OpenAIAPIKey
		}
		return chatBackend{name: openAIBackendName, baseURL: providerEndpoints.openAI, model: model, apiKey: apiKey, keyEnv: keyEnvFor(sel, apiKey, cfg.OpenAIAPIKey, "DM_OPENAI_API_KEY")}, nil
	default:
		return chatBackend{}, fmt.Errorf("unknown %s_PROVIDER %q: use %s", sel.role, provider, strings.Join(config.AIProviderOptions, ", "))
	}
}

// keyEnvFor names the variable an operator has to set when a request fails for want of a key: the tier's own, or the
// provider's when the tier borrowed it.
func keyEnvFor(sel backendSelectors, tierKey, providerKey, providerEnv string) string {
	if strings.TrimSpace(tierKey) != "" && tierKey != providerKey {
		return sel.key
	}
	if strings.TrimSpace(providerKey) != "" || tierKey == "" {
		return providerEnv
	}
	return sel.key
}

// BackendFor reports the provider, endpoint and model a tier resolves to. Startup logs both tiers, and it makes a
// misconfigured provider name or missing model id fail at boot instead of on the first player action.
func BackendFor(cfg *config.Config, heavy bool) (name, baseURL, model string, err error) {
	backend, err := resolveChatBackend(cfg, heavy)
	if err != nil {
		return "", "", "", err
	}
	return backend.name, backend.baseURL, backend.model, nil
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

func openAICompatibleChat(ctx context.Context, backend chatBackend, messages []interface{}, model string, maxTok int, temp float64, opts map[string]interface{}, effort string) (string, error) {
	key := backend.apiKey
	if key == "" {
		return "", fmt.Errorf("%s is not set", backend.keyEnv)
	}
	post := func(effort string) ([]byte, string, error) {
		body := map[string]interface{}{
			"model":       model,
			"messages":    messages,
			"max_tokens":  maxTok,
			"temperature": temp,
		}
		if rf, ok := opts["response_format"]; ok {
			body["response_format"] = rf
		}
		if effort != "" {
			body["reasoning_effort"] = effort
		}
		b, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.chatURL(), bytes.NewReader(b))
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := LongHTTPClient().Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return raw, resp.Status, nil
	}
	raw, status, err := post(effort)
	if err != nil {
		return "", err
	}
	// reasoning_effort is a knob this call added, not something the caller asked for: a server that does not know the
	// field refuses it, and the answer matters more than the knob, so ask once more without it.
	if effort != "" && strings.Contains(strings.ToLower(string(raw)), "reasoning_effort") {
		noReasoningEffort.Store(backend.baseURL, struct{}{})
		if raw, status, err = post(""); err != nil {
			return "", err
		}
	}
	if !strings.HasPrefix(status, "2") {
		return "", fmt.Errorf("%s: %s: %s", backend.name, status, string(raw))
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
		return "", truncatedReplyError(backend.name, maxTok)
	}
	if out := stripHarmonyChannels(normalizeAssistantContent(data.Choices[0].Message.Content)); out != "" {
		return out, nil
	}
	return stripHarmonyChannels(coerceAssistantOutputToString(data.Choices[0].Message.Content)), nil
}

// truncatedReplyError names the budget that ran out. Reasoning models spend part of max_tokens on hidden tokens, so
// the visible document can stop well before the budget looks spent.
func truncatedReplyError(backendName string, maxTok int) error {
	return fmt.Errorf("%s: reply hit max_tokens (%d) before finishing, so it was cut off; reasoning models spend part of that budget on hidden tokens, so raise max_tokens or use a model that does not reason", backendName, maxTok)
}

// noJsonMode remembers, per local endpoint, that the server refused response_format. Ollama takes json_object and
// enforces it as a grammar; an LM Studio build that does not take it must keep working, so one refusal is enough to stop
// asking and the same call is retried without the field.
var noJsonMode sync.Map

// noReasoningEffort remembers, per endpoint, that the server refused reasoning_effort. A reasoning model needs the
// knob to finish its visible answer inside the budget, but a server that does not know the field must keep working.
var noReasoningEffort sync.Map

// reasoningEffortFor returns the reasoning_effort to send: an explicit opts value, or the fallback, unless this endpoint
// refused the field before.
func reasoningEffortFor(opts map[string]interface{}, baseURL, fallback string) string {
	if _, refused := noReasoningEffort.Load(baseURL); refused {
		return ""
	}
	if v, _ := opts["reasoning_effort"].(string); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

// responseFormatFor returns the response_format this call should send, or nil when it has none or this endpoint refused
// it before.
func responseFormatFor(opts map[string]interface{}, baseURL string) map[string]interface{} {
	rf, ok := opts["response_format"].(map[string]interface{})
	if !ok {
		return nil
	}
	switch t, _ := rf["type"].(string); t {
	case "json_object", "json_schema", "text":
	default:
		return nil
	}
	if _, refused := noJsonMode.Load(baseURL); refused {
		return nil
	}
	return rf
}

// lmStudioRequest posts one chat-completions call and hands back the body and status as they arrived. The caller decides
// what a 4xx means: a refused response_format is retried without it, anything else falls back to the native route.
func lmStudioRequest(ctx context.Context, backend chatBackend, messages []interface{}, model string, maxTok int, temp float64, responseFormat map[string]interface{}, effort string) ([]byte, int, error) {
	body := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  maxTok,
		"temperature": temp,
	}
	if responseFormat != nil {
		body["response_format"] = responseFormat
	}
	if effort != "" {
		body["reasoning_effort"] = effort
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.baseURL+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if backend.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+backend.apiKey)
	}
	resp, err := LongHTTPClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

// lmStudioChat talks to an OpenAI-compatible local host. It takes the resolved backend rather than the configuration,
// because the endpoint is per tier: the authoring tier may name a different machine than the play tier.
func lmStudioChat(ctx context.Context, backend chatBackend, messages []interface{}, model string, maxTok int, temp float64, opts map[string]interface{}, effort string) (string, error) {
	rf := responseFormatFor(opts, backend.baseURL)
	raw, status, err := lmStudioRequest(ctx, backend, messages, model, maxTok, temp, rf, effort)
	if err != nil {
		return "", err
	}
	// A server that does not know a field says so in its 4xx body. Drop that field and ask once more, rather than
	// dropping to the native API - which an Ollama host does not serve at all.
	for status >= 400 {
		body := strings.ToLower(string(raw))
		dropped := false
		if rf != nil && strings.Contains(body, "response_format") {
			noJsonMode.Store(backend.baseURL, struct{}{})
			rf, dropped = nil, true
		} else if effort != "" && strings.Contains(body, "reasoning_effort") {
			noReasoningEffort.Store(backend.baseURL, struct{}{})
			effort, dropped = "", true
		}
		if !dropped {
			break
		}
		if raw, status, err = lmStudioRequest(ctx, backend, messages, model, maxTok, temp, rf, effort); err != nil {
			return "", err
		}
	}
	if status >= 400 {
		return lmStudioNative(ctx, backend, messages, model, temp)
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
		return lmStudioNative(ctx, backend, messages, model, temp)
	}
	if len(data.Choices) == 0 {
		return lmStudioNative(ctx, backend, messages, model, temp)
	}
	// The native fallback would re-run the whole generation, so a cut-off reply is reported, not retried.
	if strings.EqualFold(data.Choices[0].FinishReason, "length") {
		return "", truncatedReplyError(lmStudioBackendName, maxTok)
	}
	if out := stripHarmonyChannels(normalizeAssistantContent(data.Choices[0].Message.Content)); out != "" {
		return out, nil
	}
	return stripHarmonyChannels(coerceAssistantOutputToString(data.Choices[0].Message.Content)), nil
}

func lmStudioNative(ctx context.Context, backend chatBackend, messages []interface{}, model string, temp float64) (string, error) {
	prompt := messagesToPrompt(messages)
	body := map[string]interface{}{
		"model":       model,
		"input":       prompt,
		"temperature": temp,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.baseURL+"/api/v1/chat", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if backend.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+backend.apiKey)
	}
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
