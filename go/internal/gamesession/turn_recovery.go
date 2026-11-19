package gamesession

import (
	"context"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/realtime"
)

// interruptedTurnMessage explains, in the stored game, why a reply the table was waiting for will never arrive.
const interruptedTurnMessage = "The server restarted while this reply was being generated. Send your action again."

// dmTurnIsOrphaned reports whether a DM turn was left in flight by a process that is gone. A turn started after this
// process booted belongs to it and may still finish, and one that already completed did its job; anything else was
// killed mid-call by a restart, and no process will ever close it.
func dmTurnIsOrphaned(startedAt, completedAt string, bootTime time.Time) bool {
	started, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(startedAt))
	if err != nil {
		return false
	}
	if !started.Before(bootTime) {
		return false
	}
	completed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(completedAt))
	if err == nil && completed.After(started) {
		return false
	}
	return true
}

// recoverInterruptedTurns closes every DM turn a previous process left in flight. The client keeps the chat locked
// while a turn looks in progress, and gives up on its own only after fifteen minutes, so an orphaned turn freezes the
// table's input. Stamping it completed with a reason releases the chat at once and says what happened.
func recoverInterruptedTurns(ctx context.Context, coll *mongo.Collection, hub *realtime.Hub, bootTime time.Time) {
	rows, err := coll.Find(ctx, bson.M{"llmCallStartedAt": bson.M{"$exists": true, "$type": "string"}},
		options.Find().SetProjection(bson.M{"gameId": 1, "llmCallStartedAt": 1, "llmCallCompletedAt": 1}).SetLimit(500))
	if err != nil {
		log.Printf("turn recovery: could not list games with a DM call: %v", err)
		return
	}
	defer rows.Close(context.Background())
	for rows.Next(ctx) {
		var row struct {
			GameID             string `bson:"gameId"`
			LLMCallStartedAt   string `bson:"llmCallStartedAt"`
			LLMCallCompletedAt string `bson:"llmCallCompletedAt"`
		}
		if err := rows.Decode(&row); err != nil || strings.TrimSpace(row.GameID) == "" {
			continue
		}
		if !dmTurnIsOrphaned(row.LLMCallStartedAt, row.LLMCallCompletedAt, bootTime) {
			continue
		}
		if _, err := coll.UpdateOne(ctx, bson.M{"gameId": row.GameID}, bson.M{"$set": bson.M{
			"llmCallCompletedAt": time.Now().UTC().Format(time.RFC3339Nano),
			"llmCallError":       interruptedTurnMessage,
		}}); err != nil {
			log.Printf("turn recovery: game %s: %v", row.GameID, err)
			continue
		}
		log.Printf("turn recovery: game %s had a DM turn the restart killed; closed it so the table stops waiting", row.GameID)
		if hub != nil {
			hub.NotifyGameStateUpdated(row.GameID)
		}
	}
}
