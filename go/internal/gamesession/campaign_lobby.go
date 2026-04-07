package gamesession

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/draftparty"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/llm"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
	"github.com/deckofdmthings/gmai/internal/validate"
)

// LobbyCampaignOutcome is the result of runLobbyCampaignCoreWithStages.
type LobbyCampaignOutcome struct {
	OK       bool
	Status   int
	Error    string
	Code     string
	Raw      string
	Detail   string
	Combined map[string]interface{}
}

func normalizeCreativeSeed(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		return nil
	}
	titleMood := ""
	if s, ok := raw["titleMood"].(string); ok {
		titleMood = strings.TrimSpace(s)
		if len(titleMood) > 400 {
			titleMood = titleMood[:400]
		}
	}
	namingNote := ""
	if s, ok := raw["namingNote"].(string); ok {
		namingNote = strings.TrimSpace(s)
		if len(namingNote) > 600 {
			namingNote = namingNote[:600]
		}
	}
	var prefer []interface{}
	if arr, ok := raw["preferAngles"].([]interface{}); ok {
		for _, x := range arr {
			s := strings.TrimSpace(fmt.Sprint(x))
			if s != "" && len(prefer) < 8 {
				if len(s) > 200 {
					s = s[:200]
				}
				prefer = append(prefer, s)
			}
		}
	}
	var avoid []interface{}
	if arr, ok := raw["avoidRepeatedFantasyTropesThisRun"].([]interface{}); ok {
		for _, x := range arr {
			s := strings.TrimSpace(fmt.Sprint(x))
			if s != "" && len(avoid) < 12 {
				if len(s) > 120 {
					s = s[:120]
				}
				avoid = append(avoid, s)
			}
		}
	}
	out := map[string]interface{}{
		"titleMood":                       titleMood,
		"preferAngles":                    prefer,
		"avoidRepeatedFantasyTropesThisRun": avoid,
	}
	if namingNote != "" {
		out["namingNote"] = namingNote
	}
	return out
}

func ensureCampaignCoreTitle(core map[string]interface{}) {
	if core == nil {
		return
	}
	if t, ok := core["title"].(string); ok && strings.TrimSpace(t) != "" {
		s := strings.TrimSpace(t)
		if len(s) > 200 {
			s = s[:200]
		}
		core["title"] = s
		return
	}
	cc := strings.TrimSpace(fmt.Sprint(core["campaignConcept"]))
	cc = regexp.MustCompile(`\s+`).ReplaceAllString(cc, " ")
	fallback := "Untitled campaign"
	if cc != "" {
		re := regexp.MustCompile(`^[^.!?]+[.!?]?`)
		m := re.FindString(cc)
		chunk := strings.TrimSpace(m)
		if chunk == "" {
			chunk = cc
		}
		if len(chunk) > 100 {
			chunk = chunk[:100]
		}
		fallback = chunk
		if fallback == "" && len(cc) > 80 {
			fallback = cc[:80]
		} else if fallback == "" {
			fallback = cc
		}
	}
	log.Printf("Campaign core missing non-empty title; using fallback: %s", fallback[:min(100, len(fallback))])
	core["title"] = fallback
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func persistCreativeSeedToGameState(ctx context.Context, coll *mongo.Collection, gameID string, seed map[string]interface{}, raw string) error {
	if strings.TrimSpace(gameID) == "" || seed == nil {
		return fmt.Errorf("persistCreativeSeedToGameState: missing gameId or seed")
	}
	rawTrim := strings.TrimSpace(raw)
	if len(rawTrim) > 200000 {
		rawTrim = rawTrim[:200000]
	}
	res, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{
		"$set": bson.M{
			"campaignSpec.creativeSeed": seed,
			"rawModelOutput":            rawTrim,
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("persistCreativeSeedToGameState: no GameState row for gameId=%s", gameID)
	}
	return nil
}

func generateCampaignCreativeSeedStage(ctx context.Context, d *Deps, gameID, language, hostPremise string, persist bool) (ok bool, seed map[string]interface{}, raw string, code string) {
	hostPremiseTrim := ""
	if hostPremise != "" {
		if len(hostPremise) > 2000 {
			hostPremiseTrim = strings.TrimSpace(hostPremise)[:2000]
		} else {
			hostPremiseTrim = strings.TrimSpace(hostPremise)
		}
	}
	tpl := promptmgr.LoadPrompt("templates/campaign/stage_creativeSeed.txt")
	if strings.TrimSpace(tpl) == "" {
		return false, nil, "", "CAMPAIGN_STAGE_CREATIVE_SEED_TEMPLATE"
	}
	userPrompt, err := mustache.Render(tpl, map[string]interface{}{
		"language":    language,
		"hostPremise": hostPremiseTrim,
	})
	if err != nil {
		return false, nil, "", "CAMPAIGN_STAGE_CREATIVE_SEED_TEMPLATE"
	}
	consolidated := promptmgr.ConsolidateSystemMessages(promptmgr.BuildCampaignStageSystemMsgs(language))
	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userPrompt},
	}
	aiMessage, err := llm.GenerateResponse(ctx, d.Cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens": 550, "temperature": 0.95, "gameId": gameID,
	})
	if err != nil || aiMessage == "" {
		return false, nil, "", "CAMPAIGN_STAGE_CREATIVE_SEED_EMPTY"
	}
	parsedObj, ok := llm.ParseModelStructuredObject(aiMessage)
	if !ok || parsedObj == nil {
		if persist && gameID != "" {
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
				"rawModelOutput": trunc200k(aiMessage),
			}}, options.Update().SetUpsert(true))
		}
		return false, nil, aiMessage, "CAMPAIGN_STAGE_CREATIVE_SEED_PARSE"
	}
	rawSeed, _ := parsedObj["creativeSeed"].(map[string]interface{})
	norm := normalizeCreativeSeed(rawSeed)
	if !campaignspec.CreativeSeedIsUsable(norm) && campaignspec.CreativeSeedIsUsable(normalizeCreativeSeed(parsedObj)) {
		rawSeed = parsedObj
		norm = normalizeCreativeSeed(rawSeed)
	}
	if !campaignspec.CreativeSeedIsUsable(norm) {
		if persist && gameID != "" {
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
				"rawModelOutput": trunc200k(aiMessage),
			}}, options.Update().SetUpsert(true))
		}
		return false, nil, aiMessage, "CAMPAIGN_STAGE_CREATIVE_SEED_INVALID"
	}
	return true, norm, aiMessage, ""
}

