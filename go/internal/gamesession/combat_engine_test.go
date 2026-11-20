package gamesession

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/llm"
)

func encounterState(participants ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(participants))
	for _, p := range participants {
		list = append(list, p)
	}
	return map[string]interface{}{"participants": list}
}

func participant(name string, hp, maxHP, ac int) map[string]interface{} {
	return map[string]interface{}{
		"displayName": name,
		"hp_current":  hp,
		"hp_max":      maxHP,
		"ac":          ac,
	}
}

func hpOf(t *testing.T, state map[string]interface{}, name string) int {
	t.Helper()
	combatants, _ := combatantsOf(state)
	_, c := findCombatant(combatants, name)
	if c == nil {
		t.Fatalf("%s is not in the state", name)
	}
	hp, _ := intField(c, participantHPCurrentKey)
	return hp
}

// The turn that started this work: a total below the target's armor class must be a miss, and a miss deals nothing.
func TestAttackBelowArmorClassMissesAndDealsNoDamage(t *testing.T) {
	state := encounterState(participant("Idbul", 6, 8, 14), participant("Mensajero", 10, 10, 12))
	action := declaredAction{Actor: "Mensajero", Target: "Idbul", Kind: "attack", Modifier: 4, Damage: "1d4+2"}

	var line string
	for seed := int64(0); seed < 400; seed++ {
		out := resolveCombatTurn([]declaredAction{action}, state, rand.New(rand.NewSource(seed)))
		if len(out.Lines) == 0 {
			t.Fatalf("the action was not resolved")
		}
		line = out.Lines[0]
		if strings.Contains(line, "= 13 vs AC 14") {
			if !strings.Contains(line, "miss") {
				t.Fatalf("13 against 14 is a miss, got %q", line)
			}
			if out.HPChanged || hpOf(t, out.State, "Idbul") != 6 {
				t.Fatalf("a miss must not change hit points, got %q", line)
			}
			return
		}
	}
	t.Fatalf("no seed produced the 13-versus-14 case; last line %q", line)
}

// Across the whole range of dice the bookkeeping has to agree with the line the DM is told to narrate.
func TestHitPointsFollowTheRolledTotal(t *testing.T) {
	for seed := int64(0); seed < 300; seed++ {
		state := encounterState(participant("Bram", 12, 12, 12), participant("Goblin", 10, 10, 12))
		action := declaredAction{Actor: "Bram", Target: "Goblin", Kind: "attack", Modifier: 6, Damage: "1d4+4"}
		out := resolveCombatTurn([]declaredAction{action}, state, rand.New(rand.NewSource(seed)))
		line := out.Lines[0]
		hit := strings.Contains(line, "-> hit")
		if hit {
			if !out.HPChanged {
				t.Fatalf("seed %d: a hit that changed nothing: %q", seed, line)
			}
			if hp := hpOf(t, out.State, "Goblin"); hp >= 10 {
				t.Fatalf("seed %d: hit but hp is %d: %q", seed, hp, line)
			}
			continue
		}
		if hp := hpOf(t, out.State, "Goblin"); hp != 10 {
			t.Fatalf("seed %d: miss but hp is %d: %q", seed, hp, line)
		}
	}
}

// A natural 1 misses and a natural 20 lands, whatever the arithmetic says.
func TestNaturalOneAndTwentyAreHonoured(t *testing.T) {
	sawOne, sawTwenty := false, false
	for seed := int64(0); seed < 2000 && (!sawOne || !sawTwenty); seed++ {
		state := encounterState(participant("Bram", 12, 12, 12), participant("Ogro", 10, 10, 30))
		// +0 against AC 30: only a natural 20 can land.
		action := declaredAction{Actor: "Bram", Target: "Ogro", Kind: "attack", Modifier: 0, Damage: "1d6"}
		out := resolveCombatTurn([]declaredAction{action}, state, rand.New(rand.NewSource(seed)))
		line := out.Lines[0]
		if strings.Contains(line, "d20(20)") {
			sawTwenty = true
			if !strings.Contains(line, "-> hit") {
				t.Fatalf("a natural 20 must land: %q", line)
			}
		}
		if strings.Contains(line, "d20(1)") {
			sawOne = true
			if !strings.Contains(line, "-> miss") {
				t.Fatalf("a natural 1 must miss: %q", line)
			}
		}
	}
	if !sawOne || !sawTwenty {
		t.Fatalf("the dice never showed a natural 1 and 20 (one=%v twenty=%v)", sawOne, sawTwenty)
	}
}

