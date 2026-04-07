package persist

import (
	"strings"

	"github.com/deckofdmthings/gmai/internal/validate"
)

// MergePlayerCharacters mirrors playerCharacterHelpers.js mergePlayerCharacters.
func MergePlayerCharacters(existingSetup, incomingSetup map[string]interface{}, language string) map[string]interface{} {
	lang := language
	if strings.TrimSpace(lang) == "" {
		if incomingSetup != nil {
			if l, ok := incomingSetup["language"].(string); ok && strings.TrimSpace(l) != "" {
				lang = strings.TrimSpace(l)
			}
		}
		if strings.TrimSpace(lang) == "" && existingSetup != nil {
			if l, ok := existingSetup["language"].(string); ok {
				lang = strings.TrimSpace(l)
			}
		}
	}
	if strings.TrimSpace(lang) == "" {
		lang = "English"
	}
	base := map[string]interface{}{}
	if existingSetup != nil {
		for k, v := range existingSetup {
			base[k] = v
		}
	}
	if incomingSetup == nil {
		return base
	}
	incMap, _ := incomingSetup["playerCharacters"].(map[string]interface{})
	if incMap == nil {
		return base
	}
	merged := map[string]interface{}{}
	if cur, ok := base["playerCharacters"].(map[string]interface{}); ok {
		for k, v := range cur {
			merged[k] = v
		}
	}
	for k, v := range incMap {
		if m, ok := v.(map[string]interface{}); ok {
			merged[k] = validate.EnsurePlayerCharacterSheetDefaults(m, lang)
		}
	}
	base["playerCharacters"] = merged
	return base
}
