package campaignspec

import (
	"strings"
	"testing"
)

func sampleVellarisWorld(placeCount int) map[string]interface{} {
	places := make([]interface{}, 0, placeCount)
	for i := 0; i < placeCount; i++ {
		places = append(places, map[string]interface{}{
			"name": "Place" + strings.Repeat("x", i%3) + string(rune('A'+i%26)),
			"type": "Village",
			"x":    1000.5 + float64(i),
			"y":    2000.25 + float64(i),
		})
	}
	return map[string]interface{}{
		"seed":        1827671559.0,
		"worldSize":   10000.0,
		"imageWidth":  4017.0,
		"imageHeight": 2302.0,
		"regions": []interface{}{
			map[string]interface{}{"id": 7.0, "name": "Venetzaddana", "centerX": 1773.46, "centerY": 678.34, "centerCount": 812.0},
			map[string]interface{}{"id": "r2", "name": "Suste", "centerX": 2100.4, "centerY": 900.1, "centerCount": 640.0},
		},
		"places": places,
		"routes": []interface{}{
			map[string]interface{}{
				"from":         "Azureenci",
				"to":           "Aditarie",
				"distance":     428.09,
				"days":         1.27,
				"crossesWater": false,
				"regionsCrossed": []interface{}{
					map[string]interface{}{"name": "Venetzaddana"},
					map[string]interface{}{"name": "Suste"},
				},
				"terrain": []interface{}{
					map[string]interface{}{"terrain": "plains", "share": 0.51},
					map[string]interface{}{"terrain": "forest", "share": 0.45},
				},
			},
		},
	}
}

func TestNormalizeVellarisWorldPrintsWhatSurroundsEachPlace(t *testing.T) {
	world := map[string]interface{}{
		"places": []interface{}{
			map[string]interface{}{
				"name": "Azureenci", "type": "City", "x": 10.0, "y": 20.0,
				"surroundings": map[string]interface{}{
					"terrain":       "forest",
					"nearbyTerrain": "hills",
					"coastal":       true,
					"river":         true,
					"lake":          false,
					"mountains":     true,
					"island":        false,
					"unexpected":    "ignored",
				},
			},
			map[string]interface{}{
				"name": "Ostrel", "type": "City", "x": 30.0, "y": 40.0,
				"surroundings": map[string]interface{}{"terrain": "hills", "coastal": true, "island": true},
			},
			map[string]interface{}{"name": "Praino", "type": "City", "x": 1.0, "y": 2.0},
			map[string]interface{}{
				"name": "Ciudad de Kanaania", "type": "City", "x": 50.0, "y": 60.0,
				"surroundings": map[string]interface{}{"terrain": "plains", "island": true, "riverNearby": true},
			},
		},
	}

	places := NormalizeVellarisWorld(world)["places"].([]interface{})
	byName := make(map[string]map[string]interface{}, len(places))
	for _, item := range places {
		entry := item.(map[string]interface{})
		byName[entry["name"].(string)] = entry
	}

	first := byName["Azureenci"]
	if first["surroundingsSuffix"] != " — forest, bordering hills, on the coast, on a river, near mountains" {
		t.Fatalf("unexpected surroundings for %v: %q", first["name"], first["surroundingsSuffix"])
	}
	second := byName["Ostrel"]
	if second["surroundingsSuffix"] != " — hills, on an island" {
		t.Fatalf("an island should read as an island rather than as the mainland coast: %q", second["surroundingsSuffix"])
	}
	third := byName["Praino"]
	if _, ok := third["surroundingsSuffix"]; ok {
		t.Fatalf("a place the map said nothing about must not be given surroundings: %v", third["surroundingsSuffix"])
	}
	fourth := byName["Ciudad de Kanaania"]
	if fourth["surroundingsSuffix"] != " — plains, inland on an island, near a river" {
		t.Fatalf("a place inland on an island beside a river it does not reach must say so: %q", fourth["surroundingsSuffix"])
	}
}

func TestNormalizeVellarisWorldIsEmptyForNoWorld(t *testing.T) {
	if got := NormalizeVellarisWorld(nil); got != nil {
		t.Fatalf("nil input should stay nil, got %v", got)
	}
	if got := NormalizeVellarisWorld(map[string]interface{}{"seed": 1.0}); got != nil {
		t.Fatalf("a world with no regions and no places should stay nil, got %v", got)
	}
}

