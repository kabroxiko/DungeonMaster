package config

import (
	"os"
	"strconv"
	"strings"
)

// Chat-completions providers an AI call can be sent to. The provider is chosen once, by name, in DM_AI_PROVIDER.
const (
	// ProviderOpenAI is the hosted OpenAI API and the default when DM_AI_PROVIDER is unset.
	ProviderOpenAI = "openai"
	// ProviderDeepSeek is DeepSeek's hosted, OpenAI-compatible API.
	ProviderDeepSeek = "deepseek"
	// ProviderLMStudio is a local LM Studio server, also OpenAI-compatible.
	ProviderLMStudio = "lmstudio"
)

// AIProviderOptions lists the accepted DM_AI_PROVIDER values, for error messages and docs.
var AIProviderOptions = []string{ProviderOpenAI, ProviderDeepSeek, ProviderLMStudio}

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
	case ProviderLMStudio:
		return ProviderLMStudio
	}
	return v
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
	OpenAIModel    string
	// AIProvider is the chat-completions backend every AI call uses (set DM_AI_PROVIDER: openai, deepseek or
	// lmstudio). One selector replaces a boolean per provider: flags allowed two providers to be enabled at once,
	// which the server then had to refuse or guess between, while a single name cannot be ambiguous.
	AIProvider      string
	LMStudioURL     string
	LMStudioModel   string
	DeepSeekURL     string
	DeepSeekModel   string
	DeepSeekAPIKey  string
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
	// HTTPShutdownGraceSeconds caps how long graceful shutdown waits for active HTTP requests (SSE, LLM, etc.).
	HTTPShutdownGraceSeconds int
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
		Port:           getenv("PORT", "5001"),
		BindHost:       strings.TrimSpace(os.Getenv("DM_BIND_HOST")),
		MongoURI:       getenv("DM_MONGODB_URI", ""),
		JWTSecret:      getenv("DM_JWT_SECRET", ""),
		FrontendURL:    strings.TrimRight(getenv("DM_FRONTEND_URL", "http://localhost:8080"), "/"),
		PublicURL:      strings.TrimRight(strings.TrimSpace(os.Getenv("DM_PUBLIC_URL")), "/"),
		GoogleClientID: strings.TrimSpace(os.Getenv("DM_GOOGLE_CLIENT_ID")),
		NodeEnv:        getenv("NODE_ENV", ""),
		OpenAIModel:    getenv("DM_OPENAI_MODEL", "gpt-3.5-turbo"),
		AIProvider:     NormalizeAIProvider(getenv("DM_AI_PROVIDER", ProviderOpenAI)),
		LMStudioURL:    getenv("DM_LM_STUDIO_URL", "http://localhost:1234"),
		// Local LM Studio model id only — do not default to gpt-3.5-turbo (invalid on LM Studio). Set DM_LM_STUDIO_MODEL when DM_AI_PROVIDER=lmstudio.
		LMStudioModel:            strings.TrimSpace(os.Getenv("DM_LM_STUDIO_MODEL")),
		DeepSeekURL:              getenv("DM_DEEPSEEK_URL", "https://api.deepseek.com"),
		DeepSeekModel:            strings.TrimSpace(os.Getenv("DM_DEEPSEEK_MODEL")),
		DeepSeekAPIKey:           strings.TrimSpace(os.Getenv("DM_DEEPSEEK_API_KEY")),
		TrustProxy:               strings.TrimSpace(os.Getenv("DM_TRUST_PROXY")),
		ModelContextTok:          getenvInt("DM_MODEL_CONTEXT_TOKENS", 16384),
		OpenAIAPIKey:             strings.TrimSpace(os.Getenv("DM_OPENAI_API_KEY")),
		VellarisURL:              strings.TrimRight(getenv("DM_VELLARIS_URL", ""), "/"),
		VellarisSettingsFile:     getenv("DM_VELLARIS_SETTINGS_FILE", "assets/world-settings.json"),
		VellarisToken:            strings.TrimSpace(os.Getenv("DM_VELLARIS_TOKEN")),
		DebugLLM:                 TruthyEnv("DM_DEBUG_LLM"),
		DebugCharacterFlow:       TruthyEnv("DM_DEBUG_CHARACTER_FLOW"),
		HTTPShutdownGraceSeconds: getenvInt("DM_HTTP_SHUTDOWN_GRACE_SECONDS", 5),
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
