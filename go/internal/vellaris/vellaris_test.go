package vellaris

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorldPostsTheSettingsAndParsesTheAnswer(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"success":true,"world":{"regions":[{"id":7,"name":"Venetzaddana"}]},"settings":{"randomSeed":42}}`))
	}))
	defer srv.Close()

	answer, err := New(srv.URL).World(context.Background(), map[string]interface{}{"randomSeed": 42.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/world" {
		t.Fatalf("expected /api/world, got %s", gotPath)
	}
	// The settings have to survive as the request body; the service reads them as its map configuration.
	if gotBody["randomSeed"] != 42.0 {
		t.Fatalf("settings were not forwarded, body was %v", gotBody)
	}
	if _, ok := answer["world"].(map[string]interface{}); !ok {
		t.Fatalf("expected a world in the answer, got %v", answer)
	}
}

func TestRouteSendsTheEndpointsAlongsideTheSettings(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/route" {
			t.Errorf("expected /api/route, got %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"success":true,"reachable":true,"distance":428.09}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL).Route(context.Background(), map[string]interface{}{"randomSeed": 42.0}, "Azureenci", "Aditarie", "horse", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["randomSeed"] != 42.0 {
		t.Fatalf("settings were not forwarded, body was %v", gotBody)
	}
	from, _ := gotBody["from"].(map[string]interface{})
	if from["name"] != "Azureenci" {
		t.Fatalf("from was not sent as a named endpoint, got %v", gotBody["from"])
	}
	to, _ := gotBody["to"].(map[string]interface{})
	if to["name"] != "Aditarie" {
		t.Fatalf("to was not sent as a named endpoint, got %v", gotBody["to"])
	}
	if gotBody["mode"] != "horse" {
		t.Fatalf("mode was not forwarded, got %v", gotBody["mode"])
	}
	if gotBody["allowWater"] != true {
		t.Fatalf("allowWater was not forwarded, got %v", gotBody["allowWater"])
	}
}

func TestRouteOmitsOptionalFieldsItWasNotGiven(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"success":true,"reachable":true,"distance":10}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL).Route(context.Background(), map[string]interface{}{}, "A", "B", "", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := gotBody["mode"]; ok {
		t.Fatalf("an unset mode should not be sent, got %v", gotBody["mode"])
	}
	if _, ok := gotBody["allowWater"]; ok {
		t.Fatalf("allowWater should only be sent when true, got %v", gotBody["allowWater"])
	}
}

func TestRouteSurfacesTheServicesRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"message":"Could not resolve the 'from' endpoint: 'Atlantis' is not a place in this world."}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL).Route(context.Background(), map[string]interface{}{}, "Atlantis", "Aditarie", "", false)
	if err == nil {
		t.Fatal("expected an error for a rejected request")
	}
	// The service's own explanation is what makes the failure actionable, so it has to survive.
	if !strings.Contains(err.Error(), "Atlantis") {
		t.Fatalf("error should carry the service's message, got %v", err)
	}
}

func TestUnconfiguredServiceFailsClearly(t *testing.T) {
	_, err := New("").World(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error when no URL is configured")
	}
	if !strings.Contains(err.Error(), "DM_VELLARIS_URL") {
		t.Fatalf("error should name the setting that fixes it, got %v", err)
	}
}

func TestBaseURLTrailingSlashIsTolerated(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL+"/").World(context.Background(), map[string]interface{}{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/world" {
		t.Fatalf("a trailing slash produced %q", gotPath)
	}
}
