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
	if !strings.Contains(opened.Content, "still to act in this reply:") {
		t.Fatalf("opening a fight should say who has not acted: %q", opened.Content)
	}
	if !strings.Contains(opened.Content, "Goblin") || !strings.Contains(opened.Content, "Bram") {
		t.Fatalf("both sides are owed a turn after the opening: %q", opened.Content)
	}

	afterPlayer := tools.run(llm.ToolCall{ID: "2", Name: "roll", Arguments: map[string]interface{}{
		"actor": "player", "target": "Goblin", "kind": "attack", "modifier": 5.0, "damage": "1d4+3",
	}})
	if !strings.Contains(afterPlayer.Content, "still to act in this reply: Goblin") {
		t.Fatalf("after the player's turn the foe is still owed one: %q", afterPlayer.Content)
	}
	if strings.Contains(afterPlayer.Content, "still to act in this reply: Bram") {
		t.Fatalf("the player already acted: %q", afterPlayer.Content)
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
