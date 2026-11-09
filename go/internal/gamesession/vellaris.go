package gamesession

import (
	"context"

	"github.com/cbroglie/mustache"
	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
	"go.mongodb.org/mongo-driver/bson"
)

// renderVellarisWorldInject renders an imported Vellaris world into the DM system stack.
//
// It returns "" unless the campaign carries a usable vellarisWorld, which leaves the generic campaign world
// map in charge for campaigns that were never imported from the map service.
func renderVellarisWorldInject(spec map[string]interface{}) string {
	if spec == nil {
		return ""
	}
	raw, _ := spec["vellarisWorld"].(map[string]interface{})
	world := campaignspec.NormalizeVellarisWorld(raw)
	if world == nil {
		return ""
	}

	template := promptmgr.LoadPrompt("templates/dm/world_vellaris.txt")
	if template == "" {
		return ""
	}
	rendered, err := mustache.Render(template, world)
	if err != nil {
		return ""
	}

	rules := promptmgr.LoadPrompt("rules/vellaris_world.txt")
	if rules == "" {
		return rendered
	}
	return rules + "\n\n" + rendered
}

// vellarisRegionOptions loads the imported world's regions for the campaign generator, so that key locations are
// placed inside geography that already exists on the players' map instead of a second, invented one.
//
// Returns nil when the game has no imported world, which keeps the generator's own worldMap path in charge.
func vellarisRegionOptions(ctx context.Context, d *Deps, gameID string) []interface{} {
	if d == nil || d.Coll == nil || gameID == "" {
		return nil
	}
	var row map[string]interface{}
	if err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&row); err != nil || row == nil {
		return nil
	}
	spec, _ := row["campaignSpec"].(map[string]interface{})
	if spec == nil {
		return nil
	}
	raw, _ := spec["vellarisWorld"].(map[string]interface{})
	world := campaignspec.NormalizeVellarisWorld(raw)
	if world == nil {
		return nil
	}
	regions, _ := world["regions"].([]interface{})
	return regions
}
