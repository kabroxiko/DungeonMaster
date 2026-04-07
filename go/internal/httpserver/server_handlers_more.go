package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/auth"
	"github.com/deckofdmthings/gmai/internal/draftparty"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/gamesession"
	"github.com/deckofdmthings/gmai/internal/ironarachne"
	"github.com/deckofdmthings/gmai/internal/party"
	"github.com/deckofdmthings/gmai/internal/persist"
)

// summarizeGameSetupForDebug logs a small subset of gameSetup (no full generatedCharacter dump).
func summarizeGameSetupForDebug(gs map[string]interface{}) string {
	if gs == nil {
		return "{}"
	}
	keys := []string{"name", "race", "class", "level", "gender", "subclass", "subrace", "background", "language"}
	var parts []string
	for _, k := range keys {
		if v, ok := gs[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func resolveBearerOrQueryToken(r *http.Request) string {
	raw := r.Header.Get("Authorization")
	if raw != "" && strings.HasPrefix(raw, "Bearer ") {
		return strings.TrimSpace(raw[7:])
	}
	return strings.TrimSpace(r.URL.Query().Get("access_token"))
}

func (s *Server) withAuthBearerOrQuery(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := resolveBearerOrQueryToken(r)
		if tok == "" {
			writeJSONCoded(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required")
			return
		}
		sub, err := auth.VerifySessionToken(tok, s.Cfg.JWTSecret, s.Cfg.NodeEnv)
		if err != nil || sub == "" {
			writeJSONCoded(w, r, http.StatusUnauthorized, "AUTH_INVALID", "Invalid or expired session")
			return
		}
		next(w, r, sub)
	}
}

func sendAccessError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, gameaccess.ErrGameNotFound) {
		writeJSONCoded(w, r, http.StatusNotFound, "GAME_NOT_FOUND", "Game not found")
		return
	}
	if errors.Is(err, gameaccess.ErrGameIDRequired) {
		writeJSONCoded(w, r, http.StatusBadRequest, "GAME_ID_REQUIRED", "gameId required")
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	s.withAuthBearerOrQuery(func(w http.ResponseWriter, r *http.Request, uid string) {
		gid := strings.TrimSpace(chi.URLParam(r, "gameId"))
		if gid == "" {
			writeJSONCoded(w, r, http.StatusBadRequest, "GAME_ID_REQUIRED", "gameId required")
			return
		}
		if _, err := gameaccess.AssertGameMember(r.Context(), s.coll(), uid, gid); err != nil {
			sendAccessError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte(": ok\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		detach := s.Hub.AttachSSE(gid, w, uid)
		defer detach()
		<-r.Context().Done()
	})(w, r)
}

func (s *Server) handleLoad(w http.ResponseWriter, r *http.Request, uid string) {
	gid := strings.TrimSpace(chi.URLParam(r, "gameId"))
	ctx := r.Context()
	if _, err := gameaccess.AssertGameMember(ctx, s.coll(), uid, gid); err != nil {
		sendAccessError(w, r, err)
		return
	}
	var doc map[string]interface{}
	err := s.coll().FindOne(ctx, bson.M{"gameId": gid}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		writeJSONCoded(w, r, http.StatusNotFound, "GAME_LOAD_EMPTY", "No game state found for this game")
		return
	}
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "GAME_LOAD_FAILED", "Failed to load game state")
		return
	}
	if party.IsLobbyParty(doc) {
		p0 := party.GetParty(toMap(doc["gameSetup"]))
		readyRaw, _ := p0["readyUserIds"].([]interface{})
		curR := party.NormalizeReadyUserIDsArray(readyRaw)
		var pruned []string
		for _, id := range curR {
			if party.MemberHasValidSheetForUserId(doc, id) {
				pruned = append(pruned, id)
			}
		}
		pruned = party.NormalizeReadyUserIDsArray(strSliceToIface(pruned))
		normCur := party.NormalizeReadyUserIDsArray(readyRaw)
		if !readySetsEqual(pruned, normCur) {
			healed := party.MergeParty(toMap(doc["gameSetup"]), map[string]interface{}{"readyUserIds": strSliceToIface(pruned)})
			_, _ = s.coll().UpdateOne(ctx, bson.M{"gameId": gid}, bson.M{"$set": bson.M{"gameSetup": healed}})
			s.Hub.NotifyGameStateUpdated(gid)
			_ = s.coll().FindOne(ctx, bson.M{"gameId": gid}).Decode(&doc)
		}
	}
	o := gamesession.GameStateDocForClient(doc)
	m := memberOIDStrings(doc)
	ownerStr := gameaccess.EffectiveGameOwnerIDStr(doc)
	if ownerStr == "" && len(m) > 0 {
		o["ownerUserId"] = m[0]
		ownerStr = m[0]
	}
	uidNorm := strings.TrimSpace(uid)
	if ownerStr != "" {
		o["viewerIsGameOwner"] = ownerStr == uidNorm
	} else if len(m) == 0 {
		o["viewerIsGameOwner"] = true
	} else {
		o["viewerIsGameOwner"] = m[0] == uidNorm
	}
	idStrs := map[string]struct{}{}
	if ownerStr != "" {
		idStrs[ownerStr] = struct{}{}
	}
	for _, x := range m {
		if strings.TrimSpace(x) != "" {
			idStrs[x] = struct{}{}
		}
	}
	nicks, err := loadMemberNicknames(ctx, s.users(), idStrs)
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "GAME_LOAD_FAILED", "Failed to load game state")
		return
	}
	o["memberNicknamesByUserId"] = nicks
	writeJSON(w, http.StatusOK, o)
}

