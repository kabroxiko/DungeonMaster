// Package vellaris is a client for the Vellaris map service.
//
// The service takes a map-settings document as the body of every request and ignores fields it does not know,
// so the same document is passed to /api/world to generate a world and to /api/route to measure a journey in it.
package vellaris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a Vellaris instance.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for baseURL. An empty baseURL is allowed and makes every call fail with a clear error,
// so a deployment that has not configured the service still starts.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		// World generation on a large map is a multi-second render, and the first request after boot is slower.
		HTTP: &http.Client{Timeout: 180 * time.Second},
	}
}

// World generates the world for these settings and returns the service's parsed answer.
//
// The answer carries both the world description and the canonical settings, and the caller keeps both: the world
// is what the DM reads, and the settings are the handle used to ask about journeys later.
func (c *Client) World(ctx context.Context, settings map[string]interface{}) (map[string]interface{}, error) {
	return c.post(ctx, "/api/world", settings)
}

// WorldMap renders the world described by settings and returns the map image bytes and their media type.
//
// The service accepts the same settings document as /api/world, so passing the canonical settings that came back
// from World renders exactly the world the players will be shown.
func (c *Client) WorldMap(ctx context.Context, settings map[string]interface{}) ([]byte, string, error) {
	return c.postImage(ctx, "/api/world/map", settings)
}

// Route measures one journey in an existing world.
func (c *Client) Route(ctx context.Context, settings map[string]interface{}, from, to, mode string, allowWater bool) (map[string]interface{}, error) {
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
	return c.post(ctx, "/api/route", body)
}

func (c *Client) post(ctx context.Context, path string, body map[string]interface{}) (map[string]interface{}, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("vellaris: no service URL configured (set DM_VELLARIS_URL)")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("vellaris: encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("vellaris: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 180 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vellaris: calling %s: %w", path, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("vellaris: reading %s: %w", path, err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("vellaris: %s returned %d with unreadable JSON: %w", path, res.StatusCode, err)
	}
	if res.StatusCode != http.StatusOK {
		message, _ := out["message"].(string)
		if message == "" {
			message = truncate(string(raw), 200)
		}
		return nil, fmt.Errorf("vellaris: %s returned %d: %s", path, res.StatusCode, message)
	}
	if success, ok := out["success"].(bool); ok && !success {
		message, _ := out["message"].(string)
		return nil, fmt.Errorf("vellaris: %s failed: %s", path, message)
	}
	return out, nil
}

// postImage posts settings to an endpoint that answers with image bytes rather than JSON, and returns the bytes
// together with the media type the service reported.
func (c *Client) postImage(ctx context.Context, path string, body map[string]interface{}) ([]byte, string, error) {
	if c.BaseURL == "" {
		return nil, "", fmt.Errorf("vellaris: no service URL configured (set DM_VELLARIS_URL)")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("vellaris: encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, "", fmt.Errorf("vellaris: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 180 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("vellaris: calling %s: %w", path, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, "", fmt.Errorf("vellaris: reading %s: %w", path, err)
	}
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
		return nil, "", fmt.Errorf("vellaris: %s returned %d: %s", path, res.StatusCode, message)
	}
	if len(raw) == 0 {
		return nil, "", fmt.Errorf("vellaris: %s returned no image bytes", path)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("vellaris: %s answered %q instead of an image", path, contentType)
	}
	return raw, contentType, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
