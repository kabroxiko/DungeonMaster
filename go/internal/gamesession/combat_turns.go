package gamesession

import (
	"fmt"
	"math/rand"
	"strings"
)

// A fight is played one turn at a time, and the pointer in the record says whose turn it is. The engine owns that
// pointer: it decides when the reply stops - at the first player who has to declare something - and it is the only
// thing that knows the order. A model that decides for itself whose turn it is will skip a foe, play a round twice, or
// answer for a creature nobody asked it about.

// turnIndexKey is where the record keeps the pointer into the initiative order.
const turnIndexKey = "turnIndex"

// turnPointer is the position in the stored order that acts next.
func turnPointer(state map[string]interface{}) int {
	if v, ok := intField(state, turnIndexKey); ok && v >= 0 {
		return v
	}
	return 0
}

// orderOfState reads the stored initiative as display names, highest first.
func orderOfState(state map[string]interface{}) []string {
	entries := toIfaceSlice(state["initiative"])
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		m, ok := entry.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(m["displayName"]))
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// currentActorOf is whose turn it is: the next creature in the order that is still on its feet. Empty when nobody is
// standing or the fight has no order yet.
func currentActorOf(state map[string]interface{}) string {
	order := orderOfState(state)
	if len(order) == 0 {
		return ""
	}
	combatants, ok := combatantsOf(state)
	if !ok {
		return ""
	}
	start := turnPointer(state)
	for step := 0; step < len(order); step++ {
		name := order[(start+step)%len(order)]
		_, combatant := findCombatant(combatants, name)
		if combatant == nil {
			continue
		}
		if hp, has := intField(combatant, participantHPCurrentKey); has && hp <= 0 {
			continue
		}
		return name
	}
	return ""
}

// advanceTurn moves the pointer past the actor whose turn just ended. Passing the last name in the order starts a new
// round, which is the only place the round number moves.
func advanceTurn(state map[string]interface{}) map[string]interface{} {
	order := orderOfState(state)
	if len(order) == 0 {
		return state
	}
	next := (turnPointer(state) + 1) % len(order)
	out := map[string]interface{}{}
	for k, v := range state {
		out[k] = v
	}
	if next == 0 {
		out[roundKey] = roundNumberOf(state) + 1
	}
	out[turnIndexKey] = next
	return out
}

// turnDecider decides what one creature does on its own turn. It is the model, in the game; in the tests it is a stub,
// which is the point: the loop itself has to be right without a model in it.
type turnDecider func(actor string, state map[string]interface{}) (declaredAction, error)

// playCombatTurns plays turns from the pointer until the table has to speak, and returns the actor it is waiting on.
//
// The player's own declaration is used the first time the pointer reaches their character, and never twice: the answer
// the table owes is about the turn that comes next, not the one already declared. Everything else - the foe that acts
// before them, the foe that acts after - is decided one creature at a time, in order.
func playCombatTurns(state map[string]interface{}, humanNames []string, declared declaredAction, decide turnDecider, rng *rand.Rand, maxTurns int) (map[string]interface{}, []string, string) {
	if state == nil || maxTurns <= 0 {
		return state, nil, ""
	}
	lines := make([]string, 0, maxTurns)
	// A declaration exists when it names an action, not when it names where it came from: the caller knows whose turn is
	// being answered, and the record is where the actor is read from.
	declarationUsed := strings.TrimSpace(declared.Kind) == ""
	startPointer := turnPointer(state)
	for turn := 0; turn < maxTurns; turn++ {
		actor := currentActorOf(state)
		if actor == "" {
			return state, lines, ""
		}
		if isHumanActor(actor, humanNames) {
			if declarationUsed {
				// The pointer has come back to the player: the rest of this round belongs to their answer.
				return state, lines, actor
			}
			declared.Actor = actor
			outcome := resolveCombatTurn([]declaredAction{declared}, state, rng)
			state = outcome.State
			lines = append(lines, outcome.Lines...)
			for _, unknown := range outcome.Unknown {
				lines = append(lines, "not resolved: "+unknown)
			}
			declarationUsed = true
			state = advanceTurn(state)
			if turnPointer(state) == startPointer {
				// A full round is played. The reply ends here whoever is left standing: a fight of nothing but AI turns
				// would otherwise run forever, and the table is owed the word between rounds.
				return state, lines, nextHumanActor(state, humanNames)
			}
			continue
		}
		action, err := decide(actor, state)
		if err != nil || strings.TrimSpace(action.Kind) == "" {
			// The creature's turn could not be decided. The table is not left waiting on a DM that failed: the fight
			// stops where it is, and the narration says what the round has produced so far.
			return state, lines, ""
		}
		action.Actor = actor
		outcome := resolveCombatTurn([]declaredAction{action}, state, rng)
		state = outcome.State
		lines = append(lines, outcome.Lines...)
		for _, unknown := range outcome.Unknown {
			lines = append(lines, "not resolved: "+unknown)
		}
		state = advanceTurn(state)
		if turnPointer(state) == startPointer {
			return state, lines, nextHumanActor(state, humanNames)
		}
	}
	return state, lines, nextHumanActor(state, humanNames)
}

// nextHumanActor is the first player the pointer will reach, or empty when nobody at the table is left on their feet.
func nextHumanActor(state map[string]interface{}, humanNames []string) string {
	order := orderOfState(state)
	if len(order) == 0 {
		return ""
	}
	combatants, ok := combatantsOf(state)
	if !ok {
		return ""
	}
	for step := 0; step < len(order); step++ {
		name := order[(turnPointer(state)+step)%len(order)]
		if !isHumanActor(name, humanNames) {
			continue
		}
		_, combatant := findCombatant(combatants, name)
		if combatant == nil {
			continue
		}
		if hp, has := intField(combatant, participantHPCurrentKey); has && hp <= 0 {
			continue
		}
		return name
	}
	return ""
}

// isHumanActor reports whether a creature is declared by a person at the table. A party member whose player is not
// there is played like a foe, one turn at a time, which is what a one-player table needs for the rest of the party.
func isHumanActor(actor string, humanNames []string) bool {
	for _, name := range humanNames {
		if strings.EqualFold(strings.TrimSpace(actor), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}