func TestNormalizeVellarisWorldFormatsValuesForPrompt(t *testing.T) {
	got := NormalizeVellarisWorld(sampleVellarisWorld(3))
	if got == nil {
		t.Fatal("expected a normalized world")
	}
	if got["seed"] != "1827671559" {
		t.Fatalf("seed should be a whole number, got %v", got["seed"])
	}
	if got["regionCount"] != 2 {
		t.Fatalf("expected 2 regions, got %v", got["regionCount"])
	}

	regions := got["regions"].([]interface{})
	first := regions[0].(map[string]interface{})
	if first["id"] != "7" {
		t.Fatalf("a numeric region id should render as a string, got %v", first["id"])
	}
	if first["centerX"] != "1773" {
		t.Fatalf("centres should lose their float noise, got %v", first["centerX"])
	}

	routes := got["routes"].([]interface{})
	route := routes[0].(map[string]interface{})
	if route["distance"] != "428" {
		t.Fatalf("distance should be whole units, got %v", route["distance"])
	}
	if route["days"] != "1.3" {
		t.Fatalf("days should keep one decimal, got %v", route["days"])
	}
	if route["regionsText"] != "Venetzaddana, Suste" {
		t.Fatalf("regions crossed should be names, got %v", route["regionsText"])
	}
	if route["terrainSuffix"] != "; terrain plains 51%, forest 45%" {
		t.Fatalf("terrain shares should be summarised, got %v", route["terrainSuffix"])
	}
	if _, ok := route["waterSuffix"]; ok {
		t.Fatal("a dry route should not mention water")
	}
}

func TestNormalizeVellarisWorldCapsPlacesButKeepsTheTotal(t *testing.T) {
	got := NormalizeVellarisWorld(sampleVellarisWorld(vellarisMaxPlaces + 15))
	places := got["places"].([]interface{})
	if len(places) != vellarisMaxPlaces {
		t.Fatalf("expected places capped at %d, got %d", vellarisMaxPlaces, len(places))
	}
	if got["placeCount"] != vellarisMaxPlaces+15 {
		t.Fatalf("expected the true total to survive the cap, got %v", got["placeCount"])
	}
	if !strings.Contains(got["placesNotice"].(string), "most significant") {
		t.Fatalf("expected a notice about truncation, got %v", got["placesNotice"])
	}
}

func TestNormalizeVellarisWorldKeepsCitiesWhenTruncating(t *testing.T) {
	world := sampleVellarisWorld(3)
	world["places"] = append(world["places"].([]interface{}), map[string]interface{}{
		"name": "Zz Capital", "type": "City", "x": 1.0, "y": 2.0,
	})
	got := NormalizeVellarisWorld(world)
	places := got["places"].([]interface{})
	if places[0].(map[string]interface{})["name"] != "Zz Capital" {
		t.Fatalf("cities should be listed before villages, got %v", places[0])
	}
}

// A place is in exactly one region. The map reports which one, so the prompt can state it instead of leaving the model
// to guess and file a place under a territory it is not in.
func TestNormalizeVellarisWorldSaysWhichRegionEachPlaceIsIn(t *testing.T) {
	world := map[string]interface{}{
		"regions": []interface{}{
			map[string]interface{}{"id": 7.0, "name": "Venetzaddana"},
		},
		"places": []interface{}{
			map[string]interface{}{"name": "Azureenci", "type": "City", "regionId": 7.0},
			map[string]interface{}{"name": "Nowhere", "type": "City"},
		},
	}

	byName := map[string]map[string]interface{}{}
	for _, raw := range NormalizeVellarisWorld(world)["places"].([]interface{}) {
		m := raw.(map[string]interface{})
		byName[m["name"].(string)] = m
	}

	azureenci := byName["Azureenci"]
	if azureenci["regionId"] != "7" || azureenci["regionName"] != "Venetzaddana" {
		t.Fatalf("a place should name its region, got %v / %v", azureenci["regionId"], azureenci["regionName"])
	}
	if azureenci["regionSuffix"] != " — in Venetzaddana" {
		t.Fatalf("unexpected region suffix: %q", azureenci["regionSuffix"])
	}
	if _, ok := byName["Nowhere"]["regionSuffix"]; ok {
		t.Fatal("a place the map puts in no region must not be given one")
	}
}

