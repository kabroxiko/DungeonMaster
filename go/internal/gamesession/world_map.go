package gamesession

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/store"
	"github.com/deckofdmthings/gmai/internal/vellaris"
)

// storeWorldMapImage saves the rendered map image for a game, replacing any earlier one.
//
// The full picture is kept for zooming and a pre-scales copy is stored beside it, so opening the map can paint a
// sixth of the bytes immediately instead of waiting for the whole render to cross the wire.
func storeWorldMapImage(ctx context.Context, d *Deps, gameID, contentType string, data []byte) error {
	if d == nil || d.Coll == nil {
		return fmt.Errorf("world map image: no database")
	}
	// The key is trimmed here exactly as the read trims it: a stored picture must be found by the id a client asks with.
	gid := strings.TrimSpace(gameID)
	if gid == "" {
		return fmt.Errorf("world map image: gameId is required")
	}
	if len(data) == 0 {
		return fmt.Errorf("world map image: no bytes to store")
	}
	preview, err := downscaledJPEG(data, previewMaxDimension, previewJPEGQuality)
	if err != nil {
		// The full picture is what the map is; a preview that could not be built only costs the client its fast first
		// paint, so it must not cost the table its map.
		log.Printf("world map preview for gameId=%s failed: %v", gid, err)
		preview = nil
	}
	if len(preview) == len(data) {
		preview = nil
	}
	_, err = store.WorldMaps(d.Coll.Database()).ReplaceOne(ctx, bson.M{"_id": gid}, bson.M{
		"_id":          gid,
		"gameId":       gid,
		"contentType":  contentType,
		"byteLength":   len(data),
		"data":         data,
		"previewData":  preview,
		"previewBytes": len(preview),
		"updatedAt":    time.Now().UTC(),
	}, options.Replace().SetUpsert(true))
	return err
}

// LoadWorldMapImage returns the stored map image for a game. The caller must already have checked membership.
//
// Unless full is set the pre-scaled preview is answered, which is what the map is drawn from at the fitted view; full
// answers the picture as it was rendered, which is what zooming into it needs.
func LoadWorldMapImage(ctx context.Context, d *Deps, gameID string, full bool) (contentType string, data []byte, err error) {
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
		PreviewData []byte `bson:"previewData"`
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
	if !full && len(row.PreviewData) > 0 {
		return contentType, row.PreviewData, nil
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
	payload, err = buildWorldJSON(ctx, d, spec)
	if err != nil {
		return nil, 502, "VELLARIS_MAP_SETTINGS_UNAVAILABLE", err.Error()
	}
	if payload == nil {
		return nil, 404, "WORLD_JSON_MISSING", "This game has no generated world to download"
	}
	return payload, 200, "", ""
}

// buildWorldJSON puts a stored campaign's world into the map service's answer shape, so the download can be fed
// straight back into import-world.
//
// A world whose settings live on the map service carries only the settings' id, so they are read back from the
// service here; that keeps the download complete instead of hands-off.
func buildWorldJSON(ctx context.Context, d *Deps, spec map[string]interface{}) (map[string]interface{}, error) {
	world := mapFrom(spec, "vellarisWorld")
	if world == nil {
		return nil, nil
	}
	payload := map[string]interface{}{
		"success": true,
		"world":   world,
	}
	mapID := str(spec["vellarisMapId"])
	if mapID != "" {
		payload["mapId"] = mapID
	}
	if settings := mapFrom(spec, "vellarisSettings"); settings != nil {
		payload["settings"] = settings
		return payload, nil
	}
	if mapID == "" {
		return payload, nil
	}
	if d == nil || d.Cfg == nil || strings.TrimSpace(d.Cfg.VellarisURL) == "" {
		return nil, fmt.Errorf("this map's settings live on the map service, which is not configured (set DM_VELLARIS_URL)")
	}
	stored, err := vellaris.NewWithToken(d.Cfg.VellarisURL, d.Cfg.VellarisToken).StoredWorld(ctx, mapID)
	if err != nil {
		return nil, err
	}
	if settings, _ := stored["settings"].(map[string]interface{}); settings != nil {
		payload["settings"] = settings
		return payload, nil
	}
	return nil, fmt.Errorf("the map service returned no settings for map %s", mapID)
}
