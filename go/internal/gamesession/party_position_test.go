package gamesession

import (
	"strings"
	"testing"
)

func TestParsePartyPositionReadsAPlaceOrAPoint(t *testing.T) {
	atPlace := ParsePartyPosition(map[string]interface{}{"place": "  Aditarie  "})
	if atPlace == nil || atPlace.Place != "Aditarie" || atPlace.X != nil {
		t.Fatalf("a named place was not read, got %+v", atPlace)
	}

	onTheRoad := ParsePartyPosition(map[string]interface{}{"x": 2041.0, "y": 915.0, "note": "on the road to Aditarie"})
	if onTheRoad == nil || onTheRoad.X == nil || *onTheRoad.X != 2041 || onTheRoad.Y == nil || *onTheRoad.Y != 915 {
		t.Fatalf("a point was not read, got %+v", onTheRoad)
	}
	if onTheRoad.Note != "on the road to Aditarie" {
		t.Fatalf("the note was not read, got %q", onTheRoad.Note)
	}
}

func TestParsePartyPositionDropsEntriesThatPointNowhere(t *testing.T) {
	if got := ParsePartyPosition(nil); got != nil {
		t.Fatalf("a missing field must not move the pin, got %+v", got)
	}
	if got := ParsePartyPosition(map[string]interface{}{"note": "somewhere"}); got != nil {
		t.Fatalf("a note without a place or point points nowhere, got %+v", got)
	}
	if got := ParsePartyPosition(map[string]interface{}{"x": 10.0}); got != nil {
		t.Fatalf("half a point points nowhere, got %+v", got)
	}
}

func TestResolvePartyPointUsesTheWorldCoordinateOfANamedPlace(t *testing.T) {
	world := map[string]interface{}{
		"places": []interface{}{
			map[string]interface{}{"name": "Azureenci", "x": 1795.0, "y": 613.0},
			map[string]interface{}{"name": "Aditarie", "x": 2041.0, "y": 915.0},
		},
	}

	x, y, label, ok := resolvePartyPoint(world, &PartyPosition{Place: "aditarie"})
	if !ok || x != 2041 || y != 915 {
		t.Fatalf("expected the place's own coordinate, got %v, %v (ok=%v)", x, y, ok)
	}
	if label != "aditarie" {
		t.Fatalf("the pin keeps the reply's spelling, got %q", label)
	}

	if _, _, _, ok := resolvePartyPoint(world, &PartyPosition{Place: "Atlantis"}); ok {
		t.Fatal("an invented name has no coordinate on the map")
	}

	// A point wins over a name: that is how an encounter between places is marked.
	between := &PartyPosition{Place: "Aditarie", X: floatPtr(1900), Y: floatPtr(800), Note: "on the road"}
	x, y, label, ok = resolvePartyPoint(world, between)
	if !ok || x != 1900 || y != 800 || label != "on the road" {
		t.Fatalf("expected the exact point with its note, got %v, %v %q (ok=%v)", x, y, label, ok)
	}
}

func TestGeometryProjectsAWorldPointOntoAImageFraction(t *testing.T) {
	// The shape of a real generated world: a 4096x2304 map at resolution ~0.9568 with a border drawn outside it.
	settings := map[string]interface{}{
		"resolution":      0.9568106312292358,
		"generatedWidth":  4096.0,
		"generatedHeight": 2304.0,
		"drawBorder":      true,
		"borderWidth":     52.0,
		"borderPosition":  "Outside map",
	}

	geometry, problem := geometryFor(settings)
	if problem != "" {
		t.Fatalf("unexpected problem: %s", problem)
	}

	// The top-left corner of the map area sits one border padding inside the image.
	padding := 52 * 0.9568106312292358
	width := 4096*0.9568106312292358 + 2*padding
	height := 2304*0.9568106312292358 + 2*padding
	wantX := padding / width
	wantY := padding / height

	x, y := geometry.project(0, 0)
	if diff(x, wantX) > 1e-9 || diff(y, wantY) > 1e-9 {
		t.Fatalf("corner projected to %v, %v; want %v, %v", x, y, wantX, wantY)
	}

	// The far corner lands one padding short of the far edge.
	x, y = geometry.project(4096, 2304)
	if diff(x, 1-wantX) > 1e-9 || diff(y, 1-wantY) > 1e-9 {
		t.Fatalf("far corner projected to %v, %v; want %v, %v", x, y, 1-wantX, 1-wantY)
	}
}

