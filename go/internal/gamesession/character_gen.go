package gamesession

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cbroglie/mustache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/draftparty"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/ironarachne"
	"github.com/deckofdmthings/gmai/internal/llm"
	"github.com/deckofdmthings/gmai/internal/party"
	"github.com/deckofdmthings/gmai/internal/pc"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
	"github.com/deckofdmthings/gmai/internal/realtime"
	"github.com/deckofdmthings/gmai/internal/validate"
)

// HandleGenerateCharacter mirrors POST /generate-character: prompts omit PC display names; server assigns names.
func HandleGenerateCharacter(ctx context.Context, d *Deps, cfg *config.Config, hub *realtime.Hub, userID string, body map[string]interface{}) (status int, out map[string]interface{}) {
	gsIn, _ := body["gameSetup"].(map[string]interface{})
	if gsIn == nil {
		gsIn = map[string]interface{}{}
	}
	applyBackendRandomIntent(gsIn)
	gsIn = ResolveCharacterSetupForGeneration(gsIn)
	language := "English"
	if l, ok := body["language"].(string); ok && strings.TrimSpace(l) != "" {
		language = strings.TrimSpace(l)
	}
	var gameID *string
	if raw := body["gameId"]; raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		s := strings.TrimSpace(fmt.Sprint(raw))
		gameID = &s
	}
	wantsNew := body["newParty"] == true || body["newParty"] == "true"
	if gameID == nil && !wantsNew {
		return 400, map[string]interface{}{
			"error": "This request needs a gameId (join an existing party) or newParty: true (start a brand-new party on the server).",
			"code":  "GAME_ID_OR_NEW_PARTY_REQUIRED",
		}
	}
	uidOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return 400, map[string]interface{}{"error": "Invalid user", "code": "USER_INVALID"}
	}
	var gameRow map[string]interface{}
	if gameID != nil {
		_ = d.Coll.FindOne(ctx, bson.M{"gameId": *gameID}).Decode(&gameRow)
		if gameRow != nil {
			if _, err := gameaccess.AssertGameMember(ctx, d.Coll, userID, *gameID); err != nil {
				if err == gameaccess.ErrGameNotFound {
					return 404, map[string]interface{}{"error": "Game not found", "code": "GAME_NOT_FOUND"}
				}
				return 400, map[string]interface{}{"error": err.Error()}
			}
		}
	}
	setupForPrompt := map[string]interface{}{}
	for k, v := range gsIn {
		setupForPrompt[k] = v
	}
	var gsSetup, campaignSpec, encounterState map[string]interface{}
	if gameRow != nil {
		gsSetup, _ = gameRow["gameSetup"].(map[string]interface{})
		campaignSpec, _ = gameRow["campaignSpec"].(map[string]interface{})
		encounterState, _ = gameRow["encounterState"].(map[string]interface{})
		if cs, ok := gameRow["campaignSpec"].(map[string]interface{}); ok && cs != nil {
			setupForPrompt["existingCampaignWorld"] = map[string]interface{}{
				"title": cs["title"], "campaignConcept": cs["campaignConcept"],
				"majorConflicts": cs["majorConflicts"], "toneAndStyle": cs["toneAndStyle"],
				"factions": cs["factions"], "majorNPCs": cs["majorNPCs"], "keyLocations": cs["keyLocations"],
			}
		}
	}
	preassignedPcName := ""
	// Respect an explicit player-entered name from gameSetup/body as authoritative.
	// We still keep Iron Arachne fallback for truly empty names.
	if raw := gsIn["name"]; raw != nil {
		s := strings.TrimSpace(fmt.Sprint(raw))
		if s != "" && !strings.EqualFold(s, "random") {
			preassignedPcName = s
		}
	}
	if preassignedPcName == "" {
		if raw := body["preassignedDisplayName"]; raw != nil {
			s := strings.TrimSpace(fmt.Sprint(raw))
			if s != "" && !strings.EqualFold(s, "random") {
				preassignedPcName = s
			}
		}
	}
	if preassignedPcName == "" {
		if raw := body["name"]; raw != nil {
			s := strings.TrimSpace(fmt.Sprint(raw))
			if s != "" && !strings.EqualFold(s, "random") {
				preassignedPcName = s
			}
		}
	}
	if preassignedPcName == "" {
		if raw := body["characterName"]; raw != nil {
			s := strings.TrimSpace(fmt.Sprint(raw))
			if s != "" && !strings.EqualFold(s, "random") {
				preassignedPcName = s
			}
		}
	}
	if preassignedPcName == "" {
		if ok, n, _ := ironarachne.TryPreassignDisplayName(ctx, gsIn["race"], gsIn["gender"], gsSetup, campaignSpec, encounterState, userID); ok {
			preassignedPcName = n
		}
	}
	resolvedDisplayName := ironarachne.ResolveDisplayNameForGeneration(ctx, preassignedPcName, gsIn, gameRow, userID)
	// Prompt stack: YAML guard + character-only scope + skills/character.txt (large on purpose — PHB gear/spells/coinage
	// so the model returns a complete level-1 sheet). Language text uses rules/language_*_character.txt (no campaign title rules).
	scope := promptmgr.LoadPrompt("skills/character_generation_scope.txt")
	if strings.TrimSpace(scope) == "" {
		return 500, map[string]interface{}{
			"error": "generate-character: dm_character_generation_scope.txt is missing or empty.",
			"code":  "CHARACTER_SCOPE_PROMPT_MISSING",
		}
	}
	var sys []map[string]string
	if g := promptmgr.LoadPrompt("rules/json_output_guard.txt"); g != "" {
		sys = append(sys, map[string]string{"role": "system", "content": g})
	}
	sys = append(sys, map[string]string{"role": "system", "content": scope})
	langFile := "rules/language_english_character.txt"
	if strings.HasPrefix(strings.ToLower(language), "span") {
		langFile = "rules/language_spanish_character.txt"
	}
	charPrompt := promptmgr.LoadPrompt("skills/character.txt")
	langInstr := strings.TrimSpace(promptmgr.LoadPrompt(langFile))
	renderedChar, err := mustache.Render(charPrompt, map[string]interface{}{
		"languageInstruction": langInstr,
		"language":            language,
	})
	if err != nil {
		return 500, map[string]interface{}{
			"error": "generate-character: failed to render skill_character.txt (check Mustache placeholders).",
			"code":  "CHARACTER_PROMPT_RENDER_FAILED", "details": err.Error(),
		}
	}
	sys = append(sys, map[string]string{"role": "system", "content": renderedChar})
	consolidated := promptmgr.ConsolidateSystemMessages(sys)
	const userMarker = "--- USER PROMPT BELOW (render this section as the user message) ---"
	if !strings.Contains(charPrompt, userMarker) {
		return 500, map[string]interface{}{
			"error": "generate-character: skill_character.txt must contain the USER PROMPT marker; no fallback prompt is used.",
		}
	}
	userTpl := strings.TrimSpace(strings.SplitN(charPrompt, userMarker, 2)[1])
	if userTpl == "" {
		return 500, map[string]interface{}{
			"error": "generate-character: skill_character.txt user section after marker is empty.",
		}
	}
	gsJSON := "{}"
	if b, err := json.Marshal(setupForPrompt); err == nil {
		gsJSON = string(b)
	}
	userContent, err := mustache.Render(userTpl, map[string]interface{}{
		"gameSetup":           gsJSON,
		"languageInstruction": langInstr,
		"language":            language,
		"pcDisplayName":       resolvedDisplayName,
	})
	if err != nil {
		return 500, map[string]interface{}{"error": "generate-character: failed to render user prompt template.", "details": err.Error()}
	}
	msgs := []interface{}{
		map[string]interface{}{"role": "system", "content": consolidated},
		map[string]interface{}{"role": "user", "content": userContent},
	}
	aiMessage, err := llm.GenerateResponse(ctx, cfg, map[string]interface{}{"messages": msgs}, map[string]interface{}{
		"max_tokens": 4096, "temperature": 0.75,
	})
	if err != nil || strings.TrimSpace(aiMessage) == "" {
		return 503, map[string]interface{}{
			"error":  "Character generation failed: the model returned no usable text.",
			"code":   "AI_RESPONSE_EMPTY",
			"detail": errString(err),
			"hint":   "Check DM_OPENAI_API_KEY or LM Studio settings.",
		}
	}
	parsed, ok := llm.ParseModelStructuredObject(aiMessage)
	if !ok || parsed == nil {
		preview := aiMessage
		if len(preview) > 8000 {
			preview = preview[:8000]
		}
		if gameID != nil {
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": *gameID}, bson.M{"$set": bson.M{"rawModelOutput": trunc200k(aiMessage)}}, options.Update().SetUpsert(true))
		}
		return 422, map[string]interface{}{
			"error": "The model did not return valid JSON with a top-level playerCharacter object. Check server logs for a preview.",
			"code": "INVALID_MODEL_JSON", "preview": preview, "previewLength": len(aiMessage),
		}
	}
	pcRaw, ok := parsed["playerCharacter"].(map[string]interface{})
	if !ok || pcRaw == nil {
		return 422, map[string]interface{}{"error": "Missing playerCharacter object", "code": "INVALID_MODEL_JSON"}
	}
	pc.StripModelGeneratedNames(pcRaw)
	pc.SyncDisplayNameFields(pcRaw, resolvedDisplayName)
	pc.ApplyLockedCharacterChoices(pcRaw, gsIn, language)
	if chk, errStr := validate.ValidateGeneratedPlayerCharacter(pcRaw); !chk {
		if gameID != nil {
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": *gameID}, bson.M{"$set": bson.M{"rawModelOutput": trunc200k(aiMessage)}}, options.Update().SetUpsert(true))
		}
		return 422, map[string]interface{}{"error": errStr, "code": "INVALID_PLAYER_CHARACTER"}
	}
	normalized := validate.EnsurePlayerCharacterSheetDefaults(pcRaw, language)
	persistGameID := ""
	if gameID != nil {
		persistGameID = *gameID
	} else {
		persistGameID = AllocateNewPartyGameID()
	}
	if err := persistGeneratedCharacter(ctx, d.Coll, hub, persistGameID, userID, uidOID, normalized, language); err != nil {
		return 500, map[string]interface{}{
			"error": "Character was generated but could not be saved to the game. Try again.",
			"code":  "CHARACTER_PERSIST_FAILED",
		}
	}
	if gameID != nil {
		var postDoc map[string]interface{}
		_ = d.Coll.FindOne(ctx, bson.M{"gameId": persistGameID}).Decode(&postDoc)
		if postDoc != nil && party.AdventureHasBegun(postDoc) {
			gs, _ := postDoc["gameSetup"].(map[string]interface{})
			next := PendingNarrativePatch(gs, userID)
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": persistGameID}, bson.M{"$set": bson.M{"gameSetup": next}})
			if hub != nil {
				hub.NotifyGameStateUpdated(persistGameID)
			}
		}
	}
	_ = draftparty.ApplyDraftPartyTtlAfterCharacterGen(ctx, d.Coll, persistGameID)
	return 200, map[string]interface{}{
		"playerCharacter": normalized,
		"gameId":          persistGameID,
	}
}