func strSliceToIface(ss []string) []interface{} {
	var out []interface{}
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}

func readySetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ma := map[string]struct{}{}
	for _, x := range a {
		ma[x] = struct{}{}
	}
	for _, x := range b {
		if _, ok := ma[x]; !ok {
			return false
		}
	}
	return true
}

func toMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

func memberOIDStrings(doc map[string]interface{}) []string {
	var out []string
	if doc == nil {
		return out
	}
	if oid, ok := doc["ownerUserId"].(primitive.ObjectID); ok && !oid.IsZero() {
		out = append(out, oid.Hex())
	}
	if arr, ok := doc["memberUserIds"].(primitive.A); ok {
		for _, x := range arr {
			if id, ok := x.(primitive.ObjectID); ok && !id.IsZero() {
				out = append(out, id.Hex())
			}
		}
	}
	return out
}

func loadMemberNicknames(ctx context.Context, users *mongo.Collection, ids map[string]struct{}) (map[string]string, error) {
	var oids []primitive.ObjectID
	for s := range ids {
		if oid, err := primitive.ObjectIDFromHex(s); err == nil {
			oids = append(oids, oid)
		}
	}
	if len(oids) == 0 {
		return map[string]string{}, nil
	}
	cur, err := users.Find(ctx, bson.M{"_id": bson.M{"$in": oids}}, options.Find().SetProjection(bson.M{"nickname": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := map[string]string{}
	for cur.Next(ctx) {
		var row struct {
			ID       primitive.ObjectID `bson:"_id"`
			Nickname string             `bson:"nickname"`
		}
		if err := cur.Decode(&row); err != nil {
			continue
		}
		nn := strings.TrimSpace(row.Nickname)
		out[row.ID.Hex()] = nn
	}
	return out, nil
}

func (s *Server) handleDebugPrompts(w http.ResponseWriter, r *http.Request, uid string) {
	gid := strings.TrimSpace(chi.URLParam(r, "gameId"))
	ctx := r.Context()
	if _, err := gameaccess.AssertGameMember(ctx, s.coll(), uid, gid); err != nil {
		sendAccessError(w, r, err)
		return
	}
	var doc map[string]interface{}
	err := s.coll().FindOne(ctx, bson.M{"gameId": gid}, options.FindOne().SetProjection(bson.M{
		"rawModelRequest": 1, "rawModelOutput": 1, "systemCore": 1,
		"llmCallError": 1, "llmFallbackError": 1, "campaignSpec": 1, "gameSetup": 1,
		"llmCallEnteredAt": 1, "llmCallStartedAt": 1, "llmCallCompletedAt": 1,
		"llmCallFallbackAt": 1, "llmFallbackModel": 1, "llmFallbackAttemptedAt": 1,
		"llmFallbackSucceededAt": 1, "llmModelUsed": 1,
	})).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		writeJSONCoded(w, r, http.StatusNotFound, "GAME_STATE_MISSING", "No game state found")
		return
	}
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "GAME_DEBUG_FAILED", "Failed to load debug data")
		return
	}
	debug := map[string]interface{}{
		"rawModelRequest":  doc["rawModelRequest"],
		"rawModelOutput":   doc["rawModelOutput"],
		"systemCore":       doc["systemCore"],
		"campaignSpec":     doc["campaignSpec"],
		"gameSetup":        doc["gameSetup"],
		"diagnostics": map[string]interface{}{
			"llmCallEnteredAt":       doc["llmCallEnteredAt"],
			"llmCallStartedAt":       doc["llmCallStartedAt"],
			"llmCallCompletedAt":     doc["llmCallCompletedAt"],
			"llmCallError":           doc["llmCallError"],
			"llmCallFallbackAt":      doc["llmCallFallbackAt"],
			"llmFallbackModel":       doc["llmFallbackModel"],
			"llmFallbackAttemptedAt": doc["llmFallbackAttemptedAt"],
			"llmFallbackSucceededAt": doc["llmFallbackSucceededAt"],
			"llmFallbackError":       doc["llmFallbackError"],
			"llmModelUsed":           doc["llmModelUsed"],
		},
	}
	writeJSON(w, http.StatusOK, debug)
}

