package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// A local endpoint written as https against a server that speaks http fails at the TLS handshake with
// "server gave HTTP response to HTTPS client". It used to surface as a failed document generation, which reads like a
// model problem and sends the table looking for a key that a local server does not have. The call is retried over http
// and the endpoint stays on it.
func TestHttpsUrlThatAnswersPlainHttpIsRetriedOverHttp(t *testing.T) {
	// A plain-HTTP listener: an https request to it is answered by the TLS client seeing HTTP bytes come back.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answerBody))
	}))
	defer srv.Close()

	// The listener's own URL is http://host:port; writing it as https:// is exactly the misconfiguration.
	httpsURL := "https://" + strings.TrimPrefix(srv.URL, "http://")
	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: httpsURL, AILightModel: "local/model"}

	out, err := generate(t, cfg, map[string]interface{}{"max_tokens": 800})
	if err != nil {
		t.Fatalf("the call should have been repaired over http, got %v", err)
	}
	if out != "Bram hits the goblin." {
		t.Fatalf("unexpected content: %q", out)
	}
	if got := effectiveBaseURL(httpsURL); got == httpsURL {
		t.Fatalf("the endpoint should stay downgraded for the process, still %s", got)
	}
	if !strings.HasPrefix(effectiveBaseURL(httpsURL), "http://") {
		t.Fatalf("expected http after the downgrade, got %s", effectiveBaseURL(httpsURL))
	}
}

// A server that really does speak https must not be silently downgraded: that would turn a certificate problem into
// cleartext traffic.
func TestAnHttpsServerStaysHttps(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(answerBody))
	}))
	defer srv.Close()

	cfg := &config.Config{AILightProvider: config.ProviderLMStudio, AILightURL: srv.URL, AILightModel: "local/model"}
	// The test server's certificate is not trusted, so this fails - and it must fail as a TLS error, not be repaired.
	if _, err := generate(t, cfg, map[string]interface{}{"max_tokens": 800}); err == nil {
		t.Fatal("expected the untrusted certificate to fail the call")
	}
	if got := effectiveBaseURL(srv.URL); got != srv.URL {
		t.Fatalf("an https endpoint must not be downgraded, got %s", got)
	}
}
