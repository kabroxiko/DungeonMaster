package llm

import "testing"

func TestParseModelStructuredObjectYAML(t *testing.T) {
	raw := "title: Test\nplayerCharacter:\n  name: A"
	m, ok := ParseModelStructuredObject(raw)
	if !ok || m == nil {
		t.Fatal("expected parse ok")
	}
	if m["title"] != "Test" {
		t.Fatalf("title: %v", m["title"])
	}
}

// A creative-seed draw returned exactly this and failed with CAMPAIGN_STAGE_CREATIVE_SEED_PARSE: the value holds curly
// quotes, and rewriting them to straight ones closed the scalar early and made a well-formed document unparseable.
func TestParseCreativeSeedWithCurlyQuotesInsideValue(t *testing.T) {
	raw := "creativeSeed:\n" +
		"  titleMood: \"shadowed intrigue with a sardonic twist\"\n" +
		"  preferAngles:\n" +
		"    - \"bureaucratic rot within an imperial court\"\n" +
		"    - \"the fragility of forged loyalties\"\n" +
		"  avoidRepeatedFantasyTropesThisRun:\n" +
		"    - \"fated prophecy\"\n" +
		"    - \"mighty dragon guardian\"\n" +
		"    - \"ancient elven king\"\n" +
		"  namingNote: \"use evocative titles for guilds and districts, avoiding generic \u201cShadow\u201d or \u201cNight\u201d prefixes\"\n"
	m, ok := ParseModelStructuredObject(raw)
	if !ok {
		t.Fatal("expected a document with curly quotes inside a quoted value to parse")
	}
	seed, _ := m["creativeSeed"].(map[string]interface{})
	if seed == nil {
		t.Fatalf("missing creativeSeed: %#v", m)
	}
	if note, _ := seed["namingNote"].(string); note == "" {
		t.Fatalf("namingNote: %#v", seed["namingNote"])
	}
}

// Curly quotes used as the string delimiters themselves are the case the rewrite exists for.
func TestParseModelStructuredObjectCurlyQuoteDelimiters(t *testing.T) {
	m, ok := ParseModelStructuredObject("title: \u201cTest\u201d")
	if !ok {
		t.Fatal("expected parse ok")
	}
	if m["title"] != "Test" {
		t.Fatalf("title: %v", m["title"])
	}
}
