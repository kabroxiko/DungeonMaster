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
	for _, want := range []string{"Goblin arquero", `"ac":13`, `"hp_current":7`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("the stored encounter state did not reach the prompt: %q missing", want)
		}
	}
	// The sheet and the state are JSON payloads: a mustache render escaped them into &#34; entities, which is not what
	// the model was promised and reads as noise in a wall of stat data.
	if strings.Contains(prompt, "&#34;") {
		t.Fatal("a JSON payload reached the prompt HTML-escaped")
	}
}

// A game with no fight yet has no encounter state, and the block still has to render with something valid in the slot.
func TestEmptyEncounterStateRendersAnEmptyObject(t *testing.T) {
	if got := priorEncounterStateJSON(nil); got != "{}" {
		t.Fatalf("no document should give {}, got %q", got)
	}
	if got := priorEncounterStateJSON(map[string]interface{}{"encounterState": nil}); got != "{}" {
		t.Fatalf("a null state should give {}, got %q", got)
	}
	if got := priorEncounterStateJSON(map[string]interface{}{"encounterState": map[string]interface{}{}}); got != "{}" {
		t.Fatalf("an empty state should give {}, got %q", got)
	}
}
