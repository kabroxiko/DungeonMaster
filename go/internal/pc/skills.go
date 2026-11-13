package pc

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"unicode"

	"github.com/deckofdmthings/gmai/internal/dice"
	"github.com/deckofdmthings/gmai/internal/i18n"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PHB skill keys (stable ids) in typical sheet order.
var phbSkillOrder = []string{
	"acrobatics", "animal_handling", "arcana", "athletics", "deception", "history",
	"insight", "intimidation", "investigation", "medicine", "nature", "perception",
	"performance", "persuasion", "religion", "sleight_of_hand", "stealth", "survival",
}

var skillAbility = map[string]string{
	"acrobatics":       "DEX",
	"animal_handling":  "WIS",
	"arcana":           "INT",
	"athletics":        "STR",
	"deception":        "CHA",
	"history":          "INT",
	"insight":          "WIS",
	"intimidation":     "CHA",
	"investigation":    "INT",
	"medicine":         "WIS",
	"nature":           "INT",
	"perception":       "WIS",
	"performance":      "CHA",
	"persuasion":       "CHA",
	"religion":         "INT",
	"sleight_of_hand":  "DEX",
	"stealth":          "DEX",
	"survival":         "WIS",
}

// backgroundSkills: PHB Chapter 4 — two fixed skill proficiencies each.
var backgroundSkills = map[string][2]string{
	"acolyte":        {"insight", "religion"},
	"charlatan":      {"deception", "sleight_of_hand"},
	"criminal":       {"deception", "stealth"},
	"entertainer":    {"acrobatics", "performance"},
	"folk_hero":      {"animal_handling", "survival"},
	"guild_artisan":  {"insight", "persuasion"},
	"hermit":         {"medicine", "religion"},
	"noble":          {"history", "persuasion"},
	"outlander":      {"athletics", "survival"},
	"sage":           {"arcana", "history"},
	"sailor":         {"athletics", "perception"},
	"soldier":        {"athletics", "intimidation"},
	"urchin":         {"sleight_of_hand", "stealth"},
}

// backgroundLabelToID matches localized background titles from the sheet to canonical ids.
var backgroundLabelToID = map[string]string{
	// English PHB
	"acolyte": "acolyte", "charlatan": "charlatan", "criminal": "criminal",
	"entertainer": "entertainer", "folk hero": "folk_hero", "guild artisan": "guild_artisan",
	"hermit": "hermit", "noble": "noble", "outlander": "outlander", "sage": "sage",
	"sailor": "sailor", "soldier": "soldier", "urchin": "urchin",
	// Spanish PHB-style (character.md / main.js)
	"acolito": "acolyte", "charlatán": "charlatan",
	"artista": "entertainer", "héroe del pueblo": "folk_hero", "heroe del pueblo": "folk_hero",
	"artesano gremial": "guild_artisan", "ermitaño": "hermit", "ermitano": "hermit",
	"nobleza": "noble", "marginado": "outlander", "sabio": "sage", "marino": "sailor",
	"soldado": "soldier", "golfillo": "urchin",
}

// classSkillPool: PHB — choose N from this list (bard uses full phbSkillOrder).
var classSkillPool = map[string][]string{
	"barbarian": {"animal_handling", "athletics", "intimidation", "nature", "perception", "survival"},
	"bard":      {}, // any three — handled specially
	"cleric":    {"history", "insight", "medicine", "persuasion", "religion"},
	"druid":     {"animal_handling", "insight", "medicine", "nature", "perception", "religion", "survival"},
	"fighter":   {"acrobatics", "animal_handling", "athletics", "history", "insight", "intimidation", "perception", "survival"},
	"monk":      {"acrobatics", "athletics", "history", "insight", "religion", "stealth"},
	"paladin":   {"athletics", "insight", "intimidation", "medicine", "persuasion", "religion"},
	"ranger":    {"animal_handling", "athletics", "insight", "investigation", "nature", "perception", "stealth", "survival"},
	"rogue":     {"acrobatics", "athletics", "deception", "insight", "intimidation", "investigation", "perception", "performance", "persuasion", "sleight_of_hand", "stealth"},
	"sorcerer":  {"arcana", "deception", "insight", "intimidation", "persuasion", "religion"},
	"warlock":   {"arcana", "deception", "history", "intimidation", "investigation", "nature", "religion"},
	"wizard":    {"arcana", "history", "insight", "investigation", "medicine", "religion"},
	"artificer": {"arcana", "investigation", "nature", "religion", "sleight_of_hand"},
}

