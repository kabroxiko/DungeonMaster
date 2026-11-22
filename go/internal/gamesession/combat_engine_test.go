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
		if strings.Contains(line, "attack 13 against armor class 14") {
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
		hit := strings.Contains(line, "hit,")
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
		if strings.Contains(line, "(d20 20") {
			sawTwenty = true
			if !strings.Contains(line, "hit,") {
				t.Fatalf("a natural 20 must land: %q", line)
			}
		}
		if strings.Contains(line, "(d20 1") {
			sawOne = true
			if !strings.Contains(line, "miss,") {
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
	// One die line per participant, highest first: the order is stated by the callers, with these totals.
	if len(lines) != len(declared) {
		t.Fatalf("expected a line per participant, got %v", lines)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "initiative ") {
			t.Fatalf("every line should be one creature's initiative roll, got %q", line)
		}
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
	if !strings.Contains(out.Lines[0], "against armor class 16") {
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
	if !strings.Contains(rolled.Content, "against armor class 13") {
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

// A round in which only the player swings is not a round. The tools say who is still owed a turn, so the DM cannot
// answer as if the enemies had politely waited.
func TestToolsSayWhoIsStillOwedATurn(t *testing.T) {
	sheet := map[string]interface{}{"name": "Bram", "ac": 14, "max_hp": 10}
	tools := newCombatTools(nil, sheet, "Bram", rand.New(rand.NewSource(8)))

	opened := tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Bram"},
			map[string]interface{}{"displayName": "Goblin", "hp_max": 30, "ac": 13, "attack_bonus": 4, "damage": "1d6+2"},
		},
		"initiative": []interface{}{
			map[string]interface{}{"actor": "Bram", "modifier": 3},
			map[string]interface{}{"actor": "Goblin", "modifier": 2},
		},
	}})
	owedLine := func(body string) string {
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "still to act in this reply:") {
				return line
			}
		}
		return ""
	}
	if line := owedLine(opened.Content); !strings.Contains(line, "Goblin") {
		t.Fatalf("the foe is owed a turn after the opening: %q", opened.Content)
	}
	// The player plays their own character: their turn is never on the list the DM owes.
	if line := owedLine(opened.Content); strings.Contains(line, "Bram") {
		t.Fatalf("the player's own turn is not the DM's to take: %q", line)
	}

	afterPlayer := tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "player", "target": "Goblin", "kind": "attack", "modifier": 5.0, "damage": "1d4+3",
	}})
	if line := owedLine(afterPlayer.Content); !strings.Contains(line, "Goblin") {
		t.Fatalf("after the player's turn the foe is still owed one: %q", afterPlayer.Content)
	}

	afterFoe := tools.run(llm.ToolCall{ID: "3", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Goblin", "target": "Bram", "kind": "attack", "modifier": 4.0, "damage": "1d6+2",
	}})
	if strings.Contains(afterFoe.Content, "still to act in this reply:") {
		t.Fatalf("both sides acted, the round is complete: %q", afterFoe.Content)
	}
}

// A fallen foe is not owed a turn: the list is about who can still act.
func TestDownedFoesAreNotOwedATurn(t *testing.T) {
	tools := newCombatTools(nil, nil, "Bram", rand.New(rand.NewSource(6)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Bram", "hp_max": 10, "ac": 12},
			map[string]interface{}{"displayName": "Rata", "hp_max": 1, "ac": 5},
		},
	}})
	// A one-hit-point rat against a +50 attack: it is down after this, and owes nothing.
	tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Bram", "target": "Rata", "kind": "attack", "modifier": 50.0, "damage": "1d4+3",
	}})
	remaining := tools.waitingToAct()
	for _, who := range remaining {
		if strings.Contains(who, "Rata") {
			t.Fatalf("a foe at 0 hit points should not be waiting to act: %v", remaining)
		}
	}
}

