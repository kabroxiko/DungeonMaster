package gameaccess

import (
	"context"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/deckofdmthings/gmai/internal/store"
)

// EffectiveGameOwnerIDStr mirrors server/services/gameAccess.js.
func EffectiveGameOwnerIDStr(doc map[string]interface{}) string {
	if doc == nil {
		return ""
	}
	if oid, ok := doc["ownerUserId"].(primitive.ObjectID); ok && !oid.IsZero() {
		return oid.Hex()
	}
	if arr := store.SliceOf(doc["memberUserIds"]); len(arr) > 0 {
		if id, ok := arr[0].(primitive.ObjectID); ok {
			return id.Hex()
		}
	}
	return ""
}

// UserIsGameMember checks owner + memberUserIds.
func UserIsGameMember(userIDStr string, doc map[string]interface{}) bool {
	if doc == nil || userIDStr == "" {
		return false
	}
	uid := strings.ToLower(strings.TrimSpace(userIDStr))
	if o := EffectiveGameOwnerIDStr(doc); strings.ToLower(o) == uid {
		return true
	}
	arr := store.SliceOf(doc["memberUserIds"])
	for _, x := range arr {
		switch t := x.(type) {
		case primitive.ObjectID:
			if strings.ToLower(t.Hex()) == uid {
				return true
			}
		case string:
			if strings.ToLower(strings.TrimSpace(t)) == uid {
				return true
			}
		}
	}
	return false
}

// ErrGameNotFound is returned with status 404 for access errors.
var ErrGameNotFound = errors.New("game not found")

// ErrGameIDRequired is returned when gameId is missing for a route that requires it.
var ErrGameIDRequired = errors.New("gameId is required")

// AssertGameMember loads game by gameId and verifies membership.
func AssertGameMember(ctx context.Context, coll *mongo.Collection, userIDStr, gameID string) (map[string]interface{}, error) {
	if gameID == "" {
		return nil, ErrGameIDRequired
	}
	var doc map[string]interface{}
	err := coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, ErrGameNotFound
	}
	if err != nil {
		return nil, err
	}
	if !UserIsGameMember(userIDStr, doc) {
		return nil, ErrGameNotFound
	}
	return doc, nil
}

// ToObjectID parses hex string.
func ToObjectID(s string) (primitive.ObjectID, bool) {
	oid, err := primitive.ObjectIDFromHex(s)
	if err != nil {
		return primitive.ObjectID{}, false
	}
	return oid, true
}
