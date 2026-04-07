package ironarachne

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/deckofdmthings/gmai/internal/pc"
	"github.com/deckofdmthings/gmai/internal/validate"
)

const defaultBase = "https://names.ironarachne.com"
const fetchTimeout = 10 * time.Second
const batch = 12

var ironRaces = map[string]struct{}{
	"dragonborn": {}, "dwarf": {}, "elf": {}, "gnome": {}, "goblin": {},
	"half-elf": {}, "half-orc": {}, "halfling": {}, "human": {}, "orc": {},
	"tiefling": {}, "troll": {},
}

// NamesEnabled mirrors ironArachneNamesEnabled (default true).
func NamesEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("DM_USE_IRON_ARACHNE_NAMES"))
	if raw == "" {
		return true
	}
	s := strings.ToLower(raw)
	return s != "false" && s != "0" && s != "no" && s != "off"
}

func baseURL() string {
	u := strings.TrimSpace(os.Getenv("DM_IRON_ARACHNE_NAMES_URL"))
	if u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultBase
}

// MapAncestryToIronRace mirrors mapAncestryToIronRace.
func MapAncestryToIronRace(ancestry interface{}) string {
	s := strings.ToLower(strings.TrimSpace(fmt.Sprint(ancestry)))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "human"
	}
	hyphen := strings.ReplaceAll(s, " ", "-")
	if _, ok := ironRaces[hyphen]; ok {
		return hyphen
	}
	if regexp.MustCompile(`\bhalf[\s-]*elf\b`).MatchString(s) || strings.Contains(s, "half elf") {
		return "half-elf"
	}
	if regexp.MustCompile(`\bhalf[\s-]*orc\b`).MatchString(s) || strings.Contains(s, "half orc") {
		return "half-orc"
	}
	if strings.Contains(s, "dragonborn") {
		return "dragonborn"
	}
	if regexp.MustCompile(`\btiefling\b`).MatchString(s) || strings.Contains(s, "tiflin") {
		return "tiefling"
	}
	if strings.Contains(s, "halfling") || strings.Contains(s, "mediano") {
		return "halfling"
	}
	if strings.Contains(s, "dwarf") || strings.Contains(s, "duergar") || strings.Contains(s, "enano") {
		return "dwarf"
	}
	if strings.Contains(s, "gnome") || strings.Contains(s, "gnomo") {
		return "gnome"
	}
	if strings.Contains(s, "goblin") {
		return "goblin"
	}
	if strings.Contains(s, "orc") && !strings.Contains(s, "half") {
		return "orc"
	}
	if strings.Contains(s, "elf") || strings.Contains(s, "elven") || strings.Contains(s, "elfo") {
		return "elf"
	}
	if strings.Contains(s, "human") || strings.Contains(s, "humano") {
		return "human"
	}
	if strings.Contains(s, "troll") {
		return "troll"
	}
	return "human"
}

// InferSexFromLobbyGenderHint mirrors inferSexFromLobbyGenderHint.
func InferSexFromLobbyGenderHint(g interface{}) string {
	t := strings.ToLower(strings.TrimSpace(fmt.Sprint(g)))
	if strings.HasPrefix(t, "f") {
		return "female"
	}
	if strings.HasPrefix(t, "m") {
		return "male"
	}
	// random
	if time.Now().UnixNano()%2 == 0 {
		return "female"
	}
	return "male"
}

