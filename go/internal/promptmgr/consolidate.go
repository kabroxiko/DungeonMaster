package promptmgr

import (
	"strings"

	"github.com/cbroglie/mustache"
)

// ConsolidateSystemMessages mirrors server/routes/gameSession.js consolidateSystemMessages (insertDmPlayContract omitted for campaign).
func ConsolidateSystemMessages(msgs []map[string]string) string {
	guardKeys := []string{
		"OUTPUT FORMAT RULE",
		"NO PREFATORY TEXT",
		"NO PREFATORY",
		"OUTPUT FORMAT",
		"DM reply envelope",
	}
	var guards []string
	var others []string
	for _, m := range msgs {
		content := strings.TrimSpace(m["content"])
		isGuard := false
		for _, k := range guardKeys {
			if strings.Contains(content, k) {
				isGuard = true
				break
			}
		}
		if m["role"] == "assistant" {
			continue
		}
		if isGuard {
			guards = append(guards, strings.TrimSpace(content))
		} else if m["role"] == "system" {
			others = append(others, strings.TrimSpace(content))
		}
	}
	guardsDed := dedupeStrings(guards)
	othersDed := dedupeStrings(others)
	head := strings.Join(guardsDed, "\n\n")
	tail := strings.Join(othersDed, "\n\n")
	if head != "" && tail != "" {
		return head + "\n\n" + tail
	}
	if head != "" {
		return head
	}
	return tail
}

func dedupeStrings(arr []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range arr {
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// BuildCampaignStageSystemMsgs mirrors buildCampaignStageSystemMsgs(language).
func BuildCampaignStageSystemMsgs(language string) []map[string]string {
	var out []map[string]string
	if g := LoadPrompt("rules/json_output_guard.txt"); g != "" {
		out = append(out, map[string]string{"role": "system", "content": g})
	}
	if g := LoadPrompt("rules/no_prefatory_guard.txt"); g != "" {
		out = append(out, map[string]string{"role": "system", "content": g})
	}
	langFile := "rules/language_english.txt"
	if strings.ToLower(language) == "spanish" {
		langFile = "rules/language_spanish.txt"
	}
	if lp := LoadPrompt(langFile); lp != "" {
		out = append(out, map[string]string{"role": "system", "content": lp})
	}
	return out
}

// BuildCampaignCoreSystemMsgs mirrors buildCampaignCoreSystemMsgs(language, creativeSeedJson).
func BuildCampaignCoreSystemMsgs(language, creativeSeedJSON string) []map[string]string {
	buildContext, _ := LoadCampaignGeneratorParts()
	seedSlice := "{}"
	if strings.TrimSpace(creativeSeedJSON) != "" {
		seedSlice = strings.TrimSpace(creativeSeedJSON)
	}
	var out []map[string]string
	if g := LoadPrompt("rules/json_output_guard.txt"); g != "" {
		out = append(out, map[string]string{"role": "system", "content": g})
	}
	if g := LoadPrompt("rules/no_prefatory_guard.txt"); g != "" {
		out = append(out, map[string]string{"role": "system", "content": g})
	}
	if strings.TrimSpace(buildContext) != "" {
		rendered, err := mustache.Render(buildContext, map[string]interface{}{"creativeSeedJson": seedSlice})
		if err != nil {
			rendered = buildContext
		}
		out = append(out, map[string]string{"role": "system", "content": strings.TrimSpace(rendered)})
	}
	langFile := "rules/language_english.txt"
	if strings.ToLower(language) == "spanish" {
		langFile = "rules/language_spanish.txt"
	}
	if lp := LoadPrompt(langFile); lp != "" {
		out = append(out, map[string]string{"role": "system", "content": lp})
	}
	return out
}
