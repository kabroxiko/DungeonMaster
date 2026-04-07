package party

import (
	"github.com/deckofdmthings/gmai/internal/campaignspec"
)

// AdventureHasBegun mirrors partyLobbyState.js adventureHasBegun.
func AdventureHasBegun(doc map[string]interface{}) bool {
	if doc == nil {
		return false
	}
	if campaignspec.HasSubstantiveCampaignSpec(toMap(doc["campaignSpec"])) {
		return true
	}
	conv, _ := doc["conversation"].([]interface{})
	for _, raw := range conv {
		m, _ := raw.(map[string]interface{})
		if m == nil {
			continue
		}
		if r, _ := m["role"].(string); r == "assistant" {
			return true
		}
	}
	return false
}
