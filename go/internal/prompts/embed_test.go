package prompts

import (
	"io/fs"
	"strings"
	"testing"
)

func TestStripFrontmatter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "strips a closed block", in: "---\nname: x\ndescription: y\n---\n\nbody", want: "body"},
		{name: "keeps a body without frontmatter", in: "no metadata here", want: "no metadata here"},
		{name: "keeps an unterminated block", in: "---\nname: x\nbody", want: "---\nname: x\nbody"},
	}
	for _, c := range cases {
		if got := stripFrontmatter(c.in); got != c.want {
			t.Errorf("%s: stripFrontmatter(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestLoadDropsSkillFrontmatter(t *testing.T) {
	body := Load("core/style.md")
	if body == "" {
		t.Fatal("core/style.md did not load")
	}
	if strings.HasPrefix(body, "---") || strings.HasPrefix(body, "name:") || strings.Contains(body, "description:") {
		t.Fatalf("skill frontmatter leaked into the prompt body: %q", firstLine(body))
	}
}

func TestLoadKeepsGeneratorSeparator(t *testing.T) {
	body := Load("templates/campaign/generator.md")
	if body == "" {
		t.Fatal("templates/campaign/generator.md did not load")
	}
	if !strings.Contains(body, "\n---\n") {
		t.Fatal("generator body lost its --- separator")
	}
	if strings.HasPrefix(body, "---") {
		t.Fatal("generator body starts with the separator, so the frontmatter was not stripped")
	}
}

// TestAllPromptFilesAreSkills keeps the tree consistent: every prompt file is a
// markdown skill with frontmatter whose body is non-empty after stripping.
func TestAllPromptFilesAreSkills(t *testing.T) {
	docs := map[string]bool{"promptfiles/PROMPTS_DOC.md": true}
	err := fs.WalkDir(FS(), "promptfiles", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || docs[path] {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			t.Errorf("%s is not a markdown skill file", path)
			return nil
		}
		raw, err := fs.ReadFile(FS(), path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(strings.TrimSpace(string(raw)), "---\n") {
			t.Errorf("%s has no YAML frontmatter", path)
		}
		rel := strings.TrimPrefix(path, "promptfiles/")
		if Load(rel) == "" {
			t.Errorf("%s has an empty body once the frontmatter is stripped", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the prompt tree: %v", err)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
