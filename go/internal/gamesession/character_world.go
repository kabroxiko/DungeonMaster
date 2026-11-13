package gamesession

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cbroglie/mustache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/draftparty"
	"github.com/deckofdmthings/gmai/internal/party"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
)

// campaignWorldLocks serialises world creation per game.
//
// A table plays on one map, so the first character generated for a game creates the world and every later character
// waits for that world and reads it back. Two players who press "generate" at the same moment must not each invent a
// different map for the same party, which is what a plain "ask the map service if there is no world yet" would do.
var campaignWorldLocks sync.Map

// worldPrepInFlight marks the games whose world step this process is already running, so a second caller (a reload
// during the wait, a resumed load) joins the run that is going instead of queueing another world behind it.
var worldPrepInFlight sync.Map

func lockCampaignWorld(gameID string) func() {
	value, _ := campaignWorldLocks.LoadOrStore(gameID, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// campaignWorldWithLock creates the world under the game's lock, so an overlapping caller reuses the one being made
// instead of inventing a second map for the same table.
func campaignWorldWithLock(ctx context.Context, d *Deps, gameID string, body map[string]interface{}, language string) (campaignWorld, <-chan string, int, string, string) {
	unlock := lockCampaignWorld(gameID)
	defer unlock()
	return prepareCampaignWorld(ctx, d, gameID, body, language)
}

// PrepareWorldInBackground writes the table's world as soon as the party exists, instead of leaving it to the first
// character request. The lobby keeps opening while the map is made, every client sees party.stage = "world", and a
// sheet requested in the meantime waits on the same lock rather than being written on a world that does not exist yet.
func PrepareWorldInBackground(d *Deps, gameID, language string) {
	if d == nil || d.Coll == nil || strings.TrimSpace(gameID) == "" {
		return
	}
	gid := strings.TrimSpace(gameID)
	if _, running := worldPrepInFlight.LoadOrStore(gid, struct{}{}); running {
		return
	}
	go func() {
		defer worldPrepInFlight.Delete(gid)
		// The request that started this has already answered, so the work carries its own deadline.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if !setLobbyStage(ctx, d, gid, "world") {
			// The table already started: its start pipeline names the wait, and the world is made for it instead.
			return
		}
		defer clearLobbyStage(context.Background(), d, gid, "world")
		if world := storedCampaignWorld(ctx, d, gid); world != nil {
			return
		}
		if _, _, status, code, problem := campaignWorldWithLock(ctx, d, gid, map[string]interface{}{}, language); problem != "" {
			log.Printf("party world: game %s has no world (%d %s): %s", gid, status, code, problem)
		}
	}()
}

// EnsureLobbyWorld resumes a world step that nothing is running any more.
//
// The stage is written by the goroutine that generates the world and cleared by that same goroutine when it finishes,
// so a restart in the middle - a deploy while the map was being drawn - leaves the stage behind with nobody left to
// clear it, and the table watches a banner for work that will never finish. The stored world is the truth: when it is
// there the stage is dropped, and when it is not the step is started again.
func EnsureLobbyWorld(ctx context.Context, d *Deps, doc map[string]interface{}, language string) {
	if d == nil || d.Coll == nil || doc == nil {
		return
	}
	gameID := str(doc["gameId"])
	if gameID == "" {
		return
	}
	p := party.GetParty(toMap(doc["gameSetup"]))
	if ph, _ := p["phase"].(string); strings.TrimSpace(ph) != "lobby" {
		return
	}
	if stage, _ := p["stage"].(string); strings.TrimSpace(stage) != "world" {
		return
	}
	if storedWorld(doc) != nil {
		clearLobbyStage(ctx, d, gameID, "world")
		return
	}
	PrepareWorldInBackground(d, gameID, language)
}

// storedCampaignWorld returns the world already stored for a game, or nil when there is none yet.
func storedCampaignWorld(ctx context.Context, d *Deps, gameID string) map[string]interface{} {
	if d == nil || d.Coll == nil || strings.TrimSpace(gameID) == "" {
		return nil
	}
	var row map[string]interface{}
	if err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&row); err != nil || row == nil {
		return nil
	}
	return storedWorld(row)
}

