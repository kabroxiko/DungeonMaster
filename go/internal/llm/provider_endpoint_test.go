package llm

import (
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// pointProviderAt sends a hosted provider to a local fake server. Configuration cannot do this - a provider's endpoint
// is fixed by the provider - so tests reach into the table the server reads. Tests do not run in parallel, so moving the
// entry and restoring it afterwards is safe.
func pointProviderAt(t *testing.T, provider, baseURL string) {
	t.Helper()
	switch provider {
	case config.ProviderDeepSeek:
		old := providerEndpoints.deepSeek
		providerEndpoints.deepSeek = baseURL
		t.Cleanup(func() { providerEndpoints.deepSeek = old })
	case config.ProviderGemini:
		old := providerEndpoints.gemini
		providerEndpoints.gemini = baseURL
		t.Cleanup(func() { providerEndpoints.gemini = old })
	case config.ProviderOpenAI:
		old := providerEndpoints.openAI
		providerEndpoints.openAI = baseURL
		t.Cleanup(func() { providerEndpoints.openAI = old })
	default:
		t.Fatalf("%s has no hosted endpoint to move", provider)
	}
}
