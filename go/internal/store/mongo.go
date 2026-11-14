package store

import (
	"context"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Dial connects to MongoDB (same URI as Node mongoose).
func Dial(ctx context.Context, uri string) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(uri)
	opts.SetServerSelectionTimeout(10 * time.Second)
	return mongo.Connect(ctx, opts)
}

func GameStates(coll *mongo.Database) *mongo.Collection {
	return coll.Collection("gamestates")
}

func Users(coll *mongo.Database) *mongo.Collection {
	return coll.Collection("users")
}

// WorldMaps holds the rendered map image for a game, keyed by gameId.
//
// It is a separate collection rather than a field on the GameState because the image is megabytes and GameState is
// loaded on every DM turn; keeping them together would make every turn read the picture.
func WorldMaps(coll *mongo.Database) *mongo.Collection {
	return coll.Collection("gameworldmaps")
}

// RedactSecrets removes credentials from a message before it is logged.
//
// The database URI carries the password, and the driver echoes a malformed URI back inside its error, so both the
// configured URI and the password it holds are removed: a password written to a log line travels into every backup, log
// shipper and support ticket that line reaches.
func RedactSecrets(message, uri string) string {
	trimmed := strings.TrimSpace(uri)
	if message == "" || trimmed == "" {
		return message
	}
	out := strings.ReplaceAll(message, trimmed, redactedURI(trimmed))
	if password := uriPassword(trimmed); password != "" {
		out = strings.ReplaceAll(out, password, "***")
	}
	return out
}

// redactedURI keeps the URI's shape (scheme, hosts, database) so the message still says which server failed, with the
// credentials replaced.
func redactedURI(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<redacted connection string>"
	}
	parsed.User = url.UserPassword("***", "***")
	return parsed.String()
}

// uriPassword reads the password out of a URI, or "" when it has none.
func uriPassword(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return ""
	}
	password, _ := parsed.User.Password()
	return password
}