func trunc200k(s string) string {
	if len(s) <= 200000 {
		return s
	}
	return s[:200000]
}

func generateCampaignStage(ctx context.Context, d *Deps, gameID, stage string, campaignCore map[string]interface{}, language string) bool {
	var templateFile string
	switch stage {
	case "factions":
		templateFile = "templates/campaign/stage_factions.txt"
	case "majorNPCs":
		templateFile = "templates/campaign/stage_majorNPCs.txt"
	case "keyLocations":
		templateFile = "templates/campaign/stage_keyLocations.txt"
	default:
		return false
	}
	tpl := promptmgr.LoadPrompt(templateFile)
	conceptText := ""
	if campaignCore != nil {
		if c, ok := campaignCore["campaignConcept"].(string); ok {
			conceptText = strings.TrimSpace(c)
		}
		if conceptText == "" {
			if t, ok := campaignCore["title"].(string); ok {
				conceptText = strings.TrimSpace(t)
			}
		}
	}
	var userPrompt string
	if tpl != "" {
		userPrompt, _ = mustache.Render(tpl, map[string]interface{}{
			"campaignConcept": conceptText,
			"language":        language,
		})
	} else {
		if stage == "factions" {
			userPrompt = fmt.Sprintf("Based on this campaignConcept: %s\nReturn ONLY YAML: factions (list of objects)...", conceptText)
		} else if stage == "majorNPCs" {
			userPrompt = fmt.Sprintf("Based on this campaignConcept: %s\nReturn ONLY YAML: majorNPCs list...", conceptText)
		} else {
			userPrompt = fmt.Sprintf("Based on this campaignConcept: %s\nReturn ONLY YAML: keyLocations list...", conceptText)
		}
	}
	consolidated := promptmgr.ConsolidateSystemMessages(promptmgr.BuildCampaignStageSystemMsgs(language))
	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userPrompt},
	}
	aiMessage, err := llm.GenerateResponse(ctx, d.Cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens": 800, "temperature": 0.8, "gameId": gameID,
	})
	if err != nil || aiMessage == "" {
		return false
	}
	data, ok := llm.ParseCampaignStageModelOutput(aiMessage)
	if !ok {
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"rawModelOutput": trunc200k(aiMessage),
		}}, options.Update().SetUpsert(true))
		return false
	}
	coerced := llm.CoerceCampaignStageToArray(stage, data)
	if stage == "majorNPCs" && len(coerced) > 0 {
		coerced = validate.DedupeMajorNpcNamesBySuffix(coerced)
	}
	if len(coerced) == 0 {
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"rawModelOutput": trunc200k(aiMessage),
		}}, options.Update().SetUpsert(true))
		return false
	}
	key := "campaignSpec." + stage
	_, err = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		key:            coerced,
		"rawModelOutput": trunc200k(aiMessage),
	}}, options.Update().SetUpsert(true))
	if err != nil {
		return false
	}
	_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameID)
	return true
}

func sliceFirst(a []interface{}, n int) []interface{} {
	if len(a) <= n {
		return a
	}
	return a[:n]
}

