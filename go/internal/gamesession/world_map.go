package gamesession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/store"
)

// storeWorldMapImage saves the rendered map image for a game, replacing any earlier one.
func storeWorldMapImage(ctx context.Context, d *Deps, gameID, contentType string, data []byte) error {
	if d == nil || d.Coll == nil {
		return fmt.Errorf("world map image: no database")
	}
	if strings.TrimSpace(gameID) == "" {
		return fmt.Errorf("world map image: gameId is required")
	}
	if len(data) == 0 {
		return fmt.Errorf("world map image: no bytes to store")
	}
	_, err := store.WorldMaps(d.Coll.Database()).ReplaceOne(ctx, bson.M{"_id": gameID}, bson.M{
		"_id":         gameID,
		"gameId":      gameID,
		"contentType": contentType,
		"byteLength":  len(data),
		"data":        data,
		"updatedAt":   time.Now().UTC(),
	}, options.Replace().SetUpsert(true))
	return err
}

// LoadWorldMapImage returns the stored map image for a game. The caller must already have checked membership.
func LoadWorldMapImage(ctx context.Context, d *Deps, gameID string) (contentType string, data []byte, err error) {
	if d == nil || d.Coll == nil {
		return "", nil, fmt.Errorf("world map image: no database")
	}
	gid := strings.TrimSpace(gameID)
	if gid == "" {
		return "", nil, fmt.Errorf("world map image: gameId is required")
	}
	var row struct {
		ContentType string `bson:"contentType"`
		Data        []byte `bson:"data"`
	}
	err = store.WorldMaps(d.Coll.Database()).FindOne(ctx, bson.M{"_id": gid}).Decode(&row)
	if err != nil {
		return "", nil, err
	}
	if len(row.Data) == 0 {
		return "", nil, fmt.Errorf("world map image: stored image is empty")
	}
	contentType = strings.TrimSpace(row.ContentType)
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return contentType, row.Data, nil
}

// WorldJSONFor returns the game's map JSON, in the same shape POST /api/world answers so the download can be fed
// straight back into import-world.
//
// Only the table owner may take it: the document carries every coordinate and named place of the world, which is DM
// material, not something a player's browser should be able to fetch.
func WorldJSONFor(ctx context.Context, d *Deps, uid, gameID string) (payload map[string]interface{}, status int, code, problem string) {
	gid := strings.TrimSpace(gameID)
	if gid == "" {
		return nil, 400, "GAME_ID_REQUIRED", "gameId required"
	}
	if d == nil || d.Coll == nil {
		return nil, 500, "DB_ERROR", "database is not available"
	}
	doc, err := gameaccess.AssertGameMember(ctx, d.Coll, uid, gid)
	if err != nil {
		st, body := accessErrHTTP(err)
		message, _ := body["error"].(string)
		code, _ := body["code"].(string)
		return nil, st, code, message
	}
	if owner := gameaccess.EffectiveGameOwnerIDStr(doc); owner != "" && !strings.EqualFold(owner, strings.TrimSpace(uid)) {
		return nil, 403, "NOT_OWNER_MAP_JSON", "Only the table owner can download the map"
	}
	spec, _ := doc["campaignSpec"].(map[string]interface{})
	payload = buildWorldJSON(spec)
	if payload == nil {
		return nil, 404, "WORLD_JSON_MISSING", "This game has no generated world to download"
	}
	return payload, 200, "", ""
}

// buildWorldJSON puts a stored campaign's world into the map service's answer shape.
func buildWorldJSON(spec map[string]interface{}) map[string]interface{} {
	world := mapFrom(spec, "vellarisWorld")
	if world == nil {
		return nil
	}
	payload := map[string]interface{}{
		"success": true,
		"world":   world,
	}
	if settings := mapFrom(spec, "vellarisSettings"); settings != nil {
		payload["settings"] = settings
	}
	return payload
}

// HandleSetMapVisibility shows or hides the physical map for every player at the table.
//
// Only the table owner may change it: the map is the DM's screen, and a guest pulling it open or closed mid-scene
// would take that away. The flag lives on gameSetup because that document is broadcast to every client, so all
// players react to the same value.
func HandleSetMapVisibility(ctx context.Context, d *Deps, uid string, body map[string]interface{}) (int, interface{}) {
	gameID := strParam(body, "gameId")
	if gameID == "" {
		return 400, map[string]interface{}{"error": "gameId required", "code": "GAME_ID_REQUIRED"}
	}
	doc, err := gameaccess.AssertGameMember(ctx, d.Coll, uid, gameID)
	if err != nil {
		st, payload := accessErrHTTP(err)
		return st, payload
	}
	if owner := gameaccess.EffectiveGameOwnerIDStr(doc); owner != "" && !strings.EqualFold(owner, strings.TrimSpace(uid)) {
		return 403, map[string]interface{}{
			"error": "Only the table owner can show or hide the map",
			"code":  "NOT_OWNER_MAP_VISIBILITY",
		}
	}

	visible := boolParam(body, "visible", false)
	if _, _, err := LoadWorldMapImage(ctx, d, gameID); err != nil {
		return 409, map[string]interface{}{
			"error": "This game has no rendered map to show yet",
			"code":  "WORLD_MAP_IMAGE_MISSING",
		}
	}
	if _, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"gameSetup.mapVisible": visible,
	}}); err != nil {
		return 500, map[string]interface{}{"error": "failed to save the map visibility", "code": "STORE_FAILED"}
	}
	if d.Hub != nil {
		// The visibility event reaches every client straight away; the state notification keeps a client that missed
		// the event (or loads later) converging on the same value.
		d.Hub.NotifyWorldMapVisibility(gameID, visible)
		d.Hub.NotifyGameStateUpdated(gameID)
	}
	return 200, map[string]interface{}{"success": true, "mapVisible": visible}
}