// A round is resolved in one call, in initiative order, so a foe's answer lands in the turn it belongs to.
func TestRoundResolvesEveryParticipantInOrder(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 14, "max_hp": 10}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(12)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 30, "ac": 13, "attack_bonus": 4, "damage": "1d8+2"},
		},
		"initiative": []interface{}{
			map[string]interface{}{"actor": "Guardia", "modifier": 9}, // the guard acts first
			map[string]interface{}{"actor": "Thulim", "modifier": 1},
		},
	}})

	round := tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actions": []interface{}{
			// Deliberately listed player-first: the engine puts them in initiative order.
			map[string]interface{}{"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 5.0, "damage": "1d4+3"},
			map[string]interface{}{"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 4.0, "damage": "1d8+2"},
		},
	}})
	lines := strings.Split(strings.TrimSpace(round.Content), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected both actions resolved, got %q", round.Content)
	}
	if !strings.HasPrefix(lines[0], "Guardia attacks Thulim") {
		t.Fatalf("the round should run in initiative order, first line was %q", lines[0])
	}
	if !strings.Contains(round.Content, "Thulim attacks Guardia") {
		t.Fatalf("the player's action should be resolved too: %q", round.Content)
	}
	if strings.Contains(round.Content, "still to act in this reply:") {
		t.Fatalf("a complete round owes nobody a turn: %q", round.Content)
	}
	if tools.needsMore() {
		t.Fatal("nothing is outstanding after a complete round")
	}
}

// Rolling only the player's action leaves the foe owed, and the server can see it.
func TestHalfARoundIsVisible(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 14, "max_hp": 10}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(13)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 30, "ac": 13, "attack_bonus": 4, "damage": "1d8+2"},
		},
	}})
	tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 5.0, "damage": "1d4+3",
	}})
	if !tools.needsMore() {
		t.Fatal("the guard still owes a turn")
	}
	owed := tools.owed()
	if len(owed) != 1 || !strings.Contains(owed[0], "Guardia") {
		t.Fatalf("the guard should be named as owed, got %v", owed)
	}
}

// Damage taken in a fight has to land on the character sheet: the encounter state ends with the fight, and a character
// that walked out of every battle at full health is the bug this closes.
func TestDamageReachesTheCharacterSheet(t *testing.T) {
	gameSetup := map[string]interface{}{
		"playerCharacters": map[string]interface{}{
			"u1": map[string]interface{}{"name": "Thulim", "displayName": "Thulim", "ac": 14, "max_hp": 10},
		},
	}
	state := encounterState(
		map[string]interface{}{"displayName": "Thulim", "hp_current": 10, "hp_max": 10, "ac": 14},
		map[string]interface{}{"displayName": "Guardia", "hp_current": 12, "hp_max": 12, "ac": 13, "attack_bonus": 20, "damage": "1d4+1"},
	)
	out := resolveCombatTurn([]declaredAction{{
		Actor: "Guardia", Target: "Thulim", Kind: "attack", Modifier: 20,
	}}, state, rand.New(rand.NewSource(2)))
	if !syncPartySheets(gameSetup, out.State, "u1") {
		t.Fatal("the sheet should have changed")
	}
	sheet, _ := gameSetup["playerCharacters"].(map[string]interface{})["u1"].(map[string]interface{})
	hp, _ := intField(sheet, sheetHPKey)
	engineHP, _ := intField(func() map[string]interface{} {
		combatants, _ := combatantsOf(out.State)
		_, c := findCombatant(combatants, "Thulim")
		return c
	}(), participantHPCurrentKey)
	if hp != engineHP || hp >= 10 {
		t.Fatalf("sheet hp %d should match the engine %d and be below full", hp, engineHP)
	}
	// Nothing changed the second time: the sync is not a write per turn.
	if syncPartySheets(gameSetup, out.State, "u1") {
		t.Fatal("an unchanged sheet should not report a change")
	}
}

// Healing gives hit points back and stops at the maximum, and it works outside a fight as well.
func TestHealingRaisesHitPointsUpToTheMaximum(t *testing.T) {
	state := encounterState(map[string]interface{}{"displayName": "Thulim", "hp_current": 3, "hp_max": 10, "ac": 14})
	out := resolveCombatTurn([]declaredAction{{
		Actor: "Thulim", Target: "Thulim", Kind: "heal", Damage: "1d8+4",
	}}, state, rand.New(rand.NewSource(4)))
	if len(out.Lines) != 1 || !strings.Contains(out.Lines[0], "heals Thulim") {
		t.Fatalf("expected a healing line, got %v", out.Lines)
	}
	hp := hpOf(t, out.State, "Thulim")
	if hp < 3 || hp > 10 {
		t.Fatalf("healing should stay within the maximum, got %d", hp)
	}
	if !out.HPChanged {
		t.Fatal("healing that lands should report a change")
	}

	// A Cure Wounds that overshoots stops at the maximum.
	full := resolveCombatTurn([]declaredAction{{
		Actor: "Thulim", Target: "Thulim", Kind: "heal", Damage: "10d10",
	}}, state, rand.New(rand.NewSource(4)))
	if hp := hpOf(t, full.State, "Thulim"); hp != 10 {
		t.Fatalf("healing past the maximum should stop at 10, got %d", hp)
	}
}

