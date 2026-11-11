package gamesession

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/deckofdmthings/gmai/internal/llm"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
	"github.com/deckofdmthings/gmai/internal/vellaris"
)

// PartyPosition is where a DM reply says the party now stands: a named place, or an exact point when the party stops
// between places (an ambush on the road, a camp for the night).
type PartyPosition struct {
	Place string   `json:"place"`
	Note  string   `json:"note"`
	X     *float64 `json:"x"`
	Y     *float64 `json:"y"`
}

// ParsePartyPosition reads the optional partyPosition object from a DM reply.
//
// A point wins over a name when both are given: the name then only says how to label the spot. An entry that names
// nothing and gives no point is dropped, because a pin with nowhere to go would only overwrite the last good one.
func ParsePartyPosition(raw interface{}) *PartyPosition {
	obj, ok := raw.(map[string]interface{})
	if !ok || obj == nil {
		return nil
	}

	pos := &PartyPosition{
		Place: strings.TrimSpace(str(obj["place"])),
		Note:  strings.TrimSpace(str(obj["note"])),
	}
	if x, ok := numberValue(obj["x"]); ok {
		pos.X = &x
	}
	if y, ok := numberValue(obj["y"]); ok {
		pos.Y = &y
	}
	if pos.Place == "" && (pos.X == nil || pos.Y == nil) {
		return nil
	}
	return pos
}

// resolvePartyPoint turns a reply's position into a world coordinate and the label the pin carries.
//
// A named place has to exist in the stored world: the model is told to copy names from the block, and a pin for an
// invented name would point at coordinates that are not on the map.
func resolvePartyPoint(world map[string]interface{}, pos *PartyPosition) (x, y float64, label string, ok bool) {
	if pos == nil || world == nil {
		return 0, 0, "", false
	}

	if pos.X != nil && pos.Y != nil {
		label := strings.TrimSpace(pos.Note)
		if label == "" {
			label = strings.TrimSpace(pos.Place)
		}
		return *pos.X, *pos.Y, label, true
	}

	wanted := strings.TrimSpace(pos.Place)
	if wanted == "" {
		return 0, 0, "", false
	}
	for _, raw := range llm.SliceValues(world["places"]) {
		place, _ := raw.(map[string]interface{})
		if place == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(str(place["name"])), wanted) {
			continue
		}
		px, py, okPoint := placePoint(place)
		if !okPoint {
			continue
		}
		label := wanted
		if note := strings.TrimSpace(pos.Note); note != "" {
			label = wanted + " — " + note
		}
		return px, py, label, true
	}
	return 0, 0, "", false
}

// placePoint is the point a pin belongs on: the drawn place when the world says where its drawing is, otherwise the
// point the world reports for the place.
//
// A place's x/y is its label. The map service draws the name beside the pictogram — above it, or below it when the
// space above is taken — so pinning x/y puts the dot on the word rather than on the town. The centre of the drawing
// is reported as iconX/iconY, and a world that carries it is preferred; a world stored before the map service
// reported drawings has only the label, which is still the right place to stand, just less precisely drawn.
func placePoint(place map[string]interface{}) (float64, float64, bool) {
	if x, okX := numberValue(place["iconX"]); okX {
		if y, okY := numberValue(place["iconY"]); okY {
			return x, y, true
		}
	}
	x, okX := numberValue(place["x"])
	y, okY := numberValue(place["y"])
	return x, y, okX && okY
}

// mapGeometry is the projection from a world coordinate onto a fraction of the rendered image.
//
// A world coordinate is resolution-invariant, while the image is the map area at the settings' resolution plus, when
// the border is drawn outside the map, a padding of borderWidth * resolution on every side. Storing the projection as
// fractions of that unscaled extent keeps every answer valid for any copy of the picture, including a downscaled one,
// and lets the client pin a point with plain CSS over the image it already has.
type mapGeometry struct {
	XPerUnit float64 `bson:"xPerUnit"`
	YPerUnit float64 `bson:"yPerUnit"`
	XOffset  float64 `bson:"xOffset"`
	YOffset  float64 `bson:"yOffset"`
}

// project turns a world coordinate into a fraction of the rendered image.
func (g mapGeometry) project(x, y float64) (float64, float64) {
	return clampFraction(g.XOffset + x*g.XPerUnit), clampFraction(g.YOffset + y*g.YPerUnit)
}

// geometryFor reads the projection out of a map's settings.
func geometryFor(settings map[string]interface{}) (mapGeometry, string) {
	if settings == nil {
		return mapGeometry{}, "no settings describe this map"
	}
	resolution, okRes := numberValue(settings["resolution"])
	width, okWidth := numberValue(settings["generatedWidth"])
	height, okHeight := numberValue(settings["generatedHeight"])
	if !okRes || !okWidth || !okHeight || resolution <= 0 || width <= 0 || height <= 0 {
		return mapGeometry{}, "the map's settings do not pin a resolution and a size"
	}
	if rotation, _ := numberValue(settings["rightRotationCount"]); int(rotation) == 1 || int(rotation) == 3 {
		return mapGeometry{}, "the map is rotated a quarter turn, so its coordinates cannot be projected onto the image"
	}

	padding := 0.0
	if drawn, _ := settings["drawBorder"].(bool); drawn && strings.TrimSpace(str(settings["borderPosition"])) == "Outside map" {
		borderWidth, _ := numberValue(settings["borderWidth"])
		padding = borderWidth * resolution
	}

	imageWidth := width*resolution + 2*padding
	imageHeight := height*resolution + 2*padding
	if imageWidth <= 0 || imageHeight <= 0 {
		return mapGeometry{}, "the map's rendered size came out empty"
	}
	return mapGeometry{
		XPerUnit: resolution / imageWidth,
		YPerUnit: resolution / imageHeight,
		XOffset:  padding / imageWidth,
		YOffset:  padding / imageHeight,
	}, ""
}

