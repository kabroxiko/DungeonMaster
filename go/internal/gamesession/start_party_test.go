package gamesession

import "testing"

// A campaign generated in the lobby, or left behind by a start that failed part-way, is not a started adventure: only
// the party phase says the table is playing. The opening scene is the fallback marker for documents written before the
// phase was recorded.
func TestAdventureAlreadyStartedUsesThePartyPhase(t *testing.T) {
	lobbyWithCampaign := map[string]interface{}{
		"campaignSpec": map[string]interface{}{
			"title":                      "Salt and Silver",
			"campaignConcept":            "A war over the salt roads.",
			"majorConflicts":             []interface{}{"The harbour guilds"},
			"toneAndStyle":               "grim",
			"dmHiddenAdventureObjective": "Find the ledger.",
		},
		"gameSetup": map[string]interface{}{"party": map[string]interface{}{"phase": "lobby"}},
	}
	if adventureAlreadyStarted(lobbyWithCampaign) {
		t.Fatal("a substantive campaignSpec in the lobby must not read as an already-started adventure")
	}

	playing := map[string]interface{}{
		"gameSetup": map[string]interface{}{"party": map[string]interface{}{"phase": "playing"}},
	}
	if !adventureAlreadyStarted(playing) {
		t.Fatal("a playing party has started")
	}

	starting := map[string]interface{}{
		"gameSetup": map[string]interface{}{"party": map[string]interface{}{"phase": "starting"}},
	}
	if adventureAlreadyStarted(starting) {
		t.Fatal("a start still in progress is not a finished adventure")
	}

	legacyStarted := map[string]interface{}{
		"conversation": []interface{}{
			map[string]interface{}{"role": "system", "content": "bootstrap"},
			map[string]interface{}{"role": "assistant", "content": "The road bends north."},
		},
	}
	if !adventureAlreadyStarted(legacyStarted) {
		t.Fatal("a document written before the phase existed counts as started once the opening exists")
	}

	for name, doc := range map[string]map[string]interface{}{
		"no party":               {},
		"empty conversation":     {"conversation": []interface{}{}},
		"party without a phase":  {"gameSetup": map[string]interface{}{"party": map[string]interface{}{}}},
		"phase but no opening":   {"gameSetup": map[string]interface{}{"party": map[string]interface{}{"phase": ""}}},
		"conversation of system": {"conversation": []interface{}{map[string]interface{}{"role": "system"}}},
	} {
		if adventureAlreadyStarted(doc) {
			t.Fatalf("%s must not read as started", name)
		}
	}
}

// The lobby draws the table's world before anyone can start, so a start on a game that already carries a world is
// writing the campaign, and must not show "generating the world" a second time.
func TestStartStageNamesTheWorkStillAhead(t *testing.T) {
	if got := startStageForWorld(nil); got != "world" {
		t.Fatalf("a game without a world must wait on the world, got %q", got)
	}
	if got := startStageForWorld(map[string]interface{}{"regions": []interface{}{}}); got != "campaign" {
		t.Fatalf("a game that already carries its world must wait on the campaign, got %q", got)
	}
}
