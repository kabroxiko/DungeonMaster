// Package vellaris is a client for the Vellaris map service.
//
// The service accepts two shapes of settings document and ignores fields it does not know. The short one names only the
// choices the service's own generator panel offers - dimension, world size, land shape, region count, city frequency, art
// pack, city icon set and books - and describes a kind of map: /api/world and /api/world/map expand it into a full map.
// The full one is what /api/world answers with: it carries every rendering setting and the seeds, so it names one
// specific world. Keep the full document for a campaign and send it to /api/route or /api/world/places, which need a
// world that already exists.
package vellaris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a Vellaris instance.
type Client struct {
	BaseURL string
	// Token is the shared secret for the service's stored-map routes, when the deployment requires one (DM_VELLARIS_TOKEN).
	Token string
	HTTP  *http.Client
}

// New returns a client for baseURL. An empty baseURL is allowed and makes every call fail with a clear error,
// so a deployment that has not configured the service still starts.
func New(baseURL string) *Client {
	return NewWithToken(baseURL, "")
}

// NewWithToken returns a client that authenticates its stored-map calls with token. An empty token is allowed and
// sends no header, which is what a service without a shared secret expects.
func NewWithToken(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Token:   strings.TrimSpace(token),
		// World generation on a large map is a multi-second render, and the first request after boot is slower.
		HTTP: &http.Client{Timeout: 180 * time.Second},
	}
}

// APIError is an answer the service refused with, carrying the status code so a caller can tell "not configured to
// store maps" (503) from a real failure instead of treating every refusal the same way.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("vellaris: returned %d: %s", e.StatusCode, e.Message)
}

// World generates the world for these settings and returns the service's parsed answer.
//
// The answer carries both the world description and the canonical settings, and the caller keeps both: the world
// is what the DM reads, and the settings are the handle used to ask about journeys later.
func (c *Client) World(ctx context.Context, settings map[string]interface{}) (map[string]interface{}, error) {
	return c.do(ctx, http.MethodPost, "/api/world", settings)
}

// WorldStored generates the world and asks the service to store it, so later calls can name it by id instead of
// sending its settings again. The answer carries "mapId" when the service stored it.
func (c *Client) WorldStored(ctx context.Context, settings map[string]interface{}) (map[string]interface{}, error) {
	return c.do(ctx, http.MethodPost, "/api/world?persist=true", settings)
}

// StoredWorld returns a world the service stored, with the canonical settings that produce it.
func (c *Client) StoredWorld(ctx context.Context, mapID string) (map[string]interface{}, error) {
	return c.do(ctx, http.MethodGet, "/api/maps/"+url.PathEscape(strings.TrimSpace(mapID)), nil)
}

// StoredWorldImage returns the rendered image of a stored world.
func (c *Client) StoredWorldImage(ctx context.Context, mapID string) ([]byte, string, error) {
	return c.doImage(ctx, http.MethodGet, "/api/maps/"+url.PathEscape(strings.TrimSpace(mapID))+"/map", nil)
}

// DeleteStored removes a stored world once nothing needs it. A map that is already gone is not an error here.
func (c *Client) DeleteStored(ctx context.Context, mapID string) error {
	_, err := c.do(ctx, http.MethodDelete, "/api/maps/"+url.PathEscape(strings.TrimSpace(mapID)), nil)
	return err
}

// WorldMap renders the world described by settings and returns the map image bytes and their media type.
//
// The service accepts the same settings document as /api/world, so passing the canonical settings that came back
// from World renders exactly the world the players will be shown.
func (c *Client) WorldMap(ctx context.Context, settings map[string]interface{}) ([]byte, string, error) {
	return c.doImage(ctx, http.MethodPost, "/api/world/map", settings)
}

// Route measures one journey in an existing world.
func (c *Client) Route(ctx context.Context, settings map[string]interface{}, from, to, mode string, allowWater bool) (map[string]interface{}, error) {
	return c.do(ctx, http.MethodPost, "/api/route", routeBody(settings, from, to, mode, allowWater))
}

// RouteStored measures one journey in a stored world, without sending its settings again.
func (c *Client) RouteStored(ctx context.Context, mapID, from, to, mode string, allowWater bool) (map[string]interface{}, error) {
	body := routeBody(nil, from, to, mode, allowWater)
	return c.do(ctx, http.MethodPost, "/api/maps/"+url.PathEscape(strings.TrimSpace(mapID))+"/route", body)
}

