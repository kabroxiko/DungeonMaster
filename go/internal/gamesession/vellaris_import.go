package gamesession

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/vellaris"
	"go.mongodb.org/mongo-driver/bson"
)

// maxStoredVellarisSettings bounds the settings document kept for on-demand route lookups. A 4096-wide world
// carries about a megabyte of edits; anything far beyond that is not a settings document and is not stored.
const maxStoredVellarisSettings = 4 << 20

// HandleImportVellarisWorld stores a generated world on a game so DM play can ground itself in real geography.
//
// The body may carry the world directly (the answer from POST /api/world, or just its "world" object), or the
// settings to generate one with, in which case the map service is called here. Optional measured travel legs are
// stored alongside.
func HandleImportVellarisWorld(ctx context.Context, d *Deps, uid string, body map[string]interface{}) (int, interface{}) {
	gameID := strParam(body, "gameId")
	if gameID == "" {
		return 400, map[string]interface{}{"error": "gameId required", "code": "GAME_ID_REQUIRED"}
	}
	if _, err := gameaccess.AssertGameMember(ctx, d.Coll, uid, gameID); err != nil {
		st, payload := accessErrHTTP(err)
		return st, payload
	}

	world, settings, problem := vellarisWorldFromRequest(ctx, d, body)
	if problem != "" {
		return 400, map[string]interface{}{"error": problem, "code": "VELLARIS_WORLD_MISSING"}
	}

	stored := campaignspec.SanitizeVellarisWorld(world)
	if stored == nil {
		return 400, map[string]interface{}{
			"error": "the world has no regions and no places, so there is nothing to ground play in",
			"code":  "VELLARIS_WORLD_EMPTY",
		}
	}

	if extra, ok := body["routes"].([]interface{}); ok && len(extra) > 0 {
		routes, _ := stored["routes"].([]interface{})
		for _, raw := range extra {
			m, _ := raw.(map[string]interface{})
			if m == nil {
				continue
			}
			from := str(m["from"])
			to := str(m["to"])
			if from == "" || to == "" {
				continue
			}
			routes = campaignspec.AppendVellarisRoute(routes, campaignspec.NormalizeVellarisRouteLeg(from, to, m))
		}
		stored["routes"] = routes
	}

	set := bson.M{"campaignSpec.vellarisWorld": stored}
	if settings != nil {
		if encoded, err := json.Marshal(settings); err == nil && len(encoded) <= maxStoredVellarisSettings {
			set["campaignSpec.vellarisSettings"] = settings
		}
	}
	if _, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": set}); err != nil {
		return 500, map[string]interface{}{"error": "failed to store the world", "code": "STORE_FAILED"}
	}

	// Return the display form so a caller can show what was imported without re-implementing the formatting.
	display := campaignspec.NormalizeVellarisWorld(stored)
	return 200, map[string]interface{}{
		"success":     true,
		"world":       display,
		"regionCount": display["regionCount"],
		"placeCount":  display["placeCount"],
		"routeCount":  display["routeCount"],
	}
}

// vellarisWorldFromRequest pulls a world out of the request, either as supplied or by generating it.
//
// Returns the world map, the settings to keep for later route lookups (may be nil), and a problem description
// that is empty on success.
func vellarisWorldFromRequest(ctx context.Context, d *Deps, body map[string]interface{}) (map[string]interface{}, map[string]interface{}, string) {
	if raw, ok := body["world"].(map[string]interface{}); ok && raw != nil {
		// Accept the full POST /api/world answer as well as its inner "world" object, so a caller can forward
		// what the map service returned without reshaping it.
		if inner, ok := raw["world"].(map[string]interface{}); ok && inner != nil {
			settings, _ := raw["settings"].(map[string]interface{})
			if settings == nil {
				settings, _ = body["settings"].(map[string]interface{})
			}
			return inner, settings, ""
		}
		settings, _ := body["settings"].(map[string]interface{})
		return raw, settings, ""
	}

	settings, _ := body["settings"].(map[string]interface{})
	if settings == nil {
		return nil, nil, "send either the world from the map service or the settings to generate one with"
	}
	if d == nil || d.Cfg == nil || strings.TrimSpace(d.Cfg.VellarisURL) == "" {
		return nil, nil, "no map service is configured (set DM_VELLARIS_URL), so send the world directly"
	}

	answer, err := vellaris.New(d.Cfg.VellarisURL).World(ctx, settings)
	if err != nil {
		return nil, nil, err.Error()
	}
	world, _ := answer["world"].(map[string]interface{})
	if world == nil {
		return nil, nil, "the map service returned no world"
	}
	canonical, _ := answer["settings"].(map[string]interface{})
	if canonical == nil {
		canonical = settings
	}
	return world, canonical, ""
}

