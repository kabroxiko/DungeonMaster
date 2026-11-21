package gamesession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	// Detail is the provider's own reason. Without it the reply is only "LLM call failed", which says nothing to the
	// operator and nothing to the table: every backend failure - a refusal, a cut budget, an unreachable host - looked
	// identical.
	Detail     string `json:"detail,omitempty"`
	RawPreview string `json:"rawPreview,omitempty"`
}

// HandleDmGenerate answers a DM turn: it prompts the model with the messages the caller built and writes the reply back
// into the stored game. Everything it persists comes from that document, never from the caller, so a client that reloaded
// with a partial view cannot truncate the transcript by sending it back.
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
	// The stored document is the base of the turn, not the copy the browser sends. The transcript, the counters and the
	// setup all belong to the server, and a tab that reloaded with a partial view used to write that partial view back
	// over the stored history. The client now only contributes the action being asked about.
	var persistPayload map[string]interface{}
	if strings.TrimSpace(gameID) != "" {
		persistPayload = storedTurnBase(ctx, d.Coll, gameID)
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

	// The consolidated system prompt leads every request, so its order is the provider's prefix-cache key: blocks
	// that never change between turns go first, blocks that change every turn go last. Hoisting per-turn state above
	// the static rules would change the leading bytes on every turn and throw away the cache for the whole request.
	consolidated := composeDmSystemPrompt(mode, language, sessionSummary, gs, requestingUserID)

	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
	}
	for _, m := range inboundForModel {
		msgs = append(msgs, m)
	}

	_ = saveRawRequest(ctx, d.Coll, gameID, msgs)
	_ = markLlmStarted(ctx, d.Coll, gameID)

	tok := estimateTokens(msgs)
	// A reasoning model spends part of this budget on the analysis it never shows, then writes the visible turn. 550
	// left gpt-oss-20b ending play turns with nothing but thinking, so the budget covers the analysis and the turn.
	budget := 1200
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

	// The opening adventure is authored once and read for the whole campaign; every later turn is a play turn.
	heavy := mode == "initial"

	// The dice belong to the server, and the DM asks for them while it writes the turn: it opens the fight, rolls, reads
	// who is standing, and only then narrates. A host that refuses the tools field keeps working through the declaration
	// pass, which produces the same numbers in one block before the narration call.
	var engineState map[string]interface{}
	var combat *combatTools
	if !heavy {
		var sheet map[string]interface{}
		if gsetup, ok := gs["gameSetup"].(map[string]interface{}); ok {
			sheet = pc.CharacterForUser(gsetup, requestingUserID)
		}
		state, _ := gs["encounterState"].(map[string]interface{})
		combat = newCombatTools(state, sheet, pc.DisplayNameFromCharacterSheet(sheet), rand.New(rand.NewSource(time.Now().UnixNano())))
		if _, url, _, backendErr := llm.BackendFor(cfg, heavy); backendErr == nil && llm.ToolsUnsupported(url) {
			// This host already said no: use the declaration pass from the start instead of spending a call finding out.
			if block, state, engineErr := runCombatEngine(ctx, cfg, gs, sheet, inboundForModel, gameID); engineErr != nil {
				log.Printf("dm generate (gameId=%s): combat declaration failed, narrating without engine results: %v", gameID, engineErr)
			} else if block != "" {
				msgs = insertSystemBlock(msgs, block)
				engineState = state
			}
			combat = nil
		}
	}

	ask := func(messages []interface{}) (string, error) {
		opts := map[string]interface{}{
			"max_tokens":      budget,
			"temperature":     0.8,
			"gameId":          gameID,
			"response_format": map[string]interface{}{"type": "json_object"},
			"heavy":           heavy,
		}
		// A play turn is a short narrative reply, not a problem to solve: a reasoning model that thinks about it for
		// hundreds of hidden tokens makes the table wait half a minute, and can end the turn with nothing to read.
		if !heavy {
			opts["reasoning_effort"] = "low"
		}
		if combat != nil {
			// JSON mode and tool calls fight over the same reply: a grammar that forces an object leaves no room for a
			// tool call. The envelope rules are in the prompt, and the corrective retry covers the rest.
			toolOpts := map[string]interface{}{}
			for k, v := range opts {
				toolOpts[k] = v
			}
			delete(toolOpts, "response_format")
			out, toolErr := llm.GenerateWithTools(ctx, cfg, map[string]interface{}{"messages": messages}, toolOpts, combat.defs(), combat.run)
			if toolErr != llm.ErrToolsUnsupported {
				return out, toolErr
			}
			log.Printf("dm generate (gameId=%s): this endpoint refused tools; using the declaration pass instead", gameID)
			if block, state, engineErr := runCombatEngine(ctx, cfg, gs, combat.sheet, inboundForModel, gameID); engineErr == nil && block != "" {
				messages = insertSystemBlock(messages, block)
				engineState = state
			}
		}
		return llm.GenerateResponse(ctx, cfg, map[string]interface{}{"messages": messages}, opts)
	}
	// A prose reply gets one corrective attempt before the turn is failed: a refusal is not an answer the table can
	// use, and most of them come back as a real turn once the format is restated.
	raw, env, err := envelopeWithCorrection(ask, msgs)
	if err != nil {
		// The reason is logged and returned: "see server logs" was a promise nothing kept, because the only place it
		// reached was the game document.
		log.Printf("dm generate (gameId=%s): %v", gameID, err)
		_ = markLlmErr(ctx, d.Coll, gameID, err.Error())
		_ = markLlmDone(ctx, d.Coll, gameID)
		return 500, &HandleDmGenerateResult{Error: "LLM call failed", Detail: err.Error()}
	}
	if env == nil {
		_ = markLlmDone(ctx, d.Coll, gameID)
		if strings.TrimSpace(raw) == "" {
			return 500, &HandleDmGenerateResult{Error: "AI response was empty or failed (see server logs)."}
		}
		return 502, &HandleDmGenerateResult{Error: "DM reply was not valid envelope JSON", RawPreview: trunc(raw, 1200)}
	}

	if engineState == nil && combat != nil {
		if state, ok := combat.changed(); ok {
			engineState = state
		}
	}

	// The engine's state is the record. The model reads the hit points it was handed and may not rewrite them: a foe
	// declared unconscious while its stored hp was 3 is the same defect as a miss that dealt damage.
	if engineState != nil {
		env.EncounterState = engineState
	}

	// The DM's own statement that a fight is starting is checked against the engine: a turn that declares combat and
	// never asks for a die gives the table prose with no outcome, which is what "no me ataca de vuelta" looks like.
	if combat != nil && env.ImminentCombat && !combat.usedAny() {
		log.Printf("dm generate (gameId=%s): combat announced without asking the engine; asking once more", gameID)
		corrected := append([]interface{}{}, msgs...)
		corrected = append(corrected, map[string]interface{}{"role": "system", "content": dmCombatCorrection})
		if secondRaw, secondEnv, secondErr := envelopeWithCorrection(ask, corrected); secondErr == nil && secondEnv != nil {
			raw, env = secondRaw, secondEnv
		} else if secondErr != nil {
			log.Printf("dm generate (gameId=%s): the second attempt at the combat turn failed: %v", gameID, secondErr)
		}
	}

	_ = markLlmCompleted(ctx, d.Coll, gameID, raw, env)

	persisted := false
	if persistPayload != nil && str(persistPayload["gameId"]) == gameID {
		rid := requestingUserID
		merged := persist.MergePersistWithAssistantReply(persistPayload, map[string]interface{}{
			"narration":      env.Narration,
			"encounterState": env.EncounterState,
			"coinage":        env.Coinage,
		}, false, rid)
		if merged != nil {
			_, err := persist.PersistGameStateFromBody(ctx, d.Coll, d.Hub, merged, userID, true)
			persisted = err == nil
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

	// A turn may name a site the players must be able to find. Marking it re-renders the map, which takes seconds, so
	// it runs after the stored reply: the table reads the narration and the map catches up behind it. A turn that was
	// never stored marks nothing, because the marker belongs to the world the game keeps.
	if persisted {
		StartMapPlaceUpdate(d, gameID, env.NewPlaces)
	}

	// The party pin is only arithmetic over the picture the table already has, so it is stored before the reply goes
	// out: every client that loads the turn finds the position it describes.
	if persisted && env.PartyPosition != nil {
		if label, err := ApplyPartyPosition(ctx, d, gameID, env.PartyPosition); err != nil {
			log.Printf("party position: gameId=%s: %v", gameID, err)
		} else if label != "" {
			log.Printf("party position: gameId=%s now at %s", gameID, label)
		}
	}
	return 200, res
}

type dmEnvelope struct {
	Narration string
	// ImminentCombat is the DM's own statement that a fight is starting. The server reads it to check the turn against
	// itself: a fight that begins without a single die asked for has no mechanics behind it.
	ImminentCombat bool
	EncounterState interface{}
	Coinage        map[string]interface{}
	// NewPlaces are the sites this turn introduced that the players must be able to find on the map.
	NewPlaces []NewPlace
	// PartyPosition is where this turn left the party, so the map pin follows travel and encounters.
	PartyPosition *PartyPosition
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
	switch v := obj["imminentCombat"].(type) {
	case bool:
		out.ImminentCombat = v
	case string:
		out.ImminentCombat = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	if es, ok := obj["encounterState"].(map[string]interface{}); ok {
		out.EncounterState = es
	}
	if cg, ok := obj["coinage"].(map[string]interface{}); ok {
		out.Coinage = cg
	}
	out.NewPlaces = ParseNewPlaces(obj["newPlaces"])
	out.PartyPosition = ParsePartyPosition(obj["partyPosition"])
	return out
}

// composeDmSystemPrompt builds the single system message for one play turn.
//
// Blocks are ordered most-stable first so the provider's prefix cache can reuse the leading bytes across turns:
// static rules, then campaign-scoped state, then per-turn state (session summary, party position, character sheet).
// Anything that changes every turn must stay below the static rules, because changing the head of the prompt
// invalidates the cache for every message that follows it.
func composeDmSystemPrompt(mode, language, sessionSummary string, gs map[string]interface{}, requestingUserID string) string {
	// Campaign-scoped blocks: identical for every turn of this game.
	var spec map[string]interface{}
	if gs != nil {
		spec, _ = gs["campaignSpec"].(map[string]interface{})
	}
	var parts []string
	// The table's voice leads the stack: it is the register every block below is read in. It is also campaign state,
	// not an opening flourish, so a table that chose a voice keeps it on every turn instead of only the first.
	if spec != nil {
		tone, _ := spec["toneAndStyle"].(string)
		if voice := promptmgr.CampaignVoiceBlock(tone); voice != "" {
			parts = append(parts, voice)
		}
	}
	// The player never gets a menu. This leads the rules because a small model that reads it late tends to close on
	// "you could do A or B" even when the rule exists further down the stack.
	if rule := promptmgr.LoadPrompt("rules/no_player_options.md"); rule != "" {
		parts = append(parts, rule)
	}
	parts = append(parts, promptmgr.LoadPrompt("templates/dm/player_response_envelope.md"))
	if guard := promptmgr.LoadPrompt("rules/json_output_guard_dm_play.md"); guard != "" {
		parts = append(parts, guard)
	} else {
		parts = append(parts, promptmgr.LoadPrompt("rules/json_output_guard.md"))
	}
	// Empty summary here: the memory block is per-turn state and joins the per-turn group below.
	for _, m := range promptmgr.ComposeSystemMessages(mode, "", true, language) {
		parts = append(parts, m["content"])
	}
	// The browser does not decide when a fight starts - it sends no mode at all, so every play turn arrived as
	// exploration and the combat procedure never loaded. A table's first attack was therefore improvised by a model with
	// no initiative, no encounter bookkeeping and no defender: it invented a bystander with an armor class so it had
	// something to roll against. Refereeing combat is not a rare mode, it is the job; the block itself decides when
	// formal combat applies, so it rides along on every play turn and only the opening is left without it.
	if mode != "combat" && mode != "initial" {
		if skill := promptmgr.LoadPrompt("skills/combat.md"); skill != "" {
			parts = append(parts, promptmgr.RenderSkillPrompt(skill, language))
		}
	}
	if mode == "initial" && spec != nil {
		// The opening has to be written from the campaign the server stored: its concept, conflicts, factions, NPCs
		// and key locations, with location names reconciled against the players' map.
		parts = append(parts, renderInitialCampaignInject(spec))
	}
	if spec != nil {
		// The Vellaris world is the campaign's only geography: there is no second, invented topology to fall back on,
		// because two competing geographies in one prompt make the DM drift on location.
		parts = append(parts, renderVellarisWorldInject(spec))
		if inj := promptmgr.LoadPrompt("templates/dm/inject_explore.md"); inj != "" && mode == "exploration" {
			// minimal Mustache data
			data := map[string]interface{}{
				"factions":                   []interface{}{},
				"dmHiddenAdventureObjective": dmHiddenObjective(spec),
			}
			rendered, _ := mustache.Render(inj, data)
			parts = append(parts, rendered)
		}
	}
	// Per-turn state.
	parts = append(parts, promptmgr.MemorySummaryMessage(sessionSummary))
	if spec != nil {
		// Where the party stands is the one piece of state that makes travel continuous across turns.
		parts = append(parts, renderPartyPositionInject(spec))
	}
	parts = append(parts, renderPCBlock(gs, language, requestingUserID))
	return joinPromptBlocks(parts)
}

// joinPromptBlocks joins the non-empty system blocks in the order given, so a prompt file that is missing or empty
// does not leave a stray separator in the leading bytes the provider's prefix cache keys on.
func joinPromptBlocks(parts []string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n\n")
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
	tpl := promptmgr.LoadPrompt("templates/dm/player_character_context.md")
	if tpl == "" {
		return ""
	}
	data := map[string]interface{}{
		"language":                   language,
		"playerCharacterDisplayName": pc.DisplayNameFromCharacterSheet(sheet),
		"playerCharacterJson":        trunc(jsonStr(sheet), 120000),
		"priorEncounterStateJson":    priorEncounterStateJSON(gs),
	}
	s, err := mustache.Render(tpl, data)
	if err != nil {
		return ""
	}
	return s
}

// priorEncounterStateJSON is the encounter the table is in, as the server stored it last turn.
//
// The template has always had a slot for it and the slot was filled with "{}": the DM was asked to keep hit points,
// initiative and armor class in encounterState and then never shown what it had kept, so on the next beat it had no
// numbers to resolve against and asked the table for one. A value the model is not given is a value it invents or
// requests mid-fight.
func priorEncounterStateJSON(gs map[string]interface{}) string {
	if gs == nil {
		return "{}"
	}
	raw, ok := gs["encounterState"]
	if !ok || raw == nil {
		return "{}"
	}
	b, err := json.Marshal(raw)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return "{}"
	}
	return trunc(string(b), 20000)
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
	tpl := promptmgr.LoadPrompt("templates/dm/inject_initial.md")
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
		entry := map[string]interface{}{
			"name": name,
			"kind": campaignspec.PlaceKind(fmt.Sprint(m["type"])),
		}
		// The opening scene is set somewhere real, so the region and ground around each candidate travel with its name.
		for _, key := range []string{"regionSuffix", "surroundingsSuffix"} {
			if suffix, ok := m[key].(string); ok && suffix != "" {
				entry[key] = suffix
			}
		}
		mapPlaces = append(mapPlaces, entry)
		if len(mapPlaces) >= 20 {
			break
		}
	}
	data := map[string]interface{}{
		"campaignTitle":              strings.TrimSpace(fmt.Sprint(spec["title"])),
		"campaignConcept":            strings.TrimSpace(fmt.Sprint(spec["campaignConcept"])),
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

// toIfaceSlice reads a list out of either a decoded document or a request body. A BSON array decodes to primitive.A,
// not to []interface{}, so asserting only the latter silently turned every stored list - the conversation above all -
// into an empty one.
func toIfaceSlice(v interface{}) []interface{} {
	switch a := v.(type) {
	case []interface{}:
		return a
	case primitive.A:
		return []interface{}(a)
	}
	return nil
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
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{
		"$set": bson.M{
			"llmCallStartedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
		// A new attempt supersedes the reason the last one failed; leaving it would keep offering a retry for a turn
		// that is already running.
		"$unset": bson.M{"llmCallError": ""},
	}, upsert())
	return err
}

func markLlmCompleted(ctx context.Context, coll *mongo.Collection, gameID, raw string, env *dmEnvelope) error {
	if gameID == "" {
		return nil
	}
	_, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{
		"$set": bson.M{
			"rawModelOutput":     trunc(raw, 200000),
			"llmCallCompletedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
		"$unset": bson.M{"llmCallError": ""},
	}, upsert())
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
