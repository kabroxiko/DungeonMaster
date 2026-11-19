package gamesession

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"runtime/debug"
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
	"github.com/deckofdmthings/gmai/internal/party"
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
	// MapImageWarning describes why the rendered map could not be stored; the campaign itself still succeeded.
	MapImageWarning string
}

// CampaignDirection is the direction the table owner gave the campaign: the premise it is written from, and the tone
// and style it is written and played in. Both live on gameSetup.party, so they are one value read once per generation
// rather than a pair of strings carried through every function that happens to need one of them.
type CampaignDirection struct {
	Premise      string
	ToneAndStyle string
}

// newCampaignDirection normalizes the direction in one place. The cap is counted in runes after trimming: slicing bytes
// would cut a non-Latin premise in half, and capping the untrimmed length panicked before.
func newCampaignDirection(premise, toneAndStyle string) CampaignDirection {
	premise = strings.TrimSpace(premise)
	if r := []rune(premise); len(r) > 2000 {
		premise = string(r[:2000])
	}
	return CampaignDirection{Premise: premise, ToneAndStyle: strings.TrimSpace(toneAndStyle)}
}

// campaignDirectionForGame reads the direction the owner stored on the game.
//
// A read failure is an error rather than an empty direction: silently generating without the premise and voice the
// table chose is exactly the divergence this value exists to prevent.
func campaignDirectionForGame(ctx context.Context, d *Deps, gameID string) (CampaignDirection, error) {
	gid := strings.TrimSpace(gameID)
	if gid == "" {
		return CampaignDirection{}, fmt.Errorf("campaign direction: gameId is required")
	}
	if d == nil || d.Coll == nil {
		return CampaignDirection{}, fmt.Errorf("campaign direction: no database")
	}
	var doc map[string]interface{}
	if err := d.Coll.FindOne(ctx, bson.M{"gameId": gid}).Decode(&doc); err != nil {
		return CampaignDirection{}, fmt.Errorf("campaign direction: %w", err)
	}
	p := party.GetParty(toMap(doc["gameSetup"]))
	premise, _ := p["hostPremise"].(string)
	tone, _ := p["hostToneAndStyle"].(string)
	return newCampaignDirection(premise, tone), nil
}

