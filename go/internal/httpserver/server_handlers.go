package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/deckofdmthings/gmai/internal/auth"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/gamesession"
	"github.com/deckofdmthings/gmai/internal/i18n"
	"github.com/deckofdmthings/gmai/internal/persist"
)

func (s *Server) withAuthBearer(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		if raw == "" || !strings.HasPrefix(raw, "Bearer ") {
			writeJSONCoded(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required")
			return
		}
		tok := strings.TrimSpace(raw[7:])
		sub, err := auth.VerifySessionToken(tok, s.Cfg.JWTSecret, s.Cfg.NodeEnv)
		if err != nil || sub == "" {
			writeJSONCoded(w, r, http.StatusUnauthorized, "AUTH_INVALID", "Invalid or expired session")
			return
		}
		next(w, r, sub)
	}
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request, uid string) {
	var body struct {
		InviteToken string `json:"inviteToken"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	tok := strings.TrimSpace(body.InviteToken)
	if tok == "" {
		writeJSONCoded(w, r, http.StatusBadRequest, "INVITE_TOKEN_REQUIRED", "inviteToken required")
		return
	}
	ctx := r.Context()
	var gs struct {
		ID              primitive.ObjectID   `bson:"_id"`
		GameID          string               `bson:"gameId"`
		OwnerUserID     primitive.ObjectID   `bson:"ownerUserId"`
		MemberUserIDs   []primitive.ObjectID `bson:"memberUserIds"`
		InviteToken     *string              `bson:"inviteToken"`
		InviteTokenCreatedAt *time.Time      `bson:"inviteTokenCreatedAt"`
	}
	err := s.coll().FindOne(ctx, bson.M{"inviteToken": tok}).Decode(&gs)
	if err == mongo.ErrNoDocuments {
		writeJSONCoded(w, r, http.StatusNotFound, "INVITE_INVALID", "Invalid or expired invite")
		return
	}
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "JOIN_FAILED", "Join failed")
		return
	}
	oid, err := primitive.ObjectIDFromHex(uid)
	if err != nil {
		writeJSONCoded(w, r, http.StatusBadRequest, "USER_INVALID", "Invalid user")
		return
	}
	if gameaccess.UserIsGameMember(uid, bsonToMap(gs)) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"gameId": gs.GameID, "alreadyMember": true})
		return
	}
	each := []primitive.ObjectID{}
	if !gs.OwnerUserID.IsZero() {
		each = append(each, gs.OwnerUserID)
	}
	each = append(each, oid)
	res, err := s.coll().UpdateOne(ctx, bson.M{
		"_id":         gs.ID,
		"inviteToken": tok,
		"memberUserIds": bson.M{"$nin": bson.A{oid}},
	}, bson.M{
		"$addToSet": bson.M{"memberUserIds": bson.M{"$each": each}},
		"$unset":    bson.M{"inviteToken": "", "inviteTokenCreatedAt": ""},
	})
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "JOIN_FAILED", "Join failed")
		return
	}
	if res.MatchedCount == 0 {
		var gs2 map[string]interface{}
		_ = s.coll().FindOne(ctx, bson.M{"_id": gs.ID}).Decode(&gs2)
		if gs2 != nil && gameaccess.UserIsGameMember(uid, gs2) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"gameId": gs.GameID, "alreadyMember": true})
			return
		}
		writeJSONCoded(w, r, http.StatusNotFound, "INVITE_INVALID", "Invalid or expired invite")
		return
	}
	s.Hub.NotifyGameStateUpdated(gs.GameID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"gameId": gs.GameID, "alreadyMember": false})
}

func bsonToMap(gs interface{}) map[string]interface{} {
	b, _ := bson.Marshal(gs)
	var m map[string]interface{}
	_ = bson.Unmarshal(b, &m)
	return m
}

func (s *Server) handleNickname(w http.ResponseWriter, r *http.Request, uid string) {
	var body struct {
		Nickname string `json:"nickname"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	n := strings.TrimSpace(body.Nickname)
	if len(n) < 1 || len(n) > 40 {
		writeJSONCoded(w, r, http.StatusBadRequest, "NICKNAME_INVALID", "Nickname must be between 1 and 40 characters.")
		return
	}
	oid, err := primitive.ObjectIDFromHex(uid)
	if err != nil {
		writeJSONCoded(w, r, http.StatusBadRequest, "USER_INVALID", "Invalid user")
		return
	}
	_, err = s.users().UpdateOne(r.Context(), bson.M{"_id": oid}, bson.M{"$set": bson.M{"nickname": n}})
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "NICKNAME_SAVE_FAILED", "Could not save nickname")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]interface{}{
			"_id":       uid,
			"picture":   "",
			"nickname":  n,
		},
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, uid string) {
	oid, err := primitive.ObjectIDFromHex(uid)
	if err != nil {
		writeJSONCoded(w, r, http.StatusBadRequest, "USER_INVALID", "Invalid user")
		return
	}
	var u struct {
		ID       primitive.ObjectID `bson:"_id"`
		Picture  string             `bson:"picture"`
		Nickname string             `bson:"nickname"`
	}
	err = s.users().FindOne(r.Context(), bson.M{"_id": oid}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		writeJSONCoded(w, r, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		return
	}
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "USER_LOAD_FAILED", "Failed to load user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]interface{}{
			"_id":       u.ID.Hex(),
			"picture":   u.Picture,
			"nickname":  strings.TrimSpace(u.Nickname),
		},
	})
}

func (s *Server) handleAppendPlayer(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	gid := strings.TrimSpace(fmt.Sprint(body["gameId"]))
	ctx := r.Context()
	res, err := gamesession.AppendPlayerUserMessageWithPartyRound(ctx, s.coll(), s.Hub, s.Cfg, s.GS, gid, body, uid)
	if err != nil {
		var he *gamesession.HTTPError
		if errors.As(err, &he) {
			loc := i18n.LocaleFromRequest(r)
			msg := i18n.APIError(he.Code, he.Msg, loc)
			writeJSON(w, he.Status, map[string]string{"error": msg, "code": he.Code})
			return
		}
		var pe *persist.PersistError
		if errors.As(err, &pe) {
			loc := i18n.LocaleFromRequest(r)
			msg := i18n.APIError(pe.Code, pe.Message, loc)
			writeJSON(w, pe.HTTP, map[string]string{"error": msg, "code": pe.Code})
			return
		}
		writeJSONCoded(w, r, http.StatusInternalServerError, "APPEND_MESSAGE_FAILED", "Failed to append player message")
		return
	}
	if err != nil {
		_ = err
	}
	writeJSON(w, http.StatusOK, res)
}
