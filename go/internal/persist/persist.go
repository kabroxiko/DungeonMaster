package persist

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/draftparty"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/party"
	"github.com/deckofdmthings/gmai/internal/pc"
	"github.com/deckofdmthings/gmai/internal/realtime"
	"github.com/deckofdmthings/gmai/internal/validate"
)

// PersistGameStateFromBody mirrors gameStatePersist.js persistGameStateFromBody.
func PersistGameStateFromBody(ctx context.Context, coll *mongo.Collection, hub *realtime.Hub, body map[string]interface{}, userID string, clearPendingNarrativeIntroductions bool) (map[string]interface{}, error) {
	if userID == "" {
		return nil, errors.New("persistGameStateFromBody: userId is required")
	}
	gid, _ := body["gameId"].(string)
	if gid == "" {
		return nil, errors.New("persistGameStateFromBody: gameId is required")
	}

	var existing map[string]interface{}
	_ = coll.FindOne(ctx, bson.M{"gameId": gid}).Decode(&existing)

	uidOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if _, err := gameaccess.AssertGameMember(ctx, coll, userID, gid); err != nil {
			return nil, err
		}
	}

	lang := "English"
	if body != nil {
		if gs, ok := body["gameSetup"].(map[string]interface{}); ok {
			if l, ok := gs["language"].(string); ok && l != "" {
				lang = l
			}
		}
	}
	if existing != nil {
		if gs, ok := existing["gameSetup"].(map[string]interface{}); ok {
			if l, ok := gs["language"].(string); ok && l != "" {
				lang = l
			}
		}
	}

	ownerEff := gameaccess.EffectiveGameOwnerIDStr(existing)
	incomingSetup, _ := body["gameSetup"].(map[string]interface{})
	if incomingSetup == nil {
		incomingSetup = map[string]interface{}{}
	}
	if existing != nil && ownerEff != "" && userID != ownerEff {
		// non-owner: only merge this user's row
		incMap, _ := incomingSetup["playerCharacters"].(map[string]interface{})
		nextPc := map[string]interface{}{}
		if incMap != nil {
			for k, v := range incMap {
				if k == userID {
					nextPc[k] = v
				}
			}
		}
		incomingSetup = cloneMap(incomingSetup)
		incomingSetup["playerCharacters"] = nextPc
	}

	gameSetup := MergePlayerCharacters(toMap(existing["gameSetup"]), incomingSetup, lang)
	delete(gameSetup, "generatedCharacter")

	if incPC, ok := incomingSetup["playerCharacters"].(map[string]interface{}); ok {
		for k, v := range incPC {
			if m, ok := v.(map[string]interface{}); ok {
				m = validate.EnsurePlayerCharacterSheetDefaults(m, lang)
				if chk, errStr := validate.ValidateGeneratedPlayerCharacter(m); !chk {
					return nil, &PersistError{Code: "INVALID_PLAYER_CHARACTER_PERSIST", Message: errStr, HTTP: 400}
				}
				_ = k
			}
		}
	}

	if clearPendingNarrativeIntroductions {
		prevPending := []string{}
		if existing != nil {
			p := party.GetParty(toMap(existing["gameSetup"]))
			if arr, ok := p["pendingNarrativeIntroductionUserIds"].([]interface{}); ok {
				for _, x := range arr {
					prevPending = append(prevPending, fmt.Sprint(x))
				}
			}
		}
		introduced := map[string]struct{}{}
		p2 := party.GetParty(gameSetup)
		if arr, ok := p2["narrativeIntroducedUserIds"].([]interface{}); ok {
			for _, x := range arr {
				introduced[fmt.Sprint(x)] = struct{}{}
			}
		}
		for _, id := range prevPending {
			introduced[id] = struct{}{}
		}
		var introList []interface{}
		for id := range introduced {
			if id != "" && id != "<nil>" {
				introList = append(introList, id)
			}
		}
		gameSetup = party.MergeParty(gameSetup, map[string]interface{}{
			"pendingNarrativeIntroductionUserIds": []interface{}{},
			"narrativeIntroducedUserIds":          introList,
		})
	}

	conv := normalizeConversationUserDisplayNames(toIfaceSlice(body["conversation"]), gameSetup, userID)
	sumConv := normalizeConversationUserDisplayNames(toIfaceSlice(body["summaryConversation"]), gameSetup, userID)

	update := bson.M{
		"gameId":                         gid,
		"gameSetup":                      gameSetup,
		"conversation":                   conv,
		"summaryConversation":            sumConv,
		"summary":                        body["summary"],
		"totalTokenCount":                body["totalTokenCount"],
		"userAndAssistantMessageCount":   body["userAndAssistantMessageCount"],
		"systemMessageContentDM":         body["systemMessageContentDM"],
	}
	if body["mode"] != nil {
		update["mode"] = body["mode"]
	}
	if _, ok := body["encounterState"]; ok {
		update["encounterState"] = body["encounterState"]
	}

	var nextCampaign map[string]interface{}
	if _, ok := body["campaignSpec"]; ok {
		var existingSpec map[string]interface{}
		if existing != nil {
			existingSpec, _ = existing["campaignSpec"].(map[string]interface{})
		}
		inc, _ := body["campaignSpec"].(map[string]interface{})
		nextCampaign = campaignspec.MergeCampaignSpecPreservingDmSecrets(existingSpec, inc)
		update["campaignSpec"] = nextCampaign
	}

	nextSpec := nextCampaign
	if nextSpec == nil && existing != nil {
		nextSpec, _ = existing["campaignSpec"].(map[string]interface{})
	}
	nextEnc := toMap(update["encounterState"])
	if nextEnc == nil && existing != nil {
		nextEnc, _ = existing["encounterState"].(map[string]interface{})
	}
	if ok, code, msg := validate.ValidateDistinctEntityNames(gameSetup, nextSpec, nextEnc); !ok {
		return nil, &PersistError{Code: code, Message: msg, HTTP: 422}
	}

	setOnInsert := bson.M{}
	if existing == nil {
		setOnInsert["ownerUserId"] = uidOID
		setOnInsert["memberUserIds"] = bson.A{uidOID}
		if draftparty.DraftPartyTTLMs() > 0 && !campaignspec.HasSubstantiveCampaignSpec(nextSpec) {
			if at := draftparty.DraftPartyExpiresAtFromNow(); at != nil {
				setOnInsert["draftPartyExpiresAt"] = at
			}
		}
	}

	mongoUpdate := bson.M{"$set": update}
	if len(setOnInsert) > 0 {
		mongoUpdate["$setOnInsert"] = setOnInsert
	}
	if _, ok := body["campaignSpec"]; ok {
		if cs, _ := update["campaignSpec"].(map[string]interface{}); campaignspec.HasSubstantiveCampaignSpec(cs) {
			mongoUpdate["$unset"] = bson.M{"draftPartyExpiresAt": ""}
		}
	}

	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var out map[string]interface{}
	err = coll.FindOneAndUpdate(ctx, bson.M{"gameId": gid}, mongoUpdate, opts).Decode(&out)
	if err != nil {
		return nil, err
	}
	if hub != nil {
		hub.NotifyGameStateUpdated(gid)
	}
	return out, nil
}

// PersistError is returned for validation failures during persist.
type PersistError struct {
	Code    string
	Message string
	HTTP    int
}

func (e *PersistError) Error() string { return e.Message }

func toMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

func toIfaceSlice(v interface{}) []interface{} {
	a, _ := v.([]interface{})
	return a
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func normalizeConversationUserDisplayNames(conversation []interface{}, gameSetup map[string]interface{}, fallbackUserID string) []interface{} {
	if conversation == nil {
		return nil
	}
	out := make([]interface{}, 0, len(conversation))
	for _, raw := range conversation {
		m, ok := raw.(map[string]interface{})
		if !ok {
			out = append(out, raw)
			continue
		}
		if r, _ := m["role"].(string); r != "user" {
			out = append(out, raw)
			continue
		}
		uid := ""
		if u, ok := m["userId"].(string); ok {
			uid = u
		}
		if uid == "" {
			uid = fallbackUserID
		}
		if uid == "" {
			out = append(out, raw)
			continue
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
