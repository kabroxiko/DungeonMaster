package gamesession

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/llm"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
)

// The Referee's two narrow jobs.
//
// The model is not asked to run a fight any more. It is asked two small questions with a strict shape: what did the
// player just declare, and what does this one creature do on its own turn. The engine owns everything else - the order,
// the pointer, the dice, the record - because that is where the rules live and a model that improvises them skips a
// turn, plays a round twice, or invents a number.

// declaration is what the player's message amounts to once read as rules, plus the creatures a fight needs when this is
// the turn that starts one.
type declaration struct {
	Action       declaredAction
	Participants []declaredCombatant
	Initiative   []declaredInitiative
	// Ask is what the table still owes before anything can be rolled ("weapon" when the character has several and the
	// message named none). Empty when the declaration is complete.
	Ask string
}

// interpretTurn reads the player's message as one declared action.
func interpretTurn(ctx context.Context, cfg *config.Config, gs, sheet map[string]interface{}, inbound []interface{}, gameID string) (declaration, error) {
	system := promptmgr.LoadPrompt("skills/combat_declare.md")
	if system == "" {
		return declaration{}, fmt.Errorf("the declaration prompt is missing")
	}
	if record := encounterRecordBlock(gs); record != "" {
		system += "\n\n" + record
	}
	msgs := []interface{}{map[string]interface{}{"role": "system", "content": system}}
	msgs = append(msgs, tailMessages(inbound, 6)...)

	raw, err := llm.GenerateResponse(ctx, cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens":       500,
		"temperature":      0.2,
		"gameId":           gameID,
		"reasoning_effort": "low",
		"response_format":  map[string]interface{}{"type": "json_object"},
	})
	if err != nil {
		return declaration{}, err
	}
	obj, ok := llm.ParseModelStructuredObject(raw)
	if !ok || obj == nil {
		return declaration{}, nil
	}
	out := declaration{
		Participants: parseDeclaredCombatants(obj["participants"]),
		Initiative:   parseDeclaredInitiative(obj["initiative"]),
		Ask:          strings.TrimSpace(str(obj["ask"])),
	}
	actions := parseDeclaredActions(obj["actions"])
	if len(actions) > 0 {
		out.Action = actions[0]
	}
	_ = sheet
	return out, nil
}

// decideTurn asks what one creature does on its own turn. One creature, one answer: the call cannot decide the rest of
// the round, which is what keeps a foe's turn a turn instead of a batch.
func decideTurn(ctx context.Context, cfg *config.Config, gs map[string]interface{}, state map[string]interface{}, actor, gameID string) (declaredAction, error) {
	template := promptmgr.LoadPrompt("skills/npc_turn.md")
	if template == "" {
		return declaredAction{}, fmt.Errorf("the turn prompt is missing")
	}
	msgs := []interface{}{map[string]interface{}{"role": "system", "content": strings.ReplaceAll(template, "{{actor}}", actor)}}
	if record := turnRecordBlock(state, actor); record != "" {
		msgs = append(msgs, map[string]interface{}{"role": "system", "content": record})
	}
	raw, err := llm.GenerateResponse(ctx, cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens":       300,
		"temperature":      0.4,
		"gameId":           gameID,
		"reasoning_effort": "low",
		"response_format":  map[string]interface{}{"type": "json_object"},
	})
	if err != nil {
		return declaredAction{}, err
	}
	obj, ok := llm.ParseModelStructuredObject(raw)
	if !ok || obj == nil {
		return declaredAction{}, nil
	}
	actions := parseDeclaredActions(obj["actions"])
	if len(actions) == 0 {
		return declaredAction{}, nil
	}
	action := actions[0]
	action.Actor = actor
	return action, nil
}

// turnRecordBlock is what a creature needs to decide its own turn: the order, the round, who is standing and how hurt.
func turnRecordBlock(state map[string]interface{}, actor string) string {
	combatants, ok := combatantsOf(state)
	if !ok {
		return ""
	}
	lines := make([]string, 0, len(combatants))
	for _, combatant := range combatants {
		hp, _ := intField(combatant, participantHPCurrentKey)
		maxHP, _ := intField(combatant, participantHPMaxKey)
		ac, _ := intField(combatant, participantACKey)
		status := "up"
		if hp <= 0 {
			status = "down"
		}
		lines = append(lines, fmt.Sprintf("- %v: hp %d/%d, ac %d, %s", combatant[participantNameKey], hp, maxHP, ac, status))
	}
	order := initiativeOrderOf(state)
	return fmt.Sprintf("This fight, as the server records it (round %d).\nTurn order: %s\nActing now: %s\nStanding:\n%s",
		roundNumberOf(state), strings.Join(order, " -> "), actor, strings.Join(lines, "\n"))
}

