package llm

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Documents decoded by the Mongo driver carry arrays as primitive.A. Asserting []interface{} on those silently
// yields nil, which is how the opening-scene stage ended up generating from an empty campaign context.
func TestSliceValuesReadsEveryArrayShape(t *testing.T) {
	a := map[string]interface{}{"name": "Saltmere Haven"}
	b := map[string]interface{}{"name": "Greyharbor"}

	cases := map[string]interface{}{
		"primitive.A":   primitive.A{a, b},
		"[]interface{}": []interface{}{a, b},
		"[]string":      []string{"x", "y"},
	}
	for name, raw := range cases {
		if got := SliceValues(raw); len(got) != 2 {
			t.Fatalf("%s: expected 2 values, got %d", name, len(got))
		}
	}
}

func TestSliceValuesSeparatesAbsentFromEmpty(t *testing.T) {
	if got := SliceValues(nil); got != nil {
		t.Fatal("a missing field must come back nil so callers can tell it apart from an empty list")
	}
	if got := SliceValues(map[string]interface{}{}); got != nil {
		t.Fatal("a map is not a list")
	}
	if got := SliceValues("nope"); got != nil {
		t.Fatal("a string is not a list")
	}
	if got := SliceValues(primitive.A{}); got == nil || len(got) != 0 {
		t.Fatal("an empty array is present but empty, not missing")
	}
}

func TestCoerceCampaignStageToArrayReadsABSONArray(t *testing.T) {
	locs := primitive.A{
		map[string]interface{}{"name": "Saltmere Haven", "type": "puerto franco"},
		map[string]interface{}{"name": "Wreckers' Shoal", "type": "peligro marítimo"},
	}
	got := CoerceCampaignStageToArray("keyLocations", locs)
	if len(got) != 2 {
		t.Fatalf("expected both locations, got %d", len(got))
	}
	if m, _ := got[0].(map[string]interface{}); m["name"] != "Saltmere Haven" {
		t.Fatalf("unexpected first location: %v", got[0])
	}
}
