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
func syncPartySheets(gameSetup map[string]interface{}, state map[string]interface{}) bool {
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
	for _, participant := range participants {
		name := strings.TrimSpace(fmt.Sprint(participant[participantNameKey]))
		if name == "" {
			continue
		}
		hp, hasHP := intField(participant, participantHPCurrentKey)
		if !hasHP {
			continue
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
