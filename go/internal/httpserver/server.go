package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/api/idtoken"

	"github.com/deckofdmthings/gmai/internal/auth"
	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/gamesession"
	"github.com/deckofdmthings/gmai/internal/i18n"
	"github.com/deckofdmthings/gmai/internal/realtime"
	"github.com/deckofdmthings/gmai/internal/store"
)

// Server is the HTTP API (Node httpApp + routes).
type Server struct {
	Cfg  *config.Config
	DB   *mongo.Database
	Hub  *realtime.Hub
	GS   *gamesession.Deps
}

func (s *Server) coll() *mongo.Collection {
	return store.GameStates(s.DB)
}

func (s *Server) users() *mongo.Collection {
	return store.Users(s.DB)
}

// Handler returns the root HTTP handler (CORS + /api routes + optional WS upgrade).
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	allowed := buildAllowedOrigins(s.Cfg)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			origin := req.Header.Get("Origin")
			if s.Cfg.NodeEnv == "development" || s.Cfg.NodeEnv == "dev" || s.Cfg.NodeEnv == "test" {
				if origin != "" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
			} else {
				if origin == "" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if containsOrigin(allowed, origin) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else {
					w.Header().Set("Access-Control-Allow-Origin", s.Cfg.FrontendURL)
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Last-Event-ID, X-DM-Language")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	r.Get("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"api":          "gmai",
			"deployMarker": "2026-03-31-generate-character-no-campaign-gate",
		})
	})

	r.Get("/api/meta/character-options", func(w http.ResponseWriter, r *http.Request) {
		loc := i18n.LocaleFromRequest(r)
		writeJSON(w, http.StatusOK, i18n.CharacterOptionsForLocale(loc))
	})

	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/google", s.handleGoogle)
		r.Post("/join", s.withAuthBearer(s.handleJoin))
		r.Patch("/nickname", s.withAuthBearer(s.handleNickname))
		r.Get("/me", s.withAuthBearer(s.handleMe))
	})

	r.Route("/api/game-state", func(r chi.Router) {
		r.Post("/append-player-message", s.withAuthBearer(s.handleAppendPlayer))
		r.Get("/events/{gameId}", s.handleSSE)
		r.Get("/ws/{gameId}", s.handleGameStateWS)
		r.Get("/load/{gameId}", s.withAuthBearer(s.handleLoad))
		r.Get("/debug/{gameId}/prompts", s.withAuthBearer(s.handleDebugPrompts))
		r.Post("/create-party", s.withAuthBearer(s.handleCreateParty))
		r.Post("/party-ready", s.withAuthBearer(s.handlePartyReady))
		r.Patch("/party-premise", s.withAuthBearer(s.handlePartyPremise))
		r.Get("/mine", s.withAuthBearer(s.handleMine))
		r.Delete("/mine/{gameId}", s.withAuthBearer(s.handleDeleteMine))
	})

	r.Route("/api/game-session", func(r chi.Router) {
		r.Post("/generate", s.withAuthBearer(s.handleGenerate))
		r.Post("/create-invite", s.withAuthBearer(s.handleCreateInvite))
		r.Post("/bootstrap-session", s.withAuthBearer(s.handleBootstrap))
		r.Post("/start-party-adventure", s.withAuthBearer(s.handleStartParty))
		r.Post("/generate-campaign", s.withAuthBearer(s.handleGenerateCampaign))
		r.Post("/generate-campaign-core", s.withAuthBearer(s.handleGenerateCampaignCore))
		r.Post("/preview-character-name", s.withAuthBearer(s.handlePreviewCharacterName))
		r.Post("/generate-character", s.withAuthBearer(s.handleGenerateCharacter))
	})

	return r
}

func buildAllowedOrigins(c *config.Config) []string {
	seen := map[string]struct{}{}
	add := func(u string) {
		u = strings.TrimSpace(strings.TrimRight(u, "/"))
		if u != "" {
			seen[u] = struct{}{}
		}
	}
	add(c.FrontendURL)
	add(c.PublicURL)
	var out []string
	for u := range seen {
		out = append(out, u)
	}
	return out
}

func containsOrigin(list []string, o string) bool {
	o = strings.TrimRight(strings.TrimSpace(o), "/")
	for _, x := range list {
		if strings.TrimRight(strings.TrimSpace(x), "/") == o {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSONCoded(w, r, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
		return false
	}
	return true
}

// --- Auth ---

func (s *Server) handleGoogle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDToken string `json:"idToken"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.IDToken) == "" {
		writeJSONCoded(w, r, http.StatusBadRequest, "ID_TOKEN_REQUIRED", "idToken required")
		return
	}
	aud := s.Cfg.GoogleClientID
	if aud == "" {
		writeJSONCoded(w, r, http.StatusServiceUnavailable, "AUTH_CONFIG", "Server missing DM_GOOGLE_CLIENT_ID")
		return
	}
	ctx := r.Context()
	payload, err := idtoken.Validate(ctx, body.IDToken, aud)
	if err != nil {
		writeJSONCoded(w, r, http.StatusUnauthorized, "GOOGLE_AUTH_FAILED", "Google sign-in failed")
		return
	}
	sub := payload.Subject
	email := ""
	if e, ok := payload.Claims["email"].(string); ok {
		email = strings.ToLower(strings.TrimSpace(e))
	}
	name := fmt.Sprint(payload.Claims["name"])
	picture := fmt.Sprint(payload.Claims["picture"])

	var u struct {
		ID       primitive.ObjectID `bson:"_id"`
		GoogleSub string            `bson:"googleSub"`
		Email    string              `bson:"email"`
		Name     string              `bson:"name"`
		Picture  string              `bson:"picture"`
		Nickname string              `bson:"nickname"`
	}
	err = s.users().FindOne(ctx, bson.M{"googleSub": sub}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		res, ierr := s.users().InsertOne(ctx, bson.M{
			"googleSub": sub,
			"email":     email,
			"name":      name,
			"picture":   picture,
			"nickname":  "",
		})
		if ierr != nil {
			writeJSONCoded(w, r, http.StatusInternalServerError, "GOOGLE_USER_SAVE_FAILED", "save failed")
			return
		}
		switch id := res.InsertedID.(type) {
		case primitive.ObjectID:
			u.ID = id
		default:
			writeJSONCoded(w, r, http.StatusInternalServerError, "GOOGLE_INSERT_ID_TYPE", "unexpected insert id type")
			return
		}
		u.GoogleSub = sub
		u.Email = email
		u.Name = name
		u.Picture = picture
	} else if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "GOOGLE_USER_DB_ERROR", "db error")
		return
	} else {
		_, _ = s.users().UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{"$set": bson.M{
			"email": email, "name": name, "picture": picture,
		}})
	}
	tok, err := auth.SignSessionToken(u.ID.Hex(), s.Cfg.JWTSecret, s.Cfg.NodeEnv)
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "SESSION_TOKEN_FAILED", "token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": tok,
		"user": map[string]interface{}{
			"_id":       u.ID.Hex(),
			"picture":   u.Picture,
			"nickname": strings.TrimSpace(u.Nickname),
		},
	})
}

// --- more handlers in server_handlers.go to avoid huge file ---
