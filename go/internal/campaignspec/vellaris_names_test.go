package campaignspec

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func sampleWorldForNameAlignment() map[string]interface{} {
	return map[string]interface{}{
		"places": []interface{}{
			map[string]interface{}{"name": "Azureenci", "type": "City"},
			map[string]interface{}{"name": "Aditarie", "type": "City"},
			map[string]interface{}{"name": "Haykeaneile Farm", "type": "City"},
		},
		"regions": []interface{}{
			map[string]interface{}{"id": "7", "name": "Venetzaddana"},
		},
	}
}

// A key location the players are told about must be a place they can point at on their map. Names the map knows are
// kept (with the map's spelling), invented ones are replaced from the map, and no name is ever invented to fill a gap.
func TestAlignKeyLocationsToWorldUsesOnlyMapNames(t *testing.T) {
	locations := []interface{}{
		map[string]interface{}{"name": "azureenci", "type": "settlement", "significance": "a", "regionId": "7"},
		map[string]interface{}{"name": "Zorvath", "type": "ruin", "significance": "b", "regionId": "99"},
		map[string]interface{}{"name": "Aditarie", "type": "temple", "significance": "c"},
	}
	aligned, renamed := AlignKeyLocationsToWorld(locations, sampleWorldForNameAlignment())

	if len(aligned) != 3 {
		t.Fatalf("every location has a map name available, got %d", len(aligned))
	}
	names := make([]string, 0, len(aligned))
	for _, raw := range aligned {
		names = append(names, raw.(map[string]interface{})["name"].(string))
	}
	// The model's correct choice keeps its name even though an invented location came earlier in the list.
	want := []string{"Azureenci", "Haykeaneile Farm", "Aditarie"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
	if renamed != 1 {
		t.Fatalf("only the invented name should count as replaced, got %d", renamed)
	}

	first := aligned[0].(map[string]interface{})
	if first["regionId"] != "7" {
		t.Fatalf("a real region reference must be kept, got %v", first["regionId"])
	}
	second := aligned[1].(map[string]interface{})
	if _, has := second["regionId"]; has {
		t.Fatalf("a region that does not exist must be dropped, got %v", second["regionId"])
	}
	if second["significance"] != "b" {
		t.Fatal("the model's significance must survive the rename")
	}
}

// The map says which region a place sits in, so a key location named after that place is moved there even when the
// model chose another one. A location renamed to a map place takes that place's region too.
func TestAlignKeyLocationsToWorldMovesAPlaceToItsMapRegion(t *testing.T) {
	world := map[string]interface{}{
		"places": []interface{}{
			map[string]interface{}{"name": "Azureenci", "type": "City", "regionId": "7"},
			map[string]interface{}{"name": "Aditarie", "type": "City", "regionId": "4"},
		},
		"regions": []interface{}{
			map[string]interface{}{"id": "7", "name": "Venetzaddana"},
			map[string]interface{}{"id": "4", "name": "Suste"},
		},
	}
	locations := []interface{}{
		map[string]interface{}{"name": "Azureenci", "regionId": "4"},
		map[string]interface{}{"name": "Zorvath", "regionId": "7"},
	}

	aligned, _ := AlignKeyLocationsToWorld(locations, world)
	if got := aligned[0].(map[string]interface{})["regionId"]; got != "7" {
		t.Fatalf("the map's region must win over the model's guess, got %v", got)
	}
	if aligned[1].(map[string]interface{})["name"] != "Aditarie" {
		t.Fatalf("the invented name should take the next map name, got %v", aligned[1])
	}
	if got := aligned[1].(map[string]interface{})["regionId"]; got != "4" {
		t.Fatalf("a renamed location takes the region of the name it received, got %v", got)
	}
}

func TestAlignKeyLocationsToWorldDropsDuplicatesInsteadOfInventing(t *testing.T) {
	locations := []interface{}{
		map[string]interface{}{"name": "Azureenci"},
		map[string]interface{}{"name": "Azureenci"},
	}
	aligned, renamed := AlignKeyLocationsToWorld(locations, sampleWorldForNameAlignment())
	if len(aligned) != 2 {
		t.Fatalf("the duplicate can take the next map name, got %d", len(aligned))
	}
	if renamed != 1 {
		t.Fatalf("the duplicate should be renamed once, got %d", renamed)
	}
	if aligned[1].(map[string]interface{})["name"] != "Aditarie" {
		t.Fatalf("duplicate should take the next unused map name, got %v", aligned[1])
	}
}

func TestAlignKeyLocationsToWorldNeverInventsBeyondTheMap(t *testing.T) {
	world := map[string]interface{}{
		"places": []interface{}{map[string]interface{}{"name": "Azureenci"}},
	}
	locations := []interface{}{
		map[string]interface{}{"name": "Zorvath"},
		map[string]interface{}{"name": "Qorvath"},
	}
	aligned, _ := AlignKeyLocationsToWorld(locations, world)
	if len(aligned) != 1 {
		t.Fatalf("the map has one name, so only one location may survive, got %d", len(aligned))
	}
	if aligned[0].(map[string]interface{})["name"] != "Azureenci" {
		t.Fatalf("unexpected aligned location: %v", aligned[0])
	}
}

func TestAlignKeyLocationsToWorldLeavesCampaignsWithoutAMapAlone(t *testing.T) {
	locations := []interface{}{map[string]interface{}{"name": "Zorvath"}}
	aligned, renamed := AlignKeyLocationsToWorld(locations, nil)
	if renamed != 0 || len(aligned) != 1 {
		t.Fatalf("no world means nothing to align, got %v renamed=%d", aligned, renamed)
	}
	if aligned[0].(map[string]interface{})["name"] != "Zorvath" {
		t.Fatal("a campaign with no imported world keeps the model's names")
	}

	empty := map[string]interface{}{"places": []interface{}{}}
	aligned, renamed = AlignKeyLocationsToWorld(locations, empty)
	if renamed != 0 || len(aligned) != 1 {
		t.Fatalf("a world with no places means nothing to align, got %v renamed=%d", aligned, renamed)
	}
}

func TestPlaceNamesKeepsOrderAndDeduplicates(t *testing.T) {
	world := map[string]interface{}{
		"places": []interface{}{
			map[string]interface{}{"name": "Azureenci"},
			map[string]interface{}{"name": "  azureenci "},
			map[string]interface{}{"name": "Aditarie"},
			map[string]interface{}{"name": ""},
		},
	}
	names := PlaceNames(world, 0)
	if len(names) != 2 || names[0] != "Azureenci" || names[1] != "Aditarie" {
		t.Fatalf("unexpected place names: %v", names)
	}
	if limited := PlaceNames(world, 1); len(limited) != 1 || limited[0] != "Azureenci" {
		t.Fatalf("limit was not applied: %v", limited)
	}
}

// MongoDB decodes arrays into primitive.A, a named slice type. A []interface{} type switch silently yields "no
// regions, no places", which removed the world block from every prompt and left the campaign unaligned.
func TestNormalizeVellarisWorldAcceptsMongoDecodedArrays(t *testing.T) {
	in := map[string]interface{}{
		"regions": primitive.A{
			map[string]interface{}{"id": 0.0, "name": "Deoniney", "terrain": "hills", "centerCount": 157.0},
		},
		"places": primitive.A{
			map[string]interface{}{"name": "Deoniney", "type": "Region", "x": 1.0, "y": 2.0},
			map[string]interface{}{"name": "Aldea de Alequettenie", "type": "City", "x": 3.0, "y": 4.0},
			map[string]interface{}{"name": "La Tierra de Mathe", "type": "Title", "x": 5.0, "y": 6.0},
			map[string]interface{}{"name": "Lago Putting Track", "type": "Lake", "x": 7.0, "y": 8.0},
		},
	}
	got := NormalizeVellarisWorld(in)
	if got == nil {
		t.Fatal("a stored world must normalize, or the model never sees it")
	}
	regions := neighborSliceAny(got["regions"])
	if len(regions) != 1 {
		t.Fatalf("regions were lost, got %v", got["regions"])
	}
	region := regions[0].(map[string]interface{})
	if region["kind"] != "region" || region["type"] != "Region" {
		t.Fatalf("a region must say what it is, got %v", region)
	}

	places := neighborSliceAny(got["places"])
	names := make([]string, 0, len(places))
	for _, raw := range places {
		names = append(names, raw.(map[string]interface{})["name"].(string))
	}
	// Region labels and the map title are not settlements/features; regions arrive separately with ids.
	if len(names) != 2 || names[0] != "Aldea de Alequettenie" || names[1] != "Lago Putting Track" {
		t.Fatalf("unexpected places: %v", names)
	}
	kinds := map[string]string{}
	for _, raw := range places {
		m := raw.(map[string]interface{})
		kinds[m["name"].(string)] = m["kind"].(string)
	}
	if kinds["Aldea de Alequettenie"] != "settlement" || kinds["Lago Putting Track"] != "lake" {
		t.Fatalf("each place must carry its kind, got %v", kinds)
	}
}

func TestPlaceKindNamesWhatTheMapTextIs(t *testing.T) {
	cases := map[string]string{
		"City":            "settlement",
		"Region":          "region",
		"Lake":            "lake",
		"River":           "river",
		"Mountain_range":  "mountains",
		"Other_mountains": "mountains",
		"Title":           "map title",
		"":                "place",
	}
	for raw, want := range cases {
		if got := PlaceKind(raw); got != want {
			t.Fatalf("PlaceKind(%q) = %q, want %q", raw, got, want)
		}
	}
	if !IsSettlementPlaceType("City") || IsSettlementPlaceType("Region") || IsSettlementPlaceType("Lake") {
		t.Fatal("only settlements may be used as settlements")
	}
}

// A region is a territory and a lake is water: neither can be the place a party travels to, nor a city with a market.
func TestAlignKeyLocationsToWorldUsesOnlySettlements(t *testing.T) {
	world := map[string]interface{}{
		"places": []interface{}{
			map[string]interface{}{"name": "Aldea de Alequettenie", "type": "City", "kind": "settlement"},
			map[string]interface{}{"name": "Aldea de Deagus", "type": "City", "kind": "settlement"},
			map[string]interface{}{"name": "Lago Putting Track", "type": "Lake", "kind": "lake"},
		},
		"regions": []interface{}{
			map[string]interface{}{"id": "0", "name": "Deoniney", "kind": "region"},
		},
	}
	locations := []interface{}{
		map[string]interface{}{"name": "Deoniney", "type": "settlement", "significance": "a market town"},
		map[string]interface{}{"name": "Eldermist Keep", "type": "ruin"},
	}
	aligned, renamed := AlignKeyLocationsToWorld(locations, world)
	if renamed != 2 {
		t.Fatalf("both names should be reconciled to settlements, got %d", renamed)
	}
	names := make([]string, 0, len(aligned))
	for _, raw := range aligned {
		item := raw.(map[string]interface{})
		names = append(names, item["name"].(string))
		if item["type"] != "settlement" {
			t.Fatalf("a name taken from a settlement must be typed settlement, got %v", item)
		}
	}
	if len(names) != 2 || names[0] != "Aldea de Alequettenie" || names[1] != "Aldea de Deagus" {
		t.Fatalf("key locations must be settlements, got %v", names)
	}
}