func TestGeometryIgnoresAPaddingWhenTheBorderIsDrawnOverTheMap(t *testing.T) {
	settings := map[string]interface{}{
		"resolution":      1.0,
		"generatedWidth":  1000.0,
		"generatedHeight": 500.0,
		"drawBorder":      true,
		"borderWidth":     40.0,
		"borderPosition":  "Over map",
	}

	geometry, problem := geometryFor(settings)
	if problem != "" {
		t.Fatalf("unexpected problem: %s", problem)
	}
	x, y := geometry.project(0, 0)
	if x != 0 || y != 0 {
		t.Fatalf("a border over the map adds no padding, got %v, %v", x, y)
	}
	x, y = geometry.project(500, 250)
	if diff(x, 0.5) > 1e-9 || diff(y, 0.5) > 1e-9 {
		t.Fatalf("the middle of the map is half the image, got %v, %v", x, y)
	}
}

func TestGeometryRefusesAMapItCannotProject(t *testing.T) {
	missing := map[string]interface{}{"generatedWidth": 1000.0}
	if _, problem := geometryFor(missing); !strings.Contains(problem, "resolution") {
		t.Fatalf("expected a missing resolution to be reported, got %q", problem)
	}

	rotated := map[string]interface{}{
		"resolution":         1.0,
		"generatedWidth":     1000.0,
		"generatedHeight":    500.0,
		"rightRotationCount": 1.0,
	}
	if _, problem := geometryFor(rotated); !strings.Contains(problem, "rotated") {
		t.Fatalf("expected a rotated map to be refused, got %q", problem)
	}
}

func TestPartyPositionInjectTellsTheDMWhereThePartyStands(t *testing.T) {
	spec := map[string]interface{}{
		"partyMapPosition": map[string]interface{}{
			"x":     0.42,
			"y":     0.31,
			"place": "Aditarie",
			"label": "Aditarie",
		},
	}

	rendered := renderPartyPositionInject(spec)
	if !strings.Contains(rendered, "PARTY POSITION ON THE MAP") || !strings.Contains(rendered, "Aditarie") {
		t.Fatalf("the block must say where the party stands, got:\n%s", rendered)
	}

	if got := renderPartyPositionInject(nil); got != "" {
		t.Fatalf("a game with no stored position injects nothing, got %q", got)
	}
	if got := renderPartyPositionInject(map[string]interface{}{}); got != "" {
		t.Fatalf("a missing position injects nothing, got %q", got)
	}
}

func TestStoredGeometryRoundTrips(t *testing.T) {
	spec := map[string]interface{}{
		"mapGeometry": map[string]interface{}{
			"xPerUnit": 0.00025,
			"yPerUnit": 0.0004,
			"xOffset":  0.0125,
			"yOffset":  0.02,
		},
	}

	geometry, ok := storedGeometry(spec)
	if !ok {
		t.Fatal("a stored projection must be readable")
	}
	x, y := geometry.project(1000, 500)
	if diff(x, 0.0125+1000*0.00025) > 1e-9 || diff(y, 0.02+500*0.0004) > 1e-9 {
		t.Fatalf("projection changed across the stored form: %v, %v", x, y)
	}

	if _, ok := storedGeometry(map[string]interface{}{}); ok {
		t.Fatal("a game without a stored projection reports none")
	}
}

func floatPtr(v float64) *float64 { return &v }

func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
