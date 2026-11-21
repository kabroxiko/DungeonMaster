package gamesession

import (
	"log"
	"strings"
)

// dmEnvelopeCorrection is what the second attempt says. A prose reply to a live turn is usually a refusal or a model
// that forgot the envelope, and quoting the failure back at it is what turns the turn around: the rule alone was already
// in the prompt when it was ignored.
const dmEnvelopeCorrection = `Your previous message was not the DM reply: it was prose. Answer the same turn again as exactly one JSON object with a non-empty "narration", in character. An out-of-character refusal, an apology, a disclaimer or a question back to the player is not an available answer. If the action cannot be depicted, resolve it inside the fiction - the attempt fails, the scene turns away, an NPC refuses - and narrate that outcome.`

// usableDmEnvelope parses a reply and rejects the ones the table cannot act on.
func usableDmEnvelope(raw string) *dmEnvelope {
	env := parseDmEnvelope(raw)
	if env == nil || strings.TrimSpace(env.Narration) == "" {
		return nil
	}
	return env
}

// envelopeWithCorrection asks for the turn, and when the reply is not a usable envelope asks once more with the format
// restated and the offending answer named. The table is waiting on a turn; failing it on the first prose reply is how a
// refusal used to end a session, and asking again costs one call, not a lost game.
func envelopeWithCorrection(ask func(messages []interface{}) (string, error), messages []interface{}) (string, *dmEnvelope, error) {
	raw, err := ask(messages)
	if err != nil {
		return "", nil, err
	}
	if env := usableDmEnvelope(raw); env != nil {
		return raw, env, nil
	}

	log.Printf("dm generate: reply was not a usable envelope; asking once more with the format restated")
	corrected := append(append([]interface{}{}, messages...), map[string]interface{}{
		"role":    "user",
		"content": dmEnvelopeCorrection,
	})
	raw, err = ask(corrected)
	if err != nil {
		return "", nil, err
	}
	return raw, usableDmEnvelope(raw), nil
}

// dmCombatCorrection is what a turn that announced a fight without asking for a die is told. The DM decides what
// happens, but the server owns the dice: a fight declared and never rolled leaves the table with prose and no outcome,
// and asking the player to confirm the attack they already declared is the same failure wearing a question mark.
const dmCombatCorrection = `You announced that a fight is starting, but you asked the engine for nothing: no encounter was opened and no die was rolled. The dice belong to the server. Call open_encounter with the combatants and their initiative modifier, then roll every action you intend to narrate, and only then answer the same turn again as exactly one JSON object with a non-empty "narration". Do not ask the player to confirm an attack they already declared, do not ask which weapon when they named one or have only one on the sheet, and do not answer by explaining what would happen if they attacked.`
