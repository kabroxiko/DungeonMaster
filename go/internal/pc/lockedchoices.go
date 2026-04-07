package pc

import (
	"fmt"
	"strings"

	"github.com/deckofdmthings/gmai/internal/i18n"
)

func normChoiceID(raw interface{}) string {
	s := strings.TrimSpace(strings.ToLower(fmt.Sprint(raw)))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func isRandomOrEmpty(id string) bool {
	return id == "" || id == "random"
}

// pickSetupChoice returns the first non-empty, non-random string from gs for the given keys.
func pickSetupChoice(gs map[string]interface{}, keys ...string) string {
	if gs == nil {
		return ""
	}
	for _, k := range keys {
		v, ok := gs[k]
		if !ok || v == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if isRandomOrEmpty(strings.ToLower(s)) {
			continue
		}
		return s
	}
	return ""
}

// ApplyLockedCharacterChoices overwrites race, class, subrace, subclass on the generated sheet
// when the client sent fixed selections in gameSetup. Display strings use internal/i18n (EN/ES).
// Omitted or "random" setup values leave the model output unchanged for that field.
func ApplyLockedCharacterChoices(pc map[string]interface{}, gs map[string]interface{}, language string) {
	if pc == nil || gs == nil {
		return
	}
	loc := i18n.GameLanguageToLocale(language)

	if r := pickSetupChoice(gs, "race", "characterRace"); r != "" {
		id := normChoiceID(r)
		lbl := i18n.CharacterChoiceLabel("race", id, loc)
		if lbl == "" {
			lbl = humanizeID(id)
		}
		pc["race"] = lbl
	}
	if c := pickSetupChoice(gs, "class", "characterClass"); c != "" {
		id := normChoiceID(c)
		lbl := i18n.CharacterChoiceLabel("class", id, loc)
		if lbl == "" {
			lbl = humanizeID(id)
		}
		pc["class"] = lbl
	}
	if sr := pickSetupChoice(gs, "subrace", "subraceId"); sr != "" {
		id := normChoiceID(sr)
		lbl := i18n.CharacterChoiceLabel("subrace", id, loc)
		if lbl == "" {
			lbl = humanizeID(id)
		}
		pc["subrace"] = lbl
		pc["subraceId"] = id
	}
	if sc := pickSetupChoice(gs, "subclass", "subclassId"); sc != "" {
		id := normChoiceID(sc)
		lbl := i18n.CharacterChoiceLabel("subclass", id, loc)
		if lbl == "" {
			lbl = humanizeID(id)
		}
		pc["subclass"] = lbl
		pc["subclassId"] = id
	}
}

func humanizeID(id string) string {
	if id == "" {
		return ""
	}
	return strings.ReplaceAll(id, "_", " ")
}
