package gamesession

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// storedTurnBase reads the document a DM turn is written back to.
//
// Every field here is server-owned: the transcript, its summary copy, the counters, the setup and the encounter state
// are all produced by this process and stored in this document. The browser used to send its own copy of them with each
// turn, which made the client authoritative by accident: a tab that reloaded with a partial view replaced the stored
// history with the little it was holding. A turn now starts from the database, so the worst a stale tab can do is ask
// for an action; it can no longer hand back less history than the server already has.
func storedTurnBase(ctx context.Context, coll *mongo.Collection, gameID string) map[string]interface{} {
	var doc map[string]interface{}
	if err := coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc); err != nil {
		return nil
	}
	return map[string]interface{}{
		"gameId":                       gameID,
		"gameSetup":                    doc["gameSetup"],
		"conversation":                 doc["conversation"],
		"summaryConversation":          doc["summaryConversation"],
		"summary":                      doc["summary"],
		"totalTokenCount":              doc["totalTokenCount"],
		"userAndAssistantMessageCount": doc["userAndAssistantMessageCount"],
		"systemMessageContentDM":       doc["systemMessageContentDM"],
		"encounterState":               doc["encounterState"],
		"mode":                         doc["mode"],
	}
}
