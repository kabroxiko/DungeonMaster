package gamesession

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/dice"
	"github.com/deckofdmthings/gmai/internal/llm"
	"github.com/deckofdmthings/gmai/internal/pc"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
)

// declaredAction is one mechanical action the Dungeon Master declares before the table hears about it.
//
// The split is deliberate: choosing what a creature does, and which stat applies, is a referee's judgement; rolling the
// die is not. A model asked to roll invents the number, and then does the arithmetic on it - which is how a session
// ended with "13 against armor class 14, the blow penetrates" and two points of damage on a miss.
type declaredAction struct {
	Actor    string
	Target   string
	Kind     string // attack, save or check
	Modifier int    // to-hit or save/check bonus
	Damage   string // dice notation, attacks only
	// AC and DC are what the action is resolved against when the stored encounter state does not already carry it.
	AC   int
	DC   int
	Note string
}

// combatOutcome is what the engine decided: the lines the DM must narrate, the state after the turn, and whether any
// hit points moved.
type combatOutcome struct {
	Lines     []string
	State     map[string]interface{}
	HPChanged bool
	// Unknown is set when an action could not be resolved at all, so the caller can tell the model to skip it rather
	// than narrate a result nobody rolled.
	Unknown []string
}

const (
	participantNameKey        = "displayName"
	participantHPCurrentKey   = "hp_current"
	participantHPMaxKey       = "hp_max"
	participantACKey          = "ac"
	participantAttackBonusKey = "attack_bonus"
	participantDamageKey      = "damage"
)

// combatantsOf reads the participant list out of an encounter state, copying each participant so the engine can change
// hit points without touching the document the state came from.
func combatantsOf(state map[string]interface{}) ([]map[string]interface{}, bool) {
	if state == nil {
		return nil, false
	}
	raw := toIfaceSlice(state["participants"])
	if len(raw) == 0 {
		return nil, false
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		copied := make(map[string]interface{}, len(m))
		for k, v := range m {
			copied[k] = v
		}
		out = append(out, copied)
	}
	return out, len(out) > 0
}

// findCombatant matches a name the way a table writes one: trimmed, and ignoring case.
func findCombatant(combatants []map[string]interface{}, name string) (int, map[string]interface{}) {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return -1, nil
	}
	for i, c := range combatants {
		if strings.ToLower(strings.TrimSpace(fmt.Sprint(c[participantNameKey]))) == want {
			return i, c
		}
	}
	return -1, nil
}