func TestSanitizeVellarisWorldKeepsOnlyKnownKeys(t *testing.T) {
	got := SanitizeVellarisWorld(map[string]interface{}{
		"seed":        7.0,
		"imageWidth":  4000.0,
		"mapImageUrl": "https://example/map.jpg",
		"regions":     []interface{}{map[string]interface{}{"name": "Suste"}},
		"places":      []interface{}{map[string]interface{}{"name": "Azureenci"}},
		"junk":        "should not be stored",
		"settings":    map[string]interface{}{"huge": "blob"},
	})
	if got == nil {
		t.Fatal("expected a sanitized world")
	}
	if _, ok := got["junk"]; ok {
		t.Fatal("unknown keys should not be stored")
	}
	if _, ok := got["settings"]; ok {
		t.Fatal("settings do not belong inside the world")
	}
	if got["mapImageUrl"] != "https://example/map.jpg" {
		t.Fatalf("map url was lost, got %v", got["mapImageUrl"])
	}
}

func TestSanitizeVellarisWorldIsNilWithoutGeography(t *testing.T) {
	if got := SanitizeVellarisWorld(map[string]interface{}{"seed": 1.0}); got != nil {
		t.Fatalf("a world with no regions and no places should not be stored, got %v", got)
	}
}

func TestNormalizeVellarisRouteLegReadsARouteAnswer(t *testing.T) {
	answer := map[string]interface{}{
		"success":      true,
		"reachable":    true,
		"distance":     428.09,
		"crossesWater": false,
		"travel": map[string]interface{}{
			"days": 1.27,
			"terrain": []interface{}{
				map[string]interface{}{"terrain": "plains", "share": 0.51},
			},
		},
		"regionsCrossed": []interface{}{
			map[string]interface{}{"id": 7.0, "name": "Venetzaddana"},
		},
	}
	leg := NormalizeVellarisRouteLeg("Azureenci", "Aditarie", answer)
	if leg == nil {
		t.Fatal("expected a leg")
	}
	if leg["distance"] != 428.09 {
		t.Fatalf("distance should stay numeric for storage, got %v", leg["distance"])
	}
	if leg["days"] != 1.27 {
		t.Fatalf("days should stay numeric for storage, got %v", leg["days"])
	}

	// And it has to render the way the prompt expects once stored.
	formatted := FormatVellarisRoute(leg)
	if formatted["distance"] != "428" || formatted["days"] != "1.3" {
		t.Fatalf("unexpected display form: %v", formatted)
	}
	if formatted["terrainSuffix"] != "; terrain plains 51%" {
		t.Fatalf("terrain was lost, got %v", formatted["terrainSuffix"])
	}
	if formatted["regionsText"] != "Venetzaddana" {
		t.Fatalf("regions were lost, got %v", formatted["regionsText"])
	}
}

func TestNormalizeVellarisRouteLegRejectsUnreachableJourneys(t *testing.T) {
	if got := NormalizeVellarisRouteLeg("A", "B", map[string]interface{}{"reachable": false, "distance": 0.0}); got != nil {
		t.Fatalf("an unreachable journey should not be stored as a leg, got %v", got)
	}
	if got := NormalizeVellarisRouteLeg("A", "B", map[string]interface{}{"reachable": true}); got != nil {
		t.Fatalf("a leg with no distance should not be stored, got %v", got)
	}
}

func TestAppendVellarisRouteReplacesTheSameJourneyInEitherDirection(t *testing.T) {
	routes := AppendVellarisRoute(nil, map[string]interface{}{"from": "Azureenci", "to": "Aditarie", "distance": 400.0})
	routes = AppendVellarisRoute(routes, map[string]interface{}{"from": "ADITARIE", "to": "AZUREENCI", "distance": 428.0})
	if len(routes) != 1 {
		t.Fatalf("the same journey measured again should replace, got %d legs", len(routes))
	}
	if routes[0].(map[string]interface{})["distance"] != 428.0 {
		t.Fatalf("the newer measurement should win, got %v", routes[0])
	}

	routes = AppendVellarisRoute(routes, map[string]interface{}{"from": "Suste", "to": "Aditarie", "distance": 90.0})
	if len(routes) != 2 {
		t.Fatalf("a different journey should be added, got %d legs", len(routes))
	}
}
