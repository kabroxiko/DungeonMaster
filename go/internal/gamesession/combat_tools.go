package gamesession

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/deckofdmthings/gmai/internal/llm"
)

// combatTools exposes the dice engine to the model while it writes the turn.
//
// The order matters and is the whole point: the model asks for a roll, the server rolls and answers, and only then does
// the model narrate. A model that owns the die invents it and then explains the invented number; a model that asks has
// the truth in hand before it writes a word.
type combatTools struct {
	mu    sync.Mutex
	state map[string]interface{}
	sheet map[string]interface{}
	pc    string
	rng   *rand.Rand
	// used records that a tool changed something, so the caller knows the encounter state is worth storing.
	used bool
	// acted is who has already taken a turn in this reply. A round in which only the player swung is not a round, and
	// the model cannot be trusted to remember the order it was given: the answer says who is still owed a turn.
	acted map[string]struct{}
}

func newCombatTools(state map[string]interface{}, sheet map[string]interface{}, pcName string, rng *rand.Rand) *combatTools {
	if state == nil {
		state = map[string]interface{}{}
	}
	return &combatTools{state: state, sheet: sheet, pc: pcName, rng: rng, acted: map[string]struct{}{}}
}

func (t *combatTools) changed() (map[string]interface{}, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.used {
		return nil, false
	}
	return t.state, true
}

// defs is what the model is offered. Three tools, because a fight needs three questions: is there one, what is the
// order, and what did the die say.
func (t *combatTools) defs() []llm.Tool {
	return []llm.Tool{
		{
			Name: "open_encounter",
			Description: "Start a fight. Give every combatant that is not in the encounter yet, and the initiative modifier " +
				"for each. Returns the turn order. Call this once, when combat begins, before rolling anything.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"participants": map[string]interface{}{
						"type":        "array",
						"description": "Combatants to add. Include the player's character; the server takes its hit points and armor class from the sheet.",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"displayName":  map[string]interface{}{"type": "string"},
								"hp_max":       map[string]interface{}{"type": "integer"},
								"ac":           map[string]interface{}{"type": "integer"},
								"attack_bonus": map[string]interface{}{"type": "integer"},
								"damage":       map[string]interface{}{"type": "string"},
							},
							"required": []interface{}{"displayName"},
						},
					},
					"initiative": map[string]interface{}{
						"type":        "array",
						"description": "One entry per combatant, with the modifier added to the d20.",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"actor":    map[string]interface{}{"type": "string"},
								"modifier": map[string]interface{}{"type": "integer"},
							},
							"required": []interface{}{"actor"},
						},
					},
				},
			},
		},
		{
			Name: "roll",
			Description: "Roll one action and get its outcome. An attack is rolled against the target's stored armor class " +
				"and its damage applied to the target's hit points. A save or check is rolled against the DC you give.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"actor":    map[string]interface{}{"type": "string", "description": "displayName of the creature acting."},
					"target":   map[string]interface{}{"type": "string", "description": "displayName of whoever it lands on; omit for a save or check."},
					"kind":     map[string]interface{}{"type": "string", "enum": []interface{}{"attack", "save", "check"}},
					"modifier": map[string]interface{}{"type": "integer", "description": "The whole number added to the d20."},
					"damage":   map[string]interface{}{"type": "string", "description": "Damage dice such as 1d8+3; attacks only."},
					"dc":       map[string]interface{}{"type": "integer", "description": "The number a save or check resolves against."},
				},
				"required": []interface{}{"actor", "kind"},
			},
		},
		{
			Name:        "encounter",
			Description: "Read the fight as the server records it: who is in it, hit points, armor class and the turn order.",
			Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
	}
}

// run executes one call and answers with text the model can read: the rolled line first, then who is standing.
func (t *combatTools) run(call llm.ToolCall) llm.ToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch strings.ToLower(strings.TrimSpace(call.Name)) {
	case "open_encounter":
		return t.openEncounter(call)
	case "roll":
		return t.roll(call)
	case "encounter":
		return llm.ToolResult{CallID: call.ID, Content: t.describe()}
	default:
		return llm.ToolResult{CallID: call.ID, Content: fmt.Sprintf("error: there is no tool named %q", call.Name)}
	}
}

