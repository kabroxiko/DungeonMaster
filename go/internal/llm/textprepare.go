package llm

import (
	"regexp"
	"strings"
)

// PrepareWireFormatCandidates repairs a model reply into the texts worth parsing, in the order they should be tried.
//
// The fully repaired text comes first: rewriting curly quotes to straight ones rescues a reply that used curly quotes
// as string *delimiters* (`“titleMood”: “x”`). The same text without that rewrite comes second, because the rewrite
// breaks the opposite case: curly quotes *inside* a double-quoted scalar are valid YAML, and substituting them closes
// the scalar early — `namingNote: "... avoiding generic “Shadow” or “Night” prefixes"` becomes unparseable, which is
// how a creative-seed draw came back as CAMPAIGN_STAGE_CREATIVE_SEED_PARSE with a reply that was in fact well formed.
func PrepareWireFormatCandidates(raw string) []string {
	t := stripBomAndInvisible(raw)
	t = stripLlmChannelNoise(t)
	t = stripMarkdownCodeFence(t)
	rewritten := normalizeJSONLikeQuotes(t)
	if rewritten == t {
		return []string{t}
	}
	return []string{rewritten, t}
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
