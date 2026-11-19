package gamesession

import (
	"errors"
	"strings"
	"testing"
)

const proseRefusal = "Lo siento, pero no puedo continuar con esta solicitud. ¿Hay algo más en lo que pueda ayudarte?"

// A refusal used to end the turn. One corrective attempt is what turns the refusal into the envelope the table needs.
func TestEnvelopeWithCorrectionRetriesAProseReply(t *testing.T) {
	calls := 0
	var last []interface{}
	ask := func(messages []interface{}) (string, error) {
		calls++
		last = messages
		if calls == 1 {
			return proseRefusal, nil
		}
		return `{"narration":"La puerta cede y el pasillo se abre ante ustedes.","imminentCombat":false,"combatCue":""}`, nil
	}

	raw, env, err := envelopeWithCorrection(ask, []interface{}{
		map[string]interface{}{"role": "system", "content": "you are the dm"},
		map[string]interface{}{"role": "user", "content": "abro la puerta"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env == nil || !strings.Contains(env.Narration, "La puerta cede") {
		t.Fatalf("the corrected reply was not used: %q", raw)
	}
	if calls != 2 {
		t.Fatalf("expected exactly one corrective attempt, got %d calls", calls)
	}
	if len(last) != 3 {
		t.Fatalf("the correction should be appended to the turn, got %d messages", len(last))
	}
	correction, _ := last[2].(map[string]interface{})
	if !strings.Contains(strings.ToLower(correction["content"].(string)), "json object") {
		t.Fatalf("the correction must restate the format, got %v", correction["content"])
	}
}

// A first reply that is already an envelope is never asked for twice.
func TestEnvelopeWithCorrectionAcceptsTheFirstReply(t *testing.T) {
	calls := 0
	ask := func(messages []interface{}) (string, error) {
		calls++
		return `{"narration":"ok","imminentCombat":false,"combatCue":""}`, nil
	}
	if _, env, err := envelopeWithCorrection(ask, nil); err != nil || env == nil {
		t.Fatalf("an envelope was rejected: env=%v err=%v", env, err)
	}
	if calls != 1 {
		t.Fatalf("expected a single call, got %d", calls)
	}
}

// Two prose replies end the turn with the last one, and the answer is nil rather than a half-filled envelope.
func TestEnvelopeWithCorrectionGivesUpAfterOneRetry(t *testing.T) {
	calls := 0
	ask := func(messages []interface{}) (string, error) {
		calls++
		return proseRefusal, nil
	}
	raw, env, err := envelopeWithCorrection(ask, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env != nil {
		t.Fatal("a prose reply must not become an envelope")
	}
	if raw == "" || calls != 2 {
		t.Fatalf("expected two attempts and the last reply, got calls=%d raw=%q", calls, raw)
	}
}

// An empty narration is as useless as prose: the table would read a blank turn.
func TestUsableDmEnvelopeRejectsEmptyNarration(t *testing.T) {
	if usableDmEnvelope(`{"narration":"","imminentCombat":false}`) != nil {
		t.Fatal("empty narration must not count as usable")
	}
	if usableDmEnvelope(`{"narration":"hay algo","imminentCombat":false}`) == nil {
		t.Fatal("a narrated turn must be usable")
	}
}

// A transport failure is reported, not retried into a second wasted call.
func TestEnvelopeWithCorrectionPropagatesAskErrors(t *testing.T) {
	calls := 0
	ask := func(messages []interface{}) (string, error) {
		calls++
		return "", errors.New("lm studio: connection refused")
	}
	if _, _, err := envelopeWithCorrection(ask, nil); err == nil {
		t.Fatal("expected the transport error")
	}
	if calls != 1 {
		t.Fatalf("expected no corrective attempt after a transport error, got %d calls", calls)
	}
}