// A potion drunk with no fight open still reaches the sheet: the player's own character joins the engine for it.
func TestHealingOutsideAFightUpdatesTheSheet(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 14, "max_hp": 10, "hp_current": 2}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(5)))
	result := tools.run(llm.ToolCall{ID: "1", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Thulim", "target": "Thulim", "kind": "heal", "damage": "1d8+3",
	}})
	if !strings.Contains(result.Content, "heals Thulim") {
		t.Fatalf("the potion should heal, got %q", result.Content)
	}
	state, ok := tools.changed()
	if !ok {
		t.Fatal("the tool should mark the state as changed")
	}
	hp, _ := intField(func() map[string]interface{} {
		combatants, _ := combatantsOf(state)
		_, c := findCombatant(combatants, "Thulim")
		return c
	}(), participantHPCurrentKey)
	if hp <= 2 {
		t.Fatalf("the character should be above 2 hit points, got %d", hp)
	}
}

// The player's row is marked so the sheet panel can find it without guessing from names.
func TestThePlayerRowIsMarked(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim Ingotfoot", "ac": 13, "max_hp": 11, "hp_current": 11}
	merged := mergeDeclaredCombatants(map[string]interface{}{},
		[]declaredCombatant{{Name: "Thulim Ingotfoot"}, {Name: "Guardia", HPMax: 15, AC: 13}},
		sheet, "Thulim Ingotfoot")
	combatants, _ := combatantsOf(merged)
	_, row := findCombatant(combatants, "Thulim Ingotfoot")
	if str(row["kind"]) != "pc" {
		t.Fatalf("the player's row should be marked as the PC, got %v", row["kind"])
	}
	if hp, _ := intField(row, participantHPCurrentKey); hp != 11 {
		t.Fatalf("the row should start from the sheet's current hit points, got %d", hp)
	}
	_, foe := findCombatant(combatants, "Guardia")
	if str(foe["kind"]) == "pc" {
		t.Fatal("a foe must not be marked as the player")
	}
}

// Even if the sheet's display name no longer matches the label the DM uses, the damage lands on the right sheet.
func TestSheetFollowsTheMarkedRowWhenNamesDiffer(t *testing.T) {
	gameSetup := map[string]interface{}{
		"playerCharacters": map[string]interface{}{
			"u1": map[string]interface{}{"name": "Thulim el Breve", "ac": 13, "max_hp": 11, "hp_current": 11},
		},
	}
	state := encounterState(map[string]interface{}{
		"displayName": "Thulim Ingotfoot", "kind": "pc", "hp_current": 8, "hp_max": 11, "ac": 13,
	})
	if !syncPartySheets(gameSetup, state, "u1") {
		t.Fatal("the marked row should have written the sheet")
	}
	sheet, _ := gameSetup["playerCharacters"].(map[string]interface{})["u1"].(map[string]interface{})
	if hp, _ := intField(sheet, sheetHPKey); hp != 8 {
		t.Fatalf("expected 8 hit points on the sheet, got %d", hp)
	}
}

