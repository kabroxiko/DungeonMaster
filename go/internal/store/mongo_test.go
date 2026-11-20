package store

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

// Every reader of a stored list asserts []interface{}, so the client must hand back that shape.
//
// The driver's default decodes a BSON array into primitive.A, a named slice type: the assertion yields nil and the
// reader silently sees an empty list. That is how a DM turn read the stored conversation as empty and wrote a transcript
// holding only the reply it had just appended.
func TestRegistryDecodesArraysAsPlainSlices(t *testing.T) {
	raw, err := bson.Marshal(bson.M{
		"conversation": bson.A{bson.M{"role": "user", "content": "entro a la taberna"}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc map[string]interface{}
	if err := bson.UnmarshalWithRegistry(bsonRegistry(), raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	conv, ok := doc["conversation"].([]interface{})
	if !ok {
		t.Fatalf("conversation decoded as %T, want []interface{}", doc["conversation"])
	}
	if len(conv) != 1 {
		t.Fatalf("expected one stored message, got %d", len(conv))
	}
	msg, ok := conv[0].(map[string]interface{})
	if !ok {
		t.Fatalf("a stored message decoded as %T, want map[string]interface{}", conv[0])
	}
	if msg["content"] != "entro a la taberna" {
		t.Fatalf("unexpected content: %v", msg["content"])
	}
}

// SliceOf reads a list that arrived in either shape, so a reader cannot be emptied by the decode shape alone.
func TestSliceOfAcceptsBothDecodedShapes(t *testing.T) {
	plain := []interface{}{"a", "b"}
	if got := SliceOf(plain); len(got) != len(plain) {
		t.Fatalf("plain slice: got %d", len(got))
	}
	if got := SliceOf(primitive.A{"a", "b"}); len(got) != 2 {
		t.Fatalf("primitive.A: got %d", len(got))
	}
	if got := SliceOf(nil); got != nil {
		t.Fatalf("nil should stay nil, got %v", got)
	}
	if got := SliceOf("not a list"); got != nil {
		t.Fatalf("a non-list should stay nil, got %v", got)
	}
}
