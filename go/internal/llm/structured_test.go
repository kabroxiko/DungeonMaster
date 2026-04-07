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
