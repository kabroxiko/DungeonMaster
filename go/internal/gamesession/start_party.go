package gamesession

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/party"
	"github.com/deckofdmthings/gmai/internal/persist"
	"github.com/deckofdmthings/gmai/internal/realtime"
)

func revertPartyLobbyPhaseOnly(ctx context.Context, coll *mongo.Collection, gameID, errMsg string) error {
	var doc map[string]interface{}
	_ = coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc)
	gs, _ := doc["gameSetup"].(map[string]interface{})
	next := party.MergeParty(gs, map[string]interface{}{
		"phase":          "lobby",
		"lastStartError": strings.TrimSpace(errMsg)[:min(2000, len(strings.TrimSpace(errMsg)))],
		"lastStartAt":    time.Now().UTC().Format(time.RFC3339Nano),
	})
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{"gameSetup": next}})
	return err
}

func rollbackPartyStartAfterOpeningFailure(ctx context.Context, coll *mongo.Collection, gameID, errMsg string) error {
	var doc map[string]interface{}
	_ = coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc)
	gs, _ := doc["gameSetup"].(map[string]interface{})
	next := party.MergeParty(gs, map[string]interface{}{
		"phase":          "lobby",
		"lastStartError": strings.TrimSpace(errMsg)[:min(2000, len(strings.TrimSpace(errMsg)))],
		"lastStartAt":    time.Now().UTC().Format(time.RFC3339Nano),
	})
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{
		"$set": bson.M{
			"gameSetup":              next,
			"conversation":           []interface{}{},
			"summaryConversation":    []interface{}{},
			"systemMessageContentDM": "",
		},
		"$unset": bson.M{"campaignSpec": ""},
	})
	return err
}

