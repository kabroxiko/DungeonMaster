package gamesession

import (
	"context"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// Undo is a development tool: in production the same route is a button that deletes a turn the rest of the table has
// already read.
func TestUndoIsDevelopmentOnly(t *testing.T) {
	t.Setenv("DM_ALLOW_UNDO", "")
	t.Setenv("DM_SPAWN_VUE_DEV", "")

	if !UndoEnabled(&config.Config{NodeEnv: "development"}) {
		t.Fatal("a development server should allow undo")
	}
	if !UndoEnabled(&config.Config{NodeEnv: "dev"}) {
		t.Fatal("dev should allow undo")
	}
	if UndoEnabled(&config.Config{NodeEnv: "production"}) {
		t.Fatal("production must not allow undo")
	}
	if UndoEnabled(nil) {
		t.Fatal("no configuration means no undo")
	}

	// `npm start` sets no NODE_ENV; spawning the dev server is the signal a developer's machine gives.
	t.Setenv("DM_SPAWN_VUE_DEV", "true")
	if !UndoEnabled(&config.Config{}) {
		t.Fatal("a process running the Vue dev server is a development process")
	}

	// An explicit opt-in wins over the environment.
	t.Setenv("DM_SPAWN_VUE_DEV", "")
	t.Setenv("DM_ALLOW_UNDO", "true")
	if !UndoEnabled(&config.Config{NodeEnv: "production"}) {
		t.Fatal("DM_ALLOW_UNDO should enable undo anywhere")
	}
}

// The refusal has to be an answer the client can show, not an empty 500 from a nil database.
func TestUndoIsRefusedWithACodeOutsideDevelopment(t *testing.T) {
	t.Setenv("DM_ALLOW_UNDO", "")
	t.Setenv("DM_SPAWN_VUE_DEV", "")
	status, res := HandleUndo(context.Background(), &Deps{}, &config.Config{NodeEnv: "production"}, "user-1",
		map[string]interface{}{"gameId": "some-game"})
	if status != 403 {
		t.Fatalf("expected 403, got %d", status)
	}
	if res["code"] != "UNDO_DISABLED" {
		t.Fatalf("expected UNDO_DISABLED, got %v", res["code"])
	}
}

func TestUndoRequiresAGame(t *testing.T) {
	t.Setenv("DM_ALLOW_UNDO", "true")
	status, res := HandleUndo(context.Background(), &Deps{}, &config.Config{NodeEnv: "development"}, "user-1",
		map[string]interface{}{})
	if status != 400 || res["code"] != "GAME_ID_REQUIRED" {
		t.Fatalf("expected a missing-game error, got %d %v", status, res)
	}
}

// Undo keeps the head of the transcript and drops the tail the action added: it deletes, it does not restore a copy.
func TestTruncateKeepsTheHead(t *testing.T) {
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "entro"},
		map[string]interface{}{"role": "assistant", "content": "la puerta cruje"},
		map[string]interface{}{"role": "user", "content": "apuesto"},
		map[string]interface{}{"role": "assistant", "content": "el tahúr sonríe"},
	}
	kept := truncateMessages(messages, 2)
	if len(kept) != 2 {
		t.Fatalf("expected the first two, got %d", len(kept))
	}
	if str(kept[1].(map[string]interface{})["content"]) != "la puerta cruje" {
		t.Fatalf("the wrong head was kept: %v", kept)
	}
	// A mark from a transcript that has since shrunk must not panic or invent messages.
	if got := truncateMessages(messages, 9); len(got) != 4 {
		t.Fatalf("a mark past the end should keep everything, got %d", len(got))
	}
	if got := truncateMessages(messages, -3); len(got) != 0 {
		t.Fatalf("a negative mark should keep nothing, got %d", len(got))
	}
}

// The counters follow the transcript that is left, not the one that was deleted.
func TestCountersFollowTheDeletedTranscript(t *testing.T) {
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "entro a la taberna"},
		map[string]interface{}{"role": "assistant", "content": "La puerta cruje."},
	}
	pairs, tokens := countTranscript(messages)
	if pairs != 1 {
		t.Fatalf("one reply means one pair, got %d", pairs)
	}
	if tokens != (len("entro a la taberna")+len("La puerta cruje."))/4 {
		t.Fatalf("unexpected token estimate %d", tokens)
	}
	if pairs, tokens := countTranscript(nil); pairs != 0 || tokens != 0 {
		t.Fatalf("an empty transcript counts nothing, got %d/%d", pairs, tokens)
	}
}
