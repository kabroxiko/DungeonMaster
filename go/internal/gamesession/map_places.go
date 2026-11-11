package gamesession

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/vellaris"
)

// maxNewPlacesPerTurn bounds how many markers one turn may add. A turn introduces one site at most; the bound exists
// so a malformed reply cannot ask the map service for a dozen re-renders.
const maxNewPlacesPerTurn = 2

// maxPlaceNameRunes bounds a marker's name, which has to fit on the map as a label.
const maxPlaceNameRunes = 80

// mapPlaceUpdateTimeout bounds the background render. A large map takes seconds; this is long enough to finish and
// short enough that a stuck service cannot hold the marker job forever.
const mapPlaceUpdateTimeout = 3 * time.Minute

// NewPlace is one site the DM marked on the campaign map during a turn: a name and a point in the world's own
// resolution-invariant coordinates, which is the space every place in the world block is reported in.
type NewPlace struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// ParseNewPlaces reads the optional newPlaces array from a DM reply.
//
// Entries without a usable name and point are dropped rather than failing the turn: the narration is already written
// and the map mark is a bonus, not something a player should lose a scene over.
func ParseNewPlaces(raw interface{}) []NewPlace {
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	out := make([]NewPlace, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := strings.TrimSpace(str(entry["name"]))
		if name == "" || len([]rune(name)) > maxPlaceNameRunes {
			continue
		}
		x, okX := numberValue(entry["x"])
		y, okY := numberValue(entry["y"])
		if !okX || !okY {
			continue
		}
		out = append(out, NewPlace{Name: name, X: x, Y: y})
		if len(out) == maxNewPlacesPerTurn {
			break
		}
	}
	return out
}

// numberValue reads a coordinate that a JSON document decoded into interface{}.
func numberValue(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// vellarisPlaces converts the parsed marks into the map service's request shape.
func vellarisPlaces(places []NewPlace) []vellaris.Place {
	out := make([]vellaris.Place, 0, len(places))
	for _, place := range places {
		out = append(out, vellaris.Place{Name: place.Name, X: place.X, Y: place.Y})
	}
	return out
}

// ApplyMapPlaces marks places on the campaign's map and stores the re-rendered image.
//
// The world, its settings and its picture move together: the service renders the new map from the same seeds, so the
// description the DM reads and the image the players see stay the same world. Returns the names actually added,
// which is empty when the service skipped them (already on the map) or the game has no map to mark.
func ApplyMapPlaces(ctx context.Context, d *Deps, gameID string, places []NewPlace) ([]string, error) {
	if len(places) == 0 {
		return nil, nil
	}
	if d == nil || d.Coll == nil {
		return nil, fmt.Errorf("map places: no database")
	}
	if d.Cfg == nil || strings.TrimSpace(d.Cfg.VellarisURL) == "" {
		return nil, fmt.Errorf("map places: no map service is configured (set DM_VELLARIS_URL)")
	}

	var doc map[string]interface{}
	if err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc); err != nil {
		return nil, fmt.Errorf("map places: reading the game: %w", err)
	}
	spec, _ := doc["campaignSpec"].(map[string]interface{})
	if mapFrom(spec, "vellarisWorld") == nil {
		// No generated world means nothing is drawn for the players, so there is nothing to mark on.
		return nil, nil
	}

	mapID := str(spec["vellarisMapId"])
	settings := mapFrom(spec, "vellarisSettings")
	client := vellaris.NewWithToken(d.Cfg.VellarisURL, d.Cfg.VellarisToken)
	marks := vellarisPlaces(places)

	var answer map[string]interface{}
	var err error
	switch {
	case mapID != "":
		answer, err = client.AddPlaces(ctx, mapID, marks)
	case settings != nil:
		// A stateless service has no id to name the world by, so the settings travel with the marks and the answer
		// carries the canonical settings and the re-rendered image.
		answer, err = client.AddPlacesToWorld(ctx, settings, marks)
	default:
		return nil, fmt.Errorf("map places: this game keeps neither a stored map nor the settings to render one")
	}
	if err != nil {
		return nil, fmt.Errorf("map places: %w", err)
	}

	added, _ := answer["added"].([]interface{})
	addedNames := make([]string, 0, len(added))
	for _, name := range added {
		if s, ok := name.(string); ok && strings.TrimSpace(s) != "" {
			addedNames = append(addedNames, s)
		}
	}
	if len(addedNames) == 0 {
		return nil, nil
	}

	set := bson.M{}
	if stored := campaignspec.SanitizeVellarisWorld(mapFrom(answer, "world")); stored != nil {
		set["campaignSpec.vellarisWorld"] = stored
	}
	if mapID == "" {
		if canonical := mapFrom(answer, "settings"); canonical != nil {
			// A stateless service hands back the settings that now describe the world, so the next call can name them.
			if encoded, err := json.Marshal(canonical); err == nil && len(encoded) <= maxStoredVellarisSettings {
				set["campaignSpec.vellarisSettings"] = canonical
			}
		}
	}

	image, contentType, err := placeUpdateImage(ctx, client, mapID, answer)
	if err != nil {
		return addedNames, err
	}
	if err := storeWorldMapImage(ctx, d, gameID, contentType, image); err != nil {
		return addedNames, fmt.Errorf("map places: %w", err)
	}
	set["campaignSpec.mapImage"] = map[string]interface{}{
		"contentType": contentType,
		"byteLength":  len(image),
		// Clients key the image URL on this, so the new render is never served from a stale browser cache.
		"updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	set["campaignSpec.mapImageError"] = nil

	if _, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": set}); err != nil {
		return addedNames, fmt.Errorf("map places: storing the world: %w", err)
	}
	if d.Hub != nil {
		d.Hub.NotifyGameStateUpdated(gameID)
	}
	return addedNames, nil
}

// placeUpdateImage returns the re-rendered map: the service's stored copy for a stored map, the encoded image in the
// answer when the world was rendered from settings.
func placeUpdateImage(ctx context.Context, client *vellaris.Client, mapID string, answer map[string]interface{}) ([]byte, string, error) {
	if mapID != "" {
		data, contentType, err := client.StoredWorldImage(ctx, mapID)
		if err != nil {
			return nil, "", fmt.Errorf("fetching the re-rendered map: %w", err)
		}
		return data, contentType, nil
	}

	encoded, _ := answer["imageBase64"].(string)
	if strings.TrimSpace(encoded) == "" {
		return nil, "", fmt.Errorf("the map service returned no re-rendered image")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("the re-rendered image could not be read: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("the re-rendered image was empty")
	}
	return data, "image/jpeg", nil
}

// StartMapPlaceUpdate applies a turn's new markers in the background.
//
// Rendering a map takes seconds, so the players read the narration while the map catches up; the stored image's
// updatedAt is what makes every open client fetch the new picture once it lands.
func StartMapPlaceUpdate(d *Deps, gameID string, places []NewPlace) {
	if len(places) == 0 || strings.TrimSpace(gameID) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mapPlaceUpdateTimeout)
		defer cancel()

		added, err := ApplyMapPlaces(ctx, d, gameID, places)
		if err != nil {
			log.Printf("map places: gameId=%s: %v", gameID, err)
			return
		}
		if len(added) > 0 {
			log.Printf("map places: gameId=%s marked %s", gameID, strings.Join(added, ", "))
		}
	}()
}