// HandleStartPartyAdventure mirrors POST /start-party-adventure.
func HandleStartPartyAdventure(ctx context.Context, d *Deps, cfg *config.Config, hub *realtime.Hub, userID string, body map[string]interface{}) (status int, payload map[string]interface{}) {
	gameID := strings.TrimSpace(fmt.Sprint(body["gameId"]))
	if gameID == "" {
		return 400, map[string]interface{}{"error": "gameId required", "code": "GAME_ID_REQUIRED"}
	}
	if _, err := gameaccess.AssertGameMember(ctx, d.Coll, userID, gameID); err != nil {
		if err == gameaccess.ErrGameNotFound {
			return 404, map[string]interface{}{"error": "Game not found", "code": "GAME_NOT_FOUND"}
		}
		return 400, map[string]interface{}{"error": err.Error()}
	}
	var doc map[string]interface{}
	err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return 404, map[string]interface{}{"error": "Game not found", "code": "GAME_NOT_FOUND"}
	}
	if err != nil {
		return 500, map[string]interface{}{"error": "db error", "code": "DB_ERROR"}
	}
	spec, _ := doc["campaignSpec"].(map[string]interface{})
	if campaignspec.HasSubstantiveCampaignSpec(spec) {
		gs, _ := doc["gameSetup"].(map[string]interface{})
		next := party.MergeParty(gs, map[string]interface{}{"phase": "playing", "lastStartError": nil})
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{"gameSetup": next}})
		if hub != nil {
			hub.NotifyGameStateUpdated(gameID)
		}
		return 200, map[string]interface{}{"ok": true, "alreadyStarted": true}
	}
	if !party.AllMembersHaveValidSheets(doc) {
		return 400, map[string]interface{}{
			"error": "Every member must have a valid character sheet before the adventure starts.",
			"code":  "PARTY_SHEETS_INCOMPLETE",
		}
	}
	p := party.GetParty(toMap(doc["gameSetup"]))
	if !party.AllMembersReady(p, doc) {
		return 400, map[string]interface{}{
			"error": "Every member must mark ready before the adventure starts.",
			"code":  "PARTY_NOT_READY",
		}
	}
	res, err := d.Coll.UpdateOne(ctx, bson.M{
		"gameId": gameID,
		"$or": []interface{}{
			bson.M{"gameSetup.party.phase": "lobby"},
			bson.M{"gameSetup.party.phase": bson.M{"$exists": false}},
		},
	}, bson.M{"$set": bson.M{
		"gameSetup.party.phase":        "starting",
		"gameSetup.party.lastStartError": nil,
		"gameSetup.party.lastStartAt":  time.Now().UTC().Format(time.RFC3339Nano),
	}})
	if err != nil {
		return 500, map[string]interface{}{"error": err.Error()}
	}
	if res.MatchedCount == 0 {
		var cur map[string]interface{}
		_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&cur)
		if cur != nil {
			if cs, ok := cur["campaignSpec"].(map[string]interface{}); ok && campaignspec.HasSubstantiveCampaignSpec(cs) {
				if hub != nil {
					hub.NotifyGameStateUpdated(gameID)
				}
				return 200, map[string]interface{}{"ok": true, "alreadyStarted": true}
			}
			p2 := party.GetParty(toMap(cur["gameSetup"]))
			if ph, _ := p2["phase"].(string); ph == "starting" {
				return 409, map[string]interface{}{"error": "Party start already in progress.", "code": "PARTY_START_IN_PROGRESS"}
			}
		}
		return 409, map[string]interface{}{"error": "Party cannot start from this state.", "code": "PARTY_START_CONFLICT"}
	}

	var transitioned map[string]interface{}
	_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&transitioned)
	lang := "English"
	if gs, ok := transitioned["gameSetup"].(map[string]interface{}); ok {
		if l, ok := gs["language"].(string); ok && strings.TrimSpace(l) != "" {
			lang = strings.TrimSpace(l)
		}
	}
	hostPremise := ""
	if pp := party.GetParty(toMap(transitioned["gameSetup"])); pp != nil {
		if h, ok := pp["hostPremise"].(string); ok {
			hostPremise = h
		}
	}

	camp := RunLobbyCampaignCoreWithStages(ctx, d, gameID, lang, hostPremise, userID)
	if !camp.OK {
		_ = revertPartyLobbyPhaseOnly(ctx, d.Coll, gameID, camp.Error)
		if hub != nil {
			hub.NotifyGameStateUpdated(gameID)
		}
		out := map[string]interface{}{
			"error": firstNonEmpty(camp.Error, "Campaign generation failed"),
			"code":  firstNonEmpty(camp.Code, "PARTY_CAMPAIGN_FAILED"),
		}
		if camp.Raw != "" {
			out["rawPreview"] = camp.Raw[:min(1200, len(camp.Raw))]
		}
		if camp.Detail != "" {
			out["detail"] = camp.Detail
		}
		st := camp.Status
		if st < 400 {
			st = 500
		}
		return st, out
	}

	fresh := transitioned
	_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&fresh)
	spec2, _ := fresh["campaignSpec"].(map[string]interface{})
	systemMessageContentDM := BuildBootstrapSystemMessageContentDM(spec2, lang)
	bootstrapBody := map[string]interface{}{
		"gameId":                       gameID,
		"gameSetup":                    fresh["gameSetup"],
		"campaignSpec":                 spec2,
		"conversation":                 []interface{}{map[string]interface{}{"role": "system", "content": systemMessageContentDM}},
		"summaryConversation":          []interface{}{},
		"summary":                      strOrEmpty(fresh["summary"]),
		"totalTokenCount":              numOrZero(fresh["totalTokenCount"]),
		"userAndAssistantMessageCount": numOrZero(fresh["userAndAssistantMessageCount"]),
		"systemMessageContentDM":       systemMessageContentDM,
	}
	_, err = persist.PersistGameStateFromBody(ctx, d.Coll, hub, bootstrapBody, userID, false)
	if err != nil {
		_ = revertPartyLobbyPhaseOnly(ctx, d.Coll, gameID, err.Error())
		if hub != nil {
			hub.NotifyGameStateUpdated(gameID)
		}
		return 500, map[string]interface{}{
			"error": "Saved campaign but failed to bootstrap session shell.",
			"code":  "PARTY_BOOTSTRAP_FAILED",
		}
	}

	var afterBoot map[string]interface{}
	_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&afterBoot)
	persistPayload := map[string]interface{}{
		"gameId":                       gameID,
		"gameSetup":                    afterBoot["gameSetup"],
		"conversation":               afterBoot["conversation"],
		"summaryConversation":        afterBoot["summaryConversation"],
		"summary":                    afterBoot["summary"],
		"totalTokenCount":            afterBoot["totalTokenCount"],
		"userAndAssistantMessageCount": afterBoot["userAndAssistantMessageCount"],
		"systemMessageContentDM":       afterBoot["systemMessageContentDM"],
		"requestingUserId":           userID,
	}
	conv := toIfaceSlice(afterBoot["conversation"])
	nonSystem := filterNonSystem(conv)
	genBody := map[string]interface{}{
		"messages":           nonSystem,
		"mode":               "initial",
		"language":           lang,
		"gameId":             gameID,
		"persist":            persistPayload,
		"sessionSummary":     "",
		"includeFullSkill":   true,
		"requestingUserId":   userID,
	}
	st, genOut := HandleDmGenerate(ctx, d, cfg, userID, genBody)
	if st != 200 || genOut == nil || strings.TrimSpace(genOut.Narration) == "" {
		detail := ""
		if genOut != nil {
			detail = genOut.Error
		}
		if detail == "" {
			detail = fmt.Sprintf("DM generate failed (HTTP %d)", st)
		}
		_ = rollbackPartyStartAfterOpeningFailure(ctx, d.Coll, gameID, detail)
		if hub != nil {
			hub.NotifyGameStateUpdated(gameID)
		}
		out := map[string]interface{}{
			"error": detail,
			"code":  "PARTY_OPENING_FAILED",
		}
		if genOut != nil && genOut.RawPreview != "" {
			out["rawPreview"] = genOut.RawPreview
		}
		if st < 400 {
			st = 502
		}
		return st, out
	}

	gs3, _ := afterBoot["gameSetup"].(map[string]interface{})
	playingSetup := party.MergeParty(gs3, map[string]interface{}{
		"phase":          "playing",
		"lastStartError": nil,
		"lastStartAt":    time.Now().UTC().Format(time.RFC3339Nano),
	})
	_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{"gameSetup": playingSetup}})
	if hub != nil {
		hub.NotifyGameStateUpdated(gameID)
	}
	return 200, map[string]interface{}{
		"ok":      true,
		"started": true,
		"opening": map[string]interface{}{
			"narration":      genOut.Narration,
			"encounterState": genOut.EncounterState,
			"activeCombat":   genOut.ActiveCombat,
		},
	}
}

func filterNonSystem(conv []interface{}) []interface{} {
	var out []interface{}
	for _, m := range conv {
		mm, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if r, _ := mm["role"].(string); r != "system" {
			out = append(out, m)
		}
	}
	return out
}

func strOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprint(v)
}

func numOrZero(v interface{}) interface{} {
	if v == nil {
		return 0
	}
	return v
}
