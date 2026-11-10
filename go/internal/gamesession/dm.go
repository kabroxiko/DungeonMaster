package gamesession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/llm"
	"github.com/deckofdmthings/gmai/internal/pc"
	"github.com/deckofdmthings/gmai/internal/persist"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
)

// HandleDmGenerateResult is the JSON body for 200 responses.
type HandleDmGenerateResult struct {
	Narration      string                 `json:"narration"`
	EncounterState interface{}            `json:"encounterState"`
	ActiveCombat   bool                   `json:"activeCombat"`
	Coinage        map[string]interface{} `json:"coinage,omitempty"`
	Error          string                 `json:"error,omitempty"`
	Code           string                 `json:"code,omitempty"`
	RawPreview     string                 `json:"rawPreview,omitempty"`
}

// HandleDmGenerate mirrors Express handleDmGenerate (core path: exploration + persist).
func HandleDmGenerate(ctx context.Context, d *Deps, cfg *config.Config, userID string, body map[string]interface{}) (status int, out *HandleDmGenerateResult) {
	res := &HandleDmGenerateResult{}
	messages := toIfaceSlice(body["messages"])
	mode := str(body["mode"])
	if mode == "" {
		mode = "exploration"
	}
	if mode == "explore" {
		mode = "exploration"
	}
	sessionSummary := str(body["sessionSummary"])
	language := str(body["language"])
	if language == "" {
		language = "English"
	}
	gameID := str(body["gameId"])
	var persistPayload map[string]interface{}
	if p, ok := body["persist"].(map[string]interface{}); ok {
		persistPayload = p
	}
	requestingUserID := str(body["requestingUserId"])
	if requestingUserID == "" {
		requestingUserID = userID
	}

	if gameID != "" {
		if _, err := gameaccess.AssertGameMember(ctx, d.Coll, userID, gameID); err != nil {
			if err == gameaccess.ErrGameNotFound {
				return 404, &HandleDmGenerateResult{Error: "Game not found", Code: "GAME_NOT_FOUND"}
			}
			return 400, &HandleDmGenerateResult{Error: err.Error()}
		}
	}

	_ = markLlmEntered(ctx, d.Coll, gameID)

	if mode == "initial" && gameID == "" {
		return 400, &HandleDmGenerateResult{Error: "Initial adventure generation requires a gameId with an existing campaign."}
	}

	var gs map[string]interface{}
	if gameID != "" {
		_ = d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&gs)
	}
	if mode == "initial" {
		spec, _ := gs["campaignSpec"].(map[string]interface{})
		if spec == nil {
			return 400, &HandleDmGenerateResult{Error: "No campaignSpec found for this gameId. Please generate the campaign core before generating the initial adventure."}
		}
		_, _, ok := campaignspec.ResolveOpeningMandateFromCampaign(spec)
		if !ok {
			_ = markLlmError(ctx, d.Coll, gameID, "OPENING_FRAME_MISSING")
			return 422, &HandleDmGenerateResult{
				Error: "This campaign has no valid opening scene frame.",
				Code:  "OPENING_FRAME_MISSING",
			}
		}
		sessionSummary = ""
	}

	inbound := filterNoSystem(messages)
	inboundForModel := withSpeakerAttribution(inbound)

	sys := promptmgr.ComposeSystemMessages(mode, sessionSummary, true, language)
	var sysContents []string
	for _, m := range sys {
		sysContents = append(sysContents, m["content"])
	}
	consolidated := strings.Join(sysContents, "\n\n")
	guard := promptmgr.LoadPrompt("rules/json_output_guard_dm_play.txt")
	if guard == "" {
		guard = promptmgr.LoadPrompt("rules/json_output_guard.txt")
	}
	if guard != "" {
		consolidated = guard + "\n\n" + consolidated
	}
	if e := promptmgr.LoadPrompt("templates/dm/player_response_envelope.txt"); e != "" {
		consolidated = e + "\n\n" + consolidated
	}
	// The opening has to be written from the campaign the server stored: its concept, conflicts, factions, NPCs and
	// key locations, with location names reconciled against the players' map. Prepended before the world block so the
	// authoritative geography still reads first.
	if mode == "initial" && gs != nil {
		if spec, ok := gs["campaignSpec"].(map[string]interface{}); ok && spec != nil {
			if inj := renderInitialCampaignInject(spec); inj != "" {
				consolidated = inj + "\n\n" + consolidated
			}
		}
	}
	if gs != nil {
		if spec, ok := gs["campaignSpec"].(map[string]interface{}); ok && spec != nil {
			// The Vellaris world is the campaign's only geography: there is no second, invented topology to fall
			// back on, because two competing geographies in one prompt make the DM drift on location.
			if vw := renderVellarisWorldInject(spec); vw != "" {
				consolidated = vw + "\n\n" + consolidated
			}
			inj := promptmgr.LoadPrompt("templates/dm/inject_explore.txt")
			if inj != "" && (mode == "exploration" || mode == "explore") {
				// minimal Mustache data
				data := map[string]interface{}{
					"factions":                   []interface{}{},
					"dmHiddenAdventureObjective": dmHiddenObjective(spec),
				}
				rendered, _ := mustache.Render(inj, data)
				consolidated = rendered + "\n\n" + consolidated
			}
		}
	}
	pcBlock := renderPCBlock(gs, language, requestingUserID)
	if pcBlock != "" {
		consolidated = consolidated + "\n\n" + pcBlock
	}

	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
	}
	for _, m := range inboundForModel {
		msgs = append(msgs, m)
	}

	_ = saveRawRequest(ctx, d.Coll, gameID, msgs)
	_ = markLlmStarted(ctx, d.Coll, gameID)

	tok := estimateTokens(msgs)
	budget := 550
	if mode == "initial" {
		budget = 2200
	}
	cap := cfg.ModelContextTok - tok - 80
	if cap > 0 && cap < budget {
		budget = cap
	}
	if budget < 200 {
		budget = 200
	}

	raw, err := llm.GenerateResponse(ctx, cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens":      budget,
		"temperature":     0.8,
		"gameId":          gameID,
		"response_format": map[string]interface{}{"type": "json_object"},
	})
	if err != nil {
		_ = markLlmErr(ctx, d.Coll, gameID, err.Error())
		_ = markLlmDone(ctx, d.Coll, gameID)
		return 500, &HandleDmGenerateResult{Error: "LLM call failed (see server logs)."}
	}
	if strings.TrimSpace(raw) == "" {
		_ = markLlmDone(ctx, d.Coll, gameID)
		return 500, &HandleDmGenerateResult{Error: "AI response was empty or failed (see server logs)."}
	}

	env := parseDmEnvelope(raw)
	if env == nil {
		_ = markLlmDone(ctx, d.Coll, gameID)
		return 502, &HandleDmGenerateResult{Error: "DM reply was not valid envelope JSON", RawPreview: trunc(raw, 1200)}
	}
	if strings.TrimSpace(env.Narration) == "" {
		_ = markLlmDone(ctx, d.Coll, gameID)
		return 502, &HandleDmGenerateResult{Error: "The model returned empty narration.", RawPreview: trunc(raw, 1500)}
	}

	_ = markLlmCompleted(ctx, d.Coll, gameID, raw, env)

	if persistPayload != nil && str(persistPayload["gameId"]) == gameID {
		rid := requestingUserID
		merged := persist.MergePersistWithAssistantReply(persistPayload, map[string]interface{}{
			"narration":      env.Narration,
			"encounterState": env.EncounterState,
			"coinage":        env.Coinage,
		}, false, rid)
		if merged != nil {
			_, err := persist.PersistGameStateFromBody(ctx, d.Coll, d.Hub, merged, userID, true)
			if err != nil {
				var pe *persist.PersistError
				if errors.As(err, &pe) && strings.HasPrefix(pe.Code, "ENTITY_NAME_") {
					_ = markLlmDone(ctx, d.Coll, gameID)
					return 422, &HandleDmGenerateResult{Error: pe.Message, Code: pe.Code}
				}
			}
		}
	}

	_ = markLlmDone(ctx, d.Coll, gameID)
	res.Narration = env.Narration
	res.EncounterState = env.EncounterState
	res.ActiveCombat = false
	if env.Coinage != nil {
		res.Coinage = env.Coinage
	}
	return 200, res
}

