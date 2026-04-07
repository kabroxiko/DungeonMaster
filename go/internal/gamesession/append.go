package gamesession

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/party"
	"github.com/deckofdmthings/gmai/internal/pc"
	"github.com/deckofdmthings/gmai/internal/realtime"
)

// HTTPError is returned for HTTP-level failures from append helpers.
type HTTPError struct {
	Code   string
	Msg    string
	Status int
}

func (e *HTTPError) Error() string { return e.Msg }

// AppendPlayerUserMessageWithPartyRound mirrors gameStatePersist.js.
func AppendPlayerUserMessageWithPartyRound(ctx context.Context, coll *mongo.Collection, hub *realtime.Hub, cfg *config.Config, d *Deps, gameID string, body map[string]interface{}, userID string) (map[string]interface{}, error) {
	if userID == "" {
		return nil, fmt.Errorf("userId required")
	}
	gid := gameID
	if gid == "" {
		return nil, &HTTPError{Code: "GAME_ID_REQUIRED", Msg: "gameId is required", Status: http.StatusBadRequest}
	}
	content := str(body["content"])
	if strings.TrimSpace(content) == "" {
		return nil, &HTTPError{Code: "PLAYER_MESSAGE_CONTENT_REQUIRED", Msg: "content is required", Status: http.StatusBadRequest}
	}

	if _, err := gameaccess.AssertGameMember(ctx, coll, userID, gid); err != nil {
		if err == gameaccess.ErrGameNotFound {
			return nil, &HTTPError{Code: "GAME_NOT_FOUND", Msg: "Game not found", Status: http.StatusNotFound}
		}
		return nil, err
	}

	var doc map[string]interface{}
	if err := coll.FindOne(ctx, bson.M{"gameId": gid}).Decode(&doc); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, &HTTPError{Code: "GAME_NOT_FOUND", Msg: "Game not found", Status: http.StatusNotFound}
		}
		return nil, err
	}

	if !isMultiMemberParty(doc) {
		r, err := appendPlayerUserMessageForBroadcast(ctx, coll, hub, gid, body, userID)
		if err != nil {
			return nil, err
		}
		r["partyWait"] = false
		return r, nil
	}

	uidStr := userID
	gameSetup := toMap(doc["gameSetup"])
	fromSheet := pc.CharacterDisplayNameForUser(gameSetup, uidStr)
	hint := str(body["displayName"])
	userMsg := map[string]interface{}{
		"role":    "user",
		"content": content,
		"userId":  uidStr,
	}
	if fromSheet != "" {
		userMsg["displayName"] = fromSheet
	} else if strings.TrimSpace(hint) != "" {
		userMsg["displayName"] = strings.TrimSpace(hint)
	}

	conv := toIfaceSlice(doc["conversation"])
	sumConv := toIfaceSlice(doc["summaryConversation"])

	last := lastMsg(conv)
	if last != nil && str(last["role"]) == "user" && str(last["content"]) == content && str(last["userId"]) == uidStr {
		if hub != nil {
			hub.NotifyGameStateUpdated(gid)
		}
		required := party.CanonicalMemberIDStrings(doc)
		submitted := submittedIDs(gameSetup)
		partyWait := !everyMemberSubmitted(required, submitted)
		return map[string]interface{}{
			"ok":             true,
			"duplicate":      true,
			"partyWait":      partyWait,
			"partySubmitted": len(submitted),
			"partyRequired":  len(required),
		}, nil
	}

	conv, sumConv = stripTrailingUserTail(conv, sumConv, uidStr)
	conv = append(conv, userMsg)
	sumConv = append(sumConv, cloneMap(userMsg))

	submittedIDsSet := submittedIDs(gameSetup)
	submittedIDsSet[party.NormalizeUserIDString(uidStr)] = struct{}{}
	var subList []interface{}
	for id := range submittedIDsSet {
		subList = append(subList, id)
	}
	gameSetup["partySubmittedUserIds"] = subList

	required := party.CanonicalMemberIDStrings(doc)
	allReady := everyMemberSubmitted(required, submittedIDsSet)

	extraTok := int(max64(0, float64(len(content))/4))
	_, _ = coll.UpdateOne(ctx, bson.M{"gameId": gid}, bson.M{"$set": bson.M{
		"conversation":                   persistNormalize(conv, gameSetup, userID),
		"summaryConversation":            persistNormalize(sumConv, gameSetup, userID),
		"totalTokenCount":                num(doc["totalTokenCount"]) + extraTok,
		"gameSetup":                      gameSetup,
	}})

	if hub != nil {
		hub.NotifyGameStateUpdated(gid)
	}

	if !allReady {
		return map[string]interface{}{
			"ok":             true,
			"duplicate":      false,
			"partyWait":      true,
			"partySubmitted": len(submittedIDsSet),
			"partyRequired":  len(required),
		}, nil
	}

	// All submitted: run DM generate (party round)
	var fresh map[string]interface{}
	_ = coll.FindOne(ctx, bson.M{"gameId": gid}).Decode(&fresh)
	if fresh == nil {
		return nil, &HTTPError{Code: "GAME_NOT_FOUND", Msg: "Game not found", Status: http.StatusNotFound}
	}
	lang := party.GameSetupLanguage(fresh)
	msgs := filterNoSystem(toIfaceSlice(fresh["conversation"]))
	persistPayload := map[string]interface{}{
		"gameId":                       gid,
		"gameSetup":                    fresh["gameSetup"],
		"conversation":                 fresh["conversation"],
		"summaryConversation":          fresh["summaryConversation"],
		"summary":                      fresh["summary"],
		"totalTokenCount":              fresh["totalTokenCount"],
		"userAndAssistantMessageCount": fresh["userAndAssistantMessageCount"],
		"systemMessageContentDM":       fresh["systemMessageContentDM"],
		"encounterState":               fresh["encounterState"],
		"mode":                         fresh["mode"],
		"requestingUserId":             uidStr,
	}
	genBody := map[string]interface{}{
		"messages":          msgs,
		"mode":              str(fresh["mode"]),
		"language":          lang,
		"gameId":            gid,
		"persist":           persistPayload,
		"requestingUserId":  uidStr,
	}
	st, result := HandleDmGenerate(ctx, d, cfg, uidStr, genBody)
	if st != 200 || result == nil {
		errMsg := "DM generate failed"
		code := "PARTY_DM_FAILED"
		if result != nil && result.Error != "" {
			errMsg = result.Error
		}
		return map[string]interface{}{
			"ok":         false,
			"partyWait":  false,
			"error":      errMsg,
			"code":       code,
			"statusCode": st,
		}, nil
	}

	if hub != nil {
		hub.NotifyGameStateUpdated(gid)
	}
	return map[string]interface{}{
		"ok": true, "duplicate": false, "partyWait": false,
		"partyDm": map[string]interface{}{
			"narration":      result.Narration,
			"encounterState": result.EncounterState,
			"activeCombat":   result.ActiveCombat,
		},
	}, nil
}