// encounterRecordBlock is the stored fight as the interpreter sees it, so a declared attack names a creature that
// exists instead of a role it made up.
func encounterRecordBlock(gs map[string]interface{}) string {
	if gs == nil {
		return ""
	}
	state, _ := gs["encounterState"].(map[string]interface{})
	if len(state) == 0 {
		return ""
	}
	combatants, ok := combatantsOf(state)
	if !ok {
		return ""
	}
	lines := make([]string, 0, len(combatants))
	for _, combatant := range combatants {
		hp, _ := intField(combatant, participantHPCurrentKey)
		maxHP, _ := intField(combatant, participantHPMaxKey)
		status := "up"
		if hp <= 0 {
			status = "down"
		}
		lines = append(lines, fmt.Sprintf("- %v: hp %d/%d, %s", combatant[participantNameKey], hp, maxHP, status))
	}
	return fmt.Sprintf("The fight already in the record (round %d). A declared action is resolved against these names:\n%s",
		roundNumberOf(state), strings.Join(lines, "\n"))
}

// tailMessages keeps the most recent exchanges, which is the beat the declaration is about.
func tailMessages(inbound []interface{}, keep int) []interface{} {
	if keep <= 0 || len(inbound) <= keep {
		return inbound
	}
	return inbound[len(inbound)-keep:]
}

// openFight installs the participants and the order the interpreter declared, and points the record at the first name.
func openFight(state map[string]interface{}, declared declaration, sheet map[string]interface{}, pcName string, rng *rand.Rand) (map[string]interface{}, []string) {
	if state == nil {
		state = map[string]interface{}{}
	}
	if len(declared.Participants) > 0 {
		state = mergeDeclaredCombatants(state, declared.Participants, sheet, pcName)
	}
	lines, scored := resolveInitiative(declared.Initiative, state, rng)
	if scored != nil {
		state = scored
	}
	if _, ok := intField(state, turnIndexKey); !ok {
		state[turnIndexKey] = 0
	}
	return state, lines
}

// combatTurnsPerReply bounds one reply. A round is short; anything longer is a run of turns that should have stopped at
// a player, and a reply that never ends is worse than a fight that waits.
const combatTurnsPerReply = 12

// humanActors is who at the table declares their own turn. Today that is the character of whoever made the request: the
// rest of the party is played by the AI on its own turn, like any foe. A table with several connected players adds their
// characters here, and the pointer stops on each of them in turn order.
func humanActors(gs map[string]interface{}, requestingUserID, pcName string) []string {
	if strings.TrimSpace(pcName) == "" {
		return nil
	}
	return []string{pcName}
}

// combatResultsBlock is what the narrator is given: the lines the engine resolved, the record they left, and who owes
// the next declaration. The narration is written from this, not from a fight the model would have to imagine.
func combatResultsBlock(lines []string, state map[string]interface{}, waitingOn string) string {
	if len(lines) == 0 && len(state) == 0 {
		return ""
	}
	b := strings.Builder{}
	b.WriteString("ENGINE RESULTS for this turn. These are the numbers; the narration is written from them.\n")
	for _, line := range lines {
		b.WriteString("- " + line + "\n")
	}
	if len(state) > 0 {
		if order := initiativeOrderOf(state); len(order) > 0 {
			b.WriteString("turn order: " + strings.Join(order, " -> ") + "\n")
		}
		if fallen := fallenSummary(state); fallen != "" {
			b.WriteString("down: " + fallen + "\n")
		}
		b.WriteString(fmt.Sprintf("round %d\n", roundNumberOf(state)))
	}
	if waitingOn != "" {
		b.WriteString(fmt.Sprintf("the table's turn: %s declares next. End the narration by asking them what they do, and do not play the next turn.\n", waitingOn))
	} else if len(state) > 0 && !foesStandingIn(state) {
		b.WriteString("the fight is over: no foe is standing.\n")
	}
	return b.String()
}

// fallenSummary names who the round put down, and nothing else. A full roster of hit points and armor classes in the
// prompt is read back into the narration as a stat block, which is the one thing a turn of play must not sound like.
func fallenSummary(state map[string]interface{}) string {
	combatants, ok := combatantsOf(state)
	if !ok {
		return ""
	}
	names := make([]string, 0, len(combatants))
	for _, combatant := range combatants {
		if hp, has := intField(combatant, participantHPCurrentKey); has && hp <= 0 {
			names = append(names, fmt.Sprint(combatant[participantNameKey]))
		}
	}
	return strings.Join(names, ", ")
}

// foesStandingIn reports whether the record still holds a creature for the table to fight.
func foesStandingIn(state map[string]interface{}) bool {
	combatants, ok := combatantsOf(state)
	if !ok {
		return false
	}
	for _, combatant := range combatants {
		if strings.EqualFold(str(combatant["kind"]), "pc") {
			continue
		}
		if hp, has := intField(combatant, participantHPCurrentKey); !has || hp > 0 {
			return true
		}
	}
	return false
}