// The reply's own state is checked against the engine's: a foe buried at 9 hit points is a contradiction the server can
// see without reading the prose.
func TestContradictingTheRecordIsDetected(t *testing.T) {
	engine := encounterState(participant("Bram", 7, 11, 13), participant("Guardia", 9, 9, 10))
	buried := encounterState(participant("Bram", 7, 11, 13), map[string]interface{}{
		"displayName": "Guardia", "hp_current": 0, "hp_max": 9,
	})
	if conflict, bad := encounterClaimsConflict(buried, engine); !bad {
		t.Fatalf("a foe claimed dead at 9 hit points should conflict, got none")
	} else if !strings.Contains(conflict, "Guardia") || !strings.Contains(conflict, "9") {
		t.Fatalf("the conflict should name who and what: %q", conflict)
	}

	agreed := encounterState(participant("Bram", 7, 11, 13), participant("Guardia", 9, 9, 10))
	if _, bad := encounterClaimsConflict(agreed, engine); bad {
		t.Fatal("matching states do not conflict")
	}

	// The other direction: the engine has a foe down and the reply has it fighting on.
	downed := encounterState(participant("Bram", 7, 11, 13), map[string]interface{}{
		"displayName": "Guardia", "hp_current": 0, "hp_max": 9,
	})
	alive := encounterState(participant("Bram", 7, 11, 13), participant("Guardia", 4, 9, 10))
	if _, bad := encounterClaimsConflict(alive, downed); !bad {
		t.Fatal("a foe the engine has at 0 should not be described as standing")
	}
}

// The order the engine rolled is shown with its numbers: a player who declared the attack and reads that a foe moved
// first needs to see what decided it.
func TestToolResultsShowTheTurnOrder(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 13, "max_hp": 11}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(21)))
	result := tools.run(llm.ToolCall{ID: "1", Name: "roll", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 10, "ac": 12, "attack_bonus": 3, "damage": "1d8+1"},
		},
		"initiative": []interface{}{
			map[string]interface{}{"actor": "Thulim", "modifier": 1},
			map[string]interface{}{"actor": "Guardia", "modifier": 9},
		},
		"actions": []interface{}{
			map[string]interface{}{"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 5.0, "damage": "1d4+3"},
			map[string]interface{}{"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 3.0, "damage": "1d8+1"},
		},
	}})
	if !strings.Contains(result.Content, "turn order:") {
		t.Fatalf("the order should be part of the answer: %q", result.Content)
	}
	if !strings.Contains(result.Content, "Guardia (") || !strings.Contains(result.Content, "Thulim (") {
		t.Fatalf("both participants and their rolls belong in the order: %q", result.Content)
	}
	// The fight opens with the guard, which is what the narration has to explain.
	first := strings.SplitN(strings.SplitN(result.Content, "turn order: ", 2)[1], "\n", 2)[0]
	if !strings.HasPrefix(first, "Guardia") {
		t.Fatalf("the guard won initiative and should be first, got %q", first)
	}
}

// A blow that drops a foe takes that foe out of the round it fell in. A dead creature swinging back is the engine
// playing a turn the table never saw.
func TestAFoeDroppedInTheRoundTakesNoTurnInIt(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 14, "max_hp": 11}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(21)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			// One hit point, and the player acts first with a bonus a d20 cannot miss on: the guard is down before its
			// own declared attack comes up in the order.
			map[string]interface{}{"displayName": "Guardia", "hp_max": 1, "ac": 5, "attack_bonus": 40, "damage": "1d6+2"},
		},
		"initiative": []interface{}{
			map[string]interface{}{"actor": "Thulim", "modifier": 20},
			map[string]interface{}{"actor": "Guardia", "modifier": 1},
		},
	}})

	round := tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actions": []interface{}{
			map[string]interface{}{"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 50.0, "damage": "1d4+3"},
			map[string]interface{}{"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 40.0, "damage": "1d6+2"},
		},
	}})
	if !strings.Contains(round.Content, "Guardia is at 0 hit points and does not act") {
		t.Fatalf("the dropped guard should be reported as not acting: %q", round.Content)
	}
	if strings.Contains(round.Content, "Guardia attacks Thulim") {
		t.Fatalf("a guard at 0 hit points should not attack: %q", round.Content)
	}
	combatants, _ := combatantsOf(tools.state)
	if _, player := findCombatant(combatants, "Thulim"); player != nil {
		if hp, _ := intField(player, participantHPCurrentKey); hp != 11 {
			t.Fatalf("the player should be untouched, hp was %d", hp)
		}
	}
}

