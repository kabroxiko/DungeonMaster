package gamesession

import (
	"errors"
	"math/rand"
	"strings"
	"testing"
)

// initiative: Guardia 18 -> Lyra 15 -> Thulim 12. Two player characters and one foe.
func twoPlayerFight() map[string]interface{} {
	state := encounterState(
		participant("Guardia", 40, 40, 13),
		participant("Lyra", 20, 20, 14),
		participant("Thulim", 20, 20, 13),
	)
	state["initiative"] = []interface{}{
		map[string]interface{}{"displayName": "Guardia", "score": 18},
		map[string]interface{}{"displayName": "Lyra", "score": 15},
		map[string]interface{}{"displayName": "Thulim", "score": 12},
	}
	return state
}

// A foe that acts before the player is decided by the AI on its own turn, and the reply stops at the player who has to
// speak next - not at the end of a round.
func TestTurnsStopAtThePlayerWhoMustDeclare(t *testing.T) {
	decided := make([]string, 0, 2)
	decide := func(actor string, state map[string]interface{}) (declaredAction, error) {
		decided = append(decided, actor)
		return declaredAction{Kind: "attack", Target: "Lyra", Modifier: 4, Damage: "1d6+2"}, nil
	}
	declared := declaredAction{Kind: "attack", Target: "Guardia", Modifier: 5, Damage: "1d4+3"}

	state, lines, waiting := playCombatTurns(twoPlayerFight(), []string{"Lyra", "Thulim"}, declared, decide, rand.New(rand.NewSource(7)), 8)
	if waiting != "Thulim" {
		t.Fatalf("the reply should stop on the second player, got %q", waiting)
	}
	if len(decided) != 1 || decided[0] != "Guardia" {
		t.Fatalf("only the foe should have been decided by the AI, got %v", decided)
	}
	if !containsLine(lines, "Guardia attacks Lyra") {
		t.Fatalf("the foe's turn should be resolved before the player's: %v", lines)
	}
	if !containsLine(lines, "Lyra attacks Guardia") {
		t.Fatalf("the player's own declaration should be resolved on their turn: %v", lines)
	}
	if containsLine(lines, "Thulim attacks") {
		t.Fatalf("the second player's turn is theirs to declare: %v", lines)
	}
	if roundNumberOf(state) != 1 {
		t.Fatalf("the round advances when the order wraps, got %d", roundNumberOf(state))
	}
}

// With one player the same loop plays the rest of the round and hands over when the order comes back to them.
func TestTurnsPlayTheRestOfTheRoundAndHandBack(t *testing.T) {
	decide := func(actor string, state map[string]interface{}) (declaredAction, error) {
		return declaredAction{Kind: "attack", Target: "Guardia", Modifier: 4, Damage: "1d6+2"}, nil
	}
	declared := declaredAction{Kind: "attack", Target: "Guardia", Modifier: 5, Damage: "1d4+3"}
	state := twoPlayerFight()

	turns, lines, waiting := playCombatTurns(state, []string{"Lyra"}, declared, decide, rand.New(rand.NewSource(11)), 8)
	if waiting != "Lyra" {
		t.Fatalf("the table owes the next round to the player, got %q", waiting)
	}
	if !containsLine(lines, "Thulim attacks") {
		t.Fatalf("a party member with no player at the table is played on their turn: %v", lines)
	}
	if roundNumberOf(turns) != 2 {
		t.Fatalf("the order wrapped, so the record is in round 2, got %d", roundNumberOf(turns))
	}

	// A party member with nobody at the table is played like a foe: the AI decides their turn.
	aiParty, aiLines, _ := playCombatTurns(state, []string{"Lyra"}, declared, decide, rand.New(rand.NewSource(12)), 8)
	if aiParty == nil || aiLines == nil {
		t.Fatal("the loop should produce a record and its lines")
	}
}

// A creature whose turn the AI cannot decide does not leave the table waiting on a broken DM: the fight stops where it
// is and keeps the record it already produced.
func TestAFailedDecisionStopsWithoutLosingTheRecord(t *testing.T) {
	decide := func(actor string, state map[string]interface{}) (declaredAction, error) {
		return declaredAction{}, errors.New("no answer from the model")
	}
	state, lines, waiting := playCombatTurns(twoPlayerFight(), []string{"Lyra", "Thulim"}, declaredAction{}, decide, rand.New(rand.NewSource(13)), 8)
	if waiting != "" {
		t.Fatalf("nobody is asked to declare when the DM failed, got %q", waiting)
	}
	if len(lines) != 0 {
		t.Fatalf("nothing was resolved, so there are no lines: %v", lines)
	}
	if currentActorOf(state) != "Guardia" {
		t.Fatalf("the pointer should not have moved past the turn that failed, got %q", currentActorOf(state))
	}
}

// The pointer skips whoever is down, and it is the only place a round number changes.
func TestThePointerSkipsTheFallenAndCountsRounds(t *testing.T) {
	state := twoPlayerFight()
	state["turnIndex"] = 1
	if got := currentActorOf(state); got != "Lyra" {
		t.Fatalf("the pointer should be on Lyra, got %q", got)
	}
	combatants, _ := combatantsOf(state)
	if _, lyra := findCombatant(combatants, "Lyra"); lyra != nil {
		lyra[participantHPCurrentKey] = 0
	}
	state["participants"] = withCombatants(state, combatants)["participants"]
	if got := currentActorOf(state); got != "Thulim" {
		t.Fatalf("a fallen character is skipped, got %q", got)
	}

	state["turnIndex"] = 2
	next := advanceTurn(state)
	if got := turnPointer(next); got != 0 {
		t.Fatalf("the order should wrap to the first name, got %d", got)
	}
	if roundNumberOf(next) != 2 {
		t.Fatalf("wrapping the order starts a new round, got %d", roundNumberOf(next))
	}
}

func containsLine(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// A fight whose actors are all played by the AI stops after one round: a reply that never reaches a player is a fight
// the table cannot answer, and a loop with no human in it would otherwise run to its cap.
func TestAReplyEndsAfterOneRoundEvenWithNoPlayerLeft(t *testing.T) {
	decide := func(actor string, state map[string]interface{}) (declaredAction, error) {
		return declaredAction{Kind: "attack", Target: "Thulim", Modifier: 4, Damage: "1d6+2"}, nil
	}
	state := twoPlayerFight()
	started := turnPointer(state)
	// Nobody at the table: a party the AI runs entirely, which is the case with no human name in the order.
	turns, lines, waiting := playCombatTurns(state, nil, declaredAction{}, decide, rand.New(rand.NewSource(21)), 12)
	if len(lines) != 3 {
		t.Fatalf("one round of three creatures is three turns, got %d: %v", len(lines), lines)
	}
	if turnPointer(turns) != started {
		t.Fatalf("the pointer should come back to where the reply began, got %d", turnPointer(turns))
	}
	if waiting != "" {
		t.Fatalf("nobody at the table owes anything, got %q", waiting)
	}
	if roundNumberOf(turns) != 2 {
		t.Fatalf("the round advanced once the order wrapped, got %d", roundNumberOf(turns))
	}
}