func (s *Server) handleCreateParty(w http.ResponseWriter, r *http.Request, uid string) {
	var body struct {
		Language string `json:"language"`
	}
	_ = readJSON(w, r, &body)
	lang := strings.TrimSpace(body.Language)
	if lang == "" {
		lang = "English"
	}
	oid, err := primitive.ObjectIDFromHex(uid)
	if err != nil {
		writeJSONCoded(w, r, http.StatusBadRequest, "INVALID_SESSION_USER_ID", "Invalid session user id")
		return
	}
	gameID := fmt.Sprintf("%d-%s", time.Now().UnixMilli(), randomHex(6))
	gameSetup := party.MergeParty(map[string]interface{}{"language": lang}, party.DefaultParty())
	createPayload := bson.M{
		"gameId":                       gameID,
		"ownerUserId":                  oid,
		"memberUserIds":                bson.A{oid},
		"gameSetup":                    gameSetup,
		"conversation":               bson.A{},
		"summaryConversation":        bson.A{},
		"summary":                      "",
		"totalTokenCount":              0,
		"userAndAssistantMessageCount": 0,
	}
	if draftparty.DraftPartyTTLMs() > 0 {
		if at := draftparty.DraftPartyExpiresAtFromNow(); at != nil {
			createPayload["draftPartyExpiresAt"] = at
		}
	}
	ctx := r.Context()
	_, err = s.coll().InsertOne(ctx, createPayload)
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "PARTY_CREATE_FAILED", "Could not create party")
		return
	}
	s.Hub.NotifyGameStateUpdated(gameID)
	var created map[string]interface{}
	_ = s.coll().FindOne(ctx, bson.M{"gameId": gameID}).Decode(&created)
	writeJSON(w, http.StatusCreated, gamesession.GameStateDocForClient(created))
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) handlePartyReady(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	gid := strings.TrimSpace(fmt.Sprint(body["gameId"]))
	if gid == "" {
		writeJSONCoded(w, r, http.StatusBadRequest, "GAME_ID_REQUIRED", "gameId required")
		return
	}
	ctx := r.Context()
	if _, err := gameaccess.AssertGameMember(ctx, s.coll(), uid, gid); err != nil {
		sendAccessError(w, r, err)
		return
	}
	ready := body["ready"] == true || body["ready"] == "true"
	uidNorm := party.NormalizeUserIDString(uid)
	var doc map[string]interface{}
	err := s.coll().FindOne(ctx, bson.M{"gameId": gid}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		writeJSONCoded(w, r, http.StatusNotFound, "GAME_NOT_FOUND", "Game not found")
		return
	}
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "DB_ERROR", "db error")
		return
	}
	if ready && !party.MemberHasValidSheetForUserId(doc, uidNorm) {
		writeJSON(w, http.StatusBadRequest, localizeStringMap(r, map[string]string{
			"error": "Your character must be saved on the server before you can mark ready. Finish character creation (generate) and try again.",
			"code":  "PARTY_READY_NEEDS_CHARACTER",
		}))
		return
	}
	p := party.GetParty(toMap(doc["gameSetup"]))
	rawReady, _ := p["readyUserIds"].([]interface{})
	readyUserIds := party.NormalizeReadyUserIDsArray(rawReady)
	if ready {
		found := false
		for _, x := range readyUserIds {
			if x == uidNorm {
				found = true
				break
			}
		}
		if !found {
			readyUserIds = append(readyUserIds, uidNorm)
		}
	} else {
		var next []string
		for _, x := range readyUserIds {
			if x != uidNorm {
				next = append(next, x)
			}
		}
		readyUserIds = next
	}
	nextSetup := party.MergeParty(toMap(doc["gameSetup"]), map[string]interface{}{"readyUserIds": strSliceToIface(readyUserIds)})
	_, _ = s.coll().UpdateOne(ctx, bson.M{"gameId": gid}, bson.M{"$set": bson.M{"gameSetup": nextSetup}})
	s.Hub.NotifyGameStateUpdated(gid)
	var fresh map[string]interface{}
	_ = s.coll().FindOne(ctx, bson.M{"gameId": gid}).Decode(&fresh)
	lean := fresh
	pNext := party.GetParty(toMap(lean["gameSetup"]))
	out := gamesession.GameStateDocForClient(fresh)
	out["partyReadyMeta"] = map[string]interface{}{
		"allMembersHaveSheets": party.AllMembersHaveValidSheets(lean),
		"allMembersReady":      party.AllMembersReady(pNext, lean),
		"readyUserIds":         readyUserIds,
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePartyPremise(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	gid := strings.TrimSpace(fmt.Sprint(body["gameId"]))
	if gid == "" {
		writeJSONCoded(w, r, http.StatusBadRequest, "GAME_ID_REQUIRED", "gameId required")
		return
	}
	ctx := r.Context()
	if _, err := gameaccess.AssertGameMember(ctx, s.coll(), uid, gid); err != nil {
		sendAccessError(w, r, err)
		return
	}
	hostPremise := ""
	if body["hostPremise"] != nil {
		s := fmt.Sprint(body["hostPremise"])
		if len(s) > 8000 {
			s = s[:8000]
		}
		hostPremise = s
	}
	var doc map[string]interface{}
	err := s.coll().FindOne(ctx, bson.M{"gameId": gid}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		writeJSONCoded(w, r, http.StatusNotFound, "GAME_NOT_FOUND", "Game not found")
		return
	}
	ownerStr := gameaccess.EffectiveGameOwnerIDStr(doc)
	if ownerStr == "" || !strings.EqualFold(ownerStr, uid) {
		writeJSONCoded(w, r, http.StatusForbidden, "NOT_OWNER_PREMISE", "Only the table owner can set the premise")
		return
	}
	nextSetup := party.MergeParty(toMap(doc["gameSetup"]), map[string]interface{}{"hostPremise": hostPremise})
	_, _ = s.coll().UpdateOne(ctx, bson.M{"gameId": gid}, bson.M{"$set": bson.M{"gameSetup": nextSetup}})
	s.Hub.NotifyGameStateUpdated(gid)
	var fresh map[string]interface{}
	_ = s.coll().FindOne(ctx, bson.M{"gameId": gid}).Decode(&fresh)
	writeJSON(w, http.StatusOK, gamesession.GameStateDocForClient(fresh))
}

func (s *Server) handleMine(w http.ResponseWriter, r *http.Request, uid string) {
	oid, err := primitive.ObjectIDFromHex(uid)
	if err != nil {
		writeJSONCoded(w, r, http.StatusBadRequest, "INVALID_SESSION_USER_ID", "Invalid session user id")
		return
	}
	ctx := r.Context()
	cur, err := s.coll().Find(ctx, bson.M{
		"$or": []interface{}{
			bson.M{"ownerUserId": oid},
			bson.M{"memberUserIds": oid},
		},
	}, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(200))
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "GAMES_LIST_FAILED", "Failed to load your games")
		return
	}
	defer cur.Close(ctx)
	var list []interface{}
	for cur.Next(ctx) {
		var g map[string]interface{}
		if err := cur.Decode(&g); err != nil {
			continue
		}
		if sum := gamesession.GameStateSummaryForMineList(g, uid); sum != nil {
			list = append(list, sum)
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleDeleteMine(w http.ResponseWriter, r *http.Request, uid string) {
	gid := strings.TrimSpace(chi.URLParam(r, "gameId"))
	if gid == "" {
		writeJSONCoded(w, r, http.StatusBadRequest, "GAME_ID_REQUIRED", "gameId required")
		return
	}
	ctx := r.Context()
	var doc map[string]interface{}
	err := s.coll().FindOne(ctx, bson.M{"gameId": gid}, options.FindOne().SetProjection(bson.M{"ownerUserId": 1, "gameId": 1})).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		writeJSONCoded(w, r, http.StatusNotFound, "GAME_NOT_FOUND", "Game not found")
		return
	}
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "GAME_DELETE_FAILED", "Failed to delete game")
		return
	}
	ownerStr := gameaccess.EffectiveGameOwnerIDStr(doc)
	if ownerStr == "" || !strings.EqualFold(ownerStr, uid) {
		writeJSONCoded(w, r, http.StatusForbidden, "NOT_OWNER_DELETE", "Only the host can delete this game")
		return
	}
	_, _ = s.coll().DeleteOne(ctx, bson.M{"gameId": gid})
	s.Hub.NotifyGameStateUpdated(gid)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "deleted": true, "gameId": gid})
}

