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

func TestComposeSystemMessagesHasNoFrontmatter(t *testing.T) {
	for _, m := range ComposeSystemMessages("combat", "", true, "english") {
		content := m["content"]
		if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "name:") {
			t.Fatalf("system message leaked skill frontmatter: %q", content)
		}
	}
}
