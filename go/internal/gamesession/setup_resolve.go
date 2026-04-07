package gamesession

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/deckofdmthings/gmai/internal/i18n"
)

// ResolveCharacterSetupForGeneration turns random/invalid picks into concrete legal ids.
func ResolveCharacterSetupForGeneration(gsIn map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range gsIn {
		out[k] = v
	}
	cat := i18n.CharacterOptionsForLocale("en")
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

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
	return out
}

