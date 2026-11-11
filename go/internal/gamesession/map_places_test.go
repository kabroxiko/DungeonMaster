package gamesession

import (
	"encoding/json"
	"testing"
)

func TestParseNewPlacesKeepsAUsableNameAndPoint(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{"name": "  Cave of the Red Dragon ", "x": 1150.0, "y": 640.0},
	}

	places := ParseNewPlaces(raw)

	if len(places) != 1 {
		t.Fatalf("expected one place, got %v", places)
	}
	if places[0].Name != "Cave of the Red Dragon" {
		t.Fatalf("name was not trimmed, got %q", places[0].Name)
	}
	if places[0].X != 1150 || places[0].Y != 640 {
		t.Fatalf("point was not read, got %v", places[0])
	}
}

func TestParseNewPlacesDropsEntriesWithoutAPlaceToPut(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{"name": "", "x": 1.0, "y": 2.0},
		map[string]interface{}{"name": "No Point"},
		map[string]interface{}{"name": "String Point", "x": "10", "y": "20"},
		"not an object",
		map[string]interface{}{"name": "Only Landmark", "x": json.Number("12.5"), "y": json.Number("7.5")},
	}

	places := ParseNewPlaces(raw)

	if len(places) != 1 || places[0].Name != "Only Landmark" {
		t.Fatalf("expected only the fully specified place, got %v", places)
	}
	if places[0].X != 12.5 || places[0].Y != 7.5 {
		t.Fatalf("json numbers were not read, got %v", places[0])
	}
}

func TestParseNewPlacesCapsHowManyATurnCanAdd(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{"name": "One", "x": 1.0, "y": 1.0},
		map[string]interface{}{"name": "Two", "x": 2.0, "y": 2.0},
		map[string]interface{}{"name": "Three", "x": 3.0, "y": 3.0},
	}

	places := ParseNewPlaces(raw)

	if len(places) != maxNewPlacesPerTurn {
		t.Fatalf("expected at most %d places, got %v", maxNewPlacesPerTurn, places)
	}
}

func TestParseNewPlacesIgnoresAnythingButAnArray(t *testing.T) {
	if got := ParseNewPlaces(nil); got != nil {
		t.Fatalf("expected nothing for a missing field, got %v", got)
	}
	if got := ParseNewPlaces(map[string]interface{}{"name": "Wrong Shape"}); got != nil {
		t.Fatalf("expected nothing for a non-array field, got %v", got)
	}
}