// campaignDirectionForRequest resolves a generation request's direction: a game-scoped request reads the game's stored
// choice, and a stateless request carries it in the body.
func campaignDirectionForRequest(ctx context.Context, d *Deps, gameID string, body map[string]interface{}) (CampaignDirection, error) {
	if strings.TrimSpace(gameID) != "" {
		return campaignDirectionForGame(ctx, d, gameID)
	}
	return newCampaignDirection(strParam(body, "hostPremise"), strParam(body, "hostToneAndStyle")), nil
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
		"titleMood":                         titleMood,
		"preferAngles":                      prefer,
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

// applyHostToneAndStyle makes the table owner's chosen tone and style the campaign's own. The model is told to write
// the core in that voice, and this makes the stored value the choice itself rather than a paraphrase of it; when the
// owner chose nothing, the generated value stands.
func applyHostToneAndStyle(core map[string]interface{}, hostToneAndStyle string) {
	if core == nil {
		return
	}
	if tone := strings.TrimSpace(hostToneAndStyle); tone != "" {
		core["toneAndStyle"] = tone
	}
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

func generateCampaignCreativeSeedStage(ctx context.Context, d *Deps, gameID, language string, dir CampaignDirection) (ok bool, seed map[string]interface{}, raw string, code string) {
	premise := dir.Premise
	tpl := promptmgr.LoadPrompt("templates/campaign/stage_creativeSeed.md")
	if strings.TrimSpace(tpl) == "" {
		return false, nil, "", "CAMPAIGN_STAGE_CREATIVE_SEED_TEMPLATE"
	}
	userPrompt, err := mustache.Render(tpl, map[string]interface{}{
		"language":    language,
		"hostPremise": premise,
	})
	if err != nil {
		return false, nil, "", "CAMPAIGN_STAGE_CREATIVE_SEED_TEMPLATE"
	}
	consolidated := promptmgr.ConsolidateSystemMessages(promptmgr.BuildCampaignStageSystemMsgs(language))
	// The seed decides the angles the core leans into, so it is drawn for the owner's voice as well: a comedic
	// campaign seeded with grim angles can only fight its own tone.
	if voice := promptmgr.CampaignVoiceBlock(dir.ToneAndStyle); voice != "" {
		consolidated = voice + "\n\n" + consolidated
	}
	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userPrompt},
	}
	// Budgets here cover hidden reasoning tokens as well as the document itself: a reasoning model spends roughly
	// 300-800 of them per stage, and the reply is cut off (finish_reason "length") if the total is too small.
	aiMessage, err := llm.GenerateResponse(ctx, d.Cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens": 2000, "temperature": 0.95, "gameId": gameID, "heavy": true,
	})
	if err != nil || aiMessage == "" {
		return false, nil, "", "CAMPAIGN_STAGE_CREATIVE_SEED_EMPTY"
	}
	parsedObj, ok := llm.ParseModelStructuredObject(aiMessage)
	if !ok || parsedObj == nil {
		// Keep the reply even when the caller did not ask for the seed to be stored: a parse failure is exactly
		// when the text matters, and dropping it turns the error into an unanswerable "failed to parse".
		persistStageFailureRaw(ctx, d, gameID, aiMessage)
		return false, nil, aiMessage, "CAMPAIGN_STAGE_CREATIVE_SEED_PARSE"
	}
	rawSeed, _ := parsedObj["creativeSeed"].(map[string]interface{})
	norm := normalizeCreativeSeed(rawSeed)
	if !campaignspec.CreativeSeedIsUsable(norm) && campaignspec.CreativeSeedIsUsable(normalizeCreativeSeed(parsedObj)) {
		rawSeed = parsedObj
		norm = normalizeCreativeSeed(rawSeed)
	}
	if !campaignspec.CreativeSeedIsUsable(norm) {
		persistStageFailureRaw(ctx, d, gameID, aiMessage)
		return false, nil, aiMessage, "CAMPAIGN_STAGE_CREATIVE_SEED_INVALID"
	}
	return true, norm, aiMessage, ""
}

// persistStageFailureRaw keeps the model reply that a campaign stage could not use, so the next failure can be
// read instead of guessed at. Successes are persisted by their own paths; failures are always kept.
func persistStageFailureRaw(ctx context.Context, d *Deps, gameID, raw string) {
	if d == nil || d.Coll == nil || strings.TrimSpace(gameID) == "" || raw == "" {
		return
	}
	_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"rawModelOutput": trunc200k(raw),
	}}, options.Update().SetUpsert(true))
}

func trunc200k(s string) string {
	if len(s) <= 200000 {
		return s
	}
	return s[:200000]
}

