package gamesession

import (
	"fmt"
	"strings"

	"github.com/deckofdmthings/gmai/internal/pc"
)

// sheetHPKey is the current hit points on a player character's sheet. The encounter state is where a fight is tracked,
// but the sheet is what the table keeps: without this a character came out of every fight at full health, because the
// damage lived only in an encounter that ended.
const sheetHPKey = "hp_current"

// syncPartySheets writes the hit points the engine settled on into the stored character sheets.
//
// It matches by the same display name the encounter state uses, so a party of several characters each carry their own
// damage, and it writes only what changed. A sheet's maximum is left alone unless it is missing: the engine applies
// damage, not character creation.
func syncPartySheets(gameSetup map[string]interface{}, state map[string]interface{}, userID string) bool {
	if gameSetup == nil || state == nil {
		return false
	}
	sheets, _ := gameSetup["playerCharacters"].(map[string]interface{})
	if len(sheets) == 0 {
		return false
	}
	participants, ok := combatantsOf(state)
	if !ok {
		return false
	}
	changed := false
	// The player's own row is marked in the encounter state, so it can be written without relying on the two names
	// agreeing: a sheet whose display name was edited still carries the damage.
	ownSheet := pc.CharacterForUser(gameSetup, userID)
	for _, participant := range participants {
		name := strings.TrimSpace(fmt.Sprint(participant[participantNameKey]))
		if name == "" {
			continue
		}
		hp, hasHP := intField(participant, participantHPCurrentKey)
		if !hasHP {
			continue
		}
		if ownSheet != nil && str(participant["kind"]) == "pc" {
			if current, ok := intField(ownSheet, sheetHPKey); !ok || current != hp {
				ownSheet[sheetHPKey] = hp
				if _, hasMax := intField(ownSheet, "max_hp"); !hasMax {
					if maxHP, ok := intField(participant, participantHPMaxKey); ok && maxHP > 0 {
						ownSheet["max_hp"] = maxHP
					}
				}
				changed = true
			}
		}
		for _, raw := range sheets {
			sheet, ok := raw.(map[string]interface{})
			if !ok || sheet == nil {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(pc.DisplayNameFromCharacterSheet(sheet)), name) {
				continue
			}
			if current, ok := intField(sheet, sheetHPKey); ok && current == hp {
				continue
			}
			sheet[sheetHPKey] = hp
			if _, hasMax := intField(sheet, "max_hp"); !hasMax {
				if maxHP, ok := intField(participant, participantHPMaxKey); ok && maxHP > 0 {
					sheet["max_hp"] = maxHP
				}
			}
			changed = true
		}
	}
	return changed
}

// encounterClaimsConflict reports whether the state the DM wrote down contradicts the engine about who is standing.
//
// The engine's state replaces the DM's before it is stored, so the record stays true; the narration does not, and a
// prose that buries a foe the engine has at 9 hit points is the lie the table actually reads. Naming the contradiction
// back to the model is the only part of this the server can settle on its own.
func encounterClaimsConflict(claimed, engine map[string]interface{}) (string, bool) {
	engineRows, ok := combatantsOf(engine)
	if !ok {
		return "", false
	}
	claimedRows, ok := combatantsOf(claimed)
	if !ok {
		return "", false
	}
	for _, row := range claimedRows {
		name := strings.TrimSpace(fmt.Sprint(row[participantNameKey]))
		_, actual := findCombatant(engineRows, name)
		if actual == nil {
			continue
		}
		claimedHP, hasClaimed := intField(row, participantHPCurrentKey)
		actualHP, hasActual := intField(actual, participantHPCurrentKey)
		if !hasClaimed || !hasActual {
			continue
		}
		if (claimedHP <= 0) != (actualHP <= 0) {
			state := "standing"
			if actualHP <= 0 {
				state = "down"
			}
			return fmt.Sprintf("%s is at %d hit points and %s in the engine's record, not what your reply says", name, actualHP, state), true
		}
	}
	return "", false
}

// standingSummary is the record in one line, for the correction the DM is sent.
func standingSummary(state map[string]interface{}) string {
	combatants, ok := combatantsOf(state)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(combatants))
	for _, c := range combatants {
		hp, _ := intField(c, participantHPCurrentKey)
		maxHP, _ := intField(c, participantHPMaxKey)
		state := "up"
		if hp <= 0 {
			state = "down"
		}
		parts = append(parts, fmt.Sprintf("%v %d/%d (%s)", c[participantNameKey], hp, maxHP, state))
	}
	return strings.Join(parts, "; ")
}
