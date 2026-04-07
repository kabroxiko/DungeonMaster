package campaignspec

import (
	"strings"
)

var dmOnlyKeys = map[string]struct{}{
	"dmHiddenAdventureObjective": {},
	"openingSceneFrame":          {},
	"creativeSeed":               {},
}

// RedactCampaignSpecForClient removes DM-only keys.
func RedactCampaignSpecForClient(spec map[string]interface{}) map[string]interface{} {
	if spec == nil {
		return nil
	}
	out := map[string]interface{}{}
	for k, v := range spec {
		if _, skip := dmOnlyKeys[k]; skip {
			continue
		}
		out[k] = v
	}
	return out
}

// HasSubstantiveCampaignSpec mirrors campaignSpecReady.js.
func HasSubstantiveCampaignSpec(spec map[string]interface{}) bool {
	if spec == nil {
		return false
	}
	for k, v := range spec {
		if _, skip := dmOnlyKeys[k]; skip {
			continue
		}
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return true
			}
		case []interface{}:
			if len(t) > 0 {
				return true
			}
		case map[string]interface{}:
			if len(t) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// OpeningSceneFrameIsUsable checks directive length.
func OpeningSceneFrameIsUsable(obj map[string]interface{}) bool {
	if obj == nil {
		return false
	}
	d, _ := obj["directive"].(string)
	return strings.TrimSpace(d) != "" && len(strings.TrimSpace(d)) >= 60
}

// CreativeSeedIsUsable mirrors JS.
func CreativeSeedIsUsable(obj map[string]interface{}) bool {
	if obj == nil {
		return false
	}
	mood, _ := obj["titleMood"].(string)
	if strings.TrimSpace(mood) == "" || len(strings.TrimSpace(mood)) < 4 {
		return false
	}
	prefer, _ := obj["preferAngles"].([]interface{})
	avoid, _ := obj["avoidRepeatedFantasyTropesThisRun"].([]interface{})
	pn := 0
	for _, x := range prefer {
		if strings.TrimSpace(toString(x)) != "" {
			pn++
		}
	}
	an := 0
	for _, x := range avoid {
		if strings.TrimSpace(toString(x)) != "" {
			an++
		}
	}
	return pn >= 1 && an >= 2
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// MergeCampaignSpecPreservingDmSecrets merges incoming with existing DM-only preservation.
func MergeCampaignSpecPreservingDmSecrets(existing, incoming map[string]interface{}) map[string]interface{} {
	if incoming == nil {
		return incoming
	}
	next := map[string]interface{}{}
	for k, v := range incoming {
		next[k] = v
	}
	for _, k := range []string{"dmHiddenAdventureObjective", "openingSceneFrame", "creativeSeed"} {
		prev := mapVal(existing, k)
		inc := mapVal(incoming, k)
		switch k {
		case "openingSceneFrame":
			po, _ := prev.(map[string]interface{})
			io, _ := inc.(map[string]interface{})
			if OpeningSceneFrameIsUsable(po) && !OpeningSceneFrameIsUsable(io) {
				next[k] = po
			}
		case "creativeSeed":
			po, _ := prev.(map[string]interface{})
			io, _ := inc.(map[string]interface{})
			if CreativeSeedIsUsable(po) && !CreativeSeedIsUsable(io) {
				next[k] = po
			}
		default:
			ps, pOk := prev.(string)
			is, iOk := inc.(string)
			if pOk && strings.TrimSpace(ps) != "" && (!iOk || strings.TrimSpace(is) == "") {
				next[k] = prev
			}
		}
	}
	return next
}

func mapVal(m map[string]interface{}, k string) interface{} {
	if m == nil {
		return nil
	}
	return m[k]
}

// ResolveOpeningMandateFromCampaign returns id + directive or nil.
func ResolveOpeningMandateFromCampaign(spec map[string]interface{}) (id, directive string, ok bool) {
	if spec == nil {
		return "", "", false
	}
	raw, _ := spec["openingSceneFrame"].(map[string]interface{})
	if !OpeningSceneFrameIsUsable(raw) {
		return "", "", false
	}
	d := strings.TrimSpace(toString(raw["directive"]))
	idStr := strings.TrimSpace(toString(raw["id"]))
	if idStr == "" {
		idStr = "campaign_opening"
	}
	// sanitize id like JS
	var b strings.Builder
	for _, r := range idStr {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	idStr = b.String()
	if idStr == "" {
		idStr = "campaign_opening"
	}
	if len(idStr) > 48 {
		idStr = idStr[:48]
	}
	return idStr, d[:min(len(d), 4000)], true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
