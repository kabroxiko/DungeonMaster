package vellaris

import (
	"context"
	"encoding/json"
	"errors"
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

// A stored world is asked for with the persist flag, and the token that lets the service accept it travels with the
// request. The answer carries the id every later call uses.
func TestWorldStoredAsksForPersistenceAndSendsTheToken(t *testing.T) {
	var gotPath, gotQuery, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotToken = r.Header.Get("X-Vellaris-Token")
		_, _ = w.Write([]byte(`{"success":true,"mapId":"507f1f77bcf86cd799439011","world":{},"settings":{}}`))
	}))
	defer srv.Close()

	answer, err := NewWithToken(srv.URL, "secret").WorldStored(context.Background(), map[string]interface{}{"randomSeed": 7.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/world" || gotQuery != "persist=true" {
		t.Fatalf("expected POST /api/world?persist=true, got %s?%s", gotPath, gotQuery)
	}
	if gotToken != "secret" {
		t.Fatalf("the shared secret was not sent, got %q", gotToken)
	}
	if answer["mapId"] != "507f1f77bcf86cd799439011" {
		t.Fatalf("the stored id must reach the caller, got %v", answer["mapId"])
	}
}

// Reading a stored world and its image goes through the id, so neither call has to carry the settings again.
func TestStoredWorldAndImageUseTheMapIdEndpoint(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/map") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpegbytes"))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"mapId":"abc","world":{"places":[]},"settings":{"randomSeed":1}}`))
	}))
	defer srv.Close()

	client := NewWithToken(srv.URL, "secret")
	stored, err := client.StoredWorld(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored["settings"] == nil {
		t.Fatalf("the stored settings must come back, got %v", stored)
	}
	data, contentType, err := client.StoredWorldImage(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "jpegbytes" || contentType != "image/jpeg" {
		t.Fatalf("unexpected image answer: %q %q", contentType, data)
	}
	if len(paths) != 2 || paths[0] != "/api/maps/abc" || paths[1] != "/api/maps/abc/map" {
		t.Fatalf("unexpected paths: %v", paths)
	}
}

func TestRouteStoredSendsOnlyTheEndpoints(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"success":true,"reachable":true,"distance":12}`))
	}))
	defer srv.Close()

	if _, err := NewWithToken(srv.URL, "secret").RouteStored(context.Background(), "abc", "Azureenci", "Aditarie", "", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/maps/abc/route" {
		t.Fatalf("expected the stored route endpoint, got %s", gotPath)
	}
	if _, has := gotBody["randomSeed"]; has {
		t.Fatalf("stored routes must not resend settings, got %v", gotBody)
	}
	from, _ := gotBody["from"].(map[string]interface{})
	if from["name"] != "Azureenci" {
		t.Fatalf("from was not sent as a named endpoint, got %v", gotBody["from"])
	}
}

func TestDeleteStoredSendsADelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	if err := NewWithToken(srv.URL, "secret").DeleteStored(context.Background(), "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/maps/abc" {
		t.Fatalf("expected DELETE /api/maps/abc, got %s %s", gotMethod, gotPath)
	}
}

// A refusal carries its status so the caller can tell "this service cannot store maps" from a real failure.
func TestRefusalCarriesTheStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"success":false,"message":"not configured to store maps"}`))
	}))
	defer srv.Close()

	_, err := NewWithToken(srv.URL, "secret").WorldStored(context.Background(), map[string]interface{}{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected a 503 APIError, got %v", err)
	}
	if !strings.Contains(apiErr.Message, "not configured") {
		t.Fatalf("the service's explanation must survive, got %q", apiErr.Message)
	}
}