type dmEnvelope struct {
	Narration      string
	EncounterState interface{}
	Coinage        map[string]interface{}
}

func parseDmEnvelope(raw string) *dmEnvelope {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &obj); err != nil {
		var ok bool
		obj, ok = llm.ParseModelStructuredObject(raw)
		if !ok || obj == nil {
			return nil
		}
	}
	if obj == nil {
		return nil
	}
	n, ok := obj["narration"].(string)
	if !ok || strings.TrimSpace(n) == "" {
		if v, ok := obj["narration"].(float64); ok {
			n = fmt.Sprint(v)
		} else {
			return nil
		}
	}
	if strings.TrimSpace(n) == "" {
		return nil
	}
	out := &dmEnvelope{Narration: n}
	if es, ok := obj["encounterState"].(map[string]interface{}); ok {
		out.EncounterState = es
	}
	if cg, ok := obj["coinage"].(map[string]interface{}); ok {
		out.Coinage = cg
	}
	return out
}

func filterNoSystem(messages []interface{}) []interface{} {
	var out []interface{}
	for _, m := range messages {
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

func withSpeakerAttribution(messages []interface{}) []interface{} {
	var out []interface{}
	for _, raw := range messages {
		m, ok := raw.(map[string]interface{})
		if !ok {
			out = append(out, raw)
			continue
		}
		if r, _ := m["role"].(string); r != "user" {
			out = append(out, raw)
			continue
		}
		content := str(m["content"])
		dn := str(m["displayName"])
		if dn != "" {
			mm := cloneMap(m)
			mm["content"] = "[" + dn + "]: " + content
			out = append(out, mm)
			continue
		}
		uid := str(m["userId"])
		if len(uid) > 8 {
			mm := cloneMap(m)
			mm["content"] = "[player " + uid[len(uid)-8:] + "]: " + content
			out = append(out, mm)
			continue
		}
		out = append(out, raw)
	}
	return out
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	o := map[string]interface{}{}
	for k, v := range m {
		o[k] = v
	}
	return o
}

func renderPCBlock(gs map[string]interface{}, language, requestingUserID string) string {
	if gs == nil {
		return ""
	}
	gsetup, _ := gs["gameSetup"].(map[string]interface{})
	if gsetup == nil {
		return ""
	}
	sheet := pc.CharacterForUser(gsetup, requestingUserID)
	if sheet == nil {
		return ""
	}
	tpl := promptmgr.LoadPrompt("templates/dm/player_character_context.txt")
	if tpl == "" {
		return ""
	}
	data := map[string]interface{}{
		"language":                   language,
		"playerCharacterDisplayName": pc.DisplayNameFromCharacterSheet(sheet),
		"playerCharacterJson":        trunc(jsonStr(sheet), 120000),
		"priorEncounterStateJson":    "{}",
	}
	s, err := mustache.Render(tpl, data)
	if err != nil {
		return ""
	}
	return s
}

func dmHiddenObjective(spec map[string]interface{}) string {
	if spec == nil {
		return ""
	}
	if s, ok := spec["dmHiddenAdventureObjective"].(string); ok && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return "Not set in campaign core: infer one coherent arc from campaignConcept and majorConflicts."
}

// renderInitialCampaignInject renders the stored campaign for the opening turn.
//
// Without it the opening model only saw the world and the character sheet, so it invented its own story instead of
// playing the campaign that was generated. Key location names are reconciled against the players' map here as well,
// which also repairs campaigns whose locations were written before place-name alignment existed.
func renderInitialCampaignInject(spec map[string]interface{}) string {
	if spec == nil {
		return ""
	}
	tpl := promptmgr.LoadPrompt("templates/dm/inject_initial.txt")
	if tpl == "" {
		return ""
	}
	storedKeyLocations := llm.CoerceCampaignStageToArray("keyLocations", spec["keyLocations"])
	keyLocations := storedKeyLocations
	world := campaignspec.NormalizeVellarisWorld(mapFrom(spec, "vellarisWorld"))
	if world != nil {
		aligned, renamed := campaignspec.AlignKeyLocationsToWorld(keyLocations, world)
		if renamed > 0 {
			log.Printf("initial inject: reconciled %d key location name(s) with the campaign map", renamed)
		}
		keyLocations = aligned
	}
	mapPlaces := make([]interface{}, 0, 20)
	for _, item := range llm.SliceValues(world["places"]) {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		name, _ := m["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		mapPlaces = append(mapPlaces, map[string]interface{}{
			"name": name,
			"kind": campaignspec.PlaceKind(fmt.Sprint(m["type"])),
		})
		if len(mapPlaces) >= 20 {
			break
		}
	}
	data := map[string]interface{}{
		"campaignTitle":              strings.TrimSpace(fmt.Sprint(spec["title"])),
		"campaignConcept":            strings.TrimSpace(fmt.Sprint(spec["campaignConcept"])),
		"campaignToneAndStyle":       strings.TrimSpace(fmt.Sprint(spec["toneAndStyle"])),
		"factions":                   sliceFirst(llm.CoerceCampaignStageToArray("factions", spec["factions"]), 6),
		"majorConflicts":             TakeCampaignFieldItems(spec["majorConflicts"], 6),
		"majorNPCs":                  sliceFirst(llm.CoerceCampaignStageToArray("majorNPCs", spec["majorNPCs"]), 6),
		"keyLocations":               sliceFirst(keyLocations, 8),
		"dmHiddenAdventureObjective": dmHiddenObjective(spec),
		"hasMapPlaces":               len(mapPlaces) > 0,
		"mapPlaces":                  mapPlaces,
	}
	if _, directive, ok := campaignspec.ResolveOpeningMandateFromCampaign(spec); ok {
		// The frame is a mandatory instruction for the first scene; names it took from the old key locations are
		// rewritten to the map names that replaced them.
		data["openingDirective"] = reconcileDirectiveNames(directive, storedKeyLocations, keyLocations)
	}
	rendered, err := mustache.Render(tpl, data)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(rendered)
}

func jsonStr(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// reconcileDirectiveNames rewrites names from the stored key locations to the map names that replaced them.
//
// An opening frame is generated once and kept. For a campaign written before place-name alignment, its directive can
// still send the DM to an invented place, which would put the opening back off the map.
func reconcileDirectiveNames(directive string, stored, aligned []interface{}) string {
	text := strings.TrimSpace(directive)
	if text == "" {
		return text
	}
	pairs := make([][2]string, 0, len(stored))
	for i := 0; i < len(stored) && i < len(aligned); i++ {
		from, _ := stored[i].(map[string]interface{})
		to, _ := aligned[i].(map[string]interface{})
		if from == nil || to == nil {
			continue
		}
		oldName := strings.TrimSpace(fmt.Sprint(from["name"]))
		newName := strings.TrimSpace(fmt.Sprint(to["name"]))
		if oldName == "" || newName == "" || strings.EqualFold(oldName, newName) {
			continue
		}
		pairs = append(pairs, [2]string{oldName, newName})
	}
	// Longest first, so "Old Keep Gate" is replaced before "Old Keep".
	sort.SliceStable(pairs, func(i, j int) bool { return len(pairs[i][0]) > len(pairs[j][0]) })
	for _, pair := range pairs {
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(pair[0]))
		text = re.ReplaceAllStringFunc(text, func(string) string { return pair[1] })
	}
	return text
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func estimateTokens(msgs []interface{}) int {
	var chars int
	for _, m := range msgs {
		mm, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		chars += len(str(mm["content"]))
	}
	return int(math.Max(1, math.Ceil(float64(chars)/4)))
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func toIfaceSlice(v interface{}) []interface{} {
	a, _ := v.([]interface{})
	return a
}

func markLlmEntered(ctx context.Context, coll *mongo.Collection, gameID string) error {
	if gameID == "" {
		return nil
	}
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"llmCallEnteredAt": time.Now().UTC().Format(time.RFC3339Nano),
	}})
	return err
}