func fetchNameBatch(ctx context.Context, race, nameType string, count int) ([]string, error) {
	u := fmt.Sprintf("%s/race/%s/%s/%d", baseURL(), url.PathEscape(race), url.PathEscape(nameType), count)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	cctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req = req.WithContext(cctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("iron arachne HTTP %s", resp.Status)
	}
	var j struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}
	if j.Names == nil {
		return nil, fmt.Errorf("iron arachne response missing names")
	}
	var out []string
	for _, x := range j.Names {
		if t := strings.TrimSpace(x); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// CollectReservedEntityNameKeys mirrors validateEntityNameUniqueness collectReservedEntityNameKeys.
func CollectReservedEntityNameKeys(gameSetup, campaignSpec, encounterState map[string]interface{}, excludeUserID string) map[string]struct{} {
	keys := map[string]struct{}{}
	skip := strings.ToLower(strings.TrimSpace(excludeUserID))
	pcMap, _ := gameSetup["playerCharacters"].(map[string]interface{})
	if pcMap != nil {
		for uid, v := range pcMap {
			if strings.ToLower(strings.TrimSpace(uid)) == skip {
				continue
			}
			sheet, _ := v.(map[string]interface{})
			if sheet == nil {
				continue
			}
			raw := pc.DisplayNameFromCharacterSheet(sheet)
			if k := validate.NormalizeNameKeyForReserved(raw); k != "" {
				keys[k] = struct{}{}
			}
		}
	}
	if campaignSpec != nil {
		if arr, ok := campaignSpec["majorNPCs"].([]interface{}); ok {
			for _, x := range arr {
				row, _ := x.(map[string]interface{})
				if row == nil {
					continue
				}
				if k := validate.NormalizeNameKeyForReserved(str(row["name"])); k != "" {
					keys[k] = struct{}{}
				}
			}
		}
	}
	if encounterState != nil {
		if part, ok := encounterState["participants"].([]interface{}); ok {
			for _, x := range part {
				row, _ := x.(map[string]interface{})
				if row == nil {
					continue
				}
				raw := str(row["name"])
				if raw == "" {
					raw = str(row["displayName"])
				}
				if raw == "" {
					raw = str(row["label"])
				}
				if k := validate.NormalizeNameKeyForReserved(raw); k != "" {
					keys[k] = struct{}{}
				}
			}
		}
	}
	return keys
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// TryPreassignDisplayName mirrors tryPreassignIronArachneDisplayName.
func TryPreassignDisplayName(ctx context.Context, raceRaw, genderRaw interface{}, gameSetup, campaignSpec, encounterState map[string]interface{}, excludeUserID string) (ok bool, name string, reason string) {
	if !NamesEnabled() {
		return false, "", "disabled_by_env"
	}
	rawRace := strings.TrimSpace(fmt.Sprint(raceRaw))
	if rawRace == "" || strings.EqualFold(rawRace, "random") {
		return false, "", "race_not_fixed"
	}
	race := MapAncestryToIronRace(rawRace)
	sex := InferSexFromLobbyGenderHint(genderRaw)
	reserved := CollectReservedEntityNameKeys(gameSetup, campaignSpec, encounterState, excludeUserID)
	givenList, err1 := fetchNameBatch(ctx, race, sex, batch)
	familyList, err2 := fetchNameBatch(ctx, race, "family", batch)
	if err1 != nil || err2 != nil {
		msg := "fetch failed"
		if err1 != nil {
			msg = err1.Error()
		} else if err2 != nil {
			msg = err2.Error()
		}
		return false, "", msg
	}
	for _, g := range givenList {
		for _, f := range familyList {
			if g == "" || f == "" {
				continue
			}
			full := strings.TrimSpace(strings.Join(strings.Fields(g+" "+f), " "))
			parts := strings.Fields(full)
			if len(parts) < 2 {
				continue
			}
			key := validate.NormalizeNameKeyForReserved(full)
			if key == "" {
				continue
			}
			if _, coll := reserved[key]; coll {
				continue
			}
			return true, full, ""
		}
	}
	return false, "", "no_unique_pair"
}

// InferSexFromPC mirrors inferSexForNames (Node ironArachneNames.js).
func InferSexFromPC(sheet map[string]interface{}) string {
	if sheet == nil {
		if time.Now().UnixNano()%2 == 0 {
			return "female"
		}
		return "male"
	}
	if id, ok := sheet["identity"].(map[string]interface{}); ok && id != nil && id["gender"] != nil {
		return InferSexFromLobbyGenderHint(id["gender"])
	}
	if sheet["gender"] != nil {
		return InferSexFromLobbyGenderHint(sheet["gender"])
	}
	if sheet["sex"] != nil {
		return InferSexFromLobbyGenderHint(sheet["sex"])
	}
	if time.Now().UnixNano()%2 == 0 {
		return "female"
	}
	return "male"
}

// ValidateClientPreassignedDisplayName mirrors validatePreassignedDisplayNameFromClient (Node).
func ValidateClientPreassignedDisplayName(full string, gameSetup, campaignSpec, encounterState map[string]interface{}, excludeUserID string) (ok bool, cleaned string) {
	s := strings.TrimSpace(full)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		s = s[:120]
	}
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return false, ""
	}
	key := validate.NormalizeNameKeyForReserved(s)
	if key == "" {
		return false, ""
	}
	reserved := CollectReservedEntityNameKeys(gameSetup, campaignSpec, encounterState, excludeUserID)
	if _, coll := reserved[key]; coll {
		return false, ""
	}
	return true, s
}