// HandleVellarisRoute measures one journey and stores it with the world, so later DM turns can quote a real
// distance and travel time instead of guessing.
//
// The body carries from/to (place names) and optionally mode and allowWater. A caller that already measured the
// leg itself can send it as "leg" and nothing is called.
func HandleVellarisRoute(ctx context.Context, d *Deps, uid string, body map[string]interface{}) (int, interface{}) {
	gameID := strParam(body, "gameId")
	if gameID == "" {
		return 400, map[string]interface{}{"error": "gameId required", "code": "GAME_ID_REQUIRED"}
	}
	doc, err := gameaccess.AssertGameMember(ctx, d.Coll, uid, gameID)
	if err != nil {
		st, payload := accessErrHTTP(err)
		return st, payload
	}

	spec, _ := doc["campaignSpec"].(map[string]interface{})
	world, _ := spec["vellarisWorld"].(map[string]interface{})
	stored := campaignspec.SanitizeVellarisWorld(world)
	if stored == nil {
		return 400, map[string]interface{}{"error": "this game has no imported world yet", "code": "VELLARIS_WORLD_MISSING"}
	}

	from := strParam(body, "from")
	to := strParam(body, "to")
	if from == "" || to == "" {
		return 400, map[string]interface{}{"error": "from and to are required", "code": "PLACES_REQUIRED"}
	}

	var leg map[string]interface{}
	if supplied, ok := body["leg"].(map[string]interface{}); ok && supplied != nil {
		leg = campaignspec.NormalizeVellarisRouteLeg(from, to, supplied)
		if leg == nil {
			return 400, map[string]interface{}{"error": "the supplied leg is not a usable journey", "code": "LEG_UNUSABLE"}
		}
	} else {
		settings, _ := spec["vellarisSettings"].(map[string]interface{})
		if settings == nil {
			return 400, map[string]interface{}{
				"error": "no settings were stored with this world, so the map service cannot be asked; send the measured leg instead",
				"code":  "VELLARIS_SETTINGS_MISSING",
			}
		}
		if d.Cfg == nil || strings.TrimSpace(d.Cfg.VellarisURL) == "" {
			return 400, map[string]interface{}{"error": "no map service is configured (set DM_VELLARIS_URL)", "code": "VELLARIS_NOT_CONFIGURED"}
		}
		answer, err := vellaris.New(d.Cfg.VellarisURL).Route(ctx, settings, from, to, strParam(body, "mode"), boolParam(body, "allowWater", false))
		if err != nil {
			return 502, map[string]interface{}{"error": err.Error(), "code": "VELLARIS_ROUTE_FAILED"}
		}
		leg = campaignspec.NormalizeVellarisRouteLeg(from, to, answer)
		if leg == nil {
			message, _ := answer["message"].(string)
			if message == "" {
				message = from + " and " + to + " are not connected in this world"
			}
			return 200, map[string]interface{}{"success": false, "reachable": false, "message": message, "code": "UNREACHABLE"}
		}
	}

	routes, _ := stored["routes"].([]interface{})
	stored["routes"] = campaignspec.AppendVellarisRoute(routes, leg)
	if _, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{"campaignSpec.vellarisWorld": stored}}); err != nil {
		return 500, map[string]interface{}{"error": "failed to store the measured journey", "code": "STORE_FAILED"}
	}

	return 200, map[string]interface{}{
		"success": true,
		"leg":     campaignspec.FormatVellarisRoute(leg),
	}
}
