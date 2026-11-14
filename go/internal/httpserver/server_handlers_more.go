package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	// A start runs in the background, so a restart can leave a table claimed by a start that nothing is running any
	// more. The load is the moment every client asks what to show, so it is also the moment to hand the lobby back.
	if startingPhase := party.GetParty(toMap(doc["gameSetup"])); strings.TrimSpace(fmt.Sprint(startingPhase["phase"])) == "starting" {
		gamesession.EnsureLobbyStart(ctx, s.GS, s.Hub, doc)
		_ = s.coll().FindOne(ctx, bson.M{"gameId": gid}).Decode(&doc)
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
		// The world step is run by a goroutine that a restart kills without clearing its stage. A load is the moment
		// every client asks what to show, so it is also the moment to notice nothing is left running the step.
		lang := "English"
		if gs, ok := doc["gameSetup"].(map[string]interface{}); ok {
			if l, ok := gs["language"].(string); ok && strings.TrimSpace(l) != "" {
				lang = strings.TrimSpace(l)
			}
		}
		gamesession.EnsureLobbyWorld(ctx, s.GS, doc, lang)
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
	// The creative seed is the DM's steering draw for this run, not player-facing lore, so it stays redacted for
	// everyone but the host — who uses the campaign background view to see what the campaign was written from.
	if owner, _ := o["viewerIsGameOwner"].(bool); owner {
		if rawSpec, ok := doc["campaignSpec"].(map[string]interface{}); ok && rawSpec != nil {
			if seed, ok := rawSpec["creativeSeed"]; ok && seed != nil {
				if cs, ok := o["campaignSpec"].(map[string]interface{}); ok && cs != nil {
					cs["creativeSeed"] = seed
				}
			}
		}
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
		"rawModelRequest": 1, "rawModelOutput": 1, "systemCore": 1, "lastStageError": 1,
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
		"rawModelRequest": doc["rawModelRequest"],
		"rawModelOutput":  doc["rawModelOutput"],
		"systemCore":      doc["systemCore"],
		"lastStageError":  doc["lastStageError"],
		"campaignSpec":    doc["campaignSpec"],
		"gameSetup":       doc["gameSetup"],
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
		"conversation":                 bson.A{},
		"summaryConversation":          bson.A{},
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
	// The table's world is not a character's business: it starts now, so the lobby opens on a map being written while
	// the players pick what they will be.
	gamesession.PrepareWorldInBackground(s.GS, gameID, lang)
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
	readyUserIds := party.NormalizeReadyUserIDsArray(party.ListValues(p["readyUserIds"]))
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

// handlePartyCharacterBusy records that a player is rolling a character sheet. Only that player's browser knows a roll
// is in flight — the sheet itself appears only when it finishes — so the table learns it from this short-lived marker,
// which the hub then pushes to every open lobby.
func (s *Server) handlePartyCharacterBusy(w http.ResponseWriter, r *http.Request, uid string) {
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
	busy := body["busy"] == true || body["busy"] == "true"
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
	now := time.Now().UnixMilli()
	stamps := party.CharacterBusyUntil(party.GetParty(toMap(doc["gameSetup"])))
	if busy {
		stamps[uidNorm] = now + party.CharacterBusyTTLMs
	} else {
		delete(stamps, uidNorm)
	}
	for id, v := range stamps {
		if ms, ok := v.(int64); !ok || ms <= now {
			delete(stamps, id)
		}
	}
	nextSetup := party.MergeParty(toMap(doc["gameSetup"]), map[string]interface{}{"characterBusyUntil": stamps})
	_, _ = s.coll().UpdateOne(ctx, bson.M{"gameId": gid}, bson.M{"$set": bson.M{"gameSetup": nextSetup}})
	s.Hub.NotifyGameStateUpdated(gid)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "characterBusyUntil": stamps})
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
	err := s.coll().FindOne(ctx, bson.M{"gameId": gid}, options.FindOne().SetProjection(bson.M{"ownerUserId": 1, "gameId": 1, "campaignSpec.vellarisMapId": 1})).Decode(&doc)
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
	// The map service keeps this game's map; the game is gone, so nothing can reference it any more.
	spec, _ := doc["campaignSpec"].(map[string]interface{})
	gamesession.DeleteStoredMapForGame(ctx, s.GS, spec)
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
	st, out := gamesession.HandleGenerateCharacter(r.Context(), s.GS, s.Cfg, s.Hub, uid, body)
	if s.Cfg.DebugCharacterFlow {
		gsIn, _ := body["gameSetup"].(map[string]interface{})
		log.Printf("[GMAI character:generate.request] userId=%s gameId=%v newParty=%v preassignedDisplayName=%v gameSetup=%s",
			uid, body["gameId"], body["newParty"], body["preassignedDisplayName"], summarizeGameSetupForDebug(gsIn))
	}
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
	rawGS, _ := body["gameSetup"].(map[string]interface{})
	if rawGS == nil {
		rawGS = map[string]interface{}{}
	}
	originalName := strings.TrimSpace(fmt.Sprint(rawGS["name"]))
	if strings.EqualFold(originalName, "random") {
		originalName = ""
	}
	var randomIntentName bool
	if ri, ok := rawGS["randomIntent"].(map[string]interface{}); ok && ri != nil {
		if b, ok := ri["name"].(bool); ok && b {
			randomIntentName = true
		}
	}
	var nameIsRandom bool
	if randomIntentName {
		nameIsRandom = true
		originalName = ""
	} else {
		nameIsRandom = originalName == ""
	}

	gs := gamesession.ResolveCharacterSetupForGeneration(rawGS)
	if s.Cfg.DebugCharacterFlow {
		gidStr := ""
		if gameID != nil {
			gidStr = *gameID
		}
		log.Printf("[GMAI character:preview-name.request] userId=%s gameId=%s newParty=%v namesEnabled=%v gameSetup=%s",
			uid, gidStr, wantsNew, ironarachne.NamesEnabled(), summarizeGameSetupForDebug(gs))
	}
	if !nameIsRandom {
		gs["name"] = originalName
		gamesession.MergeRandomIntentNameFlag(gs, false)
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": "OK", "resolvedGameSetup": gs})
		return
	}

	if !ironarachne.NamesEnabled() {
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": "IRON_NAMES_DISABLED", "resolvedGameSetup": gs})
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
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": code, "resolvedGameSetup": gs})
		return
	}
	gs["name"] = name
	gamesession.MergeRandomIntentNameFlag(gs, true)
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": "OK", "resolvedGameSetup": gs})
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

// handleWorldMapImage streams the rendered map players are shown. The image belongs to one game and never changes,
// so it is cached privately per browser rather than served again on every open.
//
// ?full=1 answers the picture as rendered, for zooming; anything else answers the pre-scaled preview, which is a
// sixth of the bytes and enough at the fitted view.
func (s *Server) handleWorldMapImage(w http.ResponseWriter, r *http.Request, uid string) {
	gid := strings.TrimSpace(chi.URLParam(r, "gameId"))
	if gid == "" {
		writeJSONCoded(w, r, http.StatusBadRequest, "GAME_ID_REQUIRED", "gameId required")
		return
	}
	if _, err := gameaccess.AssertGameMember(r.Context(), s.coll(), uid, gid); err != nil {
		sendAccessError(w, r, err)
		return
	}
	full := r.URL.Query().Get("full") == "1"
	contentType, data, err := gamesession.LoadWorldMapImage(r.Context(), s.GS, gid, full)
	if err != nil {
		// A marker with no picture behind it is the one state nothing inside the game repairs: the world step only
		// re-renders a game whose marker is gone, so a stale marker would leave the table with a broken map forever.
		// Dropping it makes the next client reload hide the map instead of asking again, and lets the next campaign
		// step render the picture anew.
		if errors.Is(err, mongo.ErrNoDocuments) {
			res, uerr := s.coll().UpdateOne(r.Context(), bson.M{"gameId": gid},
				bson.M{"$unset": bson.M{"campaignSpec.mapImage": ""}})
			if uerr == nil && res.ModifiedCount > 0 {
				log.Printf("campaign map for gameId=%s: the game carried a map marker but no picture was stored; dropped the marker so the map is rendered again", gid)
				s.Hub.NotifyGameStateUpdated(gid)
			}
		}
		writeJSONCoded(w, r, http.StatusNotFound, "WORLD_MAP_IMAGE_MISSING", "This game has no rendered map")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	// The client keys the URL on campaignSpec.mapImage.updatedAt, so a stored picture is never stale under one URL.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleWorldMapJSON serves the stored world as a JSON download for the table owner.
func (s *Server) handleWorldMapJSON(w http.ResponseWriter, r *http.Request, uid string) {
	gid := strings.TrimSpace(chi.URLParam(r, "gameId"))
	payload, st, code, problem := gamesession.WorldJSONFor(r.Context(), s.GS, uid, gid)
	if st != http.StatusOK {
		writeJSONCoded(w, r, st, code, problem)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"map-%s.json\"", gid))
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func (s *Server) handleImportWorld(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	st, out := gamesession.HandleImportVellarisWorld(r.Context(), s.GS, uid, body)
	writeJSON(w, st, localizeIfCoded(r, out))
}
func (s *Server) handleMeasureTravel(w http.ResponseWriter, r *http.Request, uid string) {
	var body map[string]interface{}
	if !readJSON(w, r, &body) {
		return
	}
	st, out := gamesession.HandleVellarisRoute(r.Context(), s.GS, uid, body)
	writeJSON(w, st, localizeIfCoded(r, out))
}
