package store

import (
	"context"
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