// generateCampaignStage writes one campaign stage into the game state.
//
// The second return value is the reason a stage produced nothing. A stage used to answer only true or false, which
// left "Failed generating factions stage" as the entire explanation of a failed campaign: the reason existed here and
// was thrown away, so neither the log nor the client could say which of the five ways it failed had happened.
func generateCampaignStage(ctx context.Context, d *Deps, gameID, stage string, campaignCore map[string]interface{}, language string) (bool, string) {
	var templateFile string
	switch stage {
	case "factions":
		templateFile = "templates/campaign/stage_factions.md"
	case "majorNPCs":
		templateFile = "templates/campaign/stage_majorNPCs.md"
	case "keyLocations":
		templateFile = "templates/campaign/stage_keyLocations.md"
	default:
		return false, "unknown campaign stage " + stage
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
	// Every stage is written on the campaign's map. Stages that name places (all of them can) are given the world's
	// own vocabulary, so the story cannot drift onto an invented map while the players look at the real one.
	world := vellarisPromptWorld(ctx, d, gameID)
	if tpl != "" {
		data := map[string]interface{}{
			"campaignConcept": conceptText,
			"language":        language,
		}
		if world != nil {
			if regions, _ := world["regions"].([]interface{}); len(regions) > 0 {
				data["hasVellarisRegions"] = true
				data["vellarisRegions"] = regions
			}
			// Key locations are places the party travels to, so only settlements are candidates — a region is a
			// territory and a lake is water.
			if places := campaignspec.SettlementPlaces(world); len(places) > 0 {
				data["hasVellarisPlaces"] = true
				data["vellarisPlaces"] = places
			}
		}
		rendered, renderErr := mustache.Render(tpl, data)
		if renderErr != nil {
			return false, "the " + stage + " prompt could not be rendered: " + renderErr.Error()
		}
		userPrompt = rendered
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
	maxWorldPlaces := 12
	if stage == "keyLocations" {
		// The user message already carries the exact region and place lists.
		maxWorldPlaces = -1
	}
	if block := campaignWorldSystemBlock(world, maxWorldPlaces); block != "" {
		consolidated = block + "\n\n" + consolidated
	}
	tone, _ := campaignCore["toneAndStyle"].(string)
	if voice := promptmgr.CampaignVoiceBlock(tone); voice != "" {
		consolidated = voice + "\n\n" + consolidated
	}
	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userPrompt},
	}
	maxTok := 2000
	if stage == "keyLocations" {
		maxTok = 3000
	}
	aiMessage, err := llm.GenerateResponse(ctx, d.Cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens": maxTok, "temperature": 0.8, "gameId": gameID, "heavy": true,
	})
	if err != nil || aiMessage == "" {
		reason := errString(err)
		if reason == "" {
			reason = "the model returned no text"
		}
		return false, reason
	}
	data, ok := llm.ParseCampaignStageModelOutput(aiMessage)
	if !ok {
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"rawModelOutput": trunc200k(aiMessage),
		}}, options.Update().SetUpsert(true))
		return false, "the model reply was not YAML this stage could read (kept as rawModelOutput)"
	}
	coerced := llm.CoerceCampaignStageToArray(stage, data)
	if stage == "keyLocations" && world != nil {
		// The map owns place names. Anything the model invented or reused becomes a real map name here, so a
		// location the players are told about is always a place they can point at.
		aligned, renamed := campaignspec.AlignKeyLocationsToWorld(coerced, world)
		if renamed > 0 {
			log.Printf("keyLocations: replaced %d place name(s) missing from the map (gameId=%s)", renamed, gameID)
		}
		coerced = aligned
	}
	if stage == "majorNPCs" && len(coerced) > 0 {
		coerced = validate.DedupeMajorNpcNamesBySuffix(coerced)
	}
	if len(coerced) == 0 {
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"rawModelOutput": trunc200k(aiMessage),
		}}, options.Update().SetUpsert(true))
		return false, "the model reply parsed but held no " + stage + " entries this stage could use (kept as rawModelOutput)"
	}
	key := "campaignSpec." + stage
	set := bson.M{
		key:              coerced,
		"rawModelOutput": trunc200k(aiMessage),
	}
	_, err = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": set}, options.Update().SetUpsert(true))
	if err != nil {
		return false, "the " + stage + " entries could not be saved: " + err.Error()
	}
	_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameID)
	return true, ""
}

// campaignStageFailureMessage names a failed stage in the words the client already reads.
func campaignStageFailureMessage(stage string) (msg, code string) {
	switch stage {
	case "majorNPCs":
		return "Failed generating majorNPCs stage", "CAMPAIGN_STAGE_NPCS"
	case "keyLocations":
		return "Failed generating keyLocations stage", "CAMPAIGN_STAGE_LOCATIONS"
	default:
		return "Failed generating " + stage + " stage", "CAMPAIGN_STAGE_" + strings.ToUpper(stage)
	}
}

// campaignStageFailureDetail is what to tell the reader when a stage wrote nothing. A stage that never reported a
// reason ran out of its budget, which is itself the answer — and the budget is the operator's to change.
func campaignStageFailureDetail(reason, stage string, budget time.Duration) string {
	if r := strings.TrimSpace(reason); r != "" {
		return r
	}
	return fmt.Sprintf("the %s stage did not finish within %s; raise DM_STAGE_TIMEOUT_MS (and DM_LOBBY_STAGE_TIMEOUT_MS for a lobby start) if the model needs longer", stage, budget)
}

