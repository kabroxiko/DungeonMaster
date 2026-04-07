package gamesession

import (
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/party"
	"github.com/deckofdmthings/gmai/internal/validate"
)

// GameStateDocForClient mirrors server/routes/gameState.js gameStateDocForClient.
func GameStateDocForClient(doc map[string]interface{}) map[string]interface{} {
	if doc == nil {
		return nil
	}
	o := cloneMapShallow(doc)
	if cs, ok := o["campaignSpec"].(map[string]interface{}); ok && cs != nil {
		o["campaignSpec"] = campaignspec.RedactCampaignSpecForClient(cs)
	}
	if gs, ok := o["gameSetup"].(map[string]interface{}); ok && gs != nil {
		lang := "English"
		if l, ok := gs["language"].(string); ok && strings.TrimSpace(l) != "" {
			lang = strings.TrimSpace(l)
		}
		if pcm, ok := gs["playerCharacters"].(map[string]interface{}); ok && pcm != nil {
			nextPc := map[string]interface{}{}
			for k, v := range pcm {
				if c, ok := v.(map[string]interface{}); ok && c != nil {
					nextPc[k] = validate.EnsurePlayerCharacterSheetDefaults(c, lang)
				} else {
					nextPc[k] = v
				}
			}
			gs = cloneMapShallow(gs)
			gs["playerCharacters"] = nextPc
		}
		gs = cloneMapShallow(gs)
		delete(gs, "generatedCharacter")
		o["gameSetup"] = gs
	}
	return o
}

func cloneMapShallow(m map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// GameStateSummaryForMineList mirrors gameStateSummaryForMineList.
func GameStateSummaryForMineList(o map[string]interface{}, viewerUserIDStr string) map[string]interface{} {
	if o == nil {
		return nil
	}
	gameID := strings.TrimSpace(str(o["gameId"]))
	var spec map[string]interface{}
	if cs, ok := o["campaignSpec"].(map[string]interface{}); ok && cs != nil && !isArray(cs) {
		spec = cs
	}
	title := ""
	if t, ok := spec["title"].(string); ok {
		title = strings.TrimSpace(t)
	}
	gs, _ := o["gameSetup"].(map[string]interface{})
	p := party.GetParty(gs)
	ph, _ := p["phase"].(string)
	if ph == "" {
		ph = "lobby"
	}
	lang := "English"
	if gs != nil {
		if l, ok := gs["language"].(string); ok && strings.TrimSpace(l) != "" {
			lang = strings.TrimSpace(l)
		}
	}
	ownerStr := gameaccess.EffectiveGameOwnerIDStr(o)
	members, _ := o["memberUserIds"].(primitive.A)
	memberCount := len(members)
	msgCount := 0
	if n, ok := o["userAndAssistantMessageCount"].(int32); ok {
		msgCount = int(n)
	} else if n, ok := o["userAndAssistantMessageCount"].(int64); ok {
		msgCount = int(n)
	} else if n, ok := o["userAndAssistantMessageCount"].(float64); ok {
		msgCount = int(n)
	} else if n, ok := o["userAndAssistantMessageCount"].(int); ok {
		msgCount = n
	}
	var createdAt interface{}
	if id, ok := o["_id"].(primitive.ObjectID); ok {
		createdAt = id.Timestamp().UTC().Format(time.RFC3339Nano)
	}
	return map[string]interface{}{
		"gameId":          gameID,
		"campaignTitle":   title,
		"partyPhase":      ph,
		"language":        lang,
		"memberCount":     memberCount,
		"messageCount":    msgCount,
		"viewerIsOwner":   ownerStr == viewerUserIDStr,
		"hasCampaign":     campaignspec.HasSubstantiveCampaignSpec(spec),
		"createdAt":       createdAt,
	}
}

func isArray(v interface{}) bool {
	_, ok := v.([]interface{})
	return ok
}

// BuildBootstrapSystemMessageContentDM mirrors gameSession.js.
func BuildBootstrapSystemMessageContentDM(campaignSpec map[string]interface{}, language string) string {
	entry := ""
	if campaignSpec != nil {
		if c, ok := campaignSpec["campaignConcept"].(string); ok {
			entry = strings.TrimSpace(c)
		}
	}
	systemMessageContentDM := entry + " Assume the player knows nothing. Allow for an organic introduction of information. (Player character is supplied by the server on play turns, not in this message.)"
	if strings.HasPrefix(strings.ToLower(language), "span") {
		systemMessageContentDM += "\n\nPor favor responde en español. Responde todas las interacciones en español."
	}
	return systemMessageContentDM
}

// TakeCampaignFieldItems mirrors takeCampaignFieldItems.
func TakeCampaignFieldItems(field interface{}, n int) []interface{} {
	if field == nil {
		return nil
	}
	if arr, ok := field.([]interface{}); ok {
		if len(arr) > n {
			return arr[:n]
		}
		return arr
	}
	if m, ok := field.(map[string]interface{}); ok {
		vals := make([]interface{}, 0, len(m))
		for _, v := range m {
			vals = append(vals, v)
		}
		if len(vals) > n {
			return vals[:n]
		}
		return vals
	}
	return []interface{}{field}
}

// PendingNarrativePatch updates pending narrative lists when a character joins mid-adventure.
func PendingNarrativePatch(gameSetup map[string]interface{}, uid string) map[string]interface{} {
	if gameSetup == nil {
		return gameSetup
	}
	p := party.GetParty(gameSetup)
	pend, _ := p["pendingNarrativeIntroductionUserIds"].([]interface{})
	pendSet := map[string]struct{}{}
	for _, x := range pend {
		pendSet[strings.TrimSpace(fmtUID(x))] = struct{}{}
	}
	pendSet[strings.TrimSpace(uid)] = struct{}{}
	var pendList []interface{}
	for id := range pendSet {
		if id != "" {
			pendList = append(pendList, id)
		}
	}
	return party.MergeParty(gameSetup, map[string]interface{}{
		"pendingNarrativeIntroductionUserIds": pendList,
	})
}

func fmtUID(x interface{}) string {
	switch t := x.(type) {
	case string:
		return t
	case primitive.ObjectID:
		return t.Hex()
	default:
		return strings.TrimSpace(str(x))
	}
}