func intField(m map[string]interface{}, key string) (int, bool) {
	switch v := m[key].(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

// resolveCombatTurn rolls every declared action and returns what the table must be told.
//
// The encounter state is the record: hit points are read from it, written back to it, and its participants carry the
// armor class and attack numbers the DM settled on, so a foe's statistics do not drift between turns.
func resolveCombatTurn(actions []declaredAction, state map[string]interface{}, rng *rand.Rand) combatOutcome {
	combatants, ok := combatantsOf(state)
	if !ok {
		return combatOutcome{State: state}
	}
	out := combatOutcome{State: state}
	for _, action := range actions {
		actorIndex, actor := findCombatant(combatants, action.Actor)
		if actor == nil {
			out.Unknown = append(out.Unknown, fmt.Sprintf("%q is not in the encounter", action.Actor))
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(action.Kind))
		switch kind {
		case "attack":
			line, changed := resolveAttack(action, actorIndex, actor, combatants, rng, &out)
			if line != "" {
				out.Lines = append(out.Lines, line)
			}
			if changed {
				out.HPChanged = true
			}
		case "save", "check":
			out.Lines = append(out.Lines, resolveTest(action, rng))
		default:
			out.Unknown = append(out.Unknown, fmt.Sprintf("%q has an unknown action kind %q", action.Actor, action.Kind))
		}
	}
	out.State = withCombatants(state, combatants)
	return out
}

func resolveAttack(action declaredAction, actorIndex int, actor map[string]interface{}, combatants []map[string]interface{}, rng *rand.Rand, out *combatOutcome) (string, bool) {
	attacker := fmt.Sprint(actor[participantNameKey])
	_, target := findCombatant(combatants, action.Target)
	if target == nil {
		out.Unknown = append(out.Unknown, fmt.Sprintf("target %q is not in the encounter", action.Target))
		return "", false
	}
	modifier := action.Modifier
	if stored, ok := intField(actor, participantAttackBonusKey); ok && action.Modifier == 0 {
		modifier = stored
	} else if action.Modifier != 0 && !ok {
		// What the DM decides once is stored, so the same foe does not acquire a new bonus next turn.
		actor[participantAttackBonusKey] = action.Modifier
	}

	ac, acSource := action.AC, ""
	if stored, ok := intField(target, participantACKey); ok && stored > 0 {
		ac, acSource = stored, ""
	} else if action.AC > 0 {
		target[participantACKey] = action.AC
	} else {
		ac, acSource = 10, " (no armor class stored: 10 assumed)"
	}
	if ac <= 0 {
		ac = 10
	}

	toHit := dice.D20(modifier, rng)
	hit := toHit.Total >= ac
	switch toHit.Natural() {
	case 20:
		hit = true
	case 1:
		hit = false
	}
	targetName := fmt.Sprint(target[participantNameKey])

	if !hit {
		return fmt.Sprintf("%s attacks %s: %s vs AC %d -> miss%s", attacker, targetName, toHit.Detail(), ac, acSource), false
	}

	notation := strings.TrimSpace(action.Damage)
	if notation == "" {
		if stored, ok := target[participantDamageKey].(string); ok {
			notation = strings.TrimSpace(stored)
		}
	}
	if notation == "" {
		if stored, ok := intField(actor, participantDamageKey); ok && stored > 0 {
			notation = strconv.Itoa(stored)
		}
	}
	if notation != "" && action.Damage != "" {
		actor[participantDamageKey] = action.Damage
	}
	if notation == "" {
		notation = "1"
	}
	damage, err := dice.RollNotation(notation, rng)
	if err != nil {
		out.Unknown = append(out.Unknown, fmt.Sprintf("damage %q could not be rolled: %v", notation, err))
		return fmt.Sprintf("%s attacks %s: %s vs AC %d -> hit, damage could not be rolled", attacker, targetName, toHit.Detail(), ac), false
	}

	before, ok := intField(target, participantHPCurrentKey)
	if !ok {
		before, _ = intField(target, participantHPMaxKey)
	}
	after := before - damage.Total
	if after < 0 {
		after = 0
	}
	target[participantHPCurrentKey] = after
	if _, hasMax := intField(target, participantHPMaxKey); !hasMax {
		target[participantHPMaxKey] = before
	}
	return fmt.Sprintf("%s attacks %s: %s vs AC %d -> hit, damage %s, hp %d -> %d",
		attacker, targetName, toHit.Detail(), ac, damage.Detail(), before, after), after != before
}

func resolveTest(action declaredAction, rng *rand.Rand) string {
	who := strings.TrimSpace(action.Actor)
	if who == "" {
		who = "the acting creature"
	}
	roll := dice.D20(action.Modifier, rng)
	dc := action.DC
	if dc <= 0 {
		dc = 10
	}
	outcome := "failure"
	if roll.Total >= dc {
		outcome = "success"
	}
	switch roll.Natural() {
	case 20:
		outcome = "success"
	case 1:
		outcome = "failure"
	}
	label := strings.ToLower(strings.TrimSpace(action.Kind))
	if label == "" {
		label = "check"
	}
	return fmt.Sprintf("%s: %s %s vs DC %d -> %s", who, label, roll.Detail(), dc, outcome)
}

// withCombatants writes the resolved participants back into the encounter state, keeping any other field the state
// carried, and records each participant's standing so the narration cannot contradict it.
func withCombatants(state map[string]interface{}, combatants []map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range state {
		out[k] = v
	}
	list := make([]interface{}, 0, len(combatants))
	for _, c := range combatants {
		hp, ok := intField(c, participantHPCurrentKey)
		if ok {
			c["status"] = "up"
			if hp <= 0 {
				c["status"] = "down"
			}
		}
		list = append(list, c)
	}
	out["participants"] = list
	return out
}

// promptBlock is what the narration turn is given: the only dice of the turn, and the standing of each participant.
//
// It is written as an instruction rather than a data dump because the failure it repairs is narration contradicting the
// numbers: a foe described as unconscious while it still had three hit points.
func (o combatOutcome) promptBlock() string {
	var b strings.Builder
	b.WriteString("ENGINE RESULTS (rolled by the server for this turn).\n")
	b.WriteString("Every number you show in narration comes from here. Do not roll dice, do not invent a number, do not recompute a total.\n")
	if len(o.Lines) > 0 {
		b.WriteString("Actions, in order:\n")
		for _, line := range o.Lines {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	} else {
		b.WriteString("No action needed dice this turn: narrate the situation without a roll.\n")
	}
	if len(o.Unknown) > 0 {
		b.WriteString("Not resolved (do not narrate a result for these):\n")
		for _, line := range o.Unknown {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if combatants, ok := combatantsOf(o.State); ok {
		b.WriteString("Standing after this turn:\n")
		for _, c := range combatants {
			hp, _ := intField(c, participantHPCurrentKey)
			maxHP, _ := intField(c, participantHPMaxKey)
			state := "up and able to act (hit points above 0)"
			if hp <= 0 {
				state = "down (0 hit points)"
			}
			fmt.Fprintf(&b, "- %v: hp %d/%d, %s\n", c[participantNameKey], hp, maxHP, state)
		}
	}
	b.WriteString("A creature with hit points above 0 is standing and can act: never describe it as unconscious, dying or " +
		"collapsed. Only 0 hit points is down. Do not describe a blow that missed as landing, and do not narrate damage " +
		"for a miss.")
	return b.String()
}

// parseDeclaredActions reads the declaration JSON. Anything unusable is skipped rather than defaulted, because a rolled
// action the DM did not declare is a number nobody asked for.
func parseDeclaredActions(raw interface{}) []declaredAction {
	list := toIfaceSlice(raw)
	out := make([]declaredAction, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		action := declaredAction{
			Actor:  strings.TrimSpace(str(m["actor"])),
			Target: strings.TrimSpace(str(m["target"])),
			Kind:   strings.TrimSpace(str(m["kind"])),
			Damage: strings.TrimSpace(str(m["damage"])),
			Note:   strings.TrimSpace(str(m["note"])),
		}
		if action.Actor == "" {
			continue
		}
		action.Modifier = intFromValue(m["modifier"])
		action.AC = intFromValue(m["ac"])
		action.DC = intFromValue(m["dc"])
		out = append(out, action)
	}
	return out
}

func intFromValue(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "+"))); err == nil {
			return n
		}
	}
	return 0
}

// combatIsActive reports whether the stored encounter state describes a fight the engine should run: at least one
// participant other than this player's character is still standing.
func combatIsActive(state map[string]interface{}, pcName string) bool {
	combatants, ok := combatantsOf(state)
	if !ok {
		return false
	}
	pc := strings.ToLower(strings.TrimSpace(pcName))
	for _, c := range combatants {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(c[participantNameKey])))
		if pc != "" && name == pc {
			continue
		}
		if hp, has := intField(c, participantHPCurrentKey); has && hp > 0 {
			return true
		}
	}
	return false
}