// The table answers between rounds, so a reply gets exactly one. A second roll in the same reply would play a round
// nobody agreed to, and the player could not change strategy inside it.
func TestASecondRoundInTheSameReplyIsRefused(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 14, "max_hp": 30}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(22)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 60, "ac": 13, "attack_bonus": 4, "damage": "1d8+2"},
		},
	}})
	tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actions": []interface{}{
			map[string]interface{}{"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 5.0, "damage": "1d4+3"},
			map[string]interface{}{"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 4.0, "damage": "1d8+2"},
		},
	}})
	again := tools.run(llm.ToolCall{ID: "3", Name: "roll", Arguments: map[string]interface{}{
		"actions": []interface{}{
			map[string]interface{}{"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 5.0, "damage": "1d4+3"},
			map[string]interface{}{"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 4.0, "damage": "1d8+2"},
		},
	}})
	if !strings.Contains(again.Content, "already played its round") {
		t.Fatalf("a second round in one reply should be refused: %q", again.Content)
	}
}

// An action aimed at someone is an attack; the tool does not spend a round trip on a missing field the argument's own
// shape already settles.
func TestAnActionWithoutAKindIsAnAttack(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 14, "max_hp": 10}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(23)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 30, "ac": 13, "damage": "1d8+2"},
		},
	}})
	result := tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Thulim", "target": "Guardia", "modifier": 5.0, "damage": "1d4+3",
	}})
	if strings.Contains(result.Content, "unknown action kind") {
		t.Fatalf("a missing kind with a target is an attack: %q", result.Content)
	}
	if !strings.Contains(result.Content, "Thulim attacks Guardia") {
		t.Fatalf("the attack should be resolved: %q", result.Content)
	}
}

// A creature declared with no hit points used to enter the record at 0 and be read as already down: a foe born dead,
// killed by nobody. The numbers are the DM's to give, so the tool asks for them.
func TestAFoeDeclaredWithoutHitPointsIsRefused(t *testing.T) {
	tools := newCombatTools(nil, nil, "Bram", rand.New(rand.NewSource(24)))
	result := tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Bram", "hp_max": 12, "ac": 13},
			map[string]interface{}{"displayName": "Guardia", "ac": 12},
		},
	}})
	if !strings.Contains(result.Content, "no hit points given for Guardia") {
		t.Fatalf("a creature with no hit points should be named: %q", result.Content)
	}
	combatants, _ := combatantsOf(tools.state)
	if _, found := findCombatant(combatants, "Guardia"); found != nil {
		t.Fatal("a creature with no hit points should not enter the record")
	}

	// Once it is given a maximum, the fight opens with it standing.
	opened := tools.run(llm.ToolCall{ID: "2", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Bram", "hp_max": 12, "ac": 13},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 11, "ac": 12},
		},
	}})
	if strings.Contains(opened.Content, "no hit points given") {
		t.Fatalf("the fight should open once the hit points are given: %q", opened.Content)
	}
	if !strings.Contains(opened.Content, "Guardia: hp 11/11") {
		t.Fatalf("the guard should stand at its full hit points: %q", opened.Content)
	}
}

// A player who attacks a foe the record already has at 0 is swinging at a body. The engine says so instead of rolling
// damage into a fight that is over.
func TestAttackingTheFallenIsRefused(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 13, "max_hp": 11}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(31)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 1, "ac": 5, "damage": "1d6+2"},
		},
	}})
	tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 50.0, "damage": "1d4+3",
	}})

	// The next turn of play is a new reply, so a new tool set over the record the last one left.
	next := newCombatTools(tools.state, sheet, "Thulim", rand.New(rand.NewSource(33)))
	again := next.run(llm.ToolCall{ID: "3", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 50.0, "damage": "1d4+3",
	}})
	if !strings.Contains(again.Content, "already at 0 hit points") {
		t.Fatalf("a swing at a fallen foe should be refused: %q", again.Content)
	}
	if strings.Contains(again.Content, "Thulim attacks Guardia") {
		t.Fatalf("no attack should be rolled against a body: %q", again.Content)
	}
}