func persistNormalize(conv []interface{}, gameSetup map[string]interface{}, userID string) []interface{} {
	out := make([]interface{}, 0, len(conv))
	for _, raw := range conv {
		m, ok := raw.(map[string]interface{})
		if !ok {
			out = append(out, raw)
			continue
		}
		if str(m["role"]) != "user" {
			out = append(out, raw)
			continue
		}
		uid := str(m["userId"])
		if uid == "" {
			uid = userID
		}
		auth := pc.CharacterDisplayNameForUser(gameSetup, uid)
		if auth != "" {
			mm := cloneMap(m)
			mm["displayName"] = auth
			out = append(out, mm)
			continue
		}
		out = append(out, raw)
	}
	return out
}

func appendPlayerUserMessageForBroadcast(ctx context.Context, coll *mongo.Collection, hub *realtime.Hub, gid string, body map[string]interface{}, userID string) (map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := coll.FindOne(ctx, bson.M{"gameId": gid}).Decode(&doc); err != nil {
		return nil, err
	}
	content := str(body["content"])
	gameSetup := toMap(doc["gameSetup"])
	fromSheet := pc.CharacterDisplayNameForUser(gameSetup, userID)
	userMsg := map[string]interface{}{"role": "user", "content": content, "userId": userID}
	if fromSheet != "" {
		userMsg["displayName"] = fromSheet
	} else if s := strings.TrimSpace(str(body["displayName"])); s != "" {
		userMsg["displayName"] = s
	}
	conv := toIfaceSlice(doc["conversation"])
	last := lastMsg(conv)
	if last != nil && str(last["role"]) == "user" && str(last["content"]) == content && str(last["userId"]) == userID {
		if hub != nil {
			hub.NotifyGameStateUpdated(gid)
		}
		return map[string]interface{}{"ok": true, "duplicate": true}, nil
	}
	conv = append(conv, userMsg)
	sumConv := append(toIfaceSlice(doc["summaryConversation"]), cloneMap(userMsg))
	extraTok := int(max64(0, float64(len(content))/4))
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gid}, bson.M{"$set": bson.M{
		"conversation":        persistNormalize(conv, gameSetup, userID),
		"summaryConversation": persistNormalize(sumConv, gameSetup, userID),
		"totalTokenCount":     num(doc["totalTokenCount"]) + extraTok,
	}})
	if err != nil {
		return nil, err
	}
	if hub != nil {
		hub.NotifyGameStateUpdated(gid)
	}
	return map[string]interface{}{"ok": true, "duplicate": false}, nil
}

func isMultiMemberParty(doc map[string]interface{}) bool {
	return len(party.CanonicalMemberIDStrings(doc)) >= 2
}

func submittedIDs(gameSetup map[string]interface{}) map[string]struct{} {
	out := map[string]struct{}{}
	arr, _ := gameSetup["partySubmittedUserIds"].([]interface{})
	for _, x := range arr {
		out[party.NormalizeUserIDString(x)] = struct{}{}
	}
	return out
}

func everyMemberSubmitted(required []string, submitted map[string]struct{}) bool {
	for _, id := range required {
		k := party.NormalizeUserIDString(id)
		if _, ok := submitted[k]; !ok {
			return false
		}
	}
	return true
}

func lastMsg(conv []interface{}) map[string]interface{} {
	if len(conv) == 0 {
		return nil
	}
	m, _ := conv[len(conv)-1].(map[string]interface{})
	return m
}

func stripTrailingUserTail(conv, sumConv []interface{}, uidStr string) ([]interface{}, []interface{}) {
	la := lastAssistantIndex(conv)
	c := append([]interface{}{}, conv...)
	s := append([]interface{}{}, sumConv...)
	for len(c) > la+1 {
		last := lastMsg(c)
		if last != nil && str(last["role"]) == "user" && str(last["userId"]) == uidStr {
			c = c[:len(c)-1]
			if len(s) > 0 {
				s = s[:len(s)-1]
			}
		} else {
			break
		}
	}
	return c, s
}

func lastAssistantIndex(conv []interface{}) int {
	for i := len(conv) - 1; i >= 0; i-- {
		m, _ := conv[i].(map[string]interface{})
		if m != nil && str(m["role"]) == "assistant" {
			return i
		}
	}
	return -1
}

func num(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func toMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

