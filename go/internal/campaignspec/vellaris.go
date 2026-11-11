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
	places, placeTotal := normalizeVellarisPlaces(in["places"], regionNameByID(regions))
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
			"type":        "Region",
			"kind":        "region",
			"terrain":     stringFrom(m["terrain"]),
			"centerCount": numText(m["centerCount"], 0),
			"centerX":     numText(m["centerX"], 0),
			"centerY":     numText(m["centerY"], 0),
		})
	}
	return regions
}

// regionNameByID maps every kept region id to its name, so a place can say which territory it is in without the
// prompt having to join two lists itself. Regions whose label the map never generated are absent, not given a
// placeholder name.
func regionNameByID(regions []interface{}) map[string]string {
	names := make(map[string]string, len(regions))
	for _, item := range regions {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		id := stringFrom(m["id"])
		name := stringFrom(m["name"])
		if id != "" && name != "" {
			names[id] = name
		}
	}
	return names
}

// normalizeVellarisPlaces returns the kept places plus the true total, so the prompt can say how much it is showing.
//
// Region labels and the map title are not places in the sense this list means: a region is a territory (it already has
// its own entry with an id) and the title is the map's name. Listing them here is what let the model write "the market
// of <region>", so they are left out; every remaining entry carries the kind it really is and, when the map says so,
// the region it belongs to — a place is in exactly one territory, and a prompt that does not say which one invites the
// model to file it under the wrong region.
func normalizeVellarisPlaces(raw interface{}, regionNames map[string]string) ([]interface{}, int) {
	list := neighborSliceAny(raw)
	if len(list) == 0 {
		return nil, 0
	}

	type place struct {
		name         string
		kind         string
		x            string
		y            string
		rank         int
		regionID     string
		regionName   string
		surroundings map[string]interface{}
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
		typeName := stringFrom(m["type"])
		if isNonPlaceType(typeName) {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		rank, known := placeRank[strings.ToLower(typeName)]
		if !known {
			rank = 3
		}
		regionID := regionIDText(m["regionId"])
		places = append(places, place{name: name, kind: typeName, x: numText(m["x"], 0), y: numText(m["y"], 0), rank: rank,
			regionID: regionID, regionName: regionNames[regionID], surroundings: placeSurroundings(m["surroundings"])})
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
		entry := map[string]interface{}{
			"name": p.name,
			"type": p.kind,
			"kind": PlaceKind(p.kind),
			"x":    p.x,
			"y":    p.y,
		}
		if p.regionID != "" {
			entry["regionId"] = p.regionID
		}
		if p.regionName != "" {
			entry["regionName"] = p.regionName
			entry["regionSuffix"] = " — in " + p.regionName
		}
		if text := surroundingsText(p.surroundings); text != "" {
			entry["surroundingsText"] = text
			entry["surroundingsSuffix"] = " — " + text
		}
		out = append(out, entry)
	}
	return out, total
}

// regionIDText renders a region id the way the region list does, so a place and its region compare equal whether the
// service sent the id as a number or already as text.
func regionIDText(v interface{}) string {
	if n, ok := floatFrom(v); ok {
		return fmt.Sprintf("%.0f", n)
	}
	return stringFrom(v)
}

// placeSurroundings keeps the map's description of the ground around a place, in the terms the prompt speaks.
//
// Vellaris reports the terrain a place stands in, the terrain bordering it, and whether a coast, lake, river,
// mountains or an island are at hand. Only those fields are kept, so a stored world from an older server (which
// sent nothing) or a newer one (which might send more) can neither invent surroundings here nor leak raw data into
// the prompt. Returns nil when nothing was reported, which is what keeps an unknown place from being described at all.
func placeSurroundings(raw interface{}) map[string]interface{} {
	m, _ := raw.(map[string]interface{})
	if m == nil {
		return nil
	}
	out := map[string]interface{}{}
	if terrain := stringFrom(m["terrain"]); terrain != "" {
		out["terrain"] = terrain
	}
	if nearby := stringFrom(m["nearbyTerrain"]); nearby != "" {
		out["nearbyTerrain"] = nearby
	}
	for _, key := range []string{"coastal", "lake", "river", "mountains", "island"} {
		if truthy(m[key]) {
			out[key] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// surroundingsText turns a place's surroundings into the short phrase the prompts print after its name, such as
// "forest, bordering hills, on the coast".
//
// Only what the map reported is said: a world without surroundings produces no phrase, so the model is never handed
// a terrain the map service did not state. The terrain itself is not repeated as a feature ("mountains" for a place
// standing on mountains), so the phrase reads as position rather than as a list of duplicates.
func surroundingsText(surroundings map[string]interface{}) string {
	terrain := stringFrom(surroundings["terrain"])
	nearby := stringFrom(surroundings["nearbyTerrain"])
	parts := make([]string, 0, 5)
	if terrain != "" {
		parts = append(parts, terrain)
	}
	if nearby != "" && nearby != terrain {
		parts = append(parts, "bordering "+nearby)
	}
	if truthy(surroundings["island"]) {
		parts = append(parts, "on an island")
	} else if truthy(surroundings["coastal"]) {
		parts = append(parts, "on the coast")
	}
	if truthy(surroundings["river"]) && terrain != "river" {
		parts = append(parts, "on a river")
	}
	if truthy(surroundings["lake"]) && terrain != "lake" {
		parts = append(parts, "beside a lake")
	}
	if truthy(surroundings["mountains"]) && terrain != "mountain" {
		parts = append(parts, "near mountains")
	}
	return strings.Join(parts, ", ")
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

// PlaceNames returns the world's named places in prompt order, capped at limit (0 means no limit).
//
// The list is the map's own vocabulary: anything a campaign names must come from here, so every consumer that writes
// place names reads them through this function instead of reformatting the world document itself.
func PlaceNames(world map[string]interface{}, limit int) []string {
	list := neighborSliceAny(world["places"])
	names := make([]string, 0, len(list))
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
		key := normalizePlaceNameKey(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
		if limit > 0 && len(names) >= limit {
			break
		}
	}
	return names
}

// PlaceKind turns a Vellaris map-text type into the word a prompt uses for what the name actually is.
//
// Vellaris reports one settlement type ("City") and puts the structure in the name ("Aldea de X", "Castillo de Y"), so
// the useful distinction for narration is settlement / region / lake / river / mountains.
func PlaceKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "city", "capital", "town", "village":
		return "settlement"
	case "region":
		return "region"
	case "lake":
		return "lake"
	case "river":
		return "river"
	case "mountain_range", "other_mountains":
		return "mountains"
	case "title":
		return "map title"
	case "":
		return "place"
	default:
		return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), "_", " ")
	}
}

// IsSettlementPlaceType reports whether a map-text type names a settlement (a built-up place with streets and markets).
func IsSettlementPlaceType(raw string) bool {
	return PlaceKind(raw) == "settlement"
}

// isNonPlaceType reports whether the map text is not a place: the map title, or a region label (regions are delivered
// separately, with ids).
func isNonPlaceType(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "title", "region":
		return true
	default:
		return false
	}
}