func (t *combatTools) openEncounter(call llm.ToolCall) llm.ToolResult {
	declared := parseDeclaredCombatants(call.Arguments["participants"])
	initiative := parseDeclaredInitiative(call.Arguments["initiative"])
	if len(declared) == 0 && len(initiative) == 0 {
		return llm.ToolResult{CallID: call.ID, Content: "error: give at least one participant or one initiative entry"}
	}
	if len(declared) > 0 {
		t.state = mergeDeclaredCombatants(t.state, declared, t.sheet, t.pc)
		t.used = true
	}
	var order []string
	if lines, scored := resolveInitiative(initiative, t.state, t.rng); scored != nil {
		t.state = scored
		t.used = true
		order = append(order, lines...)
	}
	if existing := toIfaceSlice(t.state["initiative"]); len(order) == 0 && len(existing) > 0 {
		for _, entry := range existing {
			m, _ := entry.(map[string]interface{})
			if m != nil {
				order = append(order, fmt.Sprintf("%v %v", m["displayName"], m["score"]))
			}
		}
	}
	body := strings.Builder{}
	body.WriteString("encounter open.\n")
	if len(order) > 0 {
		body.WriteString("turn order: " + strings.Join(order, " -> ") + "\n")
	}
	body.WriteString(t.describe())
	return llm.ToolResult{CallID: call.ID, Content: body.String()}
}

func (t *combatTools) roll(call llm.ToolCall) llm.ToolResult {
	action := declaredAction{
		Actor:    t.resolveName(str(call.Arguments["actor"])),
		Target:   t.resolveName(str(call.Arguments["target"])),
		Kind:     strings.TrimSpace(str(call.Arguments["kind"])),
		Damage:   strings.TrimSpace(str(call.Arguments["damage"])),
		Modifier: intFromValue(call.Arguments["modifier"]),
		DC:       intFromValue(call.Arguments["dc"]),
		AC:       intFromValue(call.Arguments["ac"]),
	}
	if action.Actor == "" {
		return llm.ToolResult{CallID: call.ID, Content: "error: actor is required"}
	}
	outcome := resolveCombatTurn([]declaredAction{action}, t.state, t.rng)
	t.state = outcome.State
	t.used = true
	if len(outcome.Lines) > 0 {
		t.markActed(action.Actor)
	}
	body := strings.Builder{}
	for _, line := range outcome.Lines {
		body.WriteString(line + "\n")
	}
	for _, unknown := range outcome.Unknown {
		body.WriteString("not resolved: " + unknown + "\n")
	}
	body.WriteString(t.describe())
	return llm.ToolResult{CallID: call.ID, Content: body.String()}
}

// describe is the standing of each combatant, which the narration must not contradict.
func (t *combatTools) describe() string {
	combatants, ok := combatantsOf(t.state)
	if !ok {
		return "no encounter is open"
	}
	var b strings.Builder
	b.WriteString("standing:\n")
	for _, c := range combatants {
		hp, _ := intField(c, participantHPCurrentKey)
		maxHP, _ := intField(c, participantHPMaxKey)
		ac, _ := intField(c, participantACKey)
		state := "up"
		if hp <= 0 {
			state = "down"
		}
		fmt.Fprintf(&b, "- %v: hp %d/%d, ac %d, %s\n", c[participantNameKey], hp, maxHP, ac, state)
	}
	if waiting := t.waitingToAct(); len(waiting) > 0 {
		b.WriteString("still to act in this reply: " + strings.Join(waiting, ", ") + "\n")
	}
	return b.String()
}

// markActed records a turn taken. A participant who could not be found has no turn to record.
func (t *combatTools) markActed(name string) {
	name = t.resolveName(name)
	if strings.TrimSpace(name) == "" {
		return
	}
	t.acted[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
}

// waitingToAct lists the participants who are standing and have not taken their turn in this reply: the ones the DM
// still owes the table before it answers.
func (t *combatTools) waitingToAct() []string {
	combatants, ok := combatantsOf(t.state)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(combatants))
	for _, c := range combatants {
		name := fmt.Sprint(c[participantNameKey])
		hp, _ := intField(c, participantHPCurrentKey)
		if hp <= 0 {
			continue
		}
		if _, done := t.acted[strings.ToLower(strings.TrimSpace(name))]; done {
			continue
		}
		maxHP, _ := intField(c, participantHPMaxKey)
		out = append(out, fmt.Sprintf("%s (hp %d/%d)", name, hp, maxHP))
	}
	return out
}

// snapshot is the encounter state as JSON, for the debug log.
func (t *combatTools) snapshot() string {
	b, err := json.Marshal(t.state)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// resolveName accepts the words a model reaches for when it does not use the sheet's name for the player's character.
//
// A tool call is not prose: "player" or "pc" is what a model writes when it has not been shown the display name in the
// argument, and rejecting it would cost a round trip to learn something the server already knows.
func (t *combatTools) resolveName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	combatants, _ := combatantsOf(t.state)
	if _, found := findCombatant(combatants, trimmed); found != nil {
		return trimmed
	}
	lower := strings.ToLower(trimmed)
	for _, alias := range []string{"player", "pc", "jugador", "personaje del jugador", "the player", "you"} {
		if lower == alias {
			if strings.TrimSpace(t.pc) != "" {
				return t.pc
			}
		}
	}
	return trimmed
}
