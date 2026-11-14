package gamesession

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// FindUnstartedGameID returns the newest game the user belongs to that has not begun play yet, or "" when there is none.
//
// A table that has not started is what "new game" means. Creating a second lobby would leave the first one abandoned
// with its map already drawn and its invites already sent, so a caller that asks for a new game is given back the lobby
// it already has. Only an explicit lobby phase qualifies: a legacy game without one is not something to reopen, and a
// game whose campaign exists has started even when its party phase lags behind.
func FindUnstartedGameID(ctx context.Context, d *Deps, uid primitive.ObjectID) string {
	if d == nil || d.Coll == nil || uid == primitive.NilObjectID {
		return ""
	}
	filter := bson.M{
		"gameSetup.party.phase": "lobby",
		"$or": bson.A{
			bson.M{"ownerUserId": uid},
			bson.M{"memberUserIds": uid},
		},
	}
	var row struct {
		GameID string `bson:"gameId"`
	}
	err := d.Coll.FindOne(ctx, filter, options.FindOne().
		SetSort(bson.D{{Key: "_id", Value: -1}}).
		SetProjection(bson.M{"gameId": 1})).
		Decode(&row)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(row.GameID)
}
