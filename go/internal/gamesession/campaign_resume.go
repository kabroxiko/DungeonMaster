package gamesession

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// storedCampaignSpec reads the campaign document a lobby start writes into. A start that failed halfway keeps what it
// already stored there, so every step can tell whether its work is done before spending another model call on it.
func storedCampaignSpec(ctx context.Context, d *Deps, gameID string) map[string]interface{} {
	if d == nil || d.Coll == nil || strings.TrimSpace(gameID) == "" {
		return nil
	}
	var doc map[string]interface{}
	err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID},
		options.FindOne().SetProjection(bson.M{"campaignSpec": 1})).Decode(&doc)
	if err != nil {
		return nil
	}
	return toMap(doc["campaignSpec"])
}

// storedCreativeSeed reports the seed a previous attempt already wrote. The seed is the premise of the whole
// campaign: a retry that generated a new one would describe a different adventure from the stages stored on top of
// the old one.
func storedCreativeSeed(spec map[string]interface{}) (map[string]interface{}, bool) {
	seed := toMap(spec["creativeSeed"])
	if len(seed) == 0 {
		return nil, false
	}
	return seed, true
}

// storedCampaignCore reports the campaign core a previous attempt already wrote, which is what everything below it was
// written on top of. An empty document, or one from before the core was stored, keeps the start going from the top.
func storedCampaignCore(spec map[string]interface{}) (map[string]interface{}, bool) {
	if spec == nil {
		return nil, false
	}
	title, _ := spec["title"].(string)
	concept, _ := spec["campaignConcept"].(string)
	if strings.TrimSpace(title) == "" || strings.TrimSpace(concept) == "" {
		return nil, false
	}
	return spec, true
}

// campaignStageStored reports whether a stage already has entries in the document.
func campaignStageStored(spec map[string]interface{}, stage string) bool {
	return len(toIfaceSlice(spec[stage])) > 0
}

// campaignOpeningFrameStored reports whether the opening scene was already written.
func campaignOpeningFrameStored(spec map[string]interface{}) bool {
	return len(toMap(spec["openingSceneFrame"])) > 0
}

// persistCampaignCore stores the core as soon as the model writes it, so a stage that fails afterwards cannot throw
// away the campaign those stages were written on. It merges over the stored document because the world and its map
// marker were written by the world step and must survive.
func persistCampaignCore(ctx context.Context, coll *mongo.Collection, gameID string, core map[string]interface{}) error {
	existing := map[string]interface{}(nil)
	if coll != nil {
		var doc map[string]interface{}
		_ = coll.FindOne(ctx, bson.M{"gameId": gameID},
			options.FindOne().SetProjection(bson.M{"campaignSpec": 1})).Decode(&doc)
		existing = toMap(doc["campaignSpec"])
	}
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"campaignSpec": mergeCampaignSpecLobby(core, existing),
	}}, options.Update().SetUpsert(true))
	return err
}
