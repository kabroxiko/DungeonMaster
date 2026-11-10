package campaignspec

import (
	"fmt"
	"reflect"
	"strings"
)

const worldMapMaxRegions = 24
const worldMapMaxSiteLinks = 40

// NormalizeWorldMap trims and caps campaignSpec.worldMap for persistence and DM injection.
// Returns nil if there are no valid regions.
func NormalizeWorldMap(in map[string]interface{}) map[string]interface{} {
	if in == nil || len(in) == 0 {
		return nil
	}
	regsRaw := neighborSliceAny(in["regions"])
	if len(regsRaw) == 0 {
		return nil
	}
	seenID := map[string]struct{}{}
	var regions []interface{}
	for _, raw := range regsRaw {
		if len(regions) >= worldMapMaxRegions {
			break
		}
		m, ok := raw.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		id := stringFrom(m["id"])
		name := stringFrom(m["name"])
		if id == "" || name == "" {
			continue
		}
		if len(id) > 48 {
			id = id[:48]
		}
		if _, dup := seenID[id]; dup {
			continue
		}
		seenID[id] = struct{}{}
		terrain := stringFrom(m["terrain"])
		if len(terrain) > 64 {
			terrain = terrain[:64]
		}
		nb := neighborIDsList(m["neighborIds"])
		out := map[string]interface{}{
			"id":          id,
			"name":        name,
			"neighborIds": nb,
		}
		if terrain != "" {
			out["terrain"] = terrain
		}
		regions = append(regions, out)
	}
	if len(regions) == 0 {
		return nil
	}
	validIDs := map[string]struct{}{}
	for _, r := range regions {
		rm, _ := r.(map[string]interface{})
		if rm == nil {
			continue
		}
		validIDs[stringFrom(rm["id"])] = struct{}{}
	}
	// Second pass: filter neighborIds to known ids only (symmetry not required — prompts describe as undirected travel).
	for _, r := range regions {
		rm, _ := r.(map[string]interface{})
		if rm == nil {
			continue
		}
		var clean []interface{}
		seen := map[string]struct{}{}
		selfID := stringFrom(rm["id"])
		for _, x := range neighborSliceAny(rm["neighborIds"]) {
			s := stringFrom(x)
			if s == "" || s == selfID {
				continue
			}
			if _, ok := validIDs[s]; !ok {
				continue
			}
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			clean = append(clean, s)
			if len(clean) >= 12 {
				break
			}
		}
		rm["neighborIds"] = clean
	}
	linksRaw := neighborSliceAny(in["siteLinks"])
	var siteLinks []interface{}
	for _, raw := range linksRaw {
		if len(siteLinks) >= worldMapMaxSiteLinks {
			break
		}
		m, ok := raw.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		ln := stringFrom(m["locationName"])
		rid := stringFrom(m["regionId"])
		if ln == "" || rid == "" {
			continue
		}
		if _, ok := validIDs[rid]; !ok {
			continue
		}
		if len(ln) > 120 {
			ln = ln[:120]
		}
		siteLinks = append(siteLinks, map[string]interface{}{
			"locationName": ln,
			"regionId":     rid,
		})
	}
	out := map[string]interface{}{
		"regions": regions,
	}
	if len(siteLinks) > 0 {
		out["siteLinks"] = siteLinks
	}
	start := stringFrom(in["startingRegionId"])
	if start != "" {
		if _, ok := validIDs[start]; ok {
			out["startingRegionId"] = start
		}
	}
	if src := stringFrom(in["source"]); src != "" && len(src) <= 32 {
		out["source"] = src
	}
	return out
}

func neighborIDsList(v interface{}) []interface{} {
	var out []interface{}
	seen := map[string]struct{}{}
	appendStr := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if len(s) > 48 {
			s = s[:48]
		}
		if _, d := seen[s]; d {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, el := range neighborSliceAny(v) {
		appendStr(stringFrom(el))
		if len(out) >= 12 {
			break
		}
	}
	return out
}

// neighborSliceAny returns v as []interface{} for any slice type.
//
// MongoDB decodes arrays into primitive.A, which is a named slice type and therefore does not match a
// `case []interface{}` type switch. Code that reads a stored campaign document must not assume the plain type: doing so
// silently yields "no regions, no places", and the world block disappears from every prompt.
func neighborSliceAny(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	if list, ok := v.([]interface{}); ok {
		return list
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out
}

func stringFrom(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
