package gamesession

import (
	"context"
	"strings"
	"testing"

	"github.com/cbroglie/mustache"
	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
)

func sampleVellarisSpec() map[string]interface{} {
	return map[string]interface{}{
		"vellarisWorld": map[string]interface{}{
			"seed":        1827671559.0,
			"worldSize":   10000.0,
			"imageWidth":  4017.0,
			"imageHeight": 2302.0,
			"mapImageUrl": "https://vellaris.example/api/world/map.jpg",
			"regions": []interface{}{
				map[string]interface{}{"id": 7.0, "name": "Venetzaddana", "terrain": "hills", "centerX": 1773.46, "centerY": 678.34, "centerCount": 812.0},
				map[string]interface{}{"id": 4.0, "name": "Suste", "centerX": 2100.4, "centerY": 900.1, "centerCount": 640.0},
			},
			"places": []interface{}{
				map[string]interface{}{"name": "Azureenci", "type": "City", "x": 1794.97, "y": 612.86},
				map[string]interface{}{"name": "Aditarie", "type": "City", "x": 2040.92, "y": 914.52},
			},
			"routes": []interface{}{
				map[string]interface{}{
					"from": "Azureenci", "to": "Aditarie", "distance": 428.09, "days": 2.03,
					"regionsCrossed": []interface{}{"Venetzaddana", "Suste"},
					"terrain": []interface{}{
						map[string]interface{}{"terrain": "plains", "share": 0.51},
						map[string]interface{}{"terrain": "forest", "share": 0.45},
					},
				},
			},
		},
	}
}

func TestRenderVellarisWorldInjectIsEmptyWithoutAWorld(t *testing.T) {
	if got := renderVellarisWorldInject(nil); got != "" {
		t.Fatalf("nil spec should inject nothing, got %q", got)
	}
	if got := renderVellarisWorldInject(map[string]interface{}{"title": "no world here"}); got != "" {
		t.Fatalf("a spec without a world should inject nothing, got %q", got)
	}
}

func TestRenderVellarisWorldInjectCarriesRulesGeometryAndTravel(t *testing.T) {
	got := renderVellarisWorldInject(sampleVellarisSpec())
	if got == "" {
		t.Fatal("expected an injected block")
	}

	// The rules have to travel with the data, or the model has no idea the block is authoritative.
	for _, want := range []string{
		"VELLARIS WORLD",
		"Never add geography",
		"DM-ONLY VELLARIS WORLD",
		"Venetzaddana",
		"Suste",
		"Azureenci",
		"Aditarie",
		"428 units",
		"2.0 days on foot",
		"plains 51%",
		"Venetzaddana, Suste",
		"https://vellaris.example/api/world/map.jpg",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("injected block is missing %q\n---\n%s", want, got)
		}
	}

	// Coordinates are DM-only working numbers; they must not leak float noise into the prompt.
	if strings.Contains(got, "1794.970000") || strings.Contains(got, "612.860000") {
		t.Fatalf("coordinates should be rounded for display\n---\n%s", got)
	}
}

func TestRenderVellarisWorldInjectPrefersMeasuredTravelOverGuessing(t *testing.T) {
	spec := sampleVellarisSpec()
	world := spec["vellarisWorld"].(map[string]interface{})
	delete(world, "routes")

	got := renderVellarisWorldInject(spec)
	if !strings.Contains(got, "No journeys have been measured yet") {
		t.Fatalf("without routes the block should tell the DM to stay vague\n---\n%s", got)
	}
	if strings.Contains(got, "use these numbers verbatim") {
		t.Fatalf("the measured-travel instruction should not appear when there are no routes\n---\n%s", got)
	}
}

func TestKeyLocationsStageUsesTheImportedRegionsWhenPresent(t *testing.T) {
	tpl := promptmgr.LoadPrompt("templates/campaign/stage_keyLocations.txt")
	if tpl == "" {
		t.Fatal("keyLocations stage template is missing")
	}

	withWorld, err := mustache.Render(tpl, map[string]interface{}{
		"campaignConcept":    "a war of salt and silver",
		"hasVellarisRegions": true,
		"vellarisRegions": []interface{}{
			map[string]interface{}{"id": "7", "name": "Venetzaddana", "terrain": "hills"},
			map[string]interface{}{"id": "4", "name": "Suste"},
		},
	})
	if err != nil {
		t.Fatalf("rendering with an imported world failed: %v", err)
	}
	for _, want := range []string{"do not output a worldMap block", "id: \"7\" — Venetzaddana (hills)", "id: \"4\" — Suste", "regionId"} {
		if !strings.Contains(withWorld, want) {
			t.Fatalf("imported-world variant is missing %q\n---\n%s", want, withWorld)
		}
	}
	if strings.Contains(withWorld, "worldMap:") {
		t.Fatal("the stage must never ask the model to write a region graph; the map service owns geography")
	}

	withoutWorld, err := mustache.Render(tpl, map[string]interface{}{"campaignConcept": "a war of salt and silver"})
	if err != nil {
		t.Fatalf("rendering without an imported world failed: %v", err)
	}
	if strings.Contains(withoutWorld, "worldMap:") || strings.Contains(withoutWorld, "neighborIds") {
		t.Fatalf("no variant may ask for an invented region graph\n---\n%s", withoutWorld)
	}
}

