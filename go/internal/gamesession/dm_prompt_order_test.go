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

// The tone is campaign state, so every turn carries it. It used to be injected only into the opening, which let the
// voice evaporate from the second turn onward — precisely what a table that chose a tone notices.
func TestCampaignToneReachesEveryTurn(t *testing.T) {
	gs := map[string]interface{}{
		"campaignSpec": map[string]interface{}{
			"title":           "A Campaign",
			"campaignConcept": "A concept.",
			"toneAndStyle":    "dry and ominous",
		},
	}
	for _, mode := range []string{"initial", "exploration"} {
		prompt := composeDmSystemPrompt(mode, "English", "", gs, "")
		if !strings.Contains(prompt, "dry and ominous") {
			t.Fatalf("the %s turn lost the campaign tone", mode)
		}
		// A small local model anchors on the first block it reads, so the voice has to lead the stack.
		if !strings.HasPrefix(prompt, "Campaign voice (MUST") {
			t.Fatalf("the %s turn did not lead with the campaign voice", mode)
		}
	}
}

// A menu of options is the habit a tabletop GM falls into and the model imitates; the rule has to lead the stack, not
// sit inside a long prohibition list a small model may skim.
func TestNoPlayerOptionsRuleLeadsEveryTurn(t *testing.T) {
	for _, mode := range []string{"initial", "exploration"} {
		prompt := composeDmSystemPrompt(mode, "English", "", nil, "")
		if !strings.Contains(prompt, "Narration never hands the player a menu") {
			t.Fatalf("the %s turn lost the no-options rule", mode)
		}
		if !strings.HasPrefix(prompt, "Narration never hands the player a menu") {
			t.Fatalf("the %s turn did not lead with the no-options rule", mode)
		}
	}

	spanish := composeDmSystemPrompt("exploration", "Spanish", "", nil, "")
	if !strings.Contains(spanish, "Sin menús de opciones") {
		t.Fatal("a Spanish turn lost the Spanish no-options rule")
	}
}

// A campaign with no stored tone must not gain one from the renderer: formatting a missing value would print a
// placeholder into the voice block and tell the model to narrate in it.
func TestCampaignWithoutAToneGetsNoVoiceBlock(t *testing.T) {
	gs := map[string]interface{}{
		"campaignSpec": map[string]interface{}{"title": "A Campaign"},
	}
	prompt := composeDmSystemPrompt("exploration", "English", "", gs, "")
	if strings.Contains(prompt, "Campaign voice (MUST") {
		t.Fatal("a campaign with no tone must not carry a voice block")
	}
	if strings.Contains(prompt, "<nil>") {
		t.Fatal("a missing tone was rendered as text")
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

// In a fight the world does not matter: geography, factions and exploration guidance are thousands of tokens that
// crowd out the rules of the round, and the record says when a fight is running whatever mode the browser sent.
func TestCombatTurnsDropTheCampaignWorld(t *testing.T) {
	spec := map[string]interface{}{
		"title":           "A Campaign",
		"campaignConcept": "A concept.",
		"toneAndStyle":    "dry and ominous",
		"vellarisWorld": map[string]interface{}{
			"regions": []interface{}{map[string]interface{}{"id": "r1", "name": "The Hollow Coast"}},
		},
	}
	sheet := map[string]interface{}{
		"gameSetup": map[string]interface{}{
			"playerCharacters": map[string]interface{}{"u1": map[string]interface{}{
				"name": "Varek", "display_name": "Varek", "kind": "pc", "ac": 13, "max_hp": 12, "hp_current": 8,
			}},
			"playerCharactersOrder": []interface{}{"u1"},
		},
	}
	fighting := map[string]interface{}{
		"campaignSpec": spec,
		"gameSetup":    sheet["gameSetup"],
		"encounterState": map[string]interface{}{
			"round": 2,
			"participants": []interface{}{
				map[string]interface{}{"displayName": "Varek", "kind": "pc", "hp_current": 8, "hp_max": 12, "ac": 13},
				map[string]interface{}{"displayName": "Guardia", "hp_current": 9, "hp_max": 9, "ac": 12},
			},
		},
	}
	combatPrompt := composeDmSystemPrompt("exploration", "English", "", fighting, "u1")

	if strings.Contains(combatPrompt, "The Hollow Coast") {
		t.Fatal("a combat turn still carries the world's geography")
	}
	if strings.Contains(combatPrompt, "DM-ONLY EXPLORATION GUIDANCE") {
		t.Fatal("a combat turn still carries the exploration guidance")
	}
	// What decides the round has to be there.
	for _, must := range []string{"open_encounter", "dry and ominous", "Varek"} {
		if !strings.Contains(combatPrompt, must) {
			t.Fatalf("a combat turn lost %q", must)
		}
	}

	// Outside a fight the world is still the campaign's only geography.
	calm := map[string]interface{}{"campaignSpec": spec, "gameSetup": sheet["gameSetup"], "encounterState": map[string]interface{}{}}
	if prompt := composeDmSystemPrompt("exploration", "English", "", calm, "u1"); !strings.Contains(prompt, "The Hollow Coast") {
		t.Fatal("an exploration turn lost the world")
	}
}

// A combat turn reads the fight from the record, not from the campaign's whole history: the transcript is cut to the
// exchanges of this fight, and the cut lands on a player message so the model is never handed half an exchange.
func TestCombatTranscriptKeepsTheFightAndDropsTheBacklog(t *testing.T) {
	message := func(role, content string) interface{} {
		return map[string]interface{}{"role": role, "content": content}
	}
	var messages []interface{}
	for i := 0; i < 20; i++ {
		messages = append(messages, message("user", "exploración antigua"), message("assistant", "narración antigua"))
	}
	messages = append(messages, message("user", "ataco al guardia con mi daga"))

	trimmed := trimForCombat(messages, 6)
	if len(trimmed) == 0 || len(trimmed) > 6 {
		t.Fatalf("the window should be bounded by the keep count, got %d", len(trimmed))
	}
	if role, _ := trimmed[0].(map[string]interface{})["role"].(string); role != "user" {
		t.Fatalf("the cut should start at a player message, got %q", role)
	}
	last, _ := trimmed[len(trimmed)-1].(map[string]interface{})["content"].(string)
	if !strings.Contains(last, "ataco al guardia") {
		t.Fatalf("the player's declaration must survive the cut, got %q", last)
	}

	short := []interface{}{message("user", "hola")}
	if got := trimForCombat(short, 6); len(got) != 1 {
		t.Fatalf("a short transcript should be left alone, got %d", len(got))
	}
}
