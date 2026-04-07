package gamesession

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// AllocateNewPartyGameID mirrors allocateNewPartyGameId() in gameSession.js.
func AllocateNewPartyGameID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}
