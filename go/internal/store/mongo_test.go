package store

import (
	"strings"
	"testing"
)

// A database password must never reach a log line, whichever shape the driver echoes the URI back in.
func TestRedactSecretsRemovesTheDatabasePassword(t *testing.T) {
	const uri = "mongodb://dungeonmaster:s3cret-pass@mongo:27017/dungeonmaster?authSource=admin"
	messages := []string{
		"server selection error for " + uri,
		// A URI the driver normalized differs from the configured one, so matching the string alone is not enough.
		"error parsing uri: " + strings.Replace(uri, "authSource", "authsource", 1),
		"authentication failed while using password s3cret-pass",
	}
	for _, message := range messages {
		got := RedactSecrets(message, uri)
		if strings.Contains(got, "s3cret-pass") {
			t.Fatalf("the password survived redaction: %s", got)
		}
	}

	plain := "connection refused"
	if got := RedactSecrets(plain, uri); got != plain {
		t.Fatalf("an unrelated message must be left alone, got %q", got)
	}
	if got := RedactSecrets(plain, ""); got != plain {
		t.Fatalf("with no configured URI nothing is redacted, got %q", got)
	}
}
