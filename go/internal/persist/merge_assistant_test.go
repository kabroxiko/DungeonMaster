package persist

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// A turn continues the transcript; it does not replace it.
//
// A stored document decodes its arrays as primitive.A, a named slice type that does not match `case []interface{}`. When
// the merge read only the plain shape it saw an empty conversation, so the document it wrote back held nothing but the
// reply it had just appended: every turn erased the story that came before it.
func TestMergeKeepsTheStoredConversation(t *testing.T) {
	stored := func(role, content string) map[string]interface{} {
		return map[string]interface{}{"role": role, "content": content}
	}
	base := map[string]interface{}{
		"gameId": "g1",
		"conversation": primitive.A{
			stored("user", "entro a la taberna"),
			stored("assistant", "La puerta cruje al abrirse."),
			stored("user", "pido una cerveza"),
		},
		"summaryConversation": primitive.A{
			stored("user", "entro a la taberna"),
			stored("assistant", "La puerta cruje al abrirse."),
			stored("user", "pido una cerveza"),
		},
		"gameSetup": map[string]interface{}{},
	}

	merged := MergePersistWithAssistantReply(base, map[string]interface{}{"narration": "El tabernero asiente."}, false, "u1")
	if merged == nil {
		t.Fatal("merge returned nothing")
	}
	conv := toIfaceSlice(merged["conversation"])
	if len(conv) != 4 {
		t.Fatalf("expected the three stored messages plus the reply, got %d: %v", len(conv), conv)
	}
	first, _ := conv[0].(map[string]interface{})
	if got, _ := first["content"].(string); got != "entro a la taberna" {
		t.Fatalf("the first stored message was lost, got %q", got)
	}
	last, _ := conv[3].(map[string]interface{})
	if got, _ := last["content"].(string); got != "El tabernero asiente." {
		t.Fatalf("the reply should be the last message, got %q", got)
	}
	if summary := toIfaceSlice(merged["summaryConversation"]); len(summary) != 4 {
		t.Fatalf("the summary copy should keep the same history, got %d", len(summary))
	}
}

// The same history arrives as a plain slice when the document was built in this process rather than decoded.
func TestMergeKeepsAPlainSliceConversation(t *testing.T) {
	base := map[string]interface{}{
		"gameId":       "g1",
		"conversation": []interface{}{map[string]interface{}{"role": "user", "content": "sigo el sendero"}},
		"gameSetup":    map[string]interface{}{},
	}
	merged := MergePersistWithAssistantReply(base, map[string]interface{}{"narration": "El sendero se bifurca."}, false, "u1")
	if conv := toIfaceSlice(merged["conversation"]); len(conv) != 2 {
		t.Fatalf("expected the stored message plus the reply, got %d", len(conv))
	}
}