// storedGeometry returns the projection kept on the game, when there is one.
func storedGeometry(spec map[string]interface{}) (mapGeometry, bool) {
	stored := mapFrom(spec, "mapGeometry")
	if stored == nil {
		return mapGeometry{}, false
	}
	geometry := mapGeometry{}
	var okX, okY, okOffsetX, okOffsetY bool
	geometry.XPerUnit, okX = numberValue(stored["xPerUnit"])
	geometry.YPerUnit, okY = numberValue(stored["yPerUnit"])
	geometry.XOffset, okOffsetX = numberValue(stored["xOffset"])
	geometry.YOffset, okOffsetY = numberValue(stored["yOffset"])
	return geometry, okX && okY && okOffsetX && okOffsetY
}

// settingsForMap returns the settings that describe the game's map: the copy on the game when it keeps one, otherwise
// the canonical copy the map service holds beside the stored map.
func settingsForMap(ctx context.Context, d *Deps, spec map[string]interface{}) (map[string]interface{}, error) {
	if settings := mapFrom(spec, "vellarisSettings"); settings != nil {
		return settings, nil
	}
	mapID := str(spec["vellarisMapId"])
	if mapID == "" {
		return nil, fmt.Errorf("this game keeps neither a stored map nor the settings to project a point with")
	}
	if d == nil || d.Cfg == nil || strings.TrimSpace(d.Cfg.VellarisURL) == "" {
		return nil, fmt.Errorf("the map's settings live on the map service, which is not configured (set DM_VELLARIS_URL)")
	}
	stored, err := vellaris.NewWithToken(d.Cfg.VellarisURL, d.Cfg.VellarisToken).StoredWorld(ctx, mapID)
	if err != nil {
		return nil, err
	}
	settings, _ := stored["settings"].(map[string]interface{})
	if settings == nil {
		return nil, fmt.Errorf("the map service returned no settings for map %s", mapID)
	}
	return settings, nil
}

// ApplyPartyPosition stores where the party now stands, as a fraction of the rendered map.
//
// Nothing is re-rendered: the pin is a layer over the picture the table already has, so travel and encounters move it
// without paying for a new map. Returns the label that was stored, which is empty when the reply named no place this
// world knows and no usable point.
func ApplyPartyPosition(ctx context.Context, d *Deps, gameID string, pos *PartyPosition) (string, error) {
	if pos == nil || strings.TrimSpace(gameID) == "" {
		return "", nil
	}
	if d == nil || d.Coll == nil {
		return "", fmt.Errorf("party position: no database")
	}

	var doc map[string]interface{}
	if err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc); err != nil {
		return "", fmt.Errorf("party position: reading the game: %w", err)
	}
	spec, _ := doc["campaignSpec"].(map[string]interface{})
	world := mapFrom(spec, "vellarisWorld")
	if world == nil {
		// No generated world means the table is shown no map, so there is nowhere to put a pin.
		return "", nil
	}

	x, y, label, ok := resolvePartyPoint(world, pos)
	if !ok {
		return "", nil
	}

	geometry, hasGeometry := storedGeometry(spec)
	set := bson.M{}
	if !hasGeometry {
		settings, err := settingsForMap(ctx, d, spec)
		if err != nil {
			return "", fmt.Errorf("party position: %w", err)
		}
		var problem string
		geometry, problem = geometryFor(settings)
		if problem != "" {
			return "", fmt.Errorf("party position: %s", problem)
		}
		set["campaignSpec.mapGeometry"] = map[string]interface{}{
			"xPerUnit": geometry.XPerUnit,
			"yPerUnit": geometry.YPerUnit,
			"xOffset":  geometry.XOffset,
			"yOffset":  geometry.YOffset,
		}
	}

	fractionX, fractionY := geometry.project(x, y)
	position := map[string]interface{}{
		"x":         fractionX,
		"y":         fractionY,
		"label":     label,
		"updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if place := strings.TrimSpace(pos.Place); place != "" {
		position["place"] = place
	}
	if note := strings.TrimSpace(pos.Note); note != "" {
		position["note"] = note
	}
	set["campaignSpec.partyMapPosition"] = position

	if _, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": set}); err != nil {
		return "", fmt.Errorf("party position: storing it: %w", err)
	}
	if d.Hub != nil {
		d.Hub.NotifyGameStateUpdated(gameID)
	}
	return label, nil
}

// clampFraction keeps a projected point inside the picture: a coordinate on the very edge must not draw the pin off it.
func clampFraction(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

// renderPartyPositionInject tells the DM where the party stands, so a turn's travel starts where the last one ended
// instead of wherever the model imagines them.
func renderPartyPositionInject(spec map[string]interface{}) string {
	position := mapFrom(spec, "partyMapPosition")
	if position == nil {
		return ""
	}
	label := strings.TrimSpace(str(position["label"]))
	if label == "" {
		return ""
	}
	template := promptmgr.LoadPrompt("templates/dm/party_position.txt")
	if template == "" {
		return ""
	}
	place := strings.TrimSpace(str(position["place"]))
	note := strings.TrimSpace(str(position["note"]))
	rendered, err := mustache.Render(template, map[string]interface{}{
		"positionLabel":    label,
		"positionPlace":    place,
		"positionNote":     note,
		"hasPositionPlace": place != "",
		"hasPositionNote":  note != "",
	})
	if err != nil {
		return ""
	}
	return rendered
}
