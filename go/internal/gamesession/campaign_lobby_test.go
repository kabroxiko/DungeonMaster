package gamesession

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The id comes straight from the model, so any length is possible. Slicing it to a fixed width used to panic on an
// id shorter than 48 characters, and because campaign stages run in their own goroutine that panic exited the server.
func TestOpeningFrameIDAcceptsAnyLength(t *testing.T) {
	cases := []struct {
		name  string
		frame map[string]interface{}
		want  string
	}{
		{"short id", map[string]interface{}{"id": "forge_pour_shortfall"}, "forge_pour_shortfall"},
		{"24 chars", map[string]interface{}{"id": strings.Repeat("a", 24)}, strings.Repeat("a", 24)},
		{"exactly 48", map[string]interface{}{"id": strings.Repeat("b", 48)}, strings.Repeat("b", 48)},
		{"longer than 48", map[string]interface{}{"id": strings.Repeat("c", 120)}, strings.Repeat("c", 48)},
		{"missing", map[string]interface{}{}, "campaign_opening"},
		{"nil value", map[string]interface{}{"id": nil}, "campaign_opening"},
		{"nil frame", nil, "campaign_opening"},
		{"blank", map[string]interface{}{"id": "   "}, "campaign_opening"},
		{"non string", map[string]interface{}{"id": 7.0}, "7"},
		{"punctuation", map[string]interface{}{"id": "forge pour/shortfall!"}, "forge_pour_shortfall_"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := openingFrameID(tc.frame); got != tc.want {
				t.Fatalf("openingFrameID = %q, want %q", got, tc.want)
			}
		})
	}
}

// A stage that panics must look like a failed stage. Go cannot recover another goroutine's panic from the caller, so
// without the recover inside runStageWithTimeout the process dies and every in-flight request with it.
func TestStagePanicBecomesAStageFailure(t *testing.T) {
	got := runStageWithTimeout(context.Background(), 2*time.Second, func(context.Context) bool {
		panic("deliberate: model output broke a slice")
	})
	if got {
		t.Fatal("a panicking stage must report failure, not success")
	}
}

func TestStageTimeoutStillReportsFailure(t *testing.T) {
	got := runStageWithTimeout(context.Background(), 50*time.Millisecond, func(ctx context.Context) bool {
		<-ctx.Done()
		return true
	})
	if got {
		t.Fatal("a stage that ran out of time must report failure")
	}
}
