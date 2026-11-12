package gamesession

import (
	"strings"
	"testing"
	"time"
)

// A stage failure reaches the client as a code the frontend already localises, so the mapping must not drift.
func TestCampaignStageFailureMessageKeepsClientCodes(t *testing.T) {
	cases := []struct{ stage, wantMsg, wantCode string }{
		{"factions", "Failed generating factions stage", "CAMPAIGN_STAGE_FACTIONS"},
		{"majorNPCs", "Failed generating majorNPCs stage", "CAMPAIGN_STAGE_NPCS"},
		{"keyLocations", "Failed generating keyLocations stage", "CAMPAIGN_STAGE_LOCATIONS"},
	}
	for _, c := range cases {
		msg, code := campaignStageFailureMessage(c.stage)
		if msg != c.wantMsg || code != c.wantCode {
			t.Fatalf("%s: got (%q, %q), want (%q, %q)", c.stage, msg, code, c.wantMsg, c.wantCode)
		}
	}
}

// The reason a stage failed is what makes the error actionable; a stage that never reported one ran out of time, and
// that answer has to name the knob that changes it.
func TestCampaignStageFailureDetailReportsReasonOrBudget(t *testing.T) {
	if got := campaignStageFailureDetail("lm studio: connection refused", "factions", 2*time.Minute); got != "lm studio: connection refused" {
		t.Fatalf("reason was not passed through: %q", got)
	}
	got := campaignStageFailureDetail("  ", "factions", 2*time.Minute)
	if !strings.Contains(got, "did not finish within 2m0s") || !strings.Contains(got, "DM_STAGE_TIMEOUT_MS") {
		t.Fatalf("timeout detail does not name the budget or the knob: %q", got)
	}
}
