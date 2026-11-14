package httpserver

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The map image and the event stream carry the session token in the query, so the request log must never echo it.
func TestRequestLogKeepsTheSessionTokenOut(t *testing.T) {
	var logged bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logged)
	defer log.SetOutput(previous)

	handler := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/game-state/map/1794679685209-903645a6fc0d?access_token=jwt-secret-value&v=1", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	line := logged.String()
	if strings.Contains(line, "jwt-secret-value") || strings.Contains(line, "access_token") {
		t.Fatalf("the request log carried the token: %s", line)
	}
	if !strings.Contains(line, "/api/game-state/map/1794679685209-903645a6fc0d") {
		t.Fatalf("the request log lost the path it exists to show: %s", line)
	}
}
