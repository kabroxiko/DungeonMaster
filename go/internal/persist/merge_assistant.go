package persist

import (
	"fmt"
	"math"

	"github.com/deckofdmthings/gmai/internal/validate"
)

// MergePersistWithAssistantReply mirrors gameStatePersist.js mergePersistWithAssistantReply.
func MergePersistWithAssistantReply(persistBase map[string]interface{}, envelope map[string]interface{}, finalUsedCombatStack bool, requestingUserID string) map[string]interface{} {
	if persistBase == nil {
		return nil
	}
	narration := ""
	if envelope != nil {
		if n, ok := envelope["narration"].(string); ok {
			narration = n
		}
	}
	aiMsg := map[string]interface{}{"role": "assistant", "content": narration}
	conv := toIfaceSlice(persistBase["conversation"])
	sumConv := toIfaceSlice(persistBase["summaryConversation"])
	conv = append(conv, aiMsg)
	sumConv = append(sumConv, aiMsg)

	countInc := 0
	if len(conv) >= 2 {
		if prev, ok := conv[len(conv)-2].(map[string]interface{}); ok {
			if r, _ := prev["role"].(string); r == "user" {
				countInc = 1
			}
		}
	}
	extraTok := int(math.Max(0, math.Ceil(float64(len(narration))/4)))

	var encounterState interface{}
	hasEnc := false
	if envelope != nil {
		if _, ok := envelope["encounterState"]; ok {
			encounterState = envelope["encounterState"]
			hasEnc = true
		}
	}
	if !hasEnc {
		encounterState = persistBase["encounterState"]
	}

	mode := persistBase["mode"]
	if finalUsedCombatStack {
		mode = "combat"
	}

	gameSetup := map[string]interface{}{}
	if gs, ok := persistBase["gameSetup"].(map[string]interface{}); ok {
		for k, v := range gs {
			gameSetup[k] = v
		}
	}
	if envelope != nil {
		if cg, ok := envelope["coinage"].(map[string]interface{}); ok && requestingUserID != "" {
			norm := validate.NormalizeCoinageObject(cg)
			pcMap, _ := gameSetup["playerCharacters"].(map[string]interface{})
			if pcMap == nil {
				pcMap = map[string]interface{}{}
			}
			cur, _ := pcMap[requestingUserID].(map[string]interface{})
			if cur == nil {
				cur = map[string]interface{}{}
			}
			cur2 := map[string]interface{}{}
			for k, v := range cur {
				cur2[k] = v
			}
			cur2["coinage"] = norm
			pcMap[requestingUserID] = cur2
			gameSetup["playerCharacters"] = pcMap
		}
	}
	delete(gameSetup, "partySubmittedUserIds")

	out := map[string]interface{}{}
	for k, v := range persistBase {
		out[k] = v
	}
	out["gameId"] = persistBase["gameId"]
	out["conversation"] = conv
	out["summaryConversation"] = sumConv
	out["encounterState"] = encounterState
	out["mode"] = mode
	out["gameSetup"] = gameSetup

	uac := 0
	if v, ok := persistBase["userAndAssistantMessageCount"].(float64); ok {
		uac = int(v)
	}
	if v, ok := persistBase["userAndAssistantMessageCount"].(int); ok {
		uac = v
	}
	out["userAndAssistantMessageCount"] = uac + countInc

	tt := 0
	if v, ok := persistBase["totalTokenCount"].(float64); ok {
		tt = int(v)
	}
	if v, ok := persistBase["totalTokenCount"].(int); ok {
		tt = v
	}
	out["totalTokenCount"] = tt + extraTok
	return out
}

func str(v interface{}) string {
	return fmt.Sprint(v)
}