// runCombatEngine declares this turn's actions with the model, rolls them on the server, and hands back the block the
// narration turn must obey plus the encounter state after the turn.
//
// Two calls, because the split has to be real: the declaration decides what happens and cannot know the outcome, and the
// narration describes an outcome it did not compute. A single call means the model writes the die and then explains it,
// which is exactly the behaviour this replaces.
func runCombatEngine(ctx context.Context, cfg *config.Config, gs map[string]interface{}, sheet map[string]interface{}, inbound []interface{}, gameID string) (string, map[string]interface{}, error) {
	state, _ := gs["encounterState"].(map[string]interface{})
	if state == nil {
		return "", nil, nil
	}
	pcName := pc.DisplayNameFromCharacterSheet(sheet)
	if !combatIsActive(state, pcName) {
		return "", nil, nil
	}

	skill := promptmgr.LoadPrompt("skills/combat_declare.md")
	if strings.TrimSpace(skill) == "" {
		return "", nil, nil
	}
	system := skill + "\n\nEncounter state (JSON):\n" + jsonText(state)
	if sheet != nil {
		system += "\n\nPlayer character sheet (JSON):\n" + jsonText(sheet)
	}
	msgs := []interface{}{map[string]interface{}{"role": "system", "content": system}}
	// The declaration needs the beat the table just described, not the whole campaign.
	tail := inbound
	if len(tail) > 6 {
		tail = tail[len(tail)-6:]
	}
	msgs = append(msgs, tail...)

	raw, err := llm.GenerateResponse(ctx, cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens":       500,
		"temperature":      0.2,
		"gameId":           gameID,
		"reasoning_effort": "low",
		"response_format":  map[string]interface{}{"type": "json_object"},
	})
	if err != nil {
		return "", nil, err
	}
	declared, ok := llm.ParseModelStructuredObject(raw)
	if !ok || declared == nil {
		return "", nil, nil
	}
	if str(declared["ask"]) != "" {
		// The table owes an answer before anything can be rolled.
		return "", nil, nil
	}
	actions := parseDeclaredActions(declared["actions"])
	initiative := parseDeclaredInitiative(declared["initiative"])
	if len(actions) == 0 && len(initiative) == 0 {
		return "", nil, nil
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	lines, scored := resolveInitiative(initiative, state, rng)
	if scored != nil {
		state = scored
	}
	outcome := resolveCombatTurn(actions, state, rng)
	if len(lines) > 0 {
		outcome.Lines = append(lines, outcome.Lines...)
	}
	if len(outcome.Lines) == 0 && len(outcome.Unknown) == 0 {
		return "", nil, nil
	}
	return outcome.promptBlock(), outcome.State, nil
}

