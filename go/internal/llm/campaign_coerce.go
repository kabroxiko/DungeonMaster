package llm

import (
	"regexp"
	"sort"
	"strconv"
)

var digitKeyRe = regexp.MustCompile(`^\d+$`)

var stageAlternateKeys = map[string][]string{
	"factions":     {"faction", "factions_list", "relevant_factions"},
	"majorNPCs":    {"major_npcs", "npcs", "NPCs", "majorNpcs"},
	"keyLocations": {"locations", "key_locations", "places", "sites", "keyPlaces", "lugares_clave"},
}

// CoerceCampaignStageToArray mirrors server/routes/gameSession.js coerceCampaignStageToArray.
func CoerceCampaignStageToArray(stage string, parsed interface{}) []interface{} {
	if parsed == nil {
		return nil
	}
	p := parsed
	if s, ok := p.(string); ok {
		data, ok := ParseCampaignStageModelOutput(s)
		if !ok {
			return nil
		}
		p = data
	}
	if arr, ok := p.([]interface{}); ok {
		return filterNonNil(arr)
	}
	m, ok := p.(map[string]interface{})
	if !ok || m == nil {
		return nil
	}
	if top := arrayLikeObjectToArray(m); len(top) > 0 {
		if len(top) > 0 {
			if _, ok := top[0].(map[string]interface{}); ok {
				return filterNonNil(top)
			}
		}
	}
	if from := asObjectArray(m[stage]); len(from) > 0 {
		return filterNonNil(from)
	}
	for _, k := range stageAlternateKeys[stage] {
		if a := asObjectArray(m[k]); len(a) > 0 {
			return filterNonNil(a)
		}
	}
	var objectArrays [][]interface{}
	for _, v := range m {
		arr, ok := v.([]interface{})
		if !ok || len(arr) == 0 {
			continue
		}
		if _, ok := arr[0].(map[string]interface{}); ok {
			objectArrays = append(objectArrays, arr)
		}
	}
	if len(objectArrays) == 1 {
		return filterNonNil(objectArrays[0])
	}
	if len(objectArrays) == 0 {
		switch stage {
		case "keyLocations":
			if name, ok := m["name"].(string); ok && name != "" {
				if m["type"] != nil || m["significance"] != nil {
					return []interface{}{m}
				}
			}
		case "factions":
			if name, ok := m["name"].(string); ok && name != "" {
				if m["goal"] != nil || m["resources"] != nil || m["currentDisposition"] != nil {
					return []interface{}{m}
				}
			}
		case "majorNPCs":
			if name, ok := m["name"].(string); ok && name != "" {
				if m["role"] != nil || m["briefDescription"] != nil {
					return []interface{}{m}
				}
			}
		}
	}
	return nil
}

func filterNonNil(a []interface{}) []interface{} {
	var out []interface{}
	for _, x := range a {
		if x != nil {
			out = append(out, x)
		}
	}
	return out
}

func arrayLikeObjectToArray(obj map[string]interface{}) []interface{} {
	if obj == nil || len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		if !digitKeyRe.MatchString(k) {
			return nil
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, _ := strconv.Atoi(keys[i])
		aj, _ := strconv.Atoi(keys[j])
		return ai < aj
	})
	out := make([]interface{}, 0, len(keys))
	for _, k := range keys {
		out = append(out, obj[k])
	}
	return out
}

func asObjectArray(val interface{}) []interface{} {
	if val == nil {
		return nil
	}
	if arr, ok := val.([]interface{}); ok {
		return arr
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil
	}
	return arrayLikeObjectToArray(m)
}