// recordStageFailure keeps the reason a stage failed on the game, so the next attempt can be read from the database
// instead of guessed at. The same text is logged, because a stage failure is a server problem before it is the
// player's.
func recordStageFailure(ctx context.Context, d *Deps, gameID, stage, detail string) {
	if d == nil || d.Coll == nil || strings.TrimSpace(gameID) == "" {
		return
	}
	log.Printf("campaign stage %s failed (gameId=%s): %s", stage, gameID, detail)
	_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"lastStageError": stage + ": " + trunc200k(detail),
	}}, options.Update().SetUpsert(true))
}

func sliceFirst(a []interface{}, n int) []interface{} {
	if len(a) <= n {
		return a
	}
	return a[:n]
}

func generateCampaignOpeningFrameStage(ctx context.Context, d *Deps, gameID, language string, dir CampaignDirection) bool {
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
	world := campaignspec.NormalizeVellarisWorld(mapFrom(spec, "vellarisWorld"))
	compact := map[string]interface{}{
		"title":           spec["title"],
		"campaignConcept": spec["campaignConcept"],
		"majorConflicts":  TakeCampaignFieldItems(spec["majorConflicts"], 6),
		"toneAndStyle":    dir.ToneAndStyle,
		"factions":        sliceFirst(llm.CoerceCampaignStageToArray("factions", spec["factions"]), 5),
		"majorNPCs":       sliceFirst(llm.CoerceCampaignStageToArray("majorNPCs", spec["majorNPCs"]), 5),
		"keyLocations":    sliceFirst(locs, 8),
	}
	// The opening directive may only name places the players can find on their map.
	if names := campaignspec.PlaceNames(world, 20); len(names) > 0 {
		compact["mapPlaceNames"] = names
	}
	campaignContextJSON := "{}"
	if b, err := json.Marshal(compact); err == nil {
		s := string(b)
		if len(s) > 12000 {
			s = s[:12000]
		}
		campaignContextJSON = s
	}
	tpl := promptmgr.LoadPrompt("templates/campaign/stage_openingSceneFrame.md")
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
	if block := campaignWorldSystemBlock(world, 12); block != "" {
		consolidated = block + "\n\n" + consolidated
	}
	if voice := promptmgr.CampaignVoiceBlock(dir.ToneAndStyle); voice != "" {
		consolidated = voice + "\n\n" + consolidated
	}
	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userPrompt},
	}
	aiMessage, err := llm.GenerateResponse(ctx, d.Cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens": 3000, "temperature": 0.92, "gameId": gameID, "heavy": true,
	})
	if err != nil || aiMessage == "" {
		// Returning false silently is what made this stage impossible to diagnose: the reason reached neither the
		// caller nor the database.
		reason := errString(err)
		if reason == "" {
			reason = "the model returned no text"
		}
		log.Printf("opening scene frame stage failed: %s", reason)
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"lastStageError": "opening scene frame: " + trunc200k(reason),
		}}, options.Update().SetUpsert(true))
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
	idStr := openingFrameID(frame)
	directive := strings.TrimSpace(fmt.Sprint(frame["directive"]))
	if len(directive) > 4000 {
		directive = directive[:4000]
	}
	startPlace := strings.TrimSpace(str(frame["startPlace"]))
	if runes := []rune(startPlace); len(runes) > maxOpeningStartPlaceRunes {
		startPlace = string(runes[:maxOpeningStartPlaceRunes])
	}
	storedFrame := map[string]interface{}{"id": idStr, "directive": directive}
	if startPlace != "" {
		storedFrame["startPlace"] = startPlace
	}
	_, err = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"campaignSpec.openingSceneFrame": storedFrame,
		"rawModelOutput":                 trunc200k(aiMessage),
	}})
	if err != nil {
		return false
	}
	storeOpeningPartyPosition(ctx, d, gameID, startPlace, locs)
	_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameID)
	return true
}

