package campaignspec

import (
	"fmt"
	"sort"
	"strings"
)

const vellarisMaxRegions = 24
const vellarisMaxPlaces = 60
const vellarisMaxRoutes = 20

// placeRank orders named places so that truncation keeps the settlements a DM is most likely to be asked about.
var placeRank = map[string]int{
	"city":    0,
	"capital": 0,
	"town":    1,
	"village": 2,
	"region":  4,
}

// NormalizeVellarisWorld turns an imported Vellaris world into the compact shape dm play renders from.
//
// The input mirrors what Vellaris returns from /api/world (world.places, world.regions) plus any measured
// route legs from /api/route, so importing a world is a copy rather than a rewrite. Every value that reaches
// the prompt is pre-formatted here: Mustache would otherwise print coordinates with full float noise, which
// reads as false precision to the model.
//
// Returns nil when the input carries neither regions nor places, which keeps the generic campaign world map
// in charge for campaigns that were never imported.
func NormalizeVellarisWorld(in map[string]interface{}) map[string]interface{} {
	if in == nil || len(in) == 0 {
		return nil
	}

	regions := normalizeVellarisRegions(in["regions"])
	places, placeTotal := normalizeVellarisPlaces(in["places"])
	routes := normalizeVellarisRoutes(in["routes"])
	if len(regions) == 0 && len(places) == 0 {
		return nil
	}

	out := map[string]interface{}{
		"seed":        numText(in["seed"], 0),
		"worldSize":   numText(in["worldSize"], 0),
		"imageWidth":  numText(in["imageWidth"], 0),
		"imageHeight": numText(in["imageHeight"], 0),
		"hasRegions":  len(regions) > 0,
		"regionCount": len(regions),
		"regions":     regions,
		"hasPlaces":   len(places) > 0,
		"placeCount":  placeTotal,
		"places":      places,
		"hasRoutes":   len(routes) > 0,
		"routeCount":  len(routes),
		"routes":      routes,
	}

	if url := stringFrom(in["mapImageUrl"]); url != "" {
		if len(url) > 300 {
			url = url[:300]
		}
		out["mapImageSuffix"] = " · map image: " + url
	}

	if placeTotal > len(places) {
		out["placesNotice"] = fmt.Sprintf(" — showing the %d most significant", len(places))
	}

	return out
}