// A foe's numbers are the DM's decision, made once: what the declaration supplies is stored, and the next turn reuses it
// instead of letting the creature acquire a new bonus or a new armor class.
func TestFoeNumbersAreStoredAndReused(t *testing.T) {
	state := encounterState(participant("Bram", 12, 12, 12), participant("Mensajero", 10, 10, 0))
	first := resolveCombatTurn([]declaredAction{{
		Actor: "Mensajero", Target: "Bram", Kind: "attack", Modifier: 4, Damage: "1d4+2", AC: 0,
	}}, state, rand.New(rand.NewSource(3)))
	if len(first.Lines) == 0 {
		t.Fatal("the first action was not resolved")
	}
	combatants, _ := combatantsOf(first.State)
	_, foe := findCombatant(combatants, "Mensajero")
	if got, _ := intField(foe, participantAttackBonusKey); got != 4 {
		t.Fatalf("the attack bonus should be stored, got %v", foe[participantAttackBonusKey])
	}
	if got, _ := foe[participantDamageKey].(string); got != "1d4+2" {
		t.Fatalf("the damage should be stored, got %v", foe[participantDamageKey])
	}

	// Second turn: the declaration omits them, and the stored numbers are used.
	second := resolveCombatTurn([]declaredAction{{
		Actor: "Mensajero", Target: "Bram", Kind: "attack",
	}}, first.State, rand.New(rand.NewSource(9)))
	if len(second.Lines) == 0 {
		t.Fatal("the second action was not resolved")
	}
	if !strings.Contains(second.Lines[0], "+4") {
		t.Fatalf("the stored attack bonus should be used, got %q", second.Lines[0])
	}
}

// A target that is not in the encounter is reported, never resolved against an invented one.
func TestUnknownTargetIsReportedNotInvented(t *testing.T) {
	state := encounterState(participant("Bram", 12, 12, 12))
	out := resolveCombatTurn([]declaredAction{{
		Actor: "Bram", Target: "Guardia", Kind: "attack", Modifier: 6, Damage: "1d4+4",
	}}, state, rand.New(rand.NewSource(1)))
	if len(out.Lines) != 0 {
		t.Fatalf("nothing should have been resolved, got %v", out.Lines)
	}
	if len(out.Unknown) == 0 || !strings.Contains(out.Unknown[0], "Guardia") {
		t.Fatalf("the missing target should be named, got %v", out.Unknown)
	}
}

// The block the narration turn reads has to forbid the contradiction this was built for: a creature with hit points
// left described as down.
func TestPromptBlockStatesWhoIsStanding(t *testing.T) {
	state := encounterState(participant("Bram", 6, 8, 14), participant("Mensajero", 3, 10, 12))
	out := resolveCombatTurn([]declaredAction{{
		Actor: "Bram", Target: "Mensajero", Kind: "attack", Modifier: 6, Damage: "1d4+4",
	}}, state, rand.New(rand.NewSource(2)))
	block := out.promptBlock()
	for _, want := range []string{"ENGINE RESULTS", "Mensajero", "hit points above 0", "never describe it as unconscious"} {
		if !strings.Contains(block, want) {
			t.Fatalf("the block should say %q:\n%s", want, block)
		}
	}
}

// Initiative is a roll like any other: the engine makes it, orders the fight by it, and keeps that order for the rest of
// the encounter instead of letting each turn produce a new one.
func TestInitiativeIsRolledOnceAndKept(t *testing.T) {
	state := encounterState(participant("Bram", 12, 12, 12), participant("Mensajero", 10, 10, 12))
	declared := []declaredInitiative{{Actor: "Bram", Modifier: 3}, {Actor: "Mensajero", Modifier: 1}}

	lines, rolled := resolveInitiative(declared, state, rand.New(rand.NewSource(11)))
	if len(lines) == 0 || rolled == nil {
		t.Fatal("initiative was not rolled")
	}
	order := toIfaceSlice(rolled["initiative"])
	if len(order) != 2 {
		t.Fatalf("expected both participants in the order, got %d", len(order))
	}
	first, _ := intField(order[0].(map[string]interface{}), "score")
	second, _ := intField(order[1].(map[string]interface{}), "score")
	if first < second {
		t.Fatalf("the order should run highest first, got %d then %d", first, second)
	}
	if got := lines[len(lines)-1]; !strings.HasPrefix(got, "turn order: ") {
		t.Fatalf("the block should state the order, got %q", got)
	}

	// Second turn of the same fight: the stored order stands, and nothing is re-rolled.
	again, rerolled := resolveInitiative(declared, rolled, rand.New(rand.NewSource(99)))
	if len(again) != 0 || rerolled != nil {
		t.Fatalf("an existing order must be kept, got %v", again)
	}
}

