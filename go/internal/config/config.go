package config

import (
	"os"
	"strconv"
	"strings"
)

// Chat-completions providers an AI call can be sent to. Each role chooses one by name: DM_AI_LIGHT_PROVIDER for play
// turns, DM_AI_HEAVY_PROVIDER for the authoring documents.
const (
	// ProviderOpenAI is the hosted OpenAI API and the default when DM_AI_LIGHT_PROVIDER is unset.
	ProviderOpenAI = "openai"
	// ProviderDeepSeek is DeepSeek's hosted, OpenAI-compatible API.
	ProviderDeepSeek = "deepseek"
	// ProviderGemini is Google's Gemini API, the one behind Google AI Studio, through its OpenAI-compatible surface.
	// It is a plain API key on the generativelanguage endpoint; Vertex AI (a GCP project and service account) is a
	// different surface and is not what this provider selects.
	ProviderGemini = "gemini"
	// ProviderLMStudio is a local LM Studio server, also OpenAI-compatible.
	ProviderLMStudio = "lmstudio"
)

// AIProviderOptions lists the accepted provider names, for error messages and docs.
var AIProviderOptions = []string{ProviderOpenAI, ProviderDeepSeek, ProviderGemini, ProviderLMStudio}

// NormalizeAIProvider lowercases and trims a provider name and folds the spellings of LM Studio (lm-studio,
// lm_studio, lm studio) into ProviderLMStudio. An unknown value comes back trimmed and lowercased, so the backend
// resolver can name it back to the operator instead of silently falling back to a provider nobody asked for.
func NormalizeAIProvider(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.NewReplacer(" ", "", "_", "", "-", "").Replace(v)
	switch v {
	case "", ProviderOpenAI:
		return ProviderOpenAI
	case ProviderDeepSeek:
		return ProviderDeepSeek
	case ProviderGemini, "google", "googleai", "googleaistudio", "aistudio":
		// AI Studio is how an operator gets the key, so the names they read on the console fold onto one provider.
		return ProviderGemini
	case ProviderLMStudio:
		return ProviderLMStudio
	}
	return v
}

// normalizeOptionalProvider folds a provider name the same way NormalizeAIProvider does, but keeps an empty value
// empty: "no heavy provider configured" has to stay distinguishable from "OpenAI", because it means the heavy tier
// falls back to the play tier.
func normalizeOptionalProvider(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return NormalizeAIProvider(raw)
}

// Config mirrors env.example in the repository root.
type Config struct {
	Port           string
	BindHost       string
	MongoURI       string
	JWTSecret      string
	FrontendURL    string
	PublicURL      string
	GoogleClientID string
	NodeEnv        string
	// The chat-completions endpoints are chosen by role, not by provider: DM_AI_LIGHT_* is the model that answers play
	// turns, DM_AI_HEAVY_* the one that writes the long schema-bound documents (campaign, its stages, character sheets,
	// the opening adventure). Each role carries its own provider, endpoint, model and key, so the local model a table
	// plays on and the cloud model that authors can be configured without a shared "default" that is really neither.
	//
	// AILightProvider names the backend play turns use (DM_AI_LIGHT_PROVIDER: openai, deepseek, gemini or lmstudio). An
	// unknown name stops the server at boot instead of silently using another provider. AILightModel overrides the
	// model that provider runs; AILightURL is read only for lmstudio, and is required there, because it is the one
	// provider whose address is not part of its name.
	AILightProvider string
	AILightURL      string
	AILightModel    string
	// AILightAPIKey overrides the provider's own key (DM_AI_LIGHT_API_KEY), for a second account or a local endpoint
	// behind a proxy that requires one.
	AILightAPIKey string
	// AIHeavyProvider is the provider the authoring calls use. Empty means the authoring calls run on the light tier,
	// which is the single-provider setup: play local, author on the same local model.
	AIHeavyProvider string
	// AIHeavyModel and AIHeavyAPIKey override the heavy tier's model and key. AIHeavyURL follows the same rule as the
	// light one: it is read only when the resolved provider is lmstudio. With no heavy provider, these override the
	// light tier's values instead, so a better model on the same host needs one variable.
	AIHeavyURL    string
	AIHeavyModel  string
	AIHeavyAPIKey string
	// The per-provider keys stay, because a key belongs to an account and not to a role: a tier that names no key of
	// its own uses the key of the provider it resolves to.
	DeepSeekAPIKey  string
	GeminiAPIKey    string
	TrustProxy      string
	ModelContextTok int
	OpenAIAPIKey    string
	// VellarisURL is the base URL of the Vellaris map service (set DM_VELLARIS_URL). Empty disables importing a
	// world from settings and on-demand route lookups; a campaign can still be given a world directly.
	VellarisURL string
	// VellarisSettingsFile is the map settings document behind a campaign's generated world (set
	// DM_VELLARIS_SETTINGS_FILE). The server copies the document and gives each campaign a fresh seed, so the file
	// describes the kind of map (size, art pack, probabilities) rather than one fixed world.
	VellarisSettingsFile string
	// VellarisToken is the shared secret for the map service's stored-map routes (set DM_VELLARIS_TOKEN). The service
	// requires it when it is configured with one; empty is correct for a service that needs no token.
	VellarisToken string
	// DebugLLM enables verbose stderr logs for every LLM request/response in full (set DM_DEBUG_LLM=true). Use only in development.
	DebugLLM bool
	// DebugCharacterFlow logs preview-name / generate-character gameSetup summaries (set DM_DEBUG_CHARACTER_FLOW=true). Use only in development.
	DebugCharacterFlow bool
	// AdminEmails lists the accounts allowed into the admin surfaces (set DM_ADMIN_EMAILS, comma-separated). These people
	// see the server configuration and the stored prompts; everyone else plays, and nothing offers them what only an
	// operator should read.
	AdminEmails []string
	// HTTPShutdownGraceSeconds caps how long graceful shutdown waits for active HTTP requests (SSE, LLM, etc.).
	HTTPShutdownGraceSeconds int
	// LogFile mirrors the process log to a file (set DM_LOG_FILE). Empty means no file, except in development, where
	// the server writes tmp/gmai-server.log so the backend's lines survive a terminal shared with the Vue dev server.
	LogFile string
}

