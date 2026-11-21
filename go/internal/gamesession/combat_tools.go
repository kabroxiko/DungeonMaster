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
				"for each. Returns the turn order. Call it on the turn the player declares an attack: a fight that is only " +
				"announced never starts, and the table gets no turn.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"participants": participantsSchema(),
					"initiative":   initiativeSchema(),
				},
			},
		},
		{
			Name: "roll",
			Description: "Resolve this round. Pass an `actions` list holding every action of the round - the player's and " +
				"the answer of each foe still standing - and the engine rolls them in initiative order, applies damage and " +
				"reports who is up. A single action is also accepted when only one creature acts, but a foe with hit points " +
				"left always takes its turn, and its attack belongs to this round, not the next one.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"actions": map[string]interface{}{
						"type":        "array",
						"description": "Every action of this round. Preferred over the single-action fields below.",
						"items":       actionSchema(),
					},
					"participants": participantsSchema(),
					"initiative":   initiativeSchema(),
					"actor":        map[string]interface{}{"type": "string", "description": "displayName of the creature acting."},
					"target":       map[string]interface{}{"type": "string", "description": "displayName of whoever it lands on; omit for a save or check."},
					"kind":         map[string]interface{}{"type": "string", "enum": []interface{}{"attack", "heal", "save", "check"}},
					"modifier":     map[string]interface{}{"type": "integer", "description": "The whole number added to the d20."},
					"damage":       map[string]interface{}{"type": "string", "description": "Damage dice such as 1d8+3; attacks only."},
					"dc":           map[string]interface{}{"type": "integer", "description": "The number a save or check resolves against."},
				},
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

// roll resolves one action, or a whole round when it is given the round's actions.
//
// A round is every participant's turn: the player's action and the answer of each foe still standing. Resolving them in
// one call is what keeps a counterattack in the turn it belongs to - a model that rolls the player's swing and stops has
// answered the table with half a round, and the reply is checked for that.
func (t *combatTools) roll(call llm.ToolCall) llm.ToolResult {
	actions := t.actionsFromCall(call)
	if len(actions) == 0 {
		return llm.ToolResult{CallID: call.ID, Content: "error: give an action (or an actions list) with an actor and a kind"}
	}
	// A model opening the fight in the same call it rolls is doing the natural thing; refusing its participants and
	// answering "not in the encounter" cost a whole round trip and read as the engine failing.
	if declared := parseDeclaredCombatants(call.Arguments["participants"]); len(declared) > 0 {
		t.state = mergeDeclaredCombatants(t.state, declared, t.sheet, t.pc)
		t.used = true
	}
	if initiative := parseDeclaredInitiative(call.Arguments["initiative"]); len(initiative) > 0 {
		if _, scored := resolveInitiative(initiative, t.state, t.rng); scored != nil {
			t.state = scored
			t.used = true
		}
	}
	t.ensurePlayer() // a potion or a fall happens with no fight open
	ordered := t.orderByInitiative(actions)
	outcome := resolveCombatTurn(ordered, t.state, t.rng)
	t.state = outcome.State
	t.used = true
	for _, line := range outcome.Lines {
		// A line begins with the actor's name, which is who just took a turn.
		for _, action := range ordered {
			if strings.HasPrefix(line, action.Actor) {
				t.markActed(action.Actor)
				break
			}
		}
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

// actionsFromCall reads either the single-action arguments or an "actions" list.
func (t *combatTools) actionsFromCall(call llm.ToolCall) []declaredAction {
	if list := toIfaceSlice(call.Arguments["actions"]); len(list) > 0 {
		out := make([]declaredAction, 0, len(list))
		for _, item := range list {
			m, ok := item.(map[string]interface{})
			if !ok || m == nil {
				continue
			}
			if action := t.actionFromMap(m); action.Actor != "" {
				out = append(out, action)
			}
		}
		return out
	}
	if action := t.actionFromMap(call.Arguments); action.Actor != "" {
		return []declaredAction{action}
	}
	return nil
}

func (t *combatTools) actionFromMap(m map[string]interface{}) declaredAction {
	return declaredAction{
		Actor:    t.resolveName(str(m["actor"])),
		Target:   t.resolveName(str(m["target"])),
		Kind:     strings.TrimSpace(str(m["kind"])),
		Damage:   strings.TrimSpace(str(m["damage"])),
		Modifier: intFromValue(m["modifier"]),
		DC:       intFromValue(m["dc"]),
		AC:       intFromValue(m["ac"]),
	}
}

// orderByInitiative resolves the round in the order the table is playing in, whatever order the actions arrived in.
func (t *combatTools) orderByInitiative(actions []declaredAction) []declaredAction {
	scores := map[string]int{}
	for _, entry := range toIfaceSlice(t.state["initiative"]) {
		m, _ := entry.(map[string]interface{})
		if m == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(m["displayName"])))
		score, _ := intField(m, "score")
		scores[name] = score
	}
	if len(scores) == 0 {
		return actions
	}
	out := append([]declaredAction(nil), actions...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a := scores[strings.ToLower(strings.TrimSpace(out[j].Actor))]
			b := scores[strings.ToLower(strings.TrimSpace(out[j-1].Actor))]
			if a > b {
				out[j], out[j-1] = out[j-1], out[j]
				continue
			}
			break
		}
	}
	return out
}