var classSkillPickCount = map[string]int{
	"barbarian": 2,
	"bard":      3,
	"cleric":    2,
	"druid":     2,
	"fighter":   2,
	"monk":      2,
	"paladin":   2,
	"ranger":    3,
	"rogue":     4,
	"sorcerer":  2,
	"warlock":   2,
	"wizard":    2,
	"artificer": 2,
}

var skillDisplayEN = map[string]string{
	"acrobatics": "Acrobatics", "animal_handling": "Animal Handling", "arcana": "Arcana",
	"athletics": "Athletics", "deception": "Deception", "history": "History",
	"insight": "Insight", "intimidation": "Intimidation", "investigation": "Investigation",
	"medicine": "Medicine", "nature": "Nature", "perception": "Perception",
	"performance": "Performance", "persuasion": "Persuasion", "religion": "Religion",
	"sleight_of_hand": "Sleight of Hand", "stealth": "Stealth", "survival": "Survival",
}

var skillDisplayES = map[string]string{
	"acrobatics": "Acrobacias", "animal_handling": "Trato con animales", "arcana": "Conocimiento Arcano",
	"athletics": "Atletismo", "deception": "Engaño", "history": "Historia",
	"insight": "Perspicacia", "intimidation": "Intimidación", "investigation": "Investigación",
	"medicine": "Medicina", "nature": "Naturaleza", "perception": "Percepción",
	"performance": "Interpretación", "persuasion": "Persuasión", "religion": "Religión",
	"sleight_of_hand": "Juego de manos", "stealth": "Sigilo", "survival": "Supervivencia",
}

// ComputeAndSetSkills replaces playerCharacter.skills with server-defined PHB rows (bonuses + proficiency flags).
// It ignores any prior skills from the client or model. Requires stats { STR..CHA } for meaningful output.
func ComputeAndSetSkills(pc map[string]interface{}, language string) {
	if pc == nil {
		return
	}
	delete(pc, "skills")
	stats, ok := asStatsMap(pc["stats"])
	if !ok || stats == nil || len(stats) == 0 {
		pc["skills"] = []interface{}{}
		return
	}
	level := 1
	if lv, ok := toIntish(pc["level"]); ok && lv >= 1 {
		level = lv
	}
	pb := proficiencyBonus(level)
	locale := "en"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "span") {
		locale = "es"
	}
	prof := map[string]bool{}
	seed := hashSeed(
		str(pc["name"]),
		str(pc["class"]),
		str(pc["race"]),
		str(pc["subrace"]),
		str(pc["background"]),
	)
	bgID := resolveBackgroundID(str(pc["background"]))
	if bgID != "" {
		if pair, ok := backgroundSkills[bgID]; ok {
			prof[pair[0]] = true
			prof[pair[1]] = true
		}
	}
	classID := normalizeClassID(str(pc["class"]))
	nPick := classSkillPickCount[classID]
	if classID == "bard" {
		for _, k := range pickNFromAllSkills(3, seed+1) {
			prof[k] = true
		}
	} else if pool := classSkillPool[classID]; nPick > 0 && len(pool) > 0 {
		for _, k := range dice.PickNFromPool(pool, nPick, seed+1) {
			prof[k] = true
		}
	}
	if normalizeRaceID(str(pc["race"]), str(pc["subrace"])) == "half_elf" {
		for _, k := range halfElfExtraSkills(prof, seed+3) {
			prof[k] = true
		}
	}
	out := make([]interface{}, 0, len(phbSkillOrder))
	for _, key := range phbSkillOrder {
		ab := skillAbility[key]
		mod := abilityModFromStats(stats, ab)
		total := mod
		isProf := prof[key]
		if isProf {
			total += pb
		}
		row := map[string]interface{}{
			"name":       skillDisplayName(key, locale),
			"bonus":      float64(total),
			"proficient": isProf,
		}
		out = append(out, row)
	}
	pc["skills"] = out
}