// SettlementPlaces returns the world's settlement entries (name, type and kind), in the order the prompt should list
// them. These are the only places a key location may be named after: a region is a territory and a lake is water, so
// neither can be a town with a market.
func SettlementPlaces(world map[string]interface{}) []interface{} {
	out := make([]interface{}, 0)
	for _, item := range neighborSliceAny(world["places"]) {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		if !IsSettlementPlaceType(stringFrom(m["type"])) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// SettlementNames returns the names of the world's settlements, capped at limit (0 means no limit).
func SettlementNames(world map[string]interface{}, limit int) []string {
	names := make([]string, 0)
	for _, item := range SettlementPlaces(world) {
		m, _ := item.(map[string]interface{})
		name := stringFrom(m["name"])
		if name == "" {
			continue
		}
		names = append(names, name)
		if limit > 0 && len(names) >= limit {
			break
		}
	}
	return names
}

// AlignKeyLocationsToWorld makes every key location carry a name that exists on the players' map.
//
// The map owns place names, not the model: a location that renames a place the players can see would make narration
// and map disagree. A name the map already has is kept (with the map's spelling); an invented or repeated name takes
// the next unused map name, so the model's type and significance survive; a location left with no name is dropped
// rather than invented. Region references to regions that do not exist are removed the same way, and a location named
// after a map place is moved to the region that place actually belongs to, so a stage cannot file a settlement under a
// territory it is not in.
//
// Only settlements are candidates: a key location is a place the party goes to, so it cannot be named after a region or
// a lake. Worlds with no settlement at all fall back to every named place. When a settlement name is assigned, the
// location's type becomes "settlement" so it cannot describe a village as a ruin.
//
// Returns the aligned locations and how many names were replaced. With no map places the input comes back unchanged.
func AlignKeyLocationsToWorld(locations []interface{}, world map[string]interface{}) ([]interface{}, int) {
	if len(locations) == 0 || world == nil {
		return locations, 0
	}
	canonical := SettlementNames(world, 0)
	settlementsOnly := len(canonical) > 0
	if !settlementsOnly {
		canonical = PlaceNames(world, 0)
	}
	if len(canonical) == 0 {
		return locations, 0
	}
	byKey := make(map[string]string, len(canonical))
	for _, name := range canonical {
		byKey[normalizePlaceNameKey(name)] = name
	}
	validRegions := map[string]struct{}{}
	for _, item := range neighborSliceAny(world["regions"]) {
		m, _ := item.(map[string]interface{})
		if id := stringFrom(m["id"]); id != "" {
			validRegions[id] = struct{}{}
		}
	}
	// Which region each place is in, straight from the map. Worlds generated before the map service reported this
	// leave the map empty, and the model's own choice is then only checked for existence as before.
	regionByPlace := map[string]string{}
	for _, item := range neighborSliceAny(world["places"]) {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		key := normalizePlaceNameKey(stringFrom(m["name"]))
		if id := regionIDText(m["regionId"]); key != "" && id != "" {
			regionByPlace[key] = id
		}
	}

	used := map[string]struct{}{}
	type entry struct {
		source map[string]interface{}
		name   string // empty when the model's name is not on the map
		usable bool
	}
	entries := make([]entry, len(locations))

	// First pass: keep the names the model got right, in output order, first occurrence winning. A later duplicate
	// must not steal the name from the location that named the place first.
	for i, raw := range locations {
		m, ok := raw.(map[string]interface{})
		if !ok || m == nil {
			continue
		}
		e := entry{source: m, usable: true}
		if key := normalizePlaceNameKey(stringFrom(m["name"])); key != "" {
			if name, known := byKey[key]; known {
				if _, taken := used[key]; !taken {
					used[key] = struct{}{}
					e.name = name
				}
			}
		}
		entries[i] = e
	}

	// Second pass: locations the map does not know take the next unused map name, so the model's type and
	// significance survive under a name the players can actually find.
	next := 0
	renamed := 0
	aligned := make([]interface{}, 0, len(locations))
	for _, e := range entries {
		if !e.usable {
			continue
		}
		name := e.name
		if name == "" {
			for next < len(canonical) {
				candidate := canonical[next]
				next++
				key := normalizePlaceNameKey(candidate)
				if _, taken := used[key]; taken {
					continue
				}
				used[key] = struct{}{}
				name = candidate
				break
			}
			renamed++
			if name == "" {
				// The map has no names left; a made-up name is the one thing this must not produce.
				continue
			}
		}
		item := make(map[string]interface{}, len(e.source))
		for k, v := range e.source {
			item[k] = v
		}
		item["name"] = name
		if settlementsOnly {
			// The name is a village, town or stronghold on the map; calling it a ruin or a wilderness would be a
			// different mistake in the other direction. What the place is comes from the map, not the model.
			item["type"] = "settlement"
		}
		// The map knows which region the place is in, so that region wins over the model's guess.
		if mapped := regionByPlace[normalizePlaceNameKey(name)]; mapped != "" {
			item["regionId"] = mapped
		} else if regionID := stringFrom(item["regionId"]); regionID != "" {
			if _, valid := validRegions[regionID]; !valid {
				delete(item, "regionId")
			}
		}
		aligned = append(aligned, item)
	}
	return aligned, renamed
}

// normalizePlaceNameKey compares names the way a reader does: case and runs of whitespace do not make a new place.
func normalizePlaceNameKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
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
