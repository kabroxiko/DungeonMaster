package realtime

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Hub mirrors server/services/gameStateSseHub.js (SSE + WebSocket fan-out with debounce).
type Hub struct {
	mu sync.Mutex

	sseByGame map[string]map[*sseClient]struct{}
	wsByGame  map[string]map[*wsEntry]struct{}

	debounce map[string]*time.Timer
}

type sseClient struct {
	w http.ResponseWriter
	f http.Flusher
}

type wsEntry struct {
	conn *websocket.Conn
}

type Server struct {
	Hub *Hub
}

func NewHub() *Hub {
	return &Hub{
		sseByGame: make(map[string]map[*sseClient]struct{}),
		wsByGame:  make(map[string]map[*wsEntry]struct{}),
		debounce:  make(map[string]*time.Timer),
	}
}

const debounceMs = 150
const pingInterval = 28 * time.Second

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// AttachSSE registers an SSE client; returns cleanup.
func (h *Hub) AttachSSE(gameID string, w http.ResponseWriter, _ string) func() {
	gid := trim(gameID)
	if gid == "" {
		return func() {}
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		return func() {}
	}
	c := &sseClient{w: w, f: fl}

	h.mu.Lock()
	bucket, has := h.sseByGame[gid]
	if !has {
		bucket = make(map[*sseClient]struct{})
		h.sseByGame[gid] = bucket
	}
	bucket[c] = struct{}{}
	h.mu.Unlock()

	tick := time.NewTicker(pingInterval)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-tick.C:
				h.mu.Lock()
				if _, ok := h.sseByGame[gid]; !ok {
					h.mu.Unlock()
					return
				}
				h.mu.Unlock()
				_, _ = w.Write([]byte(": ping\n\n"))
				fl.Flush()
			case <-done:
				tick.Stop()
				return
			}
		}
	}()

	return func() {
		close(done)
		h.mu.Lock()
		defer h.mu.Unlock()
		if bucket := h.sseByGame[gid]; bucket != nil {
			delete(bucket, c)
			if len(bucket) == 0 {
				delete(h.sseByGame, gid)
			}
		}
	}
}

// AttachWebSocket registers a websocket.
func (h *Hub) AttachWebSocket(gameID, userID string, conn *websocket.Conn) func() {
	gid := trim(gameID)
	uid := trim(userID)
	if gid == "" || uid == "" {
		return func() {}
	}
	e := &wsEntry{conn: conn}
	h.mu.Lock()
	bucket, ok := h.wsByGame[gid]
	if !ok {
		bucket = make(map[*wsEntry]struct{})
		h.wsByGame[gid] = bucket
	}
	bucket[e] = struct{}{}
	h.mu.Unlock()

	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if bucket := h.wsByGame[gid]; bucket != nil {
			delete(bucket, e)
			if len(bucket) == 0 {
				delete(h.wsByGame, gid)
			}
		}
	}
}

// NotifyGameStateUpdated debounces flush per gameId.
func (h *Hub) NotifyGameStateUpdated(gameID string) {
	gid := trim(gameID)
	if gid == "" {
		return
	}
	h.mu.Lock()
	if prev := h.debounce[gid]; prev != nil {
		prev.Stop()
	}
	h.debounce[gid] = time.AfterFunc(time.Duration(debounceMs)*time.Millisecond, func() {
		h.flushNotify(gid)
	})
	h.mu.Unlock()
}

func (h *Hub) flushNotify(gid string) {
	h.mu.Lock()
	delete(h.debounce, gid)
	payloadBytes, _ := json.Marshal(map[string]string{"type": "game-state-updated", "gameId": gid})
	payload := string(payloadBytes)
	line := "event: message\ndata: " + payload + "\n\n"

	sseBuckets := h.sseByGame[gid]
	var sseCopy []*sseClient
	for c := range sseBuckets {
		sseCopy = append(sseCopy, c)
	}
	wsBuckets := h.wsByGame[gid]
	var wsCopy []*wsEntry
	for e := range wsBuckets {
		wsCopy = append(wsCopy, e)
	}
	h.mu.Unlock()

	for _, c := range sseCopy {
		_, err := c.w.Write([]byte(line))
		if err != nil {
			h.detachSSE(gid, c)
		} else {
			c.f.Flush()
		}
	}
	for _, e := range wsCopy {
		_ = e.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := e.conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
			h.detachWS(gid, e)
		}
	}
}

func (h *Hub) detachSSE(gid string, c *sseClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if bucket := h.sseByGame[gid]; bucket != nil {
		delete(bucket, c)
		if len(bucket) == 0 {
			delete(h.sseByGame, gid)
		}
	}
}

func (h *Hub) detachWS(gid string, e *wsEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if bucket := h.wsByGame[gid]; bucket != nil {
		delete(bucket, e)
		if len(bucket) == 0 {
			delete(h.wsByGame, gid)
		}
	}
}

// CloseWebSockets closes every active WebSocket so HTTP handlers can return quickly during server shutdown.
func (h *Hub) CloseWebSockets() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, bucket := range h.wsByGame {
		for e := range bucket {
			if e != nil && e.conn != nil {
				_ = e.conn.Close()
			}
		}
	}
	h.wsByGame = make(map[string]map[*wsEntry]struct{})
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}