func generateCampaignOpeningFrameStage(ctx context.Context, d *Deps, gameID, language string) bool {
	var doc map[string]interface{}
	err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc)
	if err != nil {
		return false
	}
	spec, _ := doc["campaignSpec"].(map[string]interface{})
	if spec == nil {
		return false
	}
	locs := llm.CoerceCampaignStageToArray("keyLocations", spec["keyLocations"])
	if len(locs) == 0 {
		return false
	}
	compact := map[string]interface{}{
		"title":           spec["title"],
		"campaignConcept": spec["campaignConcept"],
		"majorConflicts":  TakeCampaignFieldItems(spec["majorConflicts"], 6),
		"toneAndStyle":    spec["toneAndStyle"],
		"factions":        sliceFirst(llm.CoerceCampaignStageToArray("factions", spec["factions"]), 5),
		"majorNPCs":       sliceFirst(llm.CoerceCampaignStageToArray("majorNPCs", spec["majorNPCs"]), 5),
		"keyLocations":    sliceFirst(locs, 8),
	}
	campaignContextJSON := "{}"
	if b, err := json.Marshal(compact); err == nil {
		s := string(b)
		if len(s) > 12000 {
			s = s[:12000]
		}
		campaignContextJSON = s
	}
	tpl := promptmgr.LoadPrompt("templates/campaign/stage_openingSceneFrame.txt")
	if strings.TrimSpace(tpl) == "" {
		return false
	}
	userPrompt, err := mustache.Render(tpl, map[string]interface{}{
		"language":            language,
		"campaignContextJson": campaignContextJSON,
	})
	if err != nil {
		return false
	}
	consolidated := promptmgr.ConsolidateSystemMessages(promptmgr.BuildCampaignStageSystemMsgs(language))
	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userPrompt},
	}
	aiMessage, err := llm.GenerateResponse(ctx, d.Cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens": 900, "temperature": 0.92, "gameId": gameID,
	})
	if err != nil || aiMessage == "" {
		return false
	}
	parsedObj, ok := llm.ParseModelStructuredObject(aiMessage)
	if !ok || parsedObj == nil {
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"rawModelOutput": trunc200k(aiMessage),
		}}, options.Update().SetUpsert(true))
		return false
	}
	var frame map[string]interface{}
	if f, ok := parsedObj["openingSceneFrame"].(map[string]interface{}); ok {
		frame = f
	}
	if !campaignspec.OpeningSceneFrameIsUsable(frame) && campaignspec.OpeningSceneFrameIsUsable(parsedObj) {
		frame = parsedObj
	}
	if !campaignspec.OpeningSceneFrameIsUsable(frame) {
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"rawModelOutput": trunc200k(aiMessage),
		}}, options.Update().SetUpsert(true))
		return false
	}
	idStr := "campaign_opening"
	if s, ok := frame["id"].(string); ok && strings.TrimSpace(s) != "" {
		idStr = sanitizeOpeningFrameID(strings.TrimSpace(s)[:48])
	}
	directive := strings.TrimSpace(fmt.Sprint(frame["directive"]))
	if len(directive) > 4000 {
		directive = directive[:4000]
	}
	_, err = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"campaignSpec.openingSceneFrame": map[string]interface{}{"id": idStr, "directive": directive},
		"rawModelOutput":                 trunc200k(aiMessage),
	}})
	if err != nil {
		return false
	}
	_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameID)
	return true
}

func sanitizeOpeningFrameID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "campaign_opening"
	}
	if len(out) > 48 {
		return out[:48]
	}
	return out
}

func mergeCampaignSpecLobby(parsed, existing map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range parsed {
		out[k] = v
	}
	if existing != nil {
		for k, v := range existing {
			out[k] = v
		}
	}
	return out
}

func runStageWithTimeout(ctx context.Context, d time.Duration, fn func(context.Context) bool) bool {
	cctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	ch := make(chan bool, 1)
	go func() {
		r := fn(cctx)
		select {
		case ch <- r:
		case <-cctx.Done():
		}
	}()
	select {
	case <-cctx.Done():
		return false
	case ok := <-ch:
		return ok
	}
}