func skillDisplayName(key, locale string) string {
	if locale == "es" {
		if s, ok := skillDisplayES[key]; ok {
			return s
		}
	}
	if s, ok := skillDisplayEN[key]; ok {
		return s
	}
	return key
}

func proficiencyBonus(level int) int {
	if level < 1 {
		level = 1
	}
	tier := (level - 1) / 4
	switch {
	case tier <= 0:
		return 2
	case tier == 1:
		return 3
	case tier == 2:
		return 4
	case tier == 3:
		return 5
	default:
		return 6
	}
}

func abilityModFromStats(stats map[string]interface{}, ability string) int {
	v, ok := stats[ability]
	if !ok {
		return 0
	}
	n, ok := toFloat64(v)
	if !ok || math.IsNaN(n) {
		return 0
	}
	return int(math.Floor((n - 10) / 2))
}

func toFloat64(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int32:
		return float64(t), true
	case int64:
		return float64(t), true
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		s = strings.TrimPrefix(s, "+")
		var f float64
		_, err := fmt.Sscanf(s, "%f", &f)
		if err != nil {
			return 0, false
		}
		return f, true
	}
}

func toIntish(v interface{}) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case float64:
		return int(t), true
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		var n int
		_, err := fmt.Sscanf(s, "%d", &n)
		if err != nil {
			return 0, false
		}
		return n, true
	}
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func hashSeed(parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte{0})
		h.Write([]byte(strings.ToLower(strings.TrimSpace(p))))
	}
	return h.Sum64()
}

func normalizeClassID(raw string) string {
	s := foldLabel(raw)
	if s == "" || s == "random" {
		return ""
	}
	for _, id := range []string{
		"artificer", "barbarian", "bard", "cleric", "druid", "fighter",
		"monk", "paladin", "ranger", "rogue", "sorcerer", "warlock", "wizard",
	} {
		if s == id || s == strings.ReplaceAll(id, "_", " ") {
			return id
		}
		en := i18n.CharacterChoiceLabel("class", id, "en")
		es := i18n.CharacterChoiceLabel("class", id, "es")
		if foldLabel(en) == s || foldLabel(es) == s {
			return id
		}
	}
	return ""
}

func normalizeRaceID(race, subrace string) string {
	s := foldLabel(race)
	ref := foldLabel(i18n.CharacterChoiceLabel("race", "half_elf", "en"))
	if s == "halfelf" || s == "half_elf" || s == ref {
		return "half_elf"
	}
	if foldLabel(subrace) != "" {
		ss := foldLabel(subrace)
		if ss == "halfelf" || ss == "half_elf" || ss == ref {
			return "half_elf"
		}
	}
	return ""
}

func asStatsMap(raw interface{}) (map[string]interface{}, bool) {
	if raw == nil {
		return nil, false
	}
	switch m := raw.(type) {
	case map[string]interface{}:
		return m, true
	case primitive.M:
		return map[string]interface{}(m), true
	default:
		return nil, false
	}
}

func foldLabel(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// ResolveBackgroundID maps a localized PHB background title to a canonical id (e.g. folk_hero).
func ResolveBackgroundID(bg string) string {
	return resolveBackgroundID(bg)
}

func resolveBackgroundID(bg string) string {
	s := strings.TrimSpace(strings.ToLower(bg))
	if s == "" {
		return ""
	}
	if id, ok := backgroundLabelToID[s]; ok {
		return id
	}
	// strip punctuation for lookup
	folded := foldKey(bg)
	if id, ok := backgroundLabelToID[folded]; ok {
		return id
	}
	for k, id := range backgroundLabelToID {
		if strings.Contains(s, k) || strings.Contains(folded, foldKey(k)) {
			return id
		}
	}
	return ""
}

func foldKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToLower(r))
		} else if r == ' ' || r == '-' {
			b.WriteRune(' ')
		}
	}
	return strings.TrimSpace(b.String())
}

func pickNFromAllSkills(n int, seed uint64) []string {
	return dice.PickNFromPool(append([]string(nil), phbSkillOrder...), n, seed)
}

func halfElfExtraSkills(already map[string]bool, seed uint64) []string {
	var avail []string
	for _, k := range phbSkillOrder {
		if !already[k] {
			avail = append(avail, k)
		}
	}
	if len(avail) == 0 {
		return nil
	}
	return dice.PickNFromPool(avail, 2, seed)
}
