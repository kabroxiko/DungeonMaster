package gamesession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/draftparty"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/llm"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
)

func strParam(body map[string]interface{}, key string) string {
	if body == nil {
		return ""
	}
	v, ok := body[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func boolParam(body map[string]interface{}, key string, def bool) bool {
	if body == nil {
		return def
	}
	v, ok := body[key]
	if !ok || v == nil {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	s := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	if s == "false" || s == "0" {
		return false
	}
	if s == "" {
		return def
	}
	return true
}

func estimateTokenCount(messages []interface{}) int {
	total := 0
	for _, m := range messages {
		mm, _ := m.(map[string]interface{})
		if mm == nil {
			continue
		}
		switch c := mm["content"].(type) {
		case string:
			total += len(c)
		default:
			total += len(fmt.Sprint(c))
		}
	}
	if total == 0 {
		return 1
	}
	return (total + 3) / 4
}

func campaignCompletionBudget(messages []interface{}) int {
	const modelMax = 4000
	promptTokens := estimateTokenCount(messages)
	available := modelMax - promptTokens - 50
	if available <= 0 {
		return 100
	}
	// min(1500, max(600, min(available, 700)))
	inner := available
	if inner > 700 {
		inner = 700
	}
	budget := inner
	if budget < 600 {
		budget = 600
	}
	if budget > 1500 {
		budget = 1500
	}
	return budget
}

func resolveCampaignCoreStageTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("DM_STAGE_TIMEOUT_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1000 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 60 * time.Second
}

func creativeSeedRace(ctx context.Context, d *Deps, gameID, language, hostPremise string) (ok bool, seed map[string]interface{}, raw string, code string) {
	hostTrim := strings.TrimSpace(hostPremise)
	if len(hostTrim) > 2000 {
		hostTrim = hostTrim[:2000]
	}
	seedRaceMs := resolveCreativeSeedRaceTimeout()
	seedCtx, cancelSeed := context.WithTimeout(ctx, seedRaceMs)
	defer cancelSeed()
	ch := make(chan struct {
		ok   bool
		seed map[string]interface{}
		raw  string
		code string
	}, 1)
	go func() {
		o, s, r, c := generateCampaignCreativeSeedStage(seedCtx, d, gameID, language, hostTrim, false)
		ch <- struct {
			ok   bool
			seed map[string]interface{}
			raw  string
			code string
		}{o, s, r, c}
	}()
	select {
	case <-seedCtx.Done():
		return false, nil, "", "CAMPAIGN_STAGE_CREATIVE_SEED_TIMEOUT"
	case v := <-ch:
		return v.ok, v.seed, v.raw, v.code
	}
}

func accessErrHTTP(err error) (int, map[string]interface{}) {
	if err == gameaccess.ErrGameNotFound {
		return 404, map[string]interface{}{"error": "Game not found", "code": "GAME_NOT_FOUND"}
	}
	return 403, map[string]interface{}{"error": err.Error()}
}

// HandleGenerateCampaign mirrors POST /api/game-session/generate-campaign.
func HandleGenerateCampaign(ctx context.Context, d *Deps, uid string, body map[string]interface{}, queryGameID string) (int, interface{}) {
	language := strParam(body, "language")
	if language == "" {
		language = "English"
	}
	hostPremise := strParam(body, "hostPremise")
	gameID := strParam(body, "gameId")
	if gameID == "" {
		gameID = queryGameID
	}
	if gameID != "" {
		if _, err := gameaccess.AssertGameMember(ctx, d.Coll, uid, gameID); err != nil {
			st, payload := accessErrHTTP(err)
			return st, payload
		}
	}

	_, userTemplate := promptmgr.LoadCampaignGeneratorParts()
	if strings.TrimSpace(userTemplate) == "" {
		return 500, map[string]interface{}{"error": "Server misconfiguration: templates/campaign/generator.txt is required"}
	}

	ok, seed, rawSeed, code := creativeSeedRace(ctx, d, gameID, language, hostPremise)
	if !ok {
		return 500, map[string]interface{}{
			"error": "Failed generating campaign creative-seed stage",
			"code":  firstNonEmpty(code, "CAMPAIGN_STAGE_CREATIVE_SEED"),
		}
	}
	if gameID != "" {
		if err := persistCreativeSeedToGameState(ctx, d.Coll, gameID, seed, rawSeed); err != nil {
			return 500, map[string]interface{}{
				"error": "Failed persisting campaign creative-seed stage",
				"code":  "CAMPAIGN_STAGE_CREATIVE_SEED_PERSIST",
			}
		}
	}

	creativeJSON := "{}"
	if b, err := json.Marshal(seed); err == nil {
		s := string(b)
		if len(s) > 8000 {
			s = s[:8000]
		}
		creativeJSON = s
	}
	rendered, err := mustache.Render(userTemplate, map[string]interface{}{
		"sessionSummary":   "",
		"language":         language,
		"hostPremise":      strings.TrimSpace(hostPremise),
		"creativeSeedJson": creativeJSON,
	})
	if err != nil {
		return 500, map[string]interface{}{"error": "Failed rendering campaign generator prompt"}
	}
	coreSys := promptmgr.BuildCampaignCoreSystemMsgs(language, creativeJSON)
	consolidated := promptmgr.ConsolidateSystemMessages(coreSys)
	campaignOutbound := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": rendered},
	}
	budget := campaignCompletionBudget(campaignOutbound)
	aiMessage, err := llm.GenerateResponse(ctx, d.Cfg, map[string]interface{}{"messages": campaignOutbound}, map[string]interface{}{
		"max_tokens": budget, "temperature": 0.8, "gameId": gameID,
	})
	if err != nil || aiMessage == "" {
		return 500, map[string]interface{}{"error": "AI response was empty or failed (see server logs)."}
	}
	parsedObj, okParsed := llm.ParseModelStructuredObject(aiMessage)
	if !okParsed || parsedObj == nil {
		parsedObj = nil
	} else {
		ensureCampaignCoreTitle(parsedObj)
	}

	gameIDPersist := strParam(body, "gameId")
	if gameIDPersist == "" {
		gameIDPersist = queryGameID
	}
	if gameIDPersist != "" {
		if _, err := gameaccess.AssertGameMember(ctx, d.Coll, uid, gameIDPersist); err != nil {
			st, payload := accessErrHTTP(err)
			return st, payload
		}
		set := bson.M{"rawModelOutput": trunc200k(aiMessage)}
		if parsedObj != nil {
			var existing map[string]interface{}
			_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameIDPersist}).Decode(&existing)
			var existingSpec map[string]interface{}
			if existing != nil {
				existingSpec, _ = existing["campaignSpec"].(map[string]interface{})
			}
			set["campaignSpec"] = campaignspec.MergeCampaignSpecPreservingDmSecrets(existingSpec, parsedObj)
		}
		_, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameIDPersist}, bson.M{"$set": set})
		if err != nil {
			// match Node: log and continue
		} else if parsedObj != nil {
			_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameIDPersist)
		}
	}

	if parsedObj != nil {
		return 200, campaignspec.RedactCampaignSpecForClient(parsedObj)
	}
	return 200, aiMessage
}

