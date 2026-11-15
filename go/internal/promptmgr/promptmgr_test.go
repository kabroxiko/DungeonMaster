package promptmgr

import (
	"strings"
	"testing"
)

func TestLoadCampaignGeneratorParts(t *testing.T) {
	buildContext, userTemplate := LoadCampaignGeneratorParts()
	if buildContext == "" {
		t.Fatal("generator build context is empty")
	}
	if userTemplate == "" {
		t.Fatal("generator user template is empty")
	}
	if strings.HasPrefix(buildContext, "---") {
		t.Error("build context begins with a separator, so frontmatter stripping or the split is wrong")
	}
	if strings.Contains(buildContext, "name: campaign-generator") {
		t.Error("skill frontmatter leaked into the generator build context")
	}
}

// The owner's tone is a system-level constraint for generation: the creative seed's mood must not outrank it, so it
// belongs in the generator's build context and not only in the user message.
func TestCampaignGeneratorSystemContextCarriesTheHostTone(t *testing.T) {
	joined := ""
	for _, m := range BuildCampaignCoreSystemMsgs("English", "dry and ominous", "{}", "", nil) {
		joined += m["content"] + "\n"
	}
	if !strings.Contains(joined, "dry and ominous") {
		t.Fatal("the generator system context lost the host's tone")
	}

	withoutTone := ""
	for _, m := range BuildCampaignCoreSystemMsgs("English", "", "{}", "", nil) {
		withoutTone += m["content"] + "\n"
	}
	if strings.Contains(withoutTone, "Campaign voice (MUST") {
		t.Fatal("a campaign with no chosen tone must not carry the tone rule")
	}
}

func TestComposeSystemMessagesHasNoFrontmatter(t *testing.T) {
	for _, m := range ComposeSystemMessages("combat", "", true, "english") {
		content := m["content"]
		if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "name:") {
			t.Fatalf("system message leaked skill frontmatter: %q", content)
		}
	}
}
