package gamesession

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/gameaccess"
	"github.com/deckofdmthings/gmai/internal/store"
)

// undoStackLimit bounds how many actions a development table can step back.
const undoStackLimit = 20

// UndoEnabled reports whether this process lets the table take an action back.
//
// It is a development tool. In production the same route would be a data-loss button with a friendly name: any member
// could delete the turn the rest of the table already read.
func UndoEnabled(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if config.TruthyEnv("DM_ALLOW_UNDO") {
		return true
	}
	// `npm start` does not set NODE_ENV, so the dev server being spawned is the signal a developer's machine actually
	// gives. A deployment that serves the built client has neither.
	if config.TruthyEnv("DM_SPAWN_VUE_DEV") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(cfg.NodeEnv)) {
	case "development", "dev":
		return true
	}
	return false
}

// RecordActionStart writes the mark that says where the action about to be taken begins.
//
// It holds two lengths and the two values the turn overwrites; it is not a copy of the game. Undo is then a deletion:
// the player's line, the DM's answer and the state they moved are removed, and what stays is what was there before.
func RecordActionStart(ctx context.Context, coll, turns *mongo.Collection, gameID, userID string) {
	if coll == nil || turns == nil || strings.TrimSpace(gameID) == "" {
		return
	}
	projection := bson.M{
		"conversation":                  1,
		"summaryConversation":           1,
		"encounterState":                1,
		"campaignSpec.partyMapPosition": 1,
	}
	var doc map[string]interface{}
	if err := coll.FindOne(ctx, bson.M{"gameId": gameID}, options.FindOne().SetProjection(projection)).Decode(&doc); err != nil {
		return
	}
	spec, _ := doc["campaignSpec"].(map[string]interface{})
	var position interface{}
	if spec != nil {
		position = spec["partyMapPosition"]
	}
	mark := bson.M{
		"gameId":         gameID,
		"at":             time.Now().UTC().Format(time.RFC3339Nano),
		"user":           userID,
		"messages":       len(toIfaceSlice(doc["conversation"])),
		"summary":        len(toIfaceSlice(doc["summaryConversation"])),
		"encounterState": doc["encounterState"],
		"partyPosition":  position,
	}
	if _, err := turns.InsertOne(ctx, mark); err != nil {
		log.Printf("undo: could not record the start of an action gameId=%s: %v", gameID, err)
		return
	}
	trimTurnMarks(ctx, turns, gameID)
}