// jsonText marshals a document for a prompt, without the JSON encoder's map ordering surprises.
func jsonText(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return "{}"
	}
	return string(b)
}

// insertSystemBlock adds a DM-only block directly after the leading system message, where it is read before the beat it
// governs and does not disturb the cache prefix of the blocks above it.
func insertSystemBlock(msgs []interface{}, block string) []interface{} {
	if strings.TrimSpace(block) == "" || len(msgs) == 0 {
		return msgs
	}
	out := make([]interface{}, 0, len(msgs)+1)
	out = append(out, msgs[0])
	out = append(out, map[string]interface{}{"role": "system", "content": block})
	return append(out, msgs[1:]...)
}

// declaredInitiative is one participant's place in the order, declared with the modifier that belongs to it.
type declaredInitiative struct {
	Actor    string
	Modifier int
}

func parseDeclaredInitiative(raw interface{}) []declaredInitiative {
	list := toIfaceSlice(raw)
	out := make([]declaredInitiative, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		actor := strings.TrimSpace(str(m["actor"]))
		if actor == "" {
			continue
		}
		out = append(out, declaredInitiative{Actor: actor, Modifier: intFromValue(m["modifier"])})
	}
	return out
}

// resolveInitiative rolls the order and stores it, so the same fight does not reshuffle every turn. It returns the lines
// the DM must narrate and the state carrying the order; a nil state means nothing was rolled.
func resolveInitiative(declared []declaredInitiative, state map[string]interface{}, rng *rand.Rand) ([]string, map[string]interface{}) {
	if len(declared) == 0 || state == nil {
		return nil, nil
	}
	combatants, ok := combatantsOf(state)
	if !ok {
		return nil, nil
	}
	// An order already rolled is the order the table is playing in.
	if existing := toIfaceSlice(state["initiative"]); len(existing) > 0 {
		return nil, nil
	}
	lines := make([]string, 0, len(declared))
	order := make([]interface{}, 0, len(declared))
	for _, entry := range declared {
		roll := dice.D20(entry.Modifier, rng)
		lines = append(lines, fmt.Sprintf("initiative %s: %s", entry.Actor, roll.Detail()))
		order = append(order, map[string]interface{}{
			"displayName": entry.Actor,
			"score":       roll.Total,
		})
	}
	if len(order) == 0 {
		return nil, nil
	}
	// Highest first: the table reads the order in one line, not by comparing numbers.
	for i := 1; i < len(order); i++ {
		for j := i; j > 0; j-- {
			a, _ := intField(order[j].(map[string]interface{}), "score")
			b, _ := intField(order[j-1].(map[string]interface{}), "score")
			if a > b {
				order[j], order[j-1] = order[j-1], order[j]
				continue
			}
			break
		}
	}

	updated := map[string]interface{}{}
	for k, v := range state {
		updated[k] = v
	}
	updated["initiative"] = order
	updated["participants"] = withCombatants(state, combatants)["participants"]

	names := make([]string, 0, len(order))
	for _, entry := range order {
		names = append(names, fmt.Sprint(entry.(map[string]interface{})["displayName"]))
	}
	lines = append(lines, "turn order: "+strings.Join(names, " -> "))
	return lines, updated
}
