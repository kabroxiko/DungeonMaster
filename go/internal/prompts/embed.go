package prompts

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed all:promptfiles
var embedded embed.FS

const (
	frontmatterOpen  = "---\n"
	frontmatterClose = "\n---\n"
)

var (
	cache   = map[string]string{}
	cacheMu sync.RWMutex
)

// FS returns the embedded prompt tree (see promptfiles/).
func FS() fs.FS {
	return embedded
}

// ResolveRelativePath normalizes slashes like Node resolvePromptRelativePath.
func ResolveRelativePath(filename string) string {
	if filename == "" {
		return filename
	}
	return filepath.ToSlash(strings.TrimSpace(filename))
}

// Load reads from embed FS under promptfiles/<rel>.
// Prompt files are markdown skills: the YAML frontmatter (name, description) is
// metadata for humans and tooling only, so it is stripped before the body is
// cached and handed to the prompt composer.
func Load(rel string) string {
	rel = ResolveRelativePath(rel)
	if rel == "" {
		return ""
	}
	cacheMu.RLock()
	if c, ok := cache[rel]; ok {
		cacheMu.RUnlock()
		return c
	}
	cacheMu.RUnlock()

	b, err := embedded.ReadFile(filepath.Join("promptfiles", rel))
	if err != nil {
		cacheMu.Lock()
		cache[rel] = ""
		cacheMu.Unlock()
		return ""
	}
	s := strings.TrimSpace(string(b))
	s = stripFrontmatter(s)
	cacheMu.Lock()
	cache[rel] = s
	cacheMu.Unlock()
	return s
}

// stripFrontmatter removes a leading YAML frontmatter block so skill metadata
// never reaches the model. A body without frontmatter is returned unchanged.
func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, frontmatterOpen) {
		return s
	}
	rest := s[len(frontmatterOpen):]
	end := strings.Index(rest, frontmatterClose)
	if end == -1 {
		return s
	}
	return strings.TrimSpace(rest[end+len(frontmatterClose):])
}