// trimTurnMarks drops the oldest marks past the limit, so a long session does not accumulate one per turn.
func trimTurnMarks(ctx context.Context, turns *mongo.Collection, gameID string) {
	opts := options.Find().SetSort(bson.M{"_id": -1}).SetSkip(undoStackLimit).SetProjection(bson.M{"_id": 1})
	cursor, err := turns.Find(ctx, bson.M{"gameId": gameID}, opts)
	if err != nil {
		return
	}
	defer cursor.Close(ctx)
	var ids []interface{}
	for cursor.Next(ctx) {
		var item struct {
			ID interface{} `bson:"_id"`
		}
		if err := cursor.Decode(&item); err == nil && item.ID != nil {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) > 0 {
		_, _ = turns.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	}
}

// truncateMessages keeps the first n of a stored list, which is how a turn is deleted from the transcript.
func truncateMessages(messages []interface{}, n int) []interface{} {
	if n < 0 {
		n = 0
	}
	if n > len(messages) {
		n = len(messages)
	}
	return messages[:n]
}

// countTranscript recomputes the counters from the truncated transcript: a counter that still counts a deleted message
// is the same class of lie as an encounter state that still holds its damage.
func countTranscript(messages []interface{}) (pairs int, tokens int) {
	for _, raw := range messages {
		m, _ := raw.(map[string]interface{})
		if m == nil {
			continue
		}
		tokens += len(str(m["content"])) / 4
		if str(m["role"]) == "assistant" {
			pairs++
		}
	}
	return pairs, tokens
}

// HandleUndo deletes what the last action added: its messages, its encounter state and its map pin.
func HandleUndo(ctx context.Context, d *Deps, cfg *config.Config, userID string, body map[string]interface{}) (int, map[string]interface{}) {
	if !UndoEnabled(cfg) {
		return http.StatusForbidden, map[string]interface{}{
			"error": "Undo is a development tool and is not enabled on this server.",
			"code":  "UNDO_DISABLED",
		}
	}
	gameID := strings.TrimSpace(str(body["gameId"]))
	if gameID == "" {
		return http.StatusBadRequest, map[string]interface{}{"error": "gameId is required", "code": "GAME_ID_REQUIRED"}
	}
	if d == nil || d.Coll == nil {
		return http.StatusInternalServerError, map[string]interface{}{"error": "No database", "code": "UNDO_FAILED"}
	}
	coll := store.GameStates(d.Coll.Database())
	turns := store.GameStateTurns(d.Coll.Database())
	if _, err := gameaccess.AssertGameMember(ctx, coll, userID, gameID); err != nil {
		if err == gameaccess.ErrGameNotFound {
			return http.StatusNotFound, map[string]interface{}{"error": "Game not found", "code": "GAME_NOT_FOUND"}
		}
		return http.StatusBadRequest, map[string]interface{}{"error": err.Error()}
	}

	var mark struct {
		ID             interface{} `bson:"_id"`
		Messages       int         `bson:"messages"`
		Summary        int         `bson:"summary"`
		EncounterState interface{} `bson:"encounterState"`
		PartyPosition  interface{} `bson:"partyPosition"`
		At             string      `bson:"at"`
	}
	findOpts := options.FindOne().SetSort(bson.M{"_id": -1})
	markErr := turns.FindOne(ctx, bson.M{"gameId": gameID}, findOpts).Decode(&mark)
	if markErr != nil && markErr != mongo.ErrNoDocuments {
		return http.StatusInternalServerError, map[string]interface{}{"error": "Could not read the last action.", "code": "UNDO_FAILED"}
	}

	var doc map[string]interface{}
	if err := coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc); err != nil {
		return http.StatusInternalServerError, map[string]interface{}{"error": "Could not read the game.", "code": "UNDO_FAILED"}
	}
	conversation := toIfaceSlice(doc["conversation"])
	before := len(conversation)

	// The transcript itself says where the last action began: the last line a player wrote. The mark is used when it
	// describes this action, and it is the only source for the state the turn overwrote - a turn played before marks
	// existed is still undone, and its encounter state is simply dropped rather than left describing a deleted turn.
	useMark := markErr == nil && mark.Messages < before
	cut := -1
	if useMark {
		cut = mark.Messages
	} else {
		cut = lastPlayerMessageIndex(conversation)
	}
	if cut <= 0 {
		return http.StatusNotFound, map[string]interface{}{
			"error": "There is no player action left to undo.",
			"code":  "UNDO_NOTHING",
		}
	}
	messages := truncateMessages(conversation, cut)
	summary := truncateMessages(toIfaceSlice(doc["summaryConversation"]), cut)
	pairs, tokens := countTranscript(messages)

	update := bson.M{
		"$set": bson.M{
			"conversation":                 messages,
			"summaryConversation":          summary,
			"userAndAssistantMessageCount": pairs,
			"totalTokenCount":              tokens,
		},
		"$unset": bson.M{
			"llmCallError":       "",
			"llmCallStartedAt":   "",
			"llmCallCompletedAt": "",
			"llmCallEnteredAt":   "",
			"rawModelOutput":     "",
		},
	}
	if useMark && mark.EncounterState != nil {
		update["$set"].(bson.M)["encounterState"] = mark.EncounterState
	} else {
		update["$unset"].(bson.M)["encounterState"] = ""
	}
	if useMark && mark.PartyPosition != nil {
		update["$set"].(bson.M)["campaignSpec.partyMapPosition"] = mark.PartyPosition
	} else {
		update["$unset"].(bson.M)["campaignSpec.partyMapPosition"] = ""
	}
	if _, err := coll.UpdateOne(ctx, bson.M{"gameId": gameID}, update); err != nil {
		log.Printf("undo: could not delete the turn gameId=%s: %v", gameID, err)
		return http.StatusInternalServerError, map[string]interface{}{"error": "Could not delete the turn.", "code": "UNDO_FAILED"}
	}
	if markErr == nil {
		if _, err := turns.DeleteOne(ctx, bson.M{"_id": mark.ID}); err != nil {
			log.Printf("undo: deleted the turn of gameId=%s but could not drop its mark: %v", gameID, err)
		}
	}

	remaining, _ := turns.CountDocuments(ctx, bson.M{"gameId": gameID})
	log.Printf("undo: gameId=%s deleted %d message(s) (cut at %d, mark=%v); %d more step(s) available",
		gameID, before-len(messages), cut, useMark, remaining)
	if d.Hub != nil {
		d.Hub.NotifyGameStateUpdated(gameID)
	}
	return http.StatusOK, map[string]interface{}{
		"ok":              true,
		"gameId":          gameID,
		"deletedMessages": before - len(messages),
		"messageCount":    len(messages),
		"stepsRemaining":  remaining,
	}
}

// lastPlayerMessageIndex is where the player's last line starts, which is where the last action starts: the DM's answer
// and anything the engine wrote after it belong to that action.
func lastPlayerMessageIndex(conversation []interface{}) int {
	for i := len(conversation) - 1; i >= 0; i-- {
		m, _ := conversation[i].(map[string]interface{})
		if m == nil {
			continue
		}
		if str(m["role"]) == "user" {
			return i
		}
	}
	return -1
}
