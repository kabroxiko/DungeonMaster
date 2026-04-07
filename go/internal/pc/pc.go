package pc

import (
	"strings"
)

// CharacterForUser returns gameSetup.playerCharacters[uid].
func CharacterForUser(gameSetup map[string]interface{}, userIDStr string) map[string]interface{} {
	if gameSetup == nil || userIDStr == "" {
		return nil
	}
	m, _ := gameSetup["playerCharacters"].(map[string]interface{})
	if m == nil {
		return nil
	}
	if c, ok := m[userIDStr].(map[string]interface{}); ok {
		return c
	}
	// case-insensitive key
	low := strings.ToLower(userIDStr)
	for k, v := range m {
		if strings.ToLower(k) == low {
			if c, ok := v.(map[string]interface{}); ok {
				return c
			}
		}
	}
	return nil
}

// DisplayNameFromCharacterSheet returns display name for prompts.
func DisplayNameFromCharacterSheet(c map[string]interface{}) string {
	if c == nil {
		return "Adventurer"
	}
	if id, ok := c["identity"].(map[string]interface{}); ok {
		if n, ok := id["name"].(string); ok && strings.TrimSpace(n) != "" {
			return strings.TrimSpace(n)
		}
	}
	if n, ok := c["characterName"].(string); ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	if n, ok := c["name"].(string); ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	return "Adventurer"
}

// CharacterDisplayNameForUser convenience.
func CharacterDisplayNameForUser(gameSetup map[string]interface{}, userIDStr string) string {
	return DisplayNameFromCharacterSheet(CharacterForUser(gameSetup, userIDStr))
}
