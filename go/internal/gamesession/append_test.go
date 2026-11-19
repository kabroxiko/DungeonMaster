package gamesession

import "testing"

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
