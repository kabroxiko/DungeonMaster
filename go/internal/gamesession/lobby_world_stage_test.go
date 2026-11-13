package gamesession

import "testing"

// lobbyStageOf reads the party stage out of a client-facing game document.
func lobbyStageOf(t *testing.T, doc map[string]interface{}) string {
	t.Helper()
	gs, _ := doc["gameSetup"].(map[string]interface{})
	if gs == nil {
		t.Fatalf("the client document carries no gameSetup: %v", doc)
	}
	p, _ := gs["party"].(map[string]interface{})
	if p == nil {
		t.Fatalf("gameSetup carries no party: %v", gs)
	}
	stage, _ := p["stage"].(string)
	return stage
}

// A lobby's "world" stage is cleared by the goroutine that set it, so a restart in the middle of drawing the map leaves
// the stage behind with nobody to clear it and the table watching a banner forever. The stored world is the truth the
// client is shown: once it exists there is no world left to wait for.
func TestLobbyWorldStageIsDroppedOnceTheWorldIsStored(t *testing.T) {
	world := map[string]interface{}{
		"regions": []interface{}{map[string]interface{}{"id": "0", "name": "Cae"}},
	}
	withWorld := map[string]interface{}{
		"gameId":       "g1",
		"campaignSpec": map[string]interface{}{"vellarisWorld": world},
		"gameSetup": map[string]interface{}{
			"language": "English",
			"party":    map[string]interface{}{"phase": "lobby", "stage": "world"},
		},
	}
	if stage := lobbyStageOf(t, GameStateDocForClient(withWorld)); stage != "" {
		t.Fatalf("a game whose world is already stored must not report a world step, got %q", stage)
	}
	if stage := lobbyStageOf(t, withWorld); stage != "world" {
		t.Fatalf("the stored document must not be rewritten by the view, got %q", stage)
	}

	// No world yet: the stage describes real work in progress, so it stays.
	noWorld := map[string]interface{}{
		"gameId":       "g2",
		"campaignSpec": map[string]interface{}{},
		"gameSetup": map[string]interface{}{
			"language": "English",
			"party":    map[string]interface{}{"phase": "lobby", "stage": "world"},
		},
	}
	if stage := lobbyStageOf(t, GameStateDocForClient(noWorld)); stage != "world" {
		t.Fatalf("a game still waiting on its world must keep the world step, got %q", stage)
	}

	// The step that follows the world belongs to the start pipeline and is left alone.
	starting := map[string]interface{}{
		"gameId":       "g3",
		"campaignSpec": map[string]interface{}{"vellarisWorld": world},
		"gameSetup": map[string]interface{}{
			"language": "English",
			"party":    map[string]interface{}{"phase": "starting", "stage": "campaign"},
		},
	}
	if stage := lobbyStageOf(t, GameStateDocForClient(starting)); stage != "campaign" {
		t.Fatalf("the campaign step must survive the world being stored, got %q", stage)
	}
}
