package pc

import "strings"

// StripModelGeneratedNames removes LLM-invented display labels; server assigns name after parse.
func StripModelGeneratedNames(pc map[string]interface{}) {
	if pc == nil {
		return
	}
	delete(pc, "name")
	delete(pc, "characterName")
	if id, ok := pc["identity"].(map[string]interface{}); ok && id != nil {
		delete(id, "name")
	}
}

// SyncDisplayNameFields writes the roster display name to every field the client reads.
func SyncDisplayNameFields(pc map[string]interface{}, displayName string) {
	full := strings.TrimSpace(displayName)
	if full == "" || pc == nil {
		return
	}
	pc["name"] = full
	pc["characterName"] = full
	id, ok := pc["identity"].(map[string]interface{})
	if !ok || id == nil {
		id = map[string]interface{}{}
		pc["identity"] = id
	}
	id["name"] = full
}