// IsAdminEmail reports whether the address is configured as an administrator's. Google may spell an address with
// different casing, so the comparison folds case.
func (c *Config) IsAdminEmail(email string) bool {
	if c == nil {
		return false
	}
	want := strings.ToLower(strings.TrimSpace(email))
	if want == "" {
		return false
	}
	for _, e := range c.AdminEmails {
		if e == want {
			return true
		}
	}
	return false
}

// parseEmailList reads a comma-separated address list, folded and without blanks.
func parseEmailList(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if e := strings.ToLower(strings.TrimSpace(part)); e != "" {
			out = append(out, e)
		}
	}
	return out
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func getenvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// TruthyEnv parses typical DM_* boolean env vars. True for true, yes, 1, on (case-insensitive).
// Empty, false, no, 0, off, or any other value is false.
func TruthyEnv(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return false
	}
	switch v {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Load reads process environment.
func Load() *Config {
	c := &Config{
		Port:            getenv("PORT", "5001"),
		BindHost:        strings.TrimSpace(os.Getenv("DM_BIND_HOST")),
		MongoURI:        getenv("DM_MONGODB_URI", ""),
		JWTSecret:       getenv("DM_JWT_SECRET", ""),
		FrontendURL:     strings.TrimRight(getenv("DM_FRONTEND_URL", "http://localhost:8080"), "/"),
		PublicURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("DM_PUBLIC_URL")), "/"),
		GoogleClientID:  strings.TrimSpace(os.Getenv("DM_GOOGLE_CLIENT_ID")),
		NodeEnv:         getenv("NODE_ENV", ""),
		AILightProvider: NormalizeAIProvider(os.Getenv("DM_AI_LIGHT_PROVIDER")),
		AILightURL:      strings.TrimSpace(os.Getenv("DM_AI_LIGHT_URL")),
		AILightModel:    strings.TrimSpace(os.Getenv("DM_AI_LIGHT_MODEL")),
		AILightAPIKey:   strings.TrimSpace(os.Getenv("DM_AI_LIGHT_API_KEY")),
		// An empty heavy provider means "author on the light tier", which NormalizeAIProvider cannot express: it folds
		// "" onto OpenAI, and that would silently move authoring to the cloud on a local-only setup.
		AIHeavyProvider:          normalizeOptionalProvider(os.Getenv("DM_AI_HEAVY_PROVIDER")),
		AIHeavyURL:               strings.TrimSpace(os.Getenv("DM_AI_HEAVY_URL")),
		AIHeavyModel:             strings.TrimSpace(os.Getenv("DM_AI_HEAVY_MODEL")),
		AIHeavyAPIKey:            strings.TrimSpace(os.Getenv("DM_AI_HEAVY_API_KEY")),
		DeepSeekAPIKey:           strings.TrimSpace(os.Getenv("DM_DEEPSEEK_API_KEY")),
		GeminiAPIKey:             strings.TrimSpace(os.Getenv("DM_GEMINI_API_KEY")),
		TrustProxy:               strings.TrimSpace(os.Getenv("DM_TRUST_PROXY")),
		ModelContextTok:          getenvInt("DM_MODEL_CONTEXT_TOKENS", 16384),
		OpenAIAPIKey:             strings.TrimSpace(os.Getenv("DM_OPENAI_API_KEY")),
		VellarisURL:              strings.TrimRight(getenv("DM_VELLARIS_URL", ""), "/"),
		VellarisSettingsFile:     getenv("DM_VELLARIS_SETTINGS_FILE", "assets/world-settings.json"),
		VellarisToken:            strings.TrimSpace(os.Getenv("DM_VELLARIS_TOKEN")),
		DebugLLM:                 TruthyEnv("DM_DEBUG_LLM"),
		DebugCharacterFlow:       TruthyEnv("DM_DEBUG_CHARACTER_FLOW"),
		AdminEmails:              parseEmailList(getenv("DM_ADMIN_EMAILS", "")),
		HTTPShutdownGraceSeconds: getenvInt("DM_HTTP_SHUTDOWN_GRACE_SECONDS", 5),
		LogFile:                  strings.TrimSpace(os.Getenv("DM_LOG_FILE")),
	}
	if c.HTTPShutdownGraceSeconds < 1 {
		c.HTTPShutdownGraceSeconds = 1
	}
	if c.HTTPShutdownGraceSeconds > 120 {
		c.HTTPShutdownGraceSeconds = 120
	}
	if c.JWTSecret == "" && (c.NodeEnv == "development" || c.NodeEnv == "dev") {
		c.JWTSecret = "dev-only-insecure-jwt-secret-change-for-production"
	}
	return c
}