// Place is one named mark to add to a world, in the resolution-invariant coordinates the world reports.
type Place struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// AddPlaces marks places on a stored world and answers with the world as it now reads, including the new names.
//
// The service re-renders the map from the same seeds and replaces its stored copy, so the world keeps its id and
// everything already drawn stays exactly where it was.
func (c *Client) AddPlaces(ctx context.Context, mapID string, places []Place) (map[string]interface{}, error) {
	body := map[string]interface{}{"places": places}
	return c.do(ctx, http.MethodPost, "/api/maps/"+url.PathEscape(strings.TrimSpace(mapID))+"/places", body)
}

// AddPlacesToWorld marks places on a world the caller describes by settings, for a deployment whose map service
// keeps no database. The answer carries the updated world, the canonical settings to keep, and the re-rendered map
// image as imageBase64.
func (c *Client) AddPlacesToWorld(ctx context.Context, settings map[string]interface{}, places []Place) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	for k, v := range settings {
		body[k] = v
	}
	body["places"] = places
	return c.do(ctx, http.MethodPost, "/api/world/places", body)
}

// routeBody builds the endpoint fields every route call carries; settings are copied in when the caller has them.
func routeBody(settings map[string]interface{}, from, to, mode string, allowWater bool) map[string]interface{} {
	body := map[string]interface{}{}
	for k, v := range settings {
		body[k] = v
	}
	body["from"] = map[string]interface{}{"name": from}
	body["to"] = map[string]interface{}{"name": to}
	if mode != "" {
		body["mode"] = mode
	}
	if allowWater {
		body["allowWater"] = true
	}
	return body
}

func (c *Client) do(ctx context.Context, method, path string, body map[string]interface{}) (map[string]interface{}, error) {
	res, raw, err := c.send(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("vellaris: %s returned %d with unreadable JSON: %w", path, res.StatusCode, err)
	}
	if res.StatusCode != http.StatusOK {
		message, _ := out["message"].(string)
		if message == "" {
			message = truncate(string(raw), 200)
		}
		return nil, &APIError{StatusCode: res.StatusCode, Message: message}
	}
	if success, ok := out["success"].(bool); ok && !success {
		message, _ := out["message"].(string)
		return nil, fmt.Errorf("vellaris: %s failed: %s", path, message)
	}
	return out, nil
}

// send performs one request and returns the response with its body already read.
func (c *Client) send(ctx context.Context, method, path string, body map[string]interface{}) (*http.Response, []byte, error) {
	if c.BaseURL == "" {
		return nil, nil, fmt.Errorf("vellaris: no service URL configured (set DM_VELLARIS_URL)")
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("vellaris: encoding request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, nil, fmt.Errorf("vellaris: building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.applyToken(req)

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 180 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("vellaris: calling %s: %w", path, err)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		res.Body.Close()
		return nil, nil, fmt.Errorf("vellaris: reading %s: %w", path, err)
	}
	return res, raw, nil
}

func (c *Client) applyToken(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("X-Vellaris-Token", c.Token)
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// doImage calls an endpoint that answers with image bytes rather than JSON, and returns the bytes together with the
// media type the service reported.
func (c *Client) doImage(ctx context.Context, method, path string, body map[string]interface{}) ([]byte, string, error) {
	res, raw, err := c.send(ctx, method, path, body)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()

	contentType := strings.TrimSpace(res.Header.Get("Content-Type"))
	if res.StatusCode != http.StatusOK {
		// The failure body is JSON when the service rejected the settings, and plain text when the render failed.
		var out map[string]interface{}
		message := ""
		if json.Unmarshal(raw, &out) == nil {
			message, _ = out["message"].(string)
		}
		if message == "" {
			message = truncate(string(raw), 200)
		}
		return nil, "", &APIError{StatusCode: res.StatusCode, Message: message}
	}
	if len(raw) == 0 {
		return nil, "", fmt.Errorf("vellaris: %s returned no image bytes", path)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("vellaris: %s answered %q instead of an image", path, contentType)
	}
	return raw, contentType, nil
}
