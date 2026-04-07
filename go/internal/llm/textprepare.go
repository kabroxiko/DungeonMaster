package llm

import (
	"regexp"
	"strings"
)

// PrepareWireFormatText mirrors server/utils/llmTextPrepare.js prepareWireFormatText.
func PrepareWireFormatText(raw string) string {
	t := stripBomAndInvisible(raw)
	t = stripLlmChannelNoise(t)
	t = stripMarkdownCodeFence(t)
	t = normalizeJSONLikeQuotes(t)
	return t
}

func normalizeJSONLikeQuotes(s string) string {
	s = strings.ReplaceAll(s, "\u201c", `"`)
	s = strings.ReplaceAll(s, "\u201d", `"`)
	s = strings.ReplaceAll(s, "\u2018", `'`)
	s = strings.ReplaceAll(s, "\u2019", `'`)
	return s
}

func stripBomAndInvisible(s string) string {
	s = strings.TrimPrefix(s, "\uFEFF")
	// RE2: Unicode escapes use \x{...} (not \u in character classes).
	return regexp.MustCompile(`^[\x{200B}\x{200C}\x{200D}\x{FEFF}]+`).ReplaceAllString(s, "")
}

func stripMarkdownCodeFence(s string) string {
	t := strings.TrimSpace(s)
	if matched, _ := regexp.MatchString(`(?i)^`+"```"+`ya?ml\b`, t); matched {
		t = regexp.MustCompile(`(?i)^`+"```"+`ya?ml\b\s*\n?`).ReplaceAllString(t, "")
		t = regexp.MustCompile(`(?i)\n?`+"```"+`\s*$`).ReplaceAllString(t, "")
		return strings.TrimSpace(t)
	}
	if strings.HasPrefix(t, "```") {
		t = regexp.MustCompile(`(?i)^`+"```"+`(?:json)?\s*\n?`).ReplaceAllString(t, "")
		t = regexp.MustCompile(`(?i)\n?`+"```"+`\s*$`).ReplaceAllString(t, "")
		return strings.TrimSpace(t)
	}
	return t
}

func stripLlmChannelNoise(s string) string {
	t := strings.TrimSpace(s)
	if idx := strings.Index(strings.ToLower(t), "<|message|>"); idx != -1 {
		t = t[idx:]
		t = regexp.MustCompile(`(?i)^<\|message\|\>\s*`).ReplaceAllString(t, "")
		t = strings.TrimSpace(t)
	}
	guard := 0
	for guard < 24 && regexp.MustCompile(`(?i)^<\|[^|]+\|\>\s*`).MatchString(t) {
		t = regexp.MustCompile(`(?i)^<\|[^|]+\|\>\s*`).ReplaceAllString(t, "")
		t = strings.TrimSpace(t)
		guard++
	}
	t = regexp.MustCompile(`(?i)^<\|channel\|\>[^\n]*\n?`).ReplaceAllString(t, "")
	return strings.TrimSpace(t)
}