// HandleGenerateCampaignCore mirrors POST /api/game-session/generate-campaign-core.
func HandleGenerateCampaignCore(ctx context.Context, d *Deps, uid string, body map[string]interface{}) (int, interface{}) {
	gameID := strParam(body, "gameId")
	language := strParam(body, "language")
	if language == "" {
		language = "English"
	}
	hostPremise := strParam(body, "hostPremise")
	waitForStages := boolParam(body, "waitForStages", true)

	if gameID != "" {
		if _, err := gameaccess.AssertGameMember(ctx, d.Coll, uid, gameID); err != nil {
			st, payload := accessErrHTTP(err)
			return st, payload
		}
	}

	_, userTemplate := promptmgr.LoadCampaignGeneratorParts()
	if strings.TrimSpace(userTemplate) == "" {
		return 500, map[string]interface{}{"error": "Server misconfiguration: templates/campaign/generator.txt is required"}
	}

	ok, seed, rawSeed, code := creativeSeedRace(ctx, d, gameID, language, hostPremise)
	if !ok {
		return 500, map[string]interface{}{
			"error": "Failed generating campaign creative-seed stage",
			"code":  firstNonEmpty(code, "CAMPAIGN_STAGE_CREATIVE_SEED"),
		}
	}
	if gameID != "" {
		if err := persistCreativeSeedToGameState(ctx, d.Coll, gameID, seed, rawSeed); err != nil {
			return 500, map[string]interface{}{
				"error": "Failed persisting campaign creative-seed stage",
				"code":  "CAMPAIGN_STAGE_CREATIVE_SEED_PERSIST",
			}
		}
	}

	creativeJSON := "{}"
	if b, err := json.Marshal(seed); err == nil {
		s := string(b)
		if len(s) > 8000 {
			s = s[:8000]
		}
		creativeJSON = s
	}
	coreSys := promptmgr.BuildCampaignCoreSystemMsgs(language, creativeJSON)
	consolidated := promptmgr.ConsolidateSystemMessages(coreSys)
	userPromptRendered, err := mustache.Render(userTemplate, map[string]interface{}{
		"sessionSummary":   "",
		"language":         language,
		"hostPremise":      strings.TrimSpace(hostPremise),
		"creativeSeedJson": creativeJSON,
	})
	if err != nil {
		return 500, map[string]interface{}{"error": "Failed rendering campaign generator prompt"}
	}
	coreOutbound := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userPromptRendered},
	}
	if gameID != "" {
		if b, err := json.Marshal(map[string]interface{}{"messages": coreOutbound}); err == nil {
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
				"rawModelRequest": trunc200k(string(b)),
			}}, options.Update().SetUpsert(true))
		}
	}

	aiMessage, err := llm.GenerateResponse(ctx, d.Cfg, map[string]interface{}{"messages": coreOutbound}, map[string]interface{}{
		"max_tokens": 1000, "temperature": 0.8, "gameId": gameID,
	})
	if err != nil || aiMessage == "" {
		return 500, map[string]interface{}{"error": "AI response empty"}
	}

	parsedObj, ok := llm.ParseModelStructuredObject(aiMessage)
	if !ok || parsedObj == nil {
		if gameID != "" {
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
				"rawModelOutput": trunc200k(aiMessage),
			}}, options.Update().SetUpsert(true))
		}
		return 500, map[string]interface{}{"error": "Failed to parse campaign core (YAML)", "raw": aiMessage}
	}
	ensureCampaignCoreTitle(parsedObj)

	stageDur := resolveCampaignCoreStageTimeout()

	if gameID != "" && waitForStages {
		for _, stName := range []string{"factions", "majorNPCs", "keyLocations"} {
			st := stName
			if !runStageWithTimeout(ctx, stageDur, func(c context.Context) bool {
				return generateCampaignStage(c, d, gameID, st, parsedObj, language)
			}) {
				msg := "Failed generating factions stage"
				code := "CAMPAIGN_STAGE_FACTIONS"
				if stName == "majorNPCs" {
					msg = "Failed generating majorNPCs stage"
					code = "CAMPAIGN_STAGE_NPCS"
				} else if stName == "keyLocations" {
					msg = "Failed generating keyLocations stage"
					code = "CAMPAIGN_STAGE_LOCATIONS"
				}
				return 500, map[string]interface{}{"error": msg, "code": code}
			}
		}
		if !runStageWithTimeout(ctx, stageDur, func(c context.Context) bool {
			return generateCampaignOpeningFrameStage(c, d, gameID, language)
		}) {
			return 500, map[string]interface{}{"error": "Failed generating opening scene frame stage"}
		}
		var existing map[string]interface{}
		_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&existing)
		var existingSpec map[string]interface{}
		if existing != nil {
			existingSpec, _ = existing["campaignSpec"].(map[string]interface{})
		}
		combined := mergeCampaignSpecLobby(parsedObj, existingSpec)
		_, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"campaignSpec":   combined,
			"rawModelOutput": trunc200k(aiMessage),
		}}, options.Update().SetUpsert(true))
		if err != nil {
			return 500, map[string]interface{}{
				"error": "Could not save the campaign to the database. Your game was not updated — fix storage/permissions and try generating the campaign again before inviting players.",
				"code":  "CAMPAIGN_PERSIST_FAILED",
			}
		}
		_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameID)
		return 200, campaignspec.RedactCampaignSpecForClient(combined)
	}

	if gameID != "" {
		_, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"campaignSpec": parsedObj,
		}}, options.Update().SetUpsert(true))
		if err != nil {
			return 500, map[string]interface{}{
				"error": "Could not save the campaign core. Check server logs and try again.",
				"code":  "CAMPAIGN_CORE_PERSIST_FAILED",
			}
		}
		_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameID)
	}

	// Async stages (waitForStages=false) — respond first, then run stages in background.
	if gameID != "" && !waitForStages {
		d2 := d
		gid := gameID
		lang := language
		parsedCopy := parsedObj
		go func() {
			bg := context.Background()
			for _, stName := range []string{"factions", "majorNPCs", "keyLocations"} {
				st := stName
				_ = runStageWithTimeout(bg, stageDur, func(c context.Context) bool {
					return generateCampaignStage(c, d2, gid, st, parsedCopy, lang)
				})
			}
			_ = runStageWithTimeout(bg, stageDur, func(c context.Context) bool {
				return generateCampaignOpeningFrameStage(c, d2, gid, lang)
			})
		}()
	}

	return 200, campaignspec.RedactCampaignSpecForClient(parsedObj)
}
