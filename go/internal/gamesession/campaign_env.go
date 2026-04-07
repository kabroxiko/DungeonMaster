package gamesession

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func resolveCreativeSeedRaceTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("DM_CREATIVE_SEED_TIMEOUT_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10000 {
			return time.Duration(n) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(os.Getenv("DM_STAGE_TIMEOUT_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10000 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 120 * time.Second
}

func resolveLobbyPipelineStageTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("DM_LOBBY_STAGE_TIMEOUT_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10000 {
			return time.Duration(n) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(os.Getenv("DM_STAGE_TIMEOUT_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10000 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 120 * time.Second
}
