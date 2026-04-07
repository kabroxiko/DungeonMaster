package promptmgr

import (
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/deckofdmthings/gmai/internal/prompts"
)

// LoadPrompt loads from embedded server prompts.
func LoadPrompt(filename string) string {
	return prompts.Load(filename)
}

// LoadCampaignGeneratorParts splits templates/campaign/generator.txt on ---.
func LoadCampaignGeneratorParts() (buildContext, userTemplate string) {
	full := LoadPrompt("templates/campaign/generator.txt")
	sep := "\n---\n"
	idx := strings.Index(full, sep)
	if strings.TrimSpace(full) == "" || idx == -1 {
		return strings.TrimSpace(full), ""
	}
	return strings.TrimSpace(full[:idx]), strings.TrimSpace(full[idx+len(sep):])
}

// ComposeSystemMessages mirrors server/promptManager.js composeSystemMessages.
func ComposeSystemMessages(mode, sessionSummary string, includeFullSkill bool, language string) []map[string]string {
	msgs := []map[string]string{}
	if core := LoadPrompt("core/system.txt"); core != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": core})
	}
	if style := LoadPrompt("core/style.txt"); style != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": style})
	}
	if sessionSummary != "" {
		memT := LoadPrompt("rules/memory_summary.txt")
		mem := memT + "\n\nSession summary: " + sessionSummary
		msgs = append(msgs, map[string]string{"role": "system", "content": mem})
	}
	if mode == "initial" && sessionSummary != "" {
		msgs = append(msgs, map[string]string{
			"role":    "system",
			"content": "Note: Character data is available to the server. DO NOT include a character sheet or full character stats in your response. The client renders the sheet separately. For length and structure, follow the dedicated adventure-seed system block supplied by the server.",
		})
	}
	skillMap := map[string]string{
		"combat":        "skills/combat.txt",
		"investigation": "skills/investigation.txt",
		"decision":      "skills/decision.txt",
		"initial":       "skills/adventure_seed.txt",
		"exploration":   "skills/exploration.txt",
	}
	skillFile := skillMap[mode]
	if skillFile != "" {
		if mode == "initial" && skillFile == "skills/adventure_seed.txt" {
			msgs = append(msgs, map[string]string{
				"role":    "system",
				"content": "Mode: initial. Follow the opening-scene instructions in the dedicated adventure-seed system block supplied by the server (do not assume they appear here).",
			})
		} else if includeFullSkill {
			sc := LoadPrompt(skillFile)
			if sc != "" {
				msgs = append(msgs, map[string]string{"role": "system", "content": RenderSkillPrompt(sc, language)})
			}
		} else {
			msgs = append(msgs, map[string]string{"role": "system", "content": "Mode: " + mode + ". Follow the " + mode + " guidelines concisely."})
		}
	}
	langFile := "rules/language_english.txt"
	if strings.ToLower(language) == "spanish" {
		langFile = "rules/language_spanish.txt"
	}
	if lp := LoadPrompt(langFile); lp != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": lp})
	}
	if guard := LoadPrompt("rules/length_guard.txt"); guard != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": guard})
	}
	return msgs
}

// RenderSkillPrompt applies Mustache with languageInstruction.
func RenderSkillPrompt(skillContent, language string) string {
	if skillContent == "" || !strings.Contains(skillContent, "{{") {
		return skillContent
	}
	langFile := "rules/language_english.txt"
	if strings.ToLower(language) == "spanish" {
		langFile = "rules/language_spanish.txt"
	}
	li := LoadPrompt(langFile)
	data := map[string]interface{}{
		"languageInstruction": li,
		"language":            language,
	}
	out, err := mustache.Render(skillContent, data)
	if err != nil {
		return skillContent
	}
	return out
}

// LanguageInstructionForCompose returns rules/language_*.txt body.
func LanguageInstructionForCompose(language string) string {
	langFile := "rules/language_english.txt"
	if strings.ToLower(language) == "spanish" {
		langFile = "rules/language_spanish.txt"
	}
	return LoadPrompt(langFile)
}

// LastUserText returns last user message content from conversation array.
func LastUserText(conversation []interface{}) string {
	for i := len(conversation) - 1; i >= 0; i-- {
		m, _ := conversation[i].(map[string]interface{})
		if m == nil {
			continue
		}
		if r, _ := m["role"].(string); r == "user" {
			if c, ok := m["content"].(string); ok {
				return c
			}
		}
	}
	return ""
}