// A fight that ended must not carry its bodies into the next one: opening the encounter again used to find the old
// participant already there, leave it at 0 and keep its order, so the new fight began with a corpse.
func TestANewFightDoesNotInheritTheLastOne(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 13, "max_hp": 11}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(32)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 1, "ac": 5, "damage": "1d6+2"},
		},
	}})
	tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 50.0, "damage": "1d4+3",
	}})

	next := newCombatTools(tools.state, sheet, "Thulim", rand.New(rand.NewSource(34)))
	opened := next.run(llm.ToolCall{ID: "3", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 11, "ac": 13, "damage": "1d6+2"},
		},
		"initiative": []interface{}{
			map[string]interface{}{"actor": "Thulim", "modifier": 3},
			map[string]interface{}{"actor": "Guardia", "modifier": 1},
		},
	}})
	if !strings.Contains(opened.Content, "Guardia: hp 11/11") {
		t.Fatalf("the new fight should start with the guard standing: %q", opened.Content)
	}
	if strings.Contains(opened.Content, "hp 0/1") {
		t.Fatalf("the old body should not be in the record: %q", opened.Content)
	}
}

// A round that leaves a foe standing closes by handing the table the next round. Without that handover the model read a
// finished round as permission to play the rest of the fight.
func TestAFinishedRoundHandsTheTableTheNextOne(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 13, "max_hp": 30}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(41)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 60, "ac": 13, "attack_bonus": 4, "damage": "1d8+2"},
		},
	}})
	round := tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actions": []interface{}{
			map[string]interface{}{"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 5.0, "damage": "1d4+3"},
			map[string]interface{}{"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 4.0, "damage": "1d8+2"},
		},
	}})
	if !strings.Contains(round.Content, "round 1 is complete") {
		t.Fatalf("a played round should say so: %q", round.Content)
	}
	if !strings.Contains(round.Content, "the table's: Thulim declares round 2") {
		t.Fatalf("the next round belongs to the table: %q", round.Content)
	}
	// The engine hands over on its own: the record names the player before the model says anything about it.
	if !tools.HandedOver() || tools.AwaitingActor() != "Thulim" {
		t.Fatalf("the record should wait on the player, got handedOver=%v awaiting=%q", tools.HandedOver(), tools.AwaitingActor())
	}
	if roundNumberOf(tools.state) != 2 {
		t.Fatalf("the record should be in round 2, got %d", roundNumberOf(tools.state))
	}

	// The round that ends the fight has nothing to hand over.
	ending := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(42)))
	ending.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Rata", "hp_max": 1, "ac": 5},
		},
	}})
	killed := ending.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Thulim", "target": "Rata", "kind": "attack", "modifier": 50.0, "damage": "1d4+3",
	}})
	if !strings.Contains(killed.Content, "the fight is over: no foe is standing") {
		t.Fatalf("the fight that ends should be reported as over: %q", killed.Content)
	}
	if strings.Contains(killed.Content, "declares round") {
		t.Fatalf("nothing is handed over when nobody is left: %q", killed.Content)
	}
}

// The handover is the server's to guarantee: a round that closes with a foe standing ends by asking the table, in the
// game's own language, and the actor named is the player's own character when the order reaches them first.
func TestTheRoundHandoverNamesThePlayerAndSpeaksTheGamesLanguage(t *testing.T) {
	spanish := roundHandoverLine("Spanish", 3, "Varek")
	if !strings.Contains(spanish, "Ronda 3") || !strings.Contains(spanish, "Varek") || !strings.HasSuffix(spanish, "?") {
		t.Fatalf("the handover should name the round and ask the player: %q", spanish)
	}
	english := roundHandoverLine("English", 3, "Varek")
	if !strings.Contains(english, "Round 3") || !strings.HasSuffix(english, "?") {
		t.Fatalf("english handover: %q", english)
	}
	if asksTheTable("La ronda termina frente al fuego.") {
		t.Fatal("a closing reflection is not a question to the table")
	}
	if !asksTheTable("La ronda termina.\n\n¿Qué hace Varek?") {
		t.Fatal("a question to the table is a handover whatever comes before it")
	}

	sheet := map[string]interface{}{"name": "Varek", "ac": 13, "max_hp": 20}
	tools := newCombatTools(nil, sheet, "Varek", rand.New(rand.NewSource(51)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Varek"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 30, "ac": 12, "attack_bonus": 3, "damage": "1d6+1"},
		},
		"initiative": []interface{}{
			map[string]interface{}{"actor": "Guardia", "modifier": 1},
			map[string]interface{}{"actor": "Varek", "modifier": 9},
		},
	}})
	if got := tools.openingActor(); got != "Varek" {
		t.Fatalf("the player opens the round when the order reaches them first, got %q", got)
	}
}