// storedWorld reads the world out of a game document, or returns nil when the game has none yet.
func storedWorld(doc map[string]interface{}) map[string]interface{} {
	spec, _ := doc["campaignSpec"].(map[string]interface{})
	if spec == nil {
		return nil
	}
	return campaignspec.NormalizeVellarisWorld(mapFrom(spec, "vellarisWorld"))
}

// campaignWorldForCharacter returns the game's map world, creating it the first time a character needs it.
//
// Character sheets name a homeland, so they cannot be generated before the map exists: the story would be written on
// invented places and the players would be looking at a different world. Returns a status/code/problem triple that is
// empty on success, in the same shape as the campaign paths, because a character without its world is not generated at
// all rather than generated on a guess.
func campaignWorldForCharacter(ctx context.Context, d *Deps, gameID string, ownerUID primitive.ObjectID, body map[string]interface{}, language string) (map[string]interface{}, int, string, string) {
	if d == nil || d.Coll == nil {
		return nil, 500, "DB_ERROR", "database is not available"
	}
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return nil, 400, "GAME_ID_REQUIRED", "a character is generated on the table's map, so a game id is required"
	}

	if world := storedCampaignWorld(ctx, d, gameID); world != nil {
		return world, 0, "", ""
	}

	unlock := lockCampaignWorld(gameID)
	defer unlock()

	// Whoever held the lock may have just created the world; use theirs instead of generating a second one.
	if world := storedCampaignWorld(ctx, d, gameID); world != nil {
		return world, 0, "", ""
	}

	if err := ensureGameDocumentForWorld(ctx, d, gameID, ownerUID); err != nil {
		return nil, 500, "CHARACTER_WORLD_PERSIST_FAILED", "the game could not be prepared for its map: " + err.Error()
	}

	prepared, _, status, code, problem := prepareCampaignWorld(ctx, d, gameID, body, language)
	if problem != "" {
		return nil, status, code, problem
	}
	// The picture is for the players' map control; prepareCampaignWorld stores and announces it on its own, so it never
	// holds up the character sheet.
	return prepared.World, 0, "", ""
}

// ensureGameDocumentForWorld makes sure the game document exists so the world can be stored on it.
//
// A brand-new party is allocated its game id before the character is generated, and the world is stored before the
// character is; the document therefore has to exist first. It is created as a draft, so a generation that fails leaves
// nothing behind that the draft cleanup will not collect.
func ensureGameDocumentForWorld(ctx context.Context, d *Deps, gameID string, ownerUID primitive.ObjectID) error {
	onInsert := bson.M{"gameId": gameID}
	if ownerUID != primitive.NilObjectID {
		onInsert["ownerUserId"] = ownerUID
		onInsert["memberUserIds"] = bson.A{ownerUID}
	}
	if expires := draftparty.DraftPartyExpiresAtFromNow(); expires != nil {
		onInsert["draftPartyExpiresAt"] = expires
	}
	_, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID},
		bson.M{"$setOnInsert": onInsert}, options.Update().SetUpsert(true))
	return err
}

// characterWorldSystemBlock renders the table's map for /generate-character.
//
// maxPlaces caps the place list and -1 skips places entirely; a sheet needs enough names to choose a home from, and the
// rest of the list is context the model never quotes back.
func characterWorldSystemBlock(world map[string]interface{}, maxPlaces int) string {
	normalized := campaignspec.NormalizeVellarisWorld(world)
	if normalized == nil {
		return ""
	}
	regions, _ := normalized["regions"].([]interface{})
	places, _ := normalized["places"].([]interface{})
	if maxPlaces < 0 {
		places = nil
	} else if maxPlaces > 0 && len(places) > maxPlaces {
		places = places[:maxPlaces]
	}
	if len(regions) == 0 && len(places) == 0 {
		return ""
	}

	template := promptmgr.LoadPrompt("templates/character/world_context.md")
	if template == "" {
		return ""
	}
	rendered, err := mustache.Render(template, map[string]interface{}{
		"hasRegions": len(regions) > 0,
		"regions":    regions,
		"hasPlaces":  len(places) > 0,
		"places":     places,
	})
	if err != nil {
		return ""
	}
	rules := promptmgr.LoadPrompt("rules/character_world.md")
	if rules == "" {
		return rendered
	}
	return rules + "\n\n" + rendered
}
