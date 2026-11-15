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

// CampaignVoiceBlock renders the tone and style the table owner chose as one system rule. It is the single statement of
// that choice: the campaign core, every stage, the opening frame and every play turn render this same file, so no
// generation step invents a voice the owner did not ask for. Empty when the owner chose nothing.
func CampaignVoiceBlock(hostToneAndStyle string) string {
	tone := strings.TrimSpace(hostToneAndStyle)
	if tone == "" {
		return ""
	}
	tpl := LoadPrompt("rules/campaign_voice.md")
	if strings.TrimSpace(tpl) == "" {
		return ""
	}
	rendered, err := mustache.Render(tpl, map[string]interface{}{"campaignToneAndStyle": tone})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(rendered)
}

// BuildCampaignStageSystemMsgs mirrors buildCampaignStageSystemMsgs(language).
func BuildCampaignStageSystemMsgs(language string) []map[string]string {
	var out []map[string]string
	if g := LoadPrompt("rules/json_output_guard.md"); g != "" {
		out = append(out, map[string]string{"role": "system", "content": g})
	}
	if g := LoadPrompt("rules/no_prefatory_guard.md"); g != "" {
		out = append(out, map[string]string{"role": "system", "content": g})
	}
	langFile := "rules/language_english.md"
	if strings.ToLower(language) == "spanish" {
		langFile = "rules/language_spanish.md"
	}
	if lp := LoadPrompt(langFile); lp != "" {
		out = append(out, map[string]string{"role": "system", "content": lp})
	}
	return out
}

// BuildCampaignCoreSystemMsgs mirrors buildCampaignCoreSystemMsgs(language, creativeSeedJson).
//
// hostToneAndStyle is the voice the table owner chose for this campaign and is rendered into the generator's build
// context, where it outranks the creative seed's mood. worldContext is the rendered DM-ONLY block for the world the
// campaign is built on, and worldData is the Mustache data the generator's build context needs to list that world's
// regions. Both are empty for a campaign that has no generated world, which keeps the generator's own geography in
// charge.
func BuildCampaignCoreSystemMsgs(language, hostToneAndStyle, creativeSeedJSON, worldContext string, worldData map[string]interface{}) []map[string]string {
	buildContext, _ := LoadCampaignGeneratorParts()
	seedSlice := "{}"
	if strings.TrimSpace(creativeSeedJSON) != "" {
		seedSlice = strings.TrimSpace(creativeSeedJSON)
	}
	var out []map[string]string
	if voice := CampaignVoiceBlock(hostToneAndStyle); voice != "" {
		out = append(out, map[string]string{"role": "system", "content": voice})
	}
	if g := LoadPrompt("rules/json_output_guard.md"); g != "" {
		out = append(out, map[string]string{"role": "system", "content": g})
	}
	if g := LoadPrompt("rules/no_prefatory_guard.md"); g != "" {
		out = append(out, map[string]string{"role": "system", "content": g})
	}
	if block := strings.TrimSpace(worldContext); block != "" {
		rules := LoadPrompt("rules/vellaris_world.md")
		if rules != "" {
			block = rules + "\n\n" + block
		}
		out = append(out, map[string]string{"role": "system", "content": block})
	}
	if strings.TrimSpace(buildContext) != "" {
		data := map[string]interface{}{
			"creativeSeedJson": seedSlice,
		}
		for k, v := range worldData {
			data[k] = v
		}
		rendered, err := mustache.Render(buildContext, data)
		if err != nil {
			rendered = buildContext
		}
		out = append(out, map[string]string{"role": "system", "content": strings.TrimSpace(rendered)})
	}
	langFile := "rules/language_english.md"
	if strings.ToLower(language) == "spanish" {
		langFile = "rules/language_spanish.md"
	}
	if lp := LoadPrompt(langFile); lp != "" {
		out = append(out, map[string]string{"role": "system", "content": lp})
	}
	return out
}
