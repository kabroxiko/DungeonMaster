package party

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
)

// DefaultParty returns lobby defaults.
func DefaultParty() map[string]interface{} {
	return map[string]interface{}{
		"phase":                               "lobby",
		"readyUserIds":                        []interface{}{},
		"characterBusyUntil":                  map[string]interface{}{},
		"hostPremise":                         "",
		"pendingNarrativeIntroductionUserIds": []interface{}{},
		"narrativeIntroducedUserIds":          []interface{}{},
		"lastStartError":                      nil,
		"lastStartAt":                         nil,
	}
}

// CharacterBusyTTLMs bounds how long "this player is rolling a character" survives without being cleared. The rolling
// browser clears the marker when its request settles; the deadline is what stops a closed tab leaving the row spinning.
const CharacterBusyTTLMs int64 = 120000

// CharacterBusyUntil normalizes party.characterBusyUntil (user id → epoch milliseconds) from whatever Mongo decoded it as.
func CharacterBusyUntil(p map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	if p == nil {
		return out
	}
	src := map[string]interface{}{}
	switch m := p["characterBusyUntil"].(type) {
	case map[string]interface{}:
		src = m
	case primitive.M:
		src = map[string]interface{}(m)
	default:
		return out
	}
	for k, v := range src {
		id := NormalizeUserIDString(k)
		if id == "" {
			continue
		}
		if ms, ok := epochMillis(v); ok {
			out[id] = ms
		}
	}
	return out
}

func epochMillis(v interface{}) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int32:
		return int64(t), true
	case int:
		return int64(t), true
	case float64:
		return int64(t), true
	case primitive.DateTime:
		return t.Time().UnixMilli(), true
	}
	return 0, false
}

// GetParty merges gameSetup.party with defaults.
func GetParty(gameSetup map[string]interface{}) map[string]interface{} {
	if gameSetup == nil {
		return DefaultParty()
	}
	p, _ := gameSetup["party"].(map[string]interface{})
	out := DefaultParty()
	if p != nil {
		for k, v := range p {
			out[k] = v
		}
	}
	return out
}

// MergeParty patches party sub-object.
func MergeParty(gameSetup map[string]interface{}, patch map[string]interface{}) map[string]interface{} {
	if gameSetup == nil {
		gameSetup = map[string]interface{}{}
	}
	cur := GetParty(gameSetup)
	for k, v := range patch {
		cur[k] = v
	}
	gs := cloneMap(gameSetup)
	gs["party"] = cur
	return gs
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// CanonicalMemberIDStrings returns owner + members unique.
func CanonicalMemberIDStrings(doc map[string]interface{}) []string {
	seen := map[string]struct{}{}
	if doc != nil {
		if oid, ok := doc["ownerUserId"].(primitive.ObjectID); ok && !oid.IsZero() {
			seen[oid.Hex()] = struct{}{}
		}
		if arr, ok := doc["memberUserIds"]; ok {
			for _, x := range ListValues(arr) {
				if id, ok := x.(primitive.ObjectID); ok && !id.IsZero() {
					seen[id.Hex()] = struct{}{}
				}
			}
		}
	}
	var out []string
	for s := range seen {
		out = append(out, s)
	}
	return out
}

// NormalizeUserIDString lowercases hex for comparison.
func NormalizeUserIDString(raw interface{}) string {
	s := oidString(raw)
	return strings.ToLower(strings.TrimSpace(s))
}

func oidString(v interface{}) string {
	switch t := v.(type) {
	case primitive.ObjectID:
		return t.Hex()
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// NormalizeReadyUserIDsArray dedupes lowercased ids.
func NormalizeReadyUserIDsArray(arr []interface{}) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, x := range arr {
		s := NormalizeUserIDString(x)
		if s == "" || s == "000000000000000000000000" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// IsLobbyParty mirrors partyLobbyState.js.
func IsLobbyParty(doc map[string]interface{}) bool {
	if campaignspec.HasSubstantiveCampaignSpec(toMap(doc["campaignSpec"])) {
		return false
	}
	p := GetParty(toMap(doc["gameSetup"]))
	ph, _ := p["phase"].(string)
	if ph == "playing" {
		return false
	}
	return ph == "lobby" || ph == "starting"
}

func toMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}