// RunLobbyCampaignCoreWithStages mirrors server runLobbyCampaignCoreWithStages.
func RunLobbyCampaignCoreWithStages(ctx context.Context, d *Deps, gameID, language, hostPremise, actingUserID string) LobbyCampaignOutcome {
	if _, err := gameaccess.AssertGameMember(ctx, d.Coll, actingUserID, gameID); err != nil {
		st := 403
		if err == gameaccess.ErrGameNotFound {
			st = 404
		}
		return LobbyCampaignOutcome{OK: false, Status: st, Error: err.Error()}
	}
	_, userTemplate := promptmgr.LoadCampaignGeneratorParts()
	if strings.TrimSpace(userTemplate) == "" {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Server misconfiguration: templates/campaign/generator.txt is required", Code: "CAMPAIGN_TEMPLATE_MISSING"}
	}
	hostPremiseTrim := strings.TrimSpace(hostPremise)
	if len(hostPremiseTrim) > 2000 {
		hostPremiseTrim = hostPremiseTrim[:2000]
	}

	seedRaceMs := resolveCreativeSeedRaceTimeout()
	seedCtx, cancelSeed := context.WithTimeout(ctx, seedRaceMs)
	ch := make(chan struct {
		ok   bool
		seed map[string]interface{}
		raw  string
		code string
	}, 1)
	go func() {
		ok, seed, raw, code := generateCampaignCreativeSeedStage(seedCtx, d, gameID, language, hostPremiseTrim, false)
		ch <- struct {
			ok   bool
			seed map[string]interface{}
			raw  string
			code string
		}{ok, seed, raw, code}
	}()
	var seedDraw struct {
		ok   bool
		seed map[string]interface{}
		raw  string
		code string
	}
	select {
	case <-seedCtx.Done():
		cancelSeed()
		seedDraw = struct {
			ok   bool
			seed map[string]interface{}
			raw  string
			code string
		}{false, nil, "", "CAMPAIGN_STAGE_CREATIVE_SEED_TIMEOUT"}
	case v := <-ch:
		cancelSeed()
		seedDraw = v
	}
	if !seedDraw.ok {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed generating campaign creative-seed stage", Code: firstNonEmpty(seedDraw.code, "CAMPAIGN_STAGE_CREATIVE_SEED")}
	}
	if err := persistCreativeSeedToGameState(ctx, d.Coll, gameID, seedDraw.seed, seedDraw.raw); err != nil {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed persisting campaign creative-seed stage", Code: "CAMPAIGN_STAGE_CREATIVE_SEED_PERSIST"}
	}
	creativeJSON := "{}"
	if b, err := json.Marshal(seedDraw.seed); err == nil {
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
		"hostPremise":      hostPremiseTrim,
		"creativeSeedJson": creativeJSON,
	})
	if err != nil {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed rendering campaign generator prompt", Code: "CAMPAIGN_RENDER_FAILED"}
	}
	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userPromptRendered},
	}
	outbound := map[string]interface{}{"messages": msgs}
	if b, err := json.Marshal(outbound); err == nil {
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"rawModelRequest": trunc200k(string(b)),
		}}, options.Update().SetUpsert(true))
	}
	aiCore, err := llm.GenerateResponse(ctx, d.Cfg, outbound, map[string]interface{}{
		"max_tokens": 1000, "temperature": 0.8, "gameId": gameID,
	})
	if err != nil || aiCore == "" {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "AI response empty", Code: "CAMPAIGN_CORE_EMPTY", Detail: errString(err)}
	}
	parsed, ok := llm.ParseModelStructuredObject(aiCore)
	if !ok || parsed == nil {
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"rawModelOutput": trunc200k(aiCore),
		}}, options.Update().SetUpsert(true))
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed to parse campaign core (YAML)", Code: "CAMPAIGN_CORE_PARSE", Raw: trunc200k(aiCore)[:min(4000, len(trunc200k(aiCore)))]}
	}
	ensureCampaignCoreTitle(parsed)
	stageMs := resolveLobbyPipelineStageTimeout()
	for _, st := range []string{"factions", "majorNPCs", "keyLocations"} {
		ok := runStageWithTimeout(ctx, stageMs, func(c context.Context) bool {
			return generateCampaignStage(c, d, gameID, st, parsed, language)
		})
		if !ok {
			code := "CAMPAIGN_STAGE_FACTIONS"
			if st == "majorNPCs" {
				code = "CAMPAIGN_STAGE_NPCS"
			} else if st == "keyLocations" {
				code = "CAMPAIGN_STAGE_LOCATIONS"
			}
			return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed generating " + st + " stage", Code: code}
		}
	}
	if !runStageWithTimeout(ctx, stageMs, func(c context.Context) bool {
		return generateCampaignOpeningFrameStage(c, d, gameID, language)
	}) {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed generating opening scene frame stage", Code: "CAMPAIGN_STAGE_OPENING_FRAME"}
	}
	var existing map[string]interface{}
	_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&existing)
	var existingSpec map[string]interface{}
	if existing != nil {
		existingSpec, _ = existing["campaignSpec"].(map[string]interface{})
	}
	combined := mergeCampaignSpecLobby(parsed, existingSpec)
	_, err = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"campaignSpec":   combined,
		"rawModelOutput": trunc200k(aiCore),
	}}, options.Update().SetUpsert(true))
	if err != nil {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: err.Error(), Code: "CAMPAIGN_PIPELINE_EXCEPTION"}
	}
	_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameID)
	return LobbyCampaignOutcome{OK: true, Combined: combined}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

