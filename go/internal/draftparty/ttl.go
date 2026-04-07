package draftparty

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultDraftMinutes = 7 * 24 * 60

// DraftPartyTTLMinutes mirrors draftPartyTtl.js.
func DraftPartyTTLMinutes() float64 {
	raw := strings.TrimSpace(os.Getenv("DM_DRAFT_PARTY_TTL_MINUTES"))
	if raw == "0" {
		return 0
	}
	if raw == "" {
		return defaultDraftMinutes
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || !isFinite(n) || n <= 0 {
		return defaultDraftMinutes
	}
	return n
}

func isFinite(f float64) bool {
	return !((f != f) || f+1 == f)
}

// DraftPartyTTLMs returns ms or 0 if disabled.
func DraftPartyTTLMs() int64 {
	m := DraftPartyTTLMinutes()
	if m == 0 {
		return 0
	}
	return int64(m * 60 * 1000)
}

// DraftPartyExpiresAtFromNow returns future time or nil.
func DraftPartyExpiresAtFromNow() *time.Time {
	ms := DraftPartyTTLMs()
	if ms <= 0 {
		return nil
	}
	t := time.Now().Add(time.Duration(ms) * time.Millisecond)
	return &t
}