func (s *Server) handleGenerateCampaign(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	st, out := gamesession.HandleGenerateCampaign(r.Context(), s.GS, uid, body, r.URL.Query().Get("gameId"))
	writeJSON(w, st, localizeIfCoded(r, out))
}

func (s *Server) handleGenerateCampaignCore(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	st, out := gamesession.HandleGenerateCampaignCore(r.Context(), s.GS, uid, body)
	writeJSON(w, st, localizeIfCoded(r, out))
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	st, out := gamesession.HandleDmGenerate(r.Context(), s.GS, s.Cfg, uid, body)
	if out == nil {
		out = &gamesession.HandleDmGenerateResult{}
	}
	if st == 404 {
		writeJSON(w, http.StatusNotFound, localizeStringMap(r, map[string]string{"error": out.Error, "code": "GAME_NOT_FOUND"}))
		return
	}
	if st >= 400 {
		payload := map[string]interface{}{"error": out.Error}
		if out.Code != "" {
			payload["code"] = out.Code
		}
		if out.RawPreview != "" {
			payload["rawPreview"] = out.RawPreview
		}
		writeJSON(w, st, localizeIfCoded(r, payload))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	ctx := r.Context()
	if gid := body["gameId"]; gid != nil && strings.TrimSpace(fmt.Sprint(gid)) != "" {
		g := strings.TrimSpace(fmt.Sprint(gid))
		var exists map[string]interface{}
		_ = s.coll().FindOne(ctx, bson.M{"gameId": g}, options.FindOne().SetProjection(bson.M{"_id": 1})).Decode(&exists)
		if exists != nil {
			if _, err := gameaccess.AssertGameMember(ctx, s.coll(), uid, g); err != nil {
				sendAccessError(w, r, err)
				return
			}
		}
	}
	gs, err := persist.PersistGameStateFromBody(ctx, s.coll(), s.Hub, body, uid, false)
	if err != nil {
		var pe *persist.PersistError
		st := http.StatusBadRequest
		code := ""
		msg := err.Error()
		if errors.As(err, &pe) {
			st = pe.HTTP
			code = pe.Code
			msg = pe.Message
		}
		payload := map[string]interface{}{"error": msg}
		if code != "" {
			payload["code"] = code
		}
		writeJSON(w, st, localizeIfCoded(r, payload))
		return
	}
	o := gamesession.GameStateDocForClient(gs)
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleStartParty(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	st, payload := gamesession.HandleStartPartyAdventure(r.Context(), s.GS, s.Cfg, s.Hub, uid, body)
	writeJSON(w, st, localizeIfCoded(r, payload))
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request, uid string) {
	var body struct {
		GameID string `json:"gameId"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	gid := strings.TrimSpace(body.GameID)
	if gid == "" {
		writeJSONCoded(w, r, http.StatusBadRequest, "GAME_ID_REQUIRED", "gameId required")
		return
	}
	ctx := r.Context()
	gs, err := gameaccess.AssertGameMember(ctx, s.coll(), uid, gid)
	if err != nil {
		sendAccessError(w, r, err)
		return
	}
	token := randomHex(24)
	update := bson.M{"$set": bson.M{"inviteToken": token, "inviteTokenCreatedAt": time.Now().UTC()}}
	if oid, ok := gs["ownerUserId"].(primitive.ObjectID); ok && !oid.IsZero() {
		update["$addToSet"] = bson.M{"memberUserIds": oid}
	}
	_, err = s.coll().UpdateOne(ctx, bson.M{"gameId": gid}, update)
	if err != nil {
		writeJSONCoded(w, r, http.StatusInternalServerError, "INVITE_CREATE_FAILED", "Could not create invite")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"inviteToken": token, "gameId": gid})
}

func (s *Server) handleGenerateCharacter(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	if s.Cfg.DebugCharacterFlow {
		gsIn, _ := body["gameSetup"].(map[string]interface{})
		log.Printf("[GMAI character:generate.request] userId=%s gameId=%v newParty=%v preassignedDisplayName=%v gameSetup=%s",
			uid, body["gameId"], body["newParty"], body["preassignedDisplayName"], summarizeGameSetupForDebug(gsIn))
	}
	st, out := gamesession.HandleGenerateCharacter(r.Context(), s.GS, s.Cfg, s.Hub, uid, body)
	writeJSON(w, st, localizeIfCoded(r, out))
}

// handlePreviewCharacterName mirrors POST /preview-character-name (Iron Arachne MUNA preview).
func (s *Server) handlePreviewCharacterName(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	var gameID *string
	if gidRaw := body["gameId"]; gidRaw != nil && strings.TrimSpace(fmt.Sprint(gidRaw)) != "" {
		s := strings.TrimSpace(fmt.Sprint(gidRaw))
		gameID = &s
	}
	wantsNew := body["newParty"] == true || body["newParty"] == "true"
	if gameID == nil && !wantsNew {
		writeJSON(w, http.StatusBadRequest, localizeIfCoded(r, map[string]interface{}{
			"error": "This request needs a gameId (join an existing party) or newParty: true (start a brand-new party on the server).",
			"code":  "GAME_ID_OR_NEW_PARTY_REQUIRED",
		}))
		return
	}
	ctx := r.Context()
	var gameRow map[string]interface{}
	if gameID != nil {
		doc, err := gameaccess.AssertGameMember(ctx, s.coll(), uid, *gameID)
		if err != nil {
			sendAccessError(w, r, err)
			return
		}
		gameRow = doc
	}
	gs, _ := body["gameSetup"].(map[string]interface{})
	if gs == nil {
		gs = map[string]interface{}{}
	}
	gs = gamesession.ResolveCharacterSetupForGeneration(gs)
	if s.Cfg.DebugCharacterFlow {
		gidStr := ""
		if gameID != nil {
			gidStr = *gameID
		}
		log.Printf("[GMAI character:preview-name.request] userId=%s gameId=%s newParty=%v namesEnabled=%v gameSetup=%s",
			uid, gidStr, wantsNew, ironarachne.NamesEnabled(), summarizeGameSetupForDebug(gs))
	}
	if !ironarachne.NamesEnabled() {
		writeJSON(w, http.StatusOK, map[string]interface{}{"name": nil, "code": "IRON_NAMES_DISABLED"})
		return
	}
	var gsSetup, campaignSpec, encounterState map[string]interface{}
	if gameRow != nil {
		gsSetup, _ = gameRow["gameSetup"].(map[string]interface{})
		campaignSpec, _ = gameRow["campaignSpec"].(map[string]interface{})
		encounterState, _ = gameRow["encounterState"].(map[string]interface{})
	}
	ok, name, reason := ironarachne.TryPreassignDisplayName(ctx, gs["race"], gs["gender"], gsSetup, campaignSpec, encounterState, uid)
	if s.Cfg.DebugCharacterFlow {
		log.Printf("[GMAI character:preview-name.outcome] ok=%v name=%q reason=%q", ok, name, reason)
	}
	if !ok {
		code := reason
		if code == "" {
			code = "UNAVAILABLE"
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"name": nil, "code": code, "resolvedGameSetup": gs})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"name": name, "code": "OK", "resolvedGameSetup": gs})
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) handleGameStateWS(w http.ResponseWriter, r *http.Request) {
	gid := strings.TrimSpace(chi.URLParam(r, "gameId"))
	tok := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if tok == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	sub, err := auth.VerifySessionToken(tok, s.Cfg.JWTSecret, s.Cfg.NodeEnv)
	if err != nil || sub == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if _, err := gameaccess.AssertGameMember(r.Context(), s.coll(), sub, gid); err != nil {
		if err == gameaccess.ErrGameNotFound {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	detach := s.Hub.AttachWebSocket(gid, sub, conn)
	defer detach()
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