// The first attack of a fight has no encounter state yet. The declaration brings the combatants in, and the engine turns
// them into the record: hit points from the declaration (from the sheet for the player), initiative rolled, the attack
// resolved against the armor class the DM settled on.
func TestFirstAttackBootstrapsTheEncounter(t *testing.T) {
	state := map[string]interface{}{}
	sheet := map[string]interface{}{"name": "Thulim", "ac": 15, "max_hp": 9}
	declared := []declaredCombatant{
		{Name: "Thulim", HPMax: 1, AC: 1}, // the sheet wins for the player
		{Name: "Guardia de la ciudad", HPMax: 11, AC: 16, AttackBonus: 3, Damage: "1d8+1"},
	}
	merged := mergeDeclaredCombatants(state, declared, sheet, "Thulim")
	combatants, ok := combatantsOf(merged)
	if !ok || len(combatants) != 2 {
		t.Fatalf("expected both combatants, got %v", merged)
	}
	_, pc := findCombatant(combatants, "Thulim")
	if hp, _ := intField(pc, participantHPMaxKey); hp != 9 {
		t.Fatalf("the player's hit points come from the sheet, got %d", hp)
	}
	if ac, _ := intField(pc, participantACKey); ac != 15 {
		t.Fatalf("the player's armor class comes from the sheet, got %d", ac)
	}
	_, foe := findCombatant(combatants, "Guardia de la ciudad")
	if hp, _ := intField(foe, participantHPCurrentKey); hp != 11 {
		t.Fatalf("a new foe starts at full hit points, got %d", hp)
	}

	initiative := []declaredInitiative{{Actor: "Thulim", Modifier: 3}, {Actor: "Guardia de la ciudad", Modifier: 1}}
	lines, scored := resolveInitiative(initiative, merged, rand.New(rand.NewSource(5)))
	if len(lines) == 0 || scored == nil {
		t.Fatal("initiative was not rolled for the new encounter")
	}
	if order := toIfaceSlice(scored["initiative"]); len(order) != 2 {
		t.Fatalf("expected both in the order, got %d", len(order))
	}

	out := resolveCombatTurn([]declaredAction{{
		Actor: "Thulim", Target: "Guardia de la ciudad", Kind: "attack", Modifier: 5, Damage: "1d4+3",
	}}, scored, rand.New(rand.NewSource(5)))
	if len(out.Lines) != 1 {
		t.Fatalf("the declared attack should be resolved, got %v", out.Lines)
	}
	if !strings.Contains(out.Lines[0], "vs AC 16") {
		t.Fatalf("the attack should resolve against the stored armor class, got %q", out.Lines[0])
	}
}

// The tools are the DM's way to ask for dice mid-turn: open the fight, roll inside it, read the record.
func TestCombatToolsAnswerLikeTheEngine(t *testing.T) {
	sheet := map[string]interface{}{"name": "Bram", "ac": 14, "max_hp": 10}
	tools := newCombatTools(nil, sheet, "Bram", rand.New(rand.NewSource(4)))

	opened := tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Bram"},
			map[string]interface{}{"displayName": "Goblin", "hp_max": 7, "ac": 13, "attack_bonus": 4, "damage": "1d6+2"},
		},
		"initiative": []interface{}{
			map[string]interface{}{"actor": "Bram", "modifier": 3},
			map[string]interface{}{"actor": "Goblin", "modifier": 2},
		},
	}})
	if !strings.Contains(opened.Content, "turn order") {
		t.Fatalf("opening the encounter should give the order, got %q", opened.Content)
	}
	if !strings.Contains(opened.Content, "Goblin: hp 7/7") {
		t.Fatalf("the standing should be reported, got %q", opened.Content)
	}

	// A model that writes "player" instead of the sheet's name still rolls for the player's character.
	rolled := tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "player", "target": "Goblin", "kind": "attack", "modifier": 5.0, "damage": "1d4+3",
	}})
	if !strings.Contains(rolled.Content, "Bram attacks Goblin") {
		t.Fatalf("the player alias should resolve to the character, got %q", rolled.Content)
	}
	if !strings.Contains(rolled.Content, "vs AC 13") {
		t.Fatalf("the attack should resolve against the stored armor class, got %q", rolled.Content)
	}

	state, changed := tools.changed()
	if !changed || state == nil {
		t.Fatal("a tool that touched the fight should mark the state as worth storing")
	}
	read := tools.run(llm.ToolCall{ID: "3", Name: "encounter"})
	if !strings.Contains(read.Content, "Goblin") || !strings.Contains(read.Content, "ac 13") {
		t.Fatalf("reading the encounter should show its record, got %q", read.Content)
	}
}

// A tool that names nobody says so instead of inventing a combatant.
func TestCombatToolsRefuseUnknownNames(t *testing.T) {
	tools := newCombatTools(nil, nil, "Bram", rand.New(rand.NewSource(1)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{map[string]interface{}{"displayName": "Bram", "hp_max": 10, "ac": 12}},
	}})
	result := tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Bram", "target": "Guardia", "kind": "attack", "modifier": 5.0,
	}})
	if !strings.Contains(result.Content, "not resolved") || !strings.Contains(result.Content, "Guardia") {
		t.Fatalf("an unknown target should be reported, got %q", result.Content)
	}
}
