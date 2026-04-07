package validate

import (
	"strings"
	"unicode"

	"github.com/deckofdmthings/gmai/internal/pc"
)

// NormalizeNameKeyForReserved is the exported form of normalizeNameKey (reserved-name sets, Iron Arachne).
func NormalizeNameKeyForReserved(s string) string {
	return normalizeNameKey(s)
}

func normalizeNameKey(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			lastSpace = true
			continue
		}
		if lastSpace && b.Len() > 0 {
			b.WriteRune(' ')
		}
		lastSpace = false
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.TrimSpace(b.String())
}

// ValidateDistinctEntityNames mirrors validateEntityNameUniqueness.js.
func ValidateDistinctEntityNames(gameSetup, campaignSpec, encounterState map[string]interface{}) (ok bool, code, msg string) {
	if gameSetup == nil {
		gameSetup = map[string]interface{}{}
	}
	pcMap, _ := gameSetup["playerCharacters"].(map[string]interface{})
	if pcMap == nil {
		pcMap = map[string]interface{}{}
	}
	var pcNames []string
	for _, v := range pcMap {
		if m, ok := v.(map[string]interface{}); ok {
			pcNames = append(pcNames, normalizeNameKey(pc.DisplayNameFromCharacterSheet(m)))
		}
	}
	if dup := firstDup(pcNames); dup != "" {
		return false, "ENTITY_NAME_DUPLICATE_PC", "Two player characters share the same normalized name."
	}
	var npcNames []string
	if campaignSpec != nil {
		if arr, ok := campaignSpec["majorNPCs"].([]interface{}); ok {
			for _, x := range arr {
				row, _ := x.(map[string]interface{})
				if row != nil {
					npcNames = append(npcNames, normalizeNameKey(str(row["name"])))
				}
			}
		}
	}
	if dup := firstDup(npcNames); dup != "" {
		return false, "ENTITY_NAME_DUPLICATE_MAJOR_NPC", "Two campaign NPCs share the same name."
	}
	pcSet := map[string]struct{}{}
	for _, n := range pcNames {
		if n != "" {
			pcSet[n] = struct{}{}
		}
	}
	for _, n := range npcNames {
		if n != "" {
			if _, ok := pcSet[n]; ok {
				return false, "ENTITY_NAME_PC_COLLIDES_WITH_MAJOR_NPC", "Player character name collides with a campaign NPC."
			}
		}
	}
	if encounterState != nil {
		part, _ := encounterState["participants"].([]interface{})
		var labels []string
		for _, p := range part {
			row, _ := p.(map[string]interface{})
			if row == nil {
				continue
			}
			l := str(row["name"])
			if l == "" {
				l = str(row["displayName"])
			}
			if l == "" {
				l = str(row["label"])
			}
			labels = append(labels, normalizeNameKey(l))
		}
		if dup := firstDup(labels); dup != "" {
			return false, "ENTITY_NAME_DUPLICATE_ENCOUNTER_PARTICIPANT", "Two combat participants share the same label."
		}
	}
	return true, "", ""
}

func firstDup(keys []string) string {
	seen := map[string]struct{}{}
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			return k
		}
		seen[k] = struct{}{}
	}
	return ""
}