func markLlmStarted(ctx context.Context, coll *mongo.Collection, gameID string) error {
	if gameID == "" {
		return nil
	}
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"llmCallStartedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}}, upsert())
	return err
}

func markLlmCompleted(ctx context.Context, coll *mongo.Collection, gameID, raw string, env *dmEnvelope) error {
	if gameID == "" {
		return nil
	}
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"rawModelOutput":     trunc(raw, 200000),
		"llmCallCompletedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}}, upsert())
	return err
}

func markLlmErr(ctx context.Context, coll *mongo.Collection, gameID, msg string) error {
	if gameID == "" {
		return nil
	}
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"llmCallError": trunc(msg, 200000),
	}}, upsert())
	return err
}

func markLlmError(ctx context.Context, coll *mongo.Collection, gameID, msg string) error {
	return markLlmErr(ctx, coll, gameID, msg)
}

func markLlmDone(ctx context.Context, coll *mongo.Collection, gameID string) error {
	if gameID == "" {
		return nil
	}
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"llmCallCompletedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}}, upsert())
	return err
}

func saveRawRequest(ctx context.Context, coll *mongo.Collection, gameID string, msgs []interface{}) error {
	if gameID == "" {
		return nil
	}
	b, _ := json.Marshal(msgs)
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"rawModelRequest": trunc(string(b), 200000),
	}}, upsert())
	return err
}

func upsert() *options.UpdateOptions {
	return options.Update().SetUpsert(true)
}
