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
	cacheMu.Lock()
	cache[rel] = s
	cacheMu.Unlock()
	return s
}