// The key locations stage must choose among the map's own place names: a settlement the players are told about has to
// be a place they can point at on their map.
func TestKeyLocationsStageOffersTheMapPlaceNames(t *testing.T) {
	tpl := promptmgr.LoadPrompt("templates/campaign/stage_keyLocations.txt")
	if tpl == "" {
		t.Fatal("keyLocations stage template is missing")
	}
	rendered, err := mustache.Render(tpl, map[string]interface{}{
		"campaignConcept":    "a war of salt and silver",
		"hasVellarisRegions": true,
		"vellarisRegions": []interface{}{
			map[string]interface{}{"id": "7", "name": "Venetzaddana", "terrain": "hills"},
		},
		"hasVellarisPlaces": true,
		"vellarisPlaces": []interface{}{
			map[string]interface{}{"name": "Azureenci", "type": "City", "kind": "settlement"},
			map[string]interface{}{"name": "Aditarie", "type": "City", "kind": "settlement"},
		},
	})
	if err != nil {
		t.Fatalf("rendering with map places failed: %v", err)
	}
	for _, want := range []string{
		`"Azureenci" (settlement)`,
		`"Aditarie" (settlement)`,
		"copied exactly",
		"Never invent a region or a place name",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("place list variant is missing %q\n---\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "evocative or invented toponym") {
		t.Fatal("the model must not be told to invent place names when the map supplies them")
	}
}

func TestVellarisWorldFromRequestAcceptsTheServiceAnswer(t *testing.T) {
	// This is the shape POST /api/world returns, forwarded as-is — the whole point of accepting both forms.
	envelope := map[string]interface{}{
		"success": true,
		"world": map[string]interface{}{
			"seed":    1827671559.0,
			"regions": []interface{}{map[string]interface{}{"id": 7.0, "name": "Venetzaddana"}},
			"places":  []interface{}{map[string]interface{}{"name": "Azureenci", "type": "City"}},
		},
		"settings": map[string]interface{}{"randomSeed": 1827671559.0},
	}

	world, settings, problem := vellarisWorldFromRequest(context.Background(), nil, map[string]interface{}{"world": envelope}, "English")
	if problem != "" {
		t.Fatalf("unexpected problem: %s", problem)
	}
	if world == nil || world["seed"] != 1827671559.0 {
		t.Fatalf("expected the inner world, got %v", world)
	}
	if settings == nil || settings["randomSeed"] != 1827671559.0 {
		t.Fatalf("the settings should be kept for later route lookups, got %v", settings)
	}
	if got := campaignspec.NormalizeVellarisWorld(world); got == nil || got["regionCount"] != 1 {
		t.Fatalf("the imported world should render, got %v", got)
	}
}

func TestVellarisWorldFromRequestAcceptsABareWorld(t *testing.T) {
	world := map[string]interface{}{
		"regions": []interface{}{map[string]interface{}{"name": "Suste"}},
	}
	got, settings, problem := vellarisWorldFromRequest(context.Background(), nil, map[string]interface{}{"world": world}, "English")
	if problem != "" || got == nil {
		t.Fatalf("a bare world should be accepted, problem=%q", problem)
	}
	if settings != nil {
		t.Fatalf("no settings were supplied, so none should be returned, got %v", settings)
	}
}

func TestVellarisWorldFromRequestExplainsWhatIsMissing(t *testing.T) {
	_, _, problem := vellarisWorldFromRequest(context.Background(), nil, map[string]interface{}{"gameId": "g1"}, "English")
	if problem == "" {
		t.Fatal("expected an explanation when neither a world nor settings are sent")
	}
	if !strings.Contains(problem, "settings") {
		t.Fatalf("the explanation should mention both options, got %q", problem)
	}

	// Settings without a configured service is a distinct, actionable failure.
	_, _, problem = vellarisWorldFromRequest(context.Background(), nil, map[string]interface{}{"settings": map[string]interface{}{}}, "English")
	if !strings.Contains(problem, "DM_VELLARIS_URL") {
		t.Fatalf("expected the missing setting to be named, got %q", problem)
	}
}
