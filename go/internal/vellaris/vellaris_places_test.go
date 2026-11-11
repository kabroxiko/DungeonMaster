package vellaris

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAddPlacesPostsToTheStoredMapAndParsesTheWorld(t *testing.T) {
	var gotPath, gotToken string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Vellaris-Token")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"success":true,"mapId":"abc","world":{"places":[{"name":"Cave of the Red Dragon","type":"Landmark","x":1150,"y":640}]},"added":["Cave of the Red Dragon"]}`))
	}))
	defer srv.Close()

	answer, err := NewWithToken(srv.URL, "secret").AddPlaces(context.Background(), "abc", []Place{{Name: "Cave of the Red Dragon", X: 1150, Y: 640}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/maps/abc/places" {
		t.Fatalf("expected /api/maps/abc/places, got %s", gotPath)
	}
	if gotToken != "secret" {
		t.Fatalf("the stored-map token was not sent, got %q", gotToken)
	}
	places, _ := gotBody["places"].([]interface{})
	if len(places) != 1 {
		t.Fatalf("expected one place in the body, got %v", gotBody["places"])
	}
	first, _ := places[0].(map[string]interface{})
	if first["name"] != "Cave of the Red Dragon" || first["x"] != 1150.0 || first["y"] != 640.0 {
		t.Fatalf("the place was not sent as a name and a point, got %v", first)
	}
	world, _ := answer["world"].(map[string]interface{})
	if world == nil {
		t.Fatalf("expected the updated world in the answer, got %v", answer)
	}
}

func TestAddPlacesToWorldCarriesTheSettingsWithThePlaces(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"success":true,"world":{"places":[]},"settings":{"randomSeed":42},"imageBase64":"aGk="}`))
	}))
	defer srv.Close()

	answer, err := New(srv.URL).AddPlacesToWorld(context.Background(), map[string]interface{}{"randomSeed": 42.0}, []Place{{Name: "Old Shrine", X: 10, Y: 20}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/world/places" {
		t.Fatalf("expected /api/world/places, got %s", gotPath)
	}
	// A stateless service has no id to name the world by, so the settings have to travel with the places.
	if gotBody["randomSeed"] != 42.0 {
		t.Fatalf("settings were not forwarded, body was %v", gotBody)
	}
	if _, ok := gotBody["places"].([]interface{}); !ok {
		t.Fatalf("places were not sent, body was %v", gotBody)
	}
	if answer["imageBase64"] != "aGk=" {
		t.Fatalf("expected the re-rendered image in the answer, got %v", answer)
	}
}
