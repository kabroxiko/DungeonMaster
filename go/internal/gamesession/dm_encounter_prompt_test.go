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

// The combat procedure rides on a fight, and only on a fight. It used to ride on every play turn because the browser
// never says whether a fight is running; deciding when one starts belongs to the declaration call now, and nine thousand
// characters of initiative on a turn spent bargaining is the prompt crowding out the scene.
func TestCombatProcedureRidesOnFightsOnly(t *testing.T) {
	const marker = "Starting or resuming formal combat"
	fighting := map[string]interface{}{
		"gameSetup": map[string]interface{}{
			"playerCharacters": map[string]interface{}{"u1": map[string]interface{}{"name": "Varek", "kind": "pc", "ac": 13, "max_hp": 12}},
		},
		"encounterState": map[string]interface{}{"participants": []interface{}{
			map[string]interface{}{"displayName": "Varek", "kind": "pc", "hp_current": 12, "hp_max": 12, "ac": 13},
			map[string]interface{}{"displayName": "Goblin", "hp_current": 7, "hp_max": 7, "ac": 13},
		}},
	}
	if prompt := composeDmSystemPrompt("exploration", "Spanish", "", fighting, "u1"); !strings.Contains(prompt, marker) {
		t.Fatal("a turn in the middle of a fight has no combat procedure")
	}
	for _, mode := range []string{"exploration", "investigation", "decision"} {
		prompt := composeDmSystemPrompt(mode, "Spanish", "", nil, "")
		if strings.Contains(prompt, marker) {
			t.Fatalf("the %s turn carries a combat procedure it does not need", mode)
		}
	}
}

// The browser never sends a mode, so every play turn arrives as exploration.
//
// Without the combat procedure the DM improvised fights with no initiative, no encounter bookkeeping and no defender:
// it invented a bystander with an armor class so it had something to roll against. The procedure rides on every play
// turn now, and the block itself decides when formal combat applies.
// A reply that resolved a round without the record is not retried: it is diagnosed, because the three causes need
// different fixes and only one of them is the model forgetting.
func TestMissingRecordIsDiagnosedByCause(t *testing.T) {
	engine := encounterState(participant("Bram", 9, 12, 15), participant("Goblin", 7, 7, 13))

	omitted := describeMissingRecord(&dmEnvelope{Narration: "La daga entra y el goblin cae."}, `{"narration":"La daga entra y el goblin cae."}`, engine)
	if !strings.Contains(omitted, "omitted encounterState entirely") {
		t.Fatalf("a reply that never wrote the field: %q", omitted)
	}
	if !strings.Contains(omitted, "participants=2") {
		t.Fatalf("the diagnosis should say what the record held: %q", omitted)
	}

	unparsed := describeMissingRecord(&dmEnvelope{}, `encounterState: ...`, engine)
	if !strings.Contains(unparsed, "not parsed as an envelope") {
		t.Fatalf("a reply that is not an envelope at all: %q", unparsed)
	}

	wrongShape := describeMissingRecord(&dmEnvelope{Narration: "El goblin cae."}, `{"narration":"El goblin cae.","encounterState":[]}`, engine)
	if !strings.Contains(wrongShape, "not as an object") {
		t.Fatalf("a field in another shape: %q", wrongShape)
	}
}