func normalizeVellarisRegions(raw interface{}) []interface{} {
	list := neighborSliceAny(raw)
	if len(list) == 0 {
		return nil
	}
	regions := make([]interface{}, 0, len(list))
	seen := map[string]struct{}{}
	for _, item := range list {
		if len(regions) >= vellarisMaxRegions {
			break
		}
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		name := stringFrom(m["name"])
		id := stringFrom(m["id"])
		if name == "" {
			continue
		}
		if id == "" {
			id = name
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		regions = append(regions, map[string]interface{}{
			"id":          id,
			"name":        name,
			"terrain":     stringFrom(m["terrain"]),
			"centerCount": numText(m["centerCount"], 0),
			"centerX":     numText(m["centerX"], 0),
			"centerY":     numText(m["centerY"], 0),
		})
	}
	return regions
}

// normalizeVellarisPlaces returns the kept places plus the true total, so the prompt can say how much it is showing.
func normalizeVellarisPlaces(raw interface{}) ([]interface{}, int) {
	list := neighborSliceAny(raw)
	if len(list) == 0 {
		return nil, 0
	}

	type place struct {
		name string
		kind string
		x    string
		y    string
		rank int
	}
	places := make([]place, 0, len(list))
	seen := map[string]struct{}{}
	for _, item := range list {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		name := stringFrom(m["name"])
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		kind := stringFrom(m["type"])
		rank, known := placeRank[strings.ToLower(kind)]
		if !known {
			rank = 3
		}
		places = append(places, place{name: name, kind: kind, x: numText(m["x"], 0), y: numText(m["y"], 0), rank: rank})
	}
	if len(places) == 0 {
		return nil, 0
	}

	sort.SliceStable(places, func(i, j int) bool {
		if places[i].rank != places[j].rank {
			return places[i].rank < places[j].rank
		}
		return places[i].name < places[j].name
	})

	total := len(places)
	if total > vellarisMaxPlaces {
		places = places[:vellarisMaxPlaces]
	}
	out := make([]interface{}, 0, len(places))
	for _, p := range places {
		out = append(out, map[string]interface{}{"name": p.name, "type": p.kind, "x": p.x, "y": p.y})
	}
	return out, total
}

func normalizeVellarisRoutes(raw interface{}) []interface{} {
	list := neighborSliceAny(raw)
	if len(list) == 0 {
		return nil
	}
	routes := make([]interface{}, 0, len(list))
	for _, item := range list {
		if len(routes) >= vellarisMaxRoutes {
			break
		}
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		if formatted := FormatVellarisRoute(m); formatted != nil {
			routes = append(routes, formatted)
		}
	}
	return routes
}

// FormatVellarisRoute renders one stored travel leg the way the prompt reads it, and returns nil when the leg
// carries no journey.
//
// Exported so API responses can show a leg in the same words the DM is given, instead of a second formatting
// passed that would drift from this one.
func FormatVellarisRoute(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	from := stringFrom(m["from"])
	to := stringFrom(m["to"])
	if from == "" || to == "" {
		return nil
	}
	route := map[string]interface{}{
		"from":         from,
		"to":           to,
		"distance":     numText(m["distance"], 0),
		"days":         numText(m["days"], 1),
		"crossesWater": truthy(m["crossesWater"]),
		"regionsText":  joinNames(m["regionsCrossed"]),
	}
	if t := terrainText(m["terrain"]); t != "" {
		route["terrainSuffix"] = "; terrain " + t
	}
	if route["crossesWater"] == true {
		route["waterSuffix"] = " (crosses water)"
	}
	if route["regionsText"] == "" {
		route["regionsText"] = "unnamed country"
	}
	return route
}

// terrainText summarises the per-terrain shares a Vellaris route reports, such as "plains 51%, forest 45%".
func terrainText(raw interface{}) string {
	list := neighborSliceAny(raw)
	parts := make([]string, 0, len(list))
	for _, item := range list {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		name := stringFrom(m["terrain"])
		if name == "" {
			continue
		}
		if share, ok := floatFrom(m["share"]); ok {
			parts = append(parts, fmt.Sprintf("%s %.0f%%", name, share*100))
			continue
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

// joinNames accepts either plain strings or objects carrying a "name", which is how regions are reported back.
func joinNames(raw interface{}) string {
	list := neighborSliceAny(raw)
	names := make([]string, 0, len(list))
	for _, item := range list {
		var name string
		if m, ok := item.(map[string]interface{}); ok {
			name = stringFrom(m["name"])
		} else {
			name = stringFrom(item)
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// numText formats a JSON number for display. The second return of a failed lookup is 0, which reads as an
// unknown value rather than as a wrong one.
func numText(v interface{}, decimals int) string {
	f, ok := floatFrom(v)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%.*f", decimals, f)
}

func floatFrom(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%g", &f); err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func truthy(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		s := strings.ToLower(strings.TrimSpace(b))
		return s == "true" || s == "1" || s == "yes"
	default:
		return false
	}
}

// SanitizeVellarisWorld keeps a whitelisted, size-capped copy of an imported world for storage.
//
// It deliberately does not format anything: the prompt-facing formatting lives in NormalizeVellarisWorld and is
// applied per turn, so the stored copy stays as close to what the map service returned as possible.
//
// Returns nil when the input carries neither regions nor places.
func SanitizeVellarisWorld(in map[string]interface{}) map[string]interface{} {
	if in == nil || len(in) == 0 {
		return nil
	}
	regions := capList(in["regions"], vellarisMaxRegions*2)
	places := capList(in["places"], 400)
	routes := capList(in["routes"], vellarisMaxRoutes*2)
	if len(regions) == 0 && len(places) == 0 {
		return nil
	}
	out := map[string]interface{}{
		"regions": regions,
		"places":  places,
		"routes":  routes,
	}
	for _, key := range []string{"seed", "worldSize", "imageWidth", "imageHeight"} {
		if v, ok := in[key]; ok && v != nil {
			out[key] = v
		}
	}
	if url := stringFrom(in["mapImageUrl"]); url != "" {
		if len(url) > 300 {
			url = url[:300]
		}
		out["mapImageUrl"] = url
	}
	return out
}

// capList trims a JSON array to its first n elements, and drops anything that is not an object or string.
func capList(raw interface{}, n int) []interface{} {
	list := neighborSliceAny(raw)
	if len(list) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(list))
	for _, item := range list {
		if len(out) >= n {
			break
		}
		switch item.(type) {
		case map[string]interface{}, string:
			out = append(out, item)
		}
	}
	return out
}

// NormalizeVellarisRouteLeg turns a /api/route answer into a stored, renderable travel leg.
//
// Returns nil when the answer does not describe a journey that can be made, which is how "no land route" is
// reported rather than stored as a leg with no distance.
func NormalizeVellarisRouteLeg(from, to string, answer map[string]interface{}) map[string]interface{} {
	if answer == nil {
		return nil
	}
	if reachable, ok := answer["reachable"].(bool); ok && !reachable {
		return nil
	}
	distance, ok := floatFrom(answer["distance"])
	if !ok || distance <= 0 {
		return nil
	}
	leg := map[string]interface{}{
		"from":     strings.TrimSpace(from),
		"to":       strings.TrimSpace(to),
		"distance": distance,
	}
	if travel, ok := answer["travel"].(map[string]interface{}); ok && travel != nil {
		if days, ok := floatFrom(travel["days"]); ok {
			leg["days"] = days
		}
		if terrain := capList(travel["terrain"], 8); terrain != nil {
			leg["terrain"] = terrain
		}
	}
	if crosses, ok := answer["crossesWater"].(bool); ok {
		leg["crossesWater"] = crosses
	}
	if regions := capList(answer["regionsCrossed"], 12); regions != nil {
		leg["regionsCrossed"] = regions
	}
	return leg
}

// AppendVellarisRoute stores a measured leg, replacing any earlier measurement of the same pair of places.
//
// The direction is ignored when matching: a route walked one way measures the same ground the other way, and
// keeping both would spend the leg budget twice.
func AppendVellarisRoute(routes []interface{}, leg map[string]interface{}) []interface{} {
	if leg == nil {
		return routes
	}
	from := strings.ToLower(stringFrom(leg["from"]))
	to := strings.ToLower(stringFrom(leg["to"]))
	if from == "" || to == "" {
		return routes
	}
	out := make([]interface{}, 0, len(routes)+1)
	for _, existing := range routes {
		m, ok := existing.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		a := strings.ToLower(stringFrom(m["from"]))
		b := strings.ToLower(stringFrom(m["to"]))
		if (a == from && b == to) || (a == to && b == from) {
			continue
		}
		out = append(out, m)
	}
	out = append(out, leg)
	if len(out) > vellarisMaxRoutes {
		out = out[len(out)-vellarisMaxRoutes:]
	}
	return out
}