// A reply that reports a different hit point total than the engine settled is recomputing, and the server can see it.
func TestInventedHitPointsAreContradicted(t *testing.T) {
	engine := encounterState(participant("Thulim", 7, 11, 13), participant("Guardia", 40, 40, 15))
	drifted := encounterState(participant("Thulim", 4, 11, 13), participant("Guardia", 40, 40, 15))
	conflict, bad := encounterClaimsConflict(drifted, engine)
	if !bad {
		t.Fatal("a recomputed hit point total should be a contradiction")
	}
	if !strings.Contains(conflict, "7") || !strings.Contains(conflict, "4") {
		t.Fatalf("the correction should name both totals: %q", conflict)
	}
}

// The handover is a tool call with an invariant: a round that is only half played cannot be handed over, and a reply
// that hands over cannot roll again. That is what makes the table's turn structural instead of a matter of phrasing.
func TestTheHandoverIsAToolCallWithAnInvariant(t *testing.T) {
	sheet := map[string]interface{}{"name": "Thulim", "ac": 13, "max_hp": 30}
	tools := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(61)))
	tools.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 60, "ac": 13, "attack_bonus": 4, "damage": "1d8+2"},
		},
	}})

	// The foe has not acted: handing over now would skip its turn.
	early := tools.run(llm.ToolCall{ID: "2", Name: "request_action", Arguments: map[string]interface{}{}})
	if !strings.Contains(early.Content, "the round is not played yet") || !strings.Contains(early.Content, "Guardia") {
		t.Fatalf("a half-played round cannot be handed over: %q", early.Content)
	}
	if tools.HandedOver() {
		t.Fatal("a refused handover must not count")
	}

	tools.run(llm.ToolCall{ID: "3", Name: "roll", Arguments: map[string]interface{}{
		"actions": []interface{}{
			map[string]interface{}{"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 5.0, "damage": "1d4+3"},
			map[string]interface{}{"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 4.0, "damage": "1d8+2"},
		},
	}})
	handover := tools.run(llm.ToolCall{ID: "4", Name: "request_action", Arguments: map[string]interface{}{}})
	if !strings.Contains(handover.Content, "the table's turn: Thulim") {
		t.Fatalf("the handover should name the player's character: %q", handover.Content)
	}
	if !tools.HandedOver() || tools.AwaitingActor() != "Thulim" {
		t.Fatalf("the record should wait on the player, got handedOver=%v awaiting=%q", tools.HandedOver(), tools.AwaitingActor())
	}
	if str(tools.state[awaitingKey]) != "Thulim" || roundNumberOf(tools.state) != 2 {
		t.Fatalf("the state should carry the wait and the round, got %v", tools.state)
	}

	// Nothing rolls after the handover: that round belongs to the players' answer.
	after := tools.run(llm.ToolCall{ID: "5", Name: "roll", Arguments: map[string]interface{}{
		"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 4.0, "damage": "1d8+2",
	}})
	if !strings.Contains(after.Content, "already played its round and handed the table its turn") {
		t.Fatalf("a roll after the handover should be refused: %q", after.Content)
	}

	// A model that names one of its own creatures still hands the turn to the player.
	fresh := newCombatTools(nil, sheet, "Thulim", rand.New(rand.NewSource(62)))
	fresh.run(llm.ToolCall{ID: "1", Name: "open_encounter", Arguments: map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"displayName": "Thulim"},
			map[string]interface{}{"displayName": "Guardia", "hp_max": 60, "ac": 13, "damage": "1d8+2"},
		},
	}})
	fresh.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actions": []interface{}{
			map[string]interface{}{"actor": "Thulim", "target": "Guardia", "kind": "attack", "modifier": 5.0, "damage": "1d4+3"},
			map[string]interface{}{"actor": "Guardia", "target": "Thulim", "kind": "attack", "modifier": 4.0, "damage": "1d8+2"},
		},
	}})
	wrong := fresh.run(llm.ToolCall{ID: "3", Name: "request_action", Arguments: map[string]interface{}{"actor": "Guardia"}})
	if !strings.Contains(wrong.Content, "the table's turn: Thulim") {
		t.Fatalf("the wait belongs to the player, not the DM's creature: %q", wrong.Content)
	}
}