// maxOpeningStartPlaceRunes bounds the place name the opening frame may hand back, since it becomes a stored label.
const maxOpeningStartPlaceRunes = 120

// storeOpeningPartyPosition puts the party on the map where the adventure opens.
//
// The opening frame names the map place the first scene happens in, and that name becomes the stored pin, so the table
// sees the party standing somewhere exact before a single turn is played instead of waiting for the model to say where
// it is. A name the map does not have — a region, or an invented one — falls through to the campaign's own key
// locations, which are settlement names on that same map; a world carrying none of them keeps no pin until the first
// turn places one.
func storeOpeningPartyPosition(ctx context.Context, d *Deps, gameID, chosen string, keyLocations []interface{}) {
	candidates := openingStartCandidates(chosen, keyLocations)
	for _, name := range candidates {
		label, err := ApplyPartyPosition(ctx, d, gameID, &PartyPosition{Place: name})
		if err != nil {
			log.Printf("opening position: %v (gameId=%s)", err, gameID)
			return
		}
		if label != "" {
			return
		}
	}
	log.Printf("opening position: none of %d candidate place(s) is on this map (gameId=%s)", len(candidates), gameID)
}

// openingStartCandidates is the order the opening spot is looked for in: the frame's own choice first, then the
// campaign's key locations in the order the campaign rated them. Blank names are dropped, so a missing startPlace does
// not spend a lookup.
func openingStartCandidates(chosen string, keyLocations []interface{}) []string {
	candidates := make([]string, 0, len(keyLocations)+1)
	if name := strings.TrimSpace(chosen); name != "" {
		candidates = append(candidates, name)
	}
	for _, name := range keyLocationNames(keyLocations) {
		if strings.TrimSpace(name) != "" {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

// keyLocationNames lists a campaign's key locations in the order the campaign rated them, which is also the order the
// story is likely to care about them.
func keyLocationNames(locations []interface{}) []string {
	names := make([]string, 0, len(locations))
	for _, raw := range locations {
		entry, _ := raw.(map[string]interface{})
		if entry == nil {
			continue
		}
		if name := strings.TrimSpace(str(entry["name"])); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// openingFrameID turns whatever the model put in "id" into a stable identifier.
//
// The id is model output, so it can be anything: missing, a number, or a short token. Any length is acceptable -
// sanitizeOpeningFrameID caps it - which is why nothing here slices to a fixed width.
func openingFrameID(frame map[string]interface{}) string {
	if frame == nil {
		return "campaign_opening"
	}
	s, ok := frame["id"]
	if !ok || s == nil {
		return "campaign_opening"
	}
	trimmed := strings.TrimSpace(fmt.Sprint(s))
	if trimmed == "" || trimmed == "<nil>" {
		return "campaign_opening"
	}
	return sanitizeOpeningFrameID(trimmed)
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
		// A panic in here would otherwise take the whole API down: Go cannot recover a panic in another goroutine
		// from the caller. Model output reaches this code, so a bad slice or type assertion is a plausible stage
		// failure - it must stay one, not become a process exit.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("campaign stage panicked: %v\n%s", r, debug.Stack())
				select {
				case ch <- false:
				case <-cctx.Done():
				}
			}
		}()
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
//
// The premise and the tone and style the owner chose are read from the game itself, so every step of the pipeline - the
// creative seed, the core, the stages and the opening frame - works from the same value.
//
// worldSettings is the map-settings document the lobby's campaign world should be generated from; an empty value falls
// back to the server's configured settings file. A game that already carries a world ignores it.
func RunLobbyCampaignCoreWithStages(ctx context.Context, d *Deps, gameID, language, actingUserID string, worldSettings map[string]interface{}) LobbyCampaignOutcome {
	if _, err := gameaccess.AssertGameMember(ctx, d.Coll, actingUserID, gameID); err != nil {
		st := 403
		if err == gameaccess.ErrGameNotFound {
			st = 404
		}
		return LobbyCampaignOutcome{OK: false, Status: st, Error: err.Error()}
	}
	dir, err := campaignDirectionForGame(ctx, d, gameID)
	if err != nil {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: err.Error(), Code: "CAMPAIGN_DIRECTION_UNAVAILABLE"}
	}
	_, userTemplate := promptmgr.LoadCampaignGeneratorParts()
	if strings.TrimSpace(userTemplate) == "" {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Server misconfiguration: templates/campaign/generator.md is required", Code: "CAMPAIGN_TEMPLATE_MISSING"}
	}
	premise := dir.Premise

	// The party's adventure is written on a world that already exists, so the map comes first and its image is
	// rendered while the campaign text is being generated.
	worldBody := map[string]interface{}{}
	if worldSettings != nil {
		worldBody["worldSettings"] = worldSettings
	}
	world, worldMapCh, st, code, problem := campaignWorldWithLock(ctx, d, gameID, worldBody, language)
	if problem != "" {
		return LobbyCampaignOutcome{OK: false, Status: st, Error: problem, Code: code}
	}
	worldContext := renderCampaignWorldContext(world.World)
	worldData := campaignWorldTemplateData(world.World)

	// A start that failed halfway keeps what it already wrote: the creative seed, the campaign core and every stage that
	// finished are in the document. A retry resumes from there instead of asking the model for the whole adventure
	// again, which is the difference between one slow step and losing the work of five.
	spec := storedCampaignSpec(ctx, d, gameID)
	var seedDraw struct {
		ok   bool
		seed map[string]interface{}
		raw  string
		code string
	}
	if storedSeed, ok := storedCreativeSeed(spec); ok {
		// The seed is the premise of everything after it, so it is the one step a retry must not redo: a second seed
		// would describe a different adventure from the stages already stored.
		log.Printf("campaign resume: game %s keeps the stored creative seed", gameID)
		seedDraw.ok, seedDraw.seed = true, storedSeed
	} else {
		seedRaceMs := resolveCreativeSeedRaceTimeout()
		seedCtx, cancelSeed := context.WithTimeout(ctx, seedRaceMs)
		ch := make(chan struct {
			ok   bool
			seed map[string]interface{}
			raw  string
			code string
		}, 1)
		go func() {
			ok, seed, raw, code := generateCampaignCreativeSeedStage(seedCtx, d, gameID, language, dir)
			ch <- struct {
				ok   bool
				seed map[string]interface{}
				raw  string
				code string
			}{ok, seed, raw, code}
		}()
		select {
		case <-seedCtx.Done():
			cancelSeed()
			seedDraw.code = "CAMPAIGN_STAGE_CREATIVE_SEED_TIMEOUT"
		case v := <-ch:
			cancelSeed()
			seedDraw = v
		}
	}
	if !seedDraw.ok {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed generating campaign creative-seed stage", Code: firstNonEmpty(seedDraw.code, "CAMPAIGN_STAGE_CREATIVE_SEED")}
	}
	if _, resumed := storedCreativeSeed(spec); resumed {
		// Already in the document, and rewriting it would only touch rawModelOutput with an empty string.
	} else if err := persistCreativeSeedToGameState(ctx, d.Coll, gameID, seedDraw.seed, seedDraw.raw); err != nil {
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
	// The core is the campaign proper. A retry that already has one keeps it: every stage below was written on
	// top of this text, and a different core would leave the stored stages describing another adventure.
	var parsed map[string]interface{}
	if storedCore, ok := storedCampaignCore(spec); ok {
		log.Printf("campaign resume: game %s keeps the stored campaign core", gameID)
		parsed = storedCore
	} else {
		coreSys := promptmgr.BuildCampaignCoreSystemMsgs(language, dir.ToneAndStyle, creativeJSON, worldContext, worldData)
		consolidated := promptmgr.ConsolidateSystemMessages(coreSys)
		userPromptRendered, err := mustache.Render(userTemplate, map[string]interface{}{
			"sessionSummary":   "",
			"language":         language,
			"hostPremise":      premise,
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
			"max_tokens": 3000, "temperature": 0.8, "gameId": gameID, "heavy": true,
		})
		if err != nil {
			// The reason is what the lobby's lastStartError shows: this runs after the start already answered, so a
			// separate detail field would never reach the table.
			return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Campaign generation failed: " + err.Error(), Code: "CAMPAIGN_CORE_EMPTY", Detail: errString(err)}
		}
		if strings.TrimSpace(aiCore) == "" {
			return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Campaign generation failed: the model returned no text", Code: "CAMPAIGN_CORE_EMPTY"}
		}
		parsed, ok := llm.ParseModelStructuredObject(aiCore)
		if !ok || parsed == nil {
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
				"rawModelOutput": trunc200k(aiCore),
			}}, options.Update().SetUpsert(true))
			return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed to parse campaign core (YAML)", Code: "CAMPAIGN_CORE_PARSE", Raw: trunc200k(aiCore)[:min(4000, len(trunc200k(aiCore)))]}
		}
		ensureCampaignCoreTitle(parsed)
		applyHostToneAndStyle(parsed, dir.ToneAndStyle)
		// Written before the stages run, so a stage that fails does not throw away the campaign it was written on.
		if err := persistCampaignCore(ctx, d.Coll, gameID, parsed); err != nil {
			return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed persisting campaign core", Code: "CAMPAIGN_CORE_PERSIST"}
		}
	}
	stageMs := resolveLobbyPipelineStageTimeout()
	for _, stage := range []string{"factions", "majorNPCs", "keyLocations"} {
		if campaignStageStored(spec, stage) {
			log.Printf("campaign resume: game %s keeps the stored %s stage", gameID, stage)
			continue
		}
		var reason string
		ok := runStageWithTimeout(ctx, stageMs, func(c context.Context) bool {
			var ok bool
			ok, reason = generateCampaignStage(c, d, gameID, stage, parsed, language)
			return ok
		})
		if !ok {
			detail := campaignStageFailureDetail(reason, stage, stageMs)
			recordStageFailure(ctx, d, gameID, stage, detail)
			msg, code := campaignStageFailureMessage(stage)
			return LobbyCampaignOutcome{OK: false, Status: 500, Error: msg, Code: code, Detail: detail}
		}
	}
	// The world's text is written; what remains is the opening scene, which is a different wait for the table.
	if campaignOpeningFrameStored(spec) {
		log.Printf("campaign resume: game %s keeps the stored opening scene", gameID)
	} else {
		setPartyStage(ctx, d, gameID, "opening")
		if !runStageWithTimeout(ctx, stageMs, func(c context.Context) bool {
			return generateCampaignOpeningFrameStage(c, d, gameID, language, dir)
		}) {
			return LobbyCampaignOutcome{OK: false, Status: 500, Error: "Failed generating opening scene frame stage", Code: "CAMPAIGN_STAGE_OPENING_FRAME"}
		}
	}
	// The picture was stored while the stages ran; wait for that before reading the spec back, because the campaign is
	// written back as a whole and would otherwise drop the map marker the save just set.
	mapWarning := finishCampaignWorldMapDownload(worldMapCh)
	var existing map[string]interface{}
	_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&existing)
	var existingSpec map[string]interface{}
	if existing != nil {
		existingSpec, _ = existing["campaignSpec"].(map[string]interface{})
	}
	combined := mergeCampaignSpecLobby(parsed, existingSpec)
	// rawModelOutput is deliberately left as the stages wrote it: on a resumed start the core's own reply is not in
	// this process any more, and blanking the field would hide what the last step actually returned.
	_, err = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"campaignSpec": combined,
	}}, options.Update().SetUpsert(true))
	if err != nil {
		return LobbyCampaignOutcome{OK: false, Status: 500, Error: err.Error(), Code: "CAMPAIGN_PIPELINE_EXCEPTION"}
	}
	_ = draftparty.ClearDraftPartyTtlIfCampaignNowSubstantive(ctx, d.Coll, gameID)
	return LobbyCampaignOutcome{OK: true, Combined: combined, MapImageWarning: mapWarning}
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
