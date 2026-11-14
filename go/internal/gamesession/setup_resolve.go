package gamesession

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"strings"

	"github.com/deckofdmthings/gmai/internal/i18n"
	"github.com/deckofdmthings/gmai/internal/pc"
)

// ResolveCharacterSetupForGeneration turns random/invalid picks into concrete legal ids.
func ResolveCharacterSetupForGeneration(gsIn map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range gsIn {
		out[k] = v
	}
	cat := i18n.CharacterOptionsForLocale("en")
	// Deterministic seed from setup fields (excluding name) so preview-name and generate-character
	// with the same random/empty picks resolve to the same race/class/background — not a new roll per request.
	rng := rand.New(rand.NewSource(resolveRNGSeed(out)))

	norm := func(v interface{}) string {
		s := strings.TrimSpace(strings.ToLower(fmt.Sprint(v)))
		s = strings.ReplaceAll(s, "-", "_")
		s = strings.ReplaceAll(s, " ", "_")
		return s
	}
	contains := func(list []string, id string) bool {
		for _, v := range list {
			if v == id {
				return true
			}
		}
		return false
	}
	pick := func(list []string) string {
		if len(list) == 0 {
			return ""
		}
		return list[rng.Intn(len(list))]
	}

	raceIDs := []string{}
	for _, r := range cat.Races {
		id := norm(r.ID)
		if id != "" && id != "random" {
			raceIDs = append(raceIDs, id)
		}
	}
	classIDs := []string{}
	for _, c := range cat.Classes {
		id := norm(c.ID)
		if id != "" && id != "random" {
			classIDs = append(classIDs, id)
		}
	}

	race := norm(out["race"])
	if race == "" || race == "random" || !contains(raceIDs, race) {
		race = pick(raceIDs)
	}
	if race != "" {
		out["race"] = race
	}

	classID := norm(out["class"])
	allowed := cat.AllowedClassesByRace[race]
	classPool := classIDs
	if len(allowed) > 0 {
		filtered := []string{}
		for _, id := range allowed {
			nid := norm(id)
			if nid != "" && nid != "random" && contains(classIDs, nid) {
				filtered = append(filtered, nid)
			}
		}
		if len(filtered) > 0 {
			classPool = filtered
		}
	}
	if classID == "" || classID == "random" || !contains(classPool, classID) {
		classID = pick(classPool)
	}
	if classID != "" {
		out["class"] = classID
	}

	subraces := cat.SubracesByRace[strings.ReplaceAll(race, "_", "-")]
	if len(subraces) == 0 {
		delete(out, "subrace")
	} else {
		srIDs := []string{}
		for _, it := range subraces {
			id := norm(it.ID)
			if id != "" && id != "random" {
				srIDs = append(srIDs, id)
			}
		}
		sr := norm(out["subrace"])
		if sr == "" || sr == "random" || !contains(srIDs, sr) {
			sr = pick(srIDs)
		}
		if sr != "" {
			out["subrace"] = sr
		}
	}

	level := 1
	switch n := out["level"].(type) {
	case float64:
		if int(n) > 0 {
			level = int(n)
		}
	case int:
		if n > 0 {
			level = n
		}
	}
	minLvl := cat.ClassMinLevel[classID]
	subclasses := []i18n.IDLabel{}
	if level >= minLvl {
		subclasses = cat.SubclassesByClass[classID]
	}
	if len(subclasses) == 0 {
		delete(out, "subclass")
	} else {
		scIDs := []string{}
		for _, it := range subclasses {
			id := norm(it.ID)
			if id != "" && id != "random" {
				scIDs = append(scIDs, id)
			}
		}
		sc := norm(out["subclass"])
		if sc == "" || sc == "random" || !contains(scIDs, sc) {
			sc = pick(scIDs)
		}
		if sc != "" {
			out["subclass"] = sc
		}
	}
	bg := strings.TrimSpace(fmt.Sprint(out["background"]))
	if strings.EqualFold(bg, "random") {
		bg = ""
	}
	bgID := pc.ResolveBackgroundID(bg)
	backgroundIDs := []string{
		"acolyte", "charlatan", "criminal", "entertainer", "folk_hero", "guild_artisan",
		"hermit", "noble", "outlander", "sage", "sailor", "soldier", "urchin",
	}
	if bgID == "" {
		bgID = pick(backgroundIDs)
	}
	if bgID != "" {
		out["background"] = bgID
	}
	// Language is campaign-level (party owner / persisted gameSetup), not a character choice.
	delete(out, "language")
	// The draft id only seeds the resolution below: it is not part of the character.
	delete(out, "draftId")
	// Empty name is omitted: POST /preview-character-name sets resolvedGameSetup.name after resolve.
	// Non-empty names from the client stay for explicit player input.
	if raw, ok := out["name"]; ok {
		if strings.TrimSpace(fmt.Sprint(raw)) == "" {
			delete(out, "name")
		}
	}
	return out
}

func resolveRNGSeed(gs map[string]interface{}) int64 {
	if gs == nil {
		return 0
	}
	h := fnv.New64a()
	// Omit "name" / display name so assigning a previewed name does not re-roll other fields.
	//
	// `draftId` is what makes two drafts that ask for the same random character differ: without it every all-random
	// setup hashes to the same seed, so each new character came out as the same race, class and background. The client
	// mints one id per draft and sends it with both the name preview and the generation, so the two requests still
	// resolve identically.
	keys := []string{"race", "class", "subrace", "subclass", "background", "level", "gender", "draftId"}
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(strings.ToLower(strings.TrimSpace(fmt.Sprint(gs[k])))))
	}
	return int64(h.Sum64())
}

// MergeRandomIntentNameFlag sets or clears randomIntent["name"] on gs (copies the map so callers can merge safely).
func MergeRandomIntentNameFlag(gs map[string]interface{}, nameRandom bool) {
	if gs == nil {
		return
	}
	var ri map[string]interface{}
	if raw, ok := gs["randomIntent"].(map[string]interface{}); ok && raw != nil {
		ri = make(map[string]interface{}, len(raw)+1)
		for k, v := range raw {
			ri[k] = v
		}
	} else {
		ri = map[string]interface{}{}
	}
	if nameRandom {
		ri["name"] = true
	} else {
		delete(ri, "name")
	}
	if len(ri) == 0 {
		delete(gs, "randomIntent")
	} else {
		gs["randomIntent"] = ri
	}
}
