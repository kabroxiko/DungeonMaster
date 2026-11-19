package gamesession

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func appendTestMsg(role, content string) map[string]interface{} {
	return map[string]interface{}{"role": role, "content": content}
}

// Player messages that never got a DM reply are still the story. The trim only removes the unanswered tail that follows
// an existing reply, so a run of failed turns no longer deletes the transcript back to the system prompt.
func TestStripTrailingUserTailKeepsHistoryWhenNoReplyExists(t *testing.T) {
	conv := []interface{}{appendTestMsg("system", "s"), appendTestMsg("user", "hola"), appendTestMsg("user", "sigo aca")}
	for _, m := range conv {
		m.(map[string]interface{})["userId"] = "u1"
	}
	sum := append([]interface{}{}, conv...)

	got, gotSum := stripTrailingUserTail(conv, sum, "u1")
	if len(got) != len(conv) || len(gotSum) != len(sum) {
		t.Fatalf("history without an assistant reply was stripped: conv %d/%d, sum %d/%d", len(got), len(conv), len(gotSum), len(sum))
	}
}

// With a reply in place, the player's own unanswered trailing message is still dropped, so a retry does not duplicate it.
func TestStripTrailingUserTailStillDropsTheUnansweredTail(t *testing.T) {
	conv := []interface{}{appendTestMsg("system", "s"), appendTestMsg("user", "hola"), appendTestMsg("assistant", "responde"), appendTestMsg("user", "otra vez")}
	for _, m := range conv {
		m.(map[string]interface{})["userId"] = "u1"
	}
	got, _ := stripTrailingUserTail(conv, append([]interface{}{}, conv...), "u1")
	if len(got) != 3 {
		t.Fatalf("expected the trailing unanswered user message to be dropped, got %d messages", len(got))
	}
}

// A list read back from the database arrives as primitive.A, not []interface{}. When the helper only accepted the
// latter, every stored conversation read as empty and the next player message replaced the whole transcript with
// itself. This is the regression guard for that: a decoded array must survive the trip.
func TestToIfaceSliceReadsDecodedBsonArrays(t *testing.T) {
	raw, err := bson.Marshal(bson.M{"conversation": []interface{}{
		bson.M{"role": "system", "content": "s"},
		bson.M{"role": "user", "content": "abro la puerta"},
		bson.M{"role": "assistant", "content": "se abre"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	conv := toIfaceSlice(doc["conversation"])
	if len(conv) != 3 {
		t.Fatalf("a stored transcript read as %d messages, want 3", len(conv))
	}
	conv = append(conv, appendTestMsg("user", "entro"))
	if len(conv) != 4 {
		t.Fatalf("appending to a stored transcript produced %d messages, want 4", len(conv))
	}
}
