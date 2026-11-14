package gamesession

import (
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/promptmgr"
)

// The consolidated system prompt is the provider's prefix-cache key, so everything up to the per-turn blocks has to
// be byte-identical between turns. If a per-turn block is hoisted above the static rules, this breaks.
func TestDmSystemPromptPrefixIsStableAcrossTurns(t *testing.T) {
	first := composeDmSystemPrompt("exploration", "English", "The party met the harbourmaster.", nil, "")
	second := composeDmSystemPrompt("exploration", "English", "The party left the harbour and took the coast road.", nil, "")

	const marker = "Session summary:"
	firstCut := strings.Index(first, marker)
	secondCut := strings.Index(second, marker)
	if firstCut < 0 || secondCut < 0 {
		t.Fatalf("session summary block missing: first=%d second=%d", firstCut, secondCut)
	}
	if first[:firstCut] != second[:secondCut] {
		t.Fatal("the blocks before the per-turn state differ between turns, so no prefix is cached")
	}
}

func TestDmSystemPromptStaticRulesLeadPerTurnState(t *testing.T) {
	prompt := composeDmSystemPrompt("exploration", "English", "Summary text.", nil, "")

	language := promptmgr.LoadPrompt("rules/language_english.md")
	memory := promptmgr.MemorySummaryMessage("Summary text.")
	if language == "" || memory == "" {
		t.Fatal("expected language and memory prompt files to be present")
	}
	langAt := strings.Index(prompt, strings.TrimSpace(language))
	memAt := strings.Index(prompt, strings.TrimSpace(memory))
	if langAt < 0 || memAt < 0 {
		t.Fatalf("missing blocks: language=%d memory=%d", langAt, memAt)
	}
	if memAt < langAt {
		t.Fatal("the per-turn session summary is placed above the static language rule")
	}
}
