package i18n

import (
	"strings"
)

// CharacterOptions is JSON for GET /api/meta/character-options (DnD 5e PHB-style setup lists).
type CharacterOptions struct {
	Locale               string               `json:"locale"`
	Races                []IDLabel            `json:"races"`
	Classes              []IDLabel            `json:"classes"`
	SubclassLabels       map[string]string    `json:"subclass_labels"`
	SubraceLabels        map[string]string    `json:"subrace_labels"`
	SubclassesByClass    map[string][]IDLabel `json:"subclassesByClass"`
	SubracesByRace       map[string][]IDLabel `json:"subracesByRace"`
	AllowedClassesByRace map[string][]string  `json:"allowedClassesByRace"`
	ClassMinLevel        map[string]int       `json:"classMinLevel"`
}

// IDLabel is a select option (race, class, subrace, subclass).
type IDLabel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// CharacterOptionsForLocale builds the full catalog; locale is "en" or "es".
func CharacterOptionsForLocale(locale string) CharacterOptions {
	if locale != "es" {
		locale = "en"
	}
	randomLbl := "Random"
	if locale == "es" {
		randomLbl = "Aleatorio"
	}
	lbl := func(category, normID string) string {
		return CharacterChoiceLabel(category, normID, locale)
	}

	raceOrder := []string{"dwarf", "elf", "gnome", "half-elf", "half-orc", "halfling", "human", "tiefling"}
	races := []IDLabel{{ID: "random", Label: randomLbl}}
	for _, wire := range raceOrder {
		norm := strings.ReplaceAll(wire, "-", "_")
		races = append(races, IDLabel{ID: wire, Label: lbl("race", norm)})
	}

	classOrder := []string{
		"artificer", "barbarian", "bard", "cleric", "druid", "fighter",
		"monk", "paladin", "ranger", "rogue", "sorcerer", "warlock", "wizard",
	}
	classes := []IDLabel{{ID: "random", Label: randomLbl}}
	for _, id := range classOrder {
		classes = append(classes, IDLabel{ID: id, Label: lbl("class", id)})
	}

	subclassLabels := make(map[string]string, len(characterSubclassLabels))
	for id := range characterSubclassLabels {
		subclassLabels[id] = lbl("subclass", id)
	}
	subraceLabels := make(map[string]string, len(characterSubraceLabels))
	for id := range characterSubraceLabels {
		subraceLabels[id] = lbl("subrace", id)
	}

	subclassesByClass := map[string][]IDLabel{
		"random":    {},
		"barbarian": {{ID: "berserker", Label: lbl("subclass", "berserker")}, {ID: "totem", Label: lbl("subclass", "totem")}},
		"bard":      {{ID: "lore", Label: lbl("subclass", "lore")}, {ID: "valor", Label: lbl("subclass", "valor")}},
		"cleric":    {{ID: "life", Label: lbl("subclass", "life")}, {ID: "war", Label: lbl("subclass", "war")}},
		"druid":     {{ID: "land", Label: lbl("subclass", "land")}, {ID: "moon", Label: lbl("subclass", "moon")}},
		"fighter":   {{ID: "champion", Label: lbl("subclass", "champion")}, {ID: "battle_master", Label: lbl("subclass", "battle_master")}},
		"monk":      {{ID: "way_of_open_hand", Label: lbl("subclass", "way_of_open_hand")}},
		"paladin":   {{ID: "devotion", Label: lbl("subclass", "devotion")}},
		"ranger":    {{ID: "hunter", Label: lbl("subclass", "hunter")}},
		"rogue":     {{ID: "thief", Label: lbl("subclass", "thief")}, {ID: "assassin", Label: lbl("subclass", "assassin")}},
		"sorcerer":  {{ID: "draconic", Label: lbl("subclass", "draconic")}},
		"warlock":   {{ID: "fiend", Label: lbl("subclass", "fiend")}},
		"wizard":    {{ID: "evocation", Label: lbl("subclass", "evocation")}},
	}

	subracesByRace := map[string][]IDLabel{
		"random":   {},
		"human":    {},
		"half-elf": {},
		"half-orc": {},
		"tiefling": {},
		"elf":      {{ID: "high_elf", Label: lbl("subrace", "high_elf")}, {ID: "wood_elf", Label: lbl("subrace", "wood_elf")}, {ID: "drow", Label: lbl("subrace", "drow")}},
		"dwarf":    {{ID: "hill_dwarf", Label: lbl("subrace", "hill_dwarf")}, {ID: "mountain_dwarf", Label: lbl("subrace", "mountain_dwarf")}},
		"halfling": {{ID: "lightfoot", Label: lbl("subrace", "lightfoot")}, {ID: "stout", Label: lbl("subrace", "stout")}},
		"gnome":    {{ID: "forest_gnome", Label: lbl("subrace", "forest_gnome")}, {ID: "rock_gnome", Label: lbl("subrace", "rock_gnome")}},
	}

	allowed := map[string][]string{
		"halfling": {"random", "bard", "cleric", "druid", "fighter", "rogue", "wizard", "sorcerer", "warlock"},
		"half-orc": {"random", "barbarian", "fighter", "paladin", "ranger", "rogue", "cleric"},
		"gnome":    {"random", "bard", "cleric", "druid", "wizard", "sorcerer", "warlock", "rogue"},
	}

	classMinLevel := map[string]int{
		"random":    1,
		"barbarian": 3,
		"bard":      3,
		"cleric":    1,
		"druid":     2,
		"fighter":   3,
		"monk":      3,
		"paladin":   3,
		"ranger":    3,
		"rogue":     3,
		"sorcerer":  1,
		"warlock":   1,
		"wizard":    2,
		"artificer": 3,
	}

	return CharacterOptions{
		Locale:               locale,
		Races:                races,
		Classes:              classes,
		SubclassLabels:       subclassLabels,
		SubraceLabels:        subraceLabels,
		SubclassesByClass:    subclassesByClass,
		SubracesByRace:       subracesByRace,
		AllowedClassesByRace: allowed,
		ClassMinLevel:        classMinLevel,
	}
}