func applyBackendRandomIntent(gsIn map[string]interface{}) {
	if gsIn == nil {
		return
	}
	rawIntent, _ := gsIn["randomIntent"].(map[string]interface{})
	if rawIntent == nil {
		return
	}
	isTrue := func(v interface{}) bool {
		switch t := v.(type) {
		case bool:
			return t
		case string:
			return strings.EqualFold(strings.TrimSpace(t), "true")
		default:
			return false
		}
	}
	if isTrue(rawIntent["characterRace"]) {
		gsIn["race"] = "random"
		gsIn["subrace"] = "random"
	}
	if isTrue(rawIntent["characterClass"]) {
		gsIn["class"] = "random"
		gsIn["subclass"] = "random"
	}
	if isTrue(rawIntent["subrace"]) {
		gsIn["subrace"] = "random"
	}
	if isTrue(rawIntent["subclass"]) {
		gsIn["subclass"] = "random"
	}
	delete(gsIn, "randomIntent")
}

func persistGeneratedCharacter(ctx context.Context, coll *mongo.Collection, hub *realtime.Hub, gameID, uidStr string, uidOID primitive.ObjectID, normalized map[string]interface{}, language string) error {
	var existing map[string]interface{}
	err := coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&existing)
	if err == mongo.ErrNoDocuments {
		existing = nil
	} else if err != nil {
		return err
	}
	baseSetup, _ := existing["gameSetup"].(map[string]interface{})
	if baseSetup == nil {
		baseSetup = map[string]interface{}{}
	}
	prevMap, _ := baseSetup["playerCharacters"].(map[string]interface{})
	if prevMap == nil {
		prevMap = map[string]interface{}{}
	}
	nextMap := map[string]interface{}{}
	for k, v := range prevMap {
		nextMap[k] = v
	}
	nextMap[uidStr] = normalized
	nextSetup := map[string]interface{}{}
	for k, v := range baseSetup {
		nextSetup[k] = v
	}
	langToSet := language
	if l, ok := baseSetup["language"].(string); ok && strings.TrimSpace(l) != "" {
		langToSet = strings.TrimSpace(l)
	}
	nextSetup["language"] = langToSet
	nextSetup["playerCharacters"] = nextMap
	delete(nextSetup, "generatedCharacter")
	nextSetup = party.MergeParty(nextSetup, map[string]interface{}{})
	var camp map[string]interface{}
	if existing != nil {
		camp, _ = existing["campaignSpec"].(map[string]interface{})
	}
	var enc map[string]interface{}
	if existing != nil {
		enc, _ = existing["encounterState"].(map[string]interface{})
	}
	if ok, code, msg := validate.ValidateDistinctEntityNames(nextSetup, camp, enc); !ok {
		return fmt.Errorf("%s: %s", code, msg)
	}
	if existing != nil {
		_, err = coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			fmt.Sprintf("gameSetup.playerCharacters.%s", uidStr): normalized,
			"gameSetup.language": langToSet,
		}, "$unset": bson.M{"gameSetup.generatedCharacter": ""}})
		if err != nil {
			return err
		}
	} else {
		_, err = coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{
			"$set": bson.M{"gameSetup": nextSetup},
			"$setOnInsert": bson.M{
				"gameId": gameID, "ownerUserId": uidOID, "memberUserIds": bson.A{uidOID},
			},
		}, options.Update().SetUpsert(true))
		if err != nil {
			return err
		}
	}
	if hub != nil {
		hub.NotifyGameStateUpdated(gameID)
	}
	return nil
}
