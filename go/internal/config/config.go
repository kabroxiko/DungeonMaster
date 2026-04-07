package config

import (
	"os"
	"strconv"
	"strings"
)

// Config mirrors env.example in the repository root.
type Config struct {
	Port            string
	BindHost        string
	MongoURI        string
	JWTSecret       string
	FrontendURL     string
	PublicURL       string
	GoogleClientID  string
	NodeEnv         string
	OpenAIModel     string
	UseLMStudio     bool
	LMStudioURL     string
	LMStudioModel   string
	TrustProxy      string
	ModelContextTok int
	OpenAIAPIKey    string
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
		Port:            getenv("PORT", "5001"),
		BindHost:        strings.TrimSpace(os.Getenv("DM_BIND_HOST")),
		MongoURI:        getenv("DM_MONGODB_URI", ""),
		JWTSecret:       getenv("DM_JWT_SECRET", ""),
		FrontendURL:     strings.TrimRight(getenv("DM_FRONTEND_URL", "http://localhost:8080"), "/"),
		PublicURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("DM_PUBLIC_URL")), "/"),
		GoogleClientID:  strings.TrimSpace(os.Getenv("DM_GOOGLE_CLIENT_ID")),
		NodeEnv:         getenv("NODE_ENV", ""),
		OpenAIModel:     getenv("DM_OPENAI_MODEL", "gpt-3.5-turbo"),
		UseLMStudio:     TruthyEnv("DM_USE_LM_STUDIO"),
		LMStudioURL:     getenv("DM_LM_STUDIO_URL", "http://localhost:1234"),
		// Local LM Studio model id only — do not default to gpt-3.5-turbo (invalid on LM Studio). Set DM_LM_STUDIO_MODEL when DM_USE_LM_STUDIO=true.
		LMStudioModel:   strings.TrimSpace(os.Getenv("DM_LM_STUDIO_MODEL")),
		TrustProxy:      strings.TrimSpace(os.Getenv("DM_TRUST_PROXY")),
		ModelContextTok: getenvInt("DM_MODEL_CONTEXT_TOKENS", 16384),
		OpenAIAPIKey:    strings.TrimSpace(os.Getenv("DM_OPENAI_API_KEY")),
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
