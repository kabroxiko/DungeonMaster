package gamesession

import (
	"testing"
	"time"
)

// A restart leaves a turn in flight with nothing that will ever close it. The sweep must close exactly those, and
// leave alone a turn this process started or one that already finished.
func TestDmTurnIsOrphaned(t *testing.T) {
	boot := time.Date(2026, 11, 18, 21, 30, 0, 0, time.UTC)
	before := boot.Add(-2 * time.Minute).Format(time.RFC3339Nano)
	after := boot.Add(2 * time.Minute).Format(time.RFC3339Nano)
	finishedBeforeRestart := boot.Add(-90 * time.Second).Format(time.RFC3339Nano)

	cases := []struct {
		name        string
		startedAt   string
		completedAt string
		want        bool
	}{
		{"killed mid-call", before, "", true},
		{"completed before the restart", before, finishedBeforeRestart, false},
		{"started by this process", after, "", false},
		{"started and finished by this process", after, after, false},
		{"never started", "", "", false},
		{"garbage timestamp", "not a time", "", false},
		{"completed stamp is also garbage", before, "nope", true},
	}
	for _, c := range cases {
		if got := dmTurnIsOrphaned(c.startedAt, c.completedAt, boot); got != c.want {
			t.Fatalf("%s: dmTurnIsOrphaned(%q, %q) = %v, want %v", c.name, c.startedAt, c.completedAt, got, c.want)
		}
	}
}
