package gamesession

import (
	"strings"
	"testing"
)

// The encounter state the DM wrote last turn has to come back to it.
//
// Hit points, initiative and armor class are what an attack is resolved against. The template always had a slot for
// them and the server filled it with "{}", so the model kept writing state it was never shown again: on the next beat
// it had no number to attack against and asked the table for one, mid-fight.
func TestStoredEncounterStateReachesThePrompt(t *testing.T) {
	const uid = "user-1"
	gs := map[string]interface{}{
		"gameSetup": map[string]interface{}{
			"playerCharacters": map[string]interface{}{
				uid: map[string]interface{}{"name": "Bram", "ac": 15, "max_hp": 12, "stats": map[string]interface{}{}},
			},
		},
		"encounterState": map[string]interface{}{
			"participants": []interface{}{
				map[string]interface{}{"displayName": "Bram", "hp_current": 9, "hp_max": 12, "ac": 15},
				map[string]interface{}{"displayName": "Goblin arquero", "hp_current": 7, "hp_max": 7, "ac": 13},
			},
		},
	}

	prompt := composeDmSystemPrompt("combat", "Spanish", "", gs, uid)
	// Who is in the fight reaches the prompt; the numbers do not. The tools carry them, and a stat block sitting in the
	// prompt is a stat block in the narration.
	if !strings.Contains(prompt, "Goblin arquero") || !strings.Contains(prompt, "A fight is in progress") {
		t.Fatal("the fight in progress did not reach the prompt by name")
	}
	for _, unwanted := range []string{`"ac":13`, `"hp_current":7`, `"hp_max":7`} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("a combat turn still carries the stat block: %q present", unwanted)
		}
	}

	// The fight over, the state comes whole: what a corpse is, and the hit points it fell at, is what keeps it from
	// standing up in the next scene.
	gs["encounterState"] = map[string]interface{}{"participants": []interface{}{
		map[string]interface{}{"displayName": "Bram", "hp_current": 9, "hp_max": 12, "ac": 15},
		map[string]interface{}{"displayName": "Goblin arquero", "hp_current": 0, "hp_max": 7, "ac": 13},
	}}
	after := composeDmSystemPrompt("combat", "Spanish", "", gs, uid)
	if !strings.Contains(after, `"hp_current":0`) {
		t.Fatal("the state of a finished fight should reach the prompt whole")
	}
	// The sheet and the state are JSON payloads: a mustache render escaped them into &#34; entities, which is not what
	// the model was promised and reads as noise in a wall of stat data.
	if strings.Contains(after, "&#34;") {
		t.Fatal("a JSON payload reached the prompt HTML-escaped")
	}
}

// A game with no fight yet has no encounter state, and the block still has to render with something valid in the slot.
func TestEmptyEncounterStateRendersAnEmptyObject(t *testing.T) {
	if got := priorEncounterStateJSON(nil, ""); got != "{}" {
		t.Fatalf("no document should give {}, got %q", got)
	}
	if got := priorEncounterStateJSON(map[string]interface{}{"encounterState": nil}, ""); got != "{}" {
		t.Fatalf("a null state should give {}, got %q", got)
	}
	if got := priorEncounterStateJSON(map[string]interface{}{"encounterState": map[string]interface{}{}}, ""); got != "{}" {
		t.Fatalf("an empty state should give {}, got %q", got)
	}
}

// The browser never sends a mode, so every play turn arrives as exploration.
//
// Without the combat procedure the DM improvised fights with no initiative, no encounter bookkeeping and no defender:
// it invented a bystander with an armor class so it had something to roll against. The procedure rides on every play
// turn now, and the block itself decides when formal combat applies.
func TestCombatProcedureRidesOnEveryPlayTurn(t *testing.T) {
	const marker = "Starting or resuming formal combat"
	for _, mode := range []string{"exploration", "investigation", "decision"} {
		prompt := composeDmSystemPrompt(mode, "Spanish", "", nil, "")
		if !strings.Contains(prompt, marker) {
			t.Fatalf("the %s turn has no combat procedure", mode)
		}
	}
	if strings.Count(composeDmSystemPrompt("combat", "Spanish", "", nil, ""), marker) != 1 {
		t.Fatal("a combat turn should carry the procedure exactly once")
	}
	if strings.Contains(composeDmSystemPrompt("initial", "Spanish", "", nil, ""), marker) {
		t.Fatal("the opening is not a play turn and should not carry the combat procedure")
	}
}
