package gamesession

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// A retry must recognise the work a previous attempt already stored, or it asks the model for the whole adventure
// again. Every case here is a document as it comes back from the database.
func TestCampaignResumeDecisions(t *testing.T) {
	spec := map[string]interface{}{
		"title":           "La torre que no duerme",
		"campaignConcept": "una torre que despierta",
		"creativeSeed":    map[string]interface{}{"hook": "algo"},
		"factions":        primitive.A{map[string]interface{}{"name": "gremio"}},
		"majorNPCs":       primitive.A{},
		"openingSceneFrame": map[string]interface{}{
			"scene": "el puerto",
		},
	}

	if _, ok := storedCreativeSeed(spec); !ok {
		t.Fatal("a stored creative seed was not recognised")
	}
	if _, ok := storedCreativeSeed(map[string]interface{}{"creativeSeed": map[string]interface{}{}}); ok {
		t.Fatal("an empty creative seed must not count as done")
	}

	if _, ok := storedCampaignCore(spec); !ok {
		t.Fatal("a stored campaign core was not recognised")
	}
	if _, ok := storedCampaignCore(map[string]interface{}{"title": "solo titulo"}); ok {
		t.Fatal("a campaign without a concept must be regenerated")
	}

	// A stage decoded from the database arrives as primitive.A; one built in memory is a []interface{}.
	if !campaignStageStored(spec, "factions") {
		t.Fatal("a stored stage decoded as primitive.A was not recognised")
	}
	if campaignStageStored(spec, "majorNPCs") {
		t.Fatal("an empty stage must be regenerated")
	}
	if !campaignStageStored(map[string]interface{}{"factions": []interface{}{map[string]interface{}{"name": "x"}}}, "factions") {
		t.Fatal("a stage built in memory was not recognised")
	}

	if !campaignOpeningFrameStored(spec) {
		t.Fatal("a stored opening scene was not recognised")
	}
	if campaignOpeningFrameStored(map[string]interface{}{"openingSceneFrame": map[string]interface{}{}}) {
		t.Fatal("an empty opening scene must be regenerated")
	}

	// Nothing stored yet: a first start still runs every step.
	if _, ok := storedCreativeSeed(nil); ok {
		t.Fatal("a campaign with no document must not look resumed")
	}
	if _, ok := storedCampaignCore(nil); ok {
		t.Fatal("a campaign with no document must not look resumed")
	}
}
