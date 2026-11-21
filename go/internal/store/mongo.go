package store

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// bsonRegistry decodes BSON arrays as []interface{} instead of primitive.A.
//
// primitive.A is a named slice type, so `value.([]interface{})` on a stored list yields nil and the reader silently sees
// an empty list rather than a type mismatch - the server drops the transcript, the party list, the campaign arrays, and
// says nothing. Each package had grown its own helper to work around it, and each new reader was another chance to
// forget. Setting the type map once, where the driver produces the value, keeps every reader honest.
func bsonRegistry() *bsoncodec.Registry {
	registry := bson.NewRegistry()
	registry.RegisterTypeMapEntry(bsontype.Array, reflect.TypeOf([]interface{}{}))
	return registry
}

// Dial connects to MongoDB (same URI as Node mongoose).
func Dial(ctx context.Context, uri string) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(uri)
	opts.SetServerSelectionTimeout(10 * time.Second)
	opts.SetRegistry(bsonRegistry())
	return mongo.Connect(ctx, opts)
}

// SliceOf reads a stored array whatever shape it arrived in: []interface{} from a document this process decoded or
// built, primitive.A from one decoded before the client registry was set. Reading only the first shape is the mistake
// that emptied the transcript, so the readers that assert on a stored list go through here.
func SliceOf(v interface{}) []interface{} {
	switch a := v.(type) {
	case []interface{}:
		return a
	case primitive.A:
		return []interface{}(a)
	}
	return nil
}

func GameStates(coll *mongo.Database) *mongo.Collection {
	return coll.Collection("gamestates")
}

// GameStateTurns holds one mark per player action: where in the transcript the action began, and the two values the
// turn overwrites. Undo deletes the mark and the messages it guards, so a development table can step back an action
// without a copy of the whole game lying around. It lives apart from the game document because the transcript is read
// on every turn and a growing array inside it would be read every time.
func GameStateTurns(coll *mongo.Database) *mongo.Collection {
	return coll.Collection("gamestateturns")
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