// AssignDisplayNameAfterParse draws Iron Arachne given+family and writes sheet fields (when names enabled).
func AssignDisplayNameAfterParse(ctx context.Context, sheet map[string]interface{}, gameSetup, campaignSpec, encounterState map[string]interface{}, excludeUserID string) (ok bool, name string, reason string) {
	if !NamesEnabled() {
		return false, "", "disabled_by_env"
	}
	race := MapAncestryToIronRace(sheet["race"])
	sex := InferSexFromPC(sheet)
	reserved := CollectReservedEntityNameKeys(gameSetup, campaignSpec, encounterState, excludeUserID)
	givenList, err1 := fetchNameBatch(ctx, race, sex, batch)
	familyList, err2 := fetchNameBatch(ctx, race, "family", batch)
	if err1 != nil || err2 != nil {
		msg := "fetch failed"
		if err1 != nil {
			msg = err1.Error()
		} else if err2 != nil {
			msg = err2.Error()
		}
		return false, "", msg
	}
	for _, g := range givenList {
		for _, f := range familyList {
			if g == "" || f == "" {
				continue
			}
			full := strings.TrimSpace(strings.Join(strings.Fields(g+" "+f), " "))
			parts := strings.Fields(full)
			if len(parts) < 2 {
				continue
			}
			key := validate.NormalizeNameKeyForReserved(full)
			if key == "" {
				continue
			}
			if _, coll := reserved[key]; coll {
				continue
			}
			pc.SyncDisplayNameFields(sheet, full)
			return true, full, ""
		}
	}
	return false, "", "no_unique_pair"
}

// AssignFallbackDisplayName sets a two-word server-side name when Iron Arachne is off or failed.
func AssignFallbackDisplayName(sheet map[string]interface{}, gameSetup, campaignSpec, encounterState map[string]interface{}, excludeUserID string) {
	if sheet == nil {
		return
	}
	reserved := CollectReservedEntityNameKeys(gameSetup, campaignSpec, encounterState, excludeUserID)
	given := []string{"Morgan", "River", "Ash", "Sage", "Rowan", "Brook", "Cedar", "Jade", "Corin", "Edda"}
	family := []string{"Greenleaf", "Ashford", "Thorn", "Blackwood", "Mariner", "Stormhaven", "Coldwater", "Brightcoin", "Ironhand", "Fairwick"}
	for i := 0; i < 80; i++ {
		g := given[rand.Intn(len(given))]
		f := family[rand.Intn(len(family))]
		full := strings.TrimSpace(g + " " + f)
		key := validate.NormalizeNameKeyForReserved(full)
		if key == "" {
			continue
		}
		if _, coll := reserved[key]; coll {
			continue
		}
		pc.SyncDisplayNameFields(sheet, full)
		return
	}
	pc.SyncDisplayNameFields(sheet, "Wayfarer Adventurer")
}

// ResolveDisplayNameForGeneration returns the single display name for pcDisplayName and post-parse sync.
func ResolveDisplayNameForGeneration(ctx context.Context, preassigned string, gsIn map[string]interface{}, gameRow map[string]interface{}, userID string) string {
	s := strings.TrimSpace(preassigned)
	if s != "" {
		return s
	}
	stub := map[string]interface{}{
		"race":     gsIn["race"],
		"gender":   gsIn["gender"],
		"ancestry": gsIn["race"],
	}
	var gsSetup, camp, enc map[string]interface{}
	if gameRow != nil {
		gsSetup, _ = gameRow["gameSetup"].(map[string]interface{})
		camp, _ = gameRow["campaignSpec"].(map[string]interface{})
		enc, _ = gameRow["encounterState"].(map[string]interface{})
	}
	if ok, _, _ := AssignDisplayNameAfterParse(ctx, stub, gsSetup, camp, enc, userID); ok {
		if n, ok := stub["name"].(string); ok && strings.TrimSpace(n) != "" {
			return strings.TrimSpace(n)
		}
	}
	AssignFallbackDisplayName(stub, gsSetup, camp, enc, userID)
	if n, ok := stub["name"].(string); ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	return "Wayfarer Adventurer"
}
