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