// describe is the standing of each combatant, which the narration must not contradict, plus the order the engine rolled.
//
// The order is not bookkeeping: a player who declared the attack and reads that someone else moved first needs the
// numbers that decided it, or the fight looks broken.
func (t *combatTools) describe() string {
	combatants, ok := combatantsOf(t.state)
	if !ok {
		return "no encounter is open"
	}
	var b strings.Builder
	if order := t.initiativeOrder(); len(order) > 0 {
		b.WriteString("turn order: " + strings.Join(order, " -> ") + "\n")
	}
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

// usedAny reports whether the DM asked the engine for anything this turn. A turn that announced a fight and asked for
// nothing is a fight with no dice in it.
func (t *combatTools) usedAny() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.used
}

// needsMore reports whether the reply is stopping while a foe is still owed a turn. The fight being over - no foe left
// standing - means nobody is owed anything: the player's own turn does not need chasing after that.
func (t *combatTools) needsMore() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	combatants, ok := combatantsOf(t.state)
	if !ok {
		return false
	}
	pc := strings.ToLower(strings.TrimSpace(t.pc))
	foeStanding := false
	for _, c := range combatants {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(c[participantNameKey])))
		if pc != "" && name == pc {
			continue
		}
		if hp, has := intField(c, participantHPCurrentKey); has && hp > 0 {
			foeStanding = true
			break
		}
	}
	if !foeStanding {
		return false
	}
	return len(t.waitingToAct()) > 0
}

// owed is who still has to act, for the correction the server sends back.
func (t *combatTools) owed() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.waitingToAct()
}

// actionSchema is one declared action, shared by the round list and the single-action fields.
func actionSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"actor":    map[string]interface{}{"type": "string", "description": "displayName of the creature acting."},
			"target":   map[string]interface{}{"type": "string", "description": "displayName of whoever it lands on; omit for a save or check."},
			"kind":     map[string]interface{}{"type": "string", "enum": []interface{}{"attack", "heal", "save", "check"}},
			"modifier": map[string]interface{}{"type": "integer", "description": "The whole number added to the d20."},
			"damage":   map[string]interface{}{"type": "string", "description": "Damage dice such as 1d8+3; attacks only."},
			"dc":       map[string]interface{}{"type": "integer", "description": "The number a save or check resolves against."},
		},
		"required": []interface{}{"actor", "kind"},
	}
}

// ensurePlayer puts the player's own character in the encounter when there is none, so damage and healing that happen
// outside a fight still reach the engine - and from there the sheet.
func (t *combatTools) ensurePlayer() {
	if t.sheet == nil || strings.TrimSpace(t.pc) == "" {
		return
	}
	if combatants, ok := combatantsOf(t.state); ok && len(combatants) > 0 {
		return
	}
	entry := map[string]interface{}{participantNameKey: t.pc, "kind": "pc"}
	maxHP := 0
	if v, ok := intField(t.sheet, "max_hp"); ok && v > 0 {
		maxHP = v
	}
	current := maxHP
	if v, ok := intField(t.sheet, sheetHPKey); ok {
		current = v
	}
	if maxHP > 0 {
		entry[participantHPMaxKey] = maxHP
		entry[participantHPCurrentKey] = current
	}
	if ac, ok := intField(t.sheet, "ac"); ok && ac > 0 {
		entry[participantACKey] = ac
	}
	t.state = withCombatants(t.state, []map[string]interface{}{entry})
}

// participantsSchema is the combatant list, offered by both tools: a model that opens the fight while it rolls should
// not have to know which call is supposed to carry them.
func participantsSchema() map[string]interface{} {
	return map[string]interface{}{
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
	}
}

func initiativeSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "One entry per combatant, with the modifier added to the d20. Rolled once; an existing order is kept.",
		"items": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"actor":    map[string]interface{}{"type": "string"},
				"modifier": map[string]interface{}{"type": "integer"},
			},
			"required": []interface{}{"actor"},
		},
	}
}

// initiativeOrder reads the stored order as "name (roll)" pairs, highest first.
func (t *combatTools) initiativeOrder() []string {
	entries := toIfaceSlice(t.state["initiative"])
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		m, ok := entry.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		score, _ := intField(m, "score")
		out = append(out, fmt.Sprintf("%v (%d)", m["displayName"], score))
	}
	return out
}
