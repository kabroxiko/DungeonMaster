package party

import (
	"strings"

	"github.com/deckofdmthings/gmai/internal/validate"
)

// GameSetupLanguage returns language from gameSetup.
func GameSetupLanguage(doc map[string]interface{}) string {
	gs, _ := doc["gameSetup"].(map[string]interface{})
	if gs == nil {
		return "English"
	}
	if l, ok := gs["language"].(string); ok && strings.TrimSpace(l) != "" {
		return strings.TrimSpace(l)
	}
	return "English"
}

// ResolvePlayerCharacterSheet finds sheet by user id with case-insensitive key match.
func ResolvePlayerCharacterSheet(pcMap map[string]interface{}, userIDStr string) map[string]interface{} {
	if pcMap == nil || userIDStr == "" {
		return nil
	}
	uid := strings.TrimSpace(userIDStr)
	if c, ok := pcMap[uid].(map[string]interface{}); ok {
		return c
	}
	low := strings.ToLower(uid)
	for k, v := range pcMap {
		if strings.ToLower(strings.TrimSpace(k)) == low {
			if c, ok := v.(map[string]interface{}); ok {
				return c
			}
		}
	}
	return nil
}

// MemberHasValidSheetForUserId mirrors partyLobbyState.js.
func MemberHasValidSheetForUserId(doc map[string]interface{}, userIDStr string) bool {
	gs, _ := doc["gameSetup"].(map[string]interface{})
	if gs == nil {
		return false
	}
	pcMap, _ := gs["playerCharacters"].(map[string]interface{})
	if pcMap == nil {
		return false
	}
	sheet := ResolvePlayerCharacterSheet(pcMap, userIDStr)
	return validate.SheetLooksValid(sheet, GameSetupLanguage(doc))
}

// AllMembersHaveValidSheets checks every canonical member.
func AllMembersHaveValidSheets(doc map[string]interface{}) bool {
	ids := CanonicalMemberIDStrings(doc)
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if !MemberHasValidSheetForUserId(doc, id) {
			return false
		}
	}
	return true
}

// AllMembersReady checks readyUserIds covers all members.
func AllMembersReady(party map[string]interface{}, doc map[string]interface{}) bool {
	ids := CanonicalMemberIDStrings(doc)
	if len(ids) == 0 {
		return false
	}
	readyRaw, _ := party["readyUserIds"].([]interface{})
	ready := map[string]struct{}{}
	for _, x := range readyRaw {
		s := NormalizeUserIDString(x)
		if s != "" {
			ready[s] = struct{}{}
		}
	}
	for _, id := range ids {
		n := NormalizeUserIDString(id)
		if _, ok := ready[n]; !ok {
			return false
		}
	}
	return true
}
