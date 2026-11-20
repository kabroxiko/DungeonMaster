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

// undoStackLimit bounds how many actions a development table can step back. Each entry is a full copy of the game, so
// the stack is short on purpose: this is for "that turn was wrong", not for a timeline.
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

// SnapshotBeforeAction stores the game as it stands before a player action changes it.
//
// The snapshot is taken at the action, not at the reply: undo has to remove the player's line and the DM's answer
// together, because the pair is what a turn is.
func SnapshotBeforeAction(ctx context.Context, coll, snapshots *mongo.Collection, gameID string) {
	if coll == nil || snapshots == nil || strings.TrimSpace(gameID) == "" {
		return
	}
	var doc map[string]interface{}
	if err := coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc); err != nil || doc == nil {
		return
	}
	if _, err := snapshots.InsertOne(ctx, bson.M{
		"gameId":  gameID,
		"takenAt": time.Now().UTC().Format(time.RFC3339Nano),
		"doc":     doc,
	}); err != nil {
		log.Printf("undo: could not snapshot gameId=%s: %v", gameID, err)
		return
	}
	trimUndoStack(ctx, snapshots, gameID)
}

// trimUndoStack drops the oldest snapshots past the limit, so a long session does not accumulate a document per turn.
func trimUndoStack(ctx context.Context, snapshots *mongo.Collection, gameID string) {
	opts := options.Find().SetSort(bson.M{"_id": -1}).SetSkip(undoStackLimit).SetProjection(bson.M{"_id": 1})
	cursor, err := snapshots.Find(ctx, bson.M{"gameId": gameID}, opts)
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
	if len(ids) == 0 {
		return
	}
	_, _ = snapshots.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
}

// HandleUndo puts the game back to the snapshot taken before the last player action.
//
// It answers with the restored document so the caller can re-render without a second request, and it is refused outside
// development: see UndoEnabled.
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
	coll := store.GameStates(d.Coll.Database())
	snapshots := store.GameStateSnapshots(d.Coll.Database())
	if _, err := gameaccess.AssertGameMember(ctx, coll, userID, gameID); err != nil {
		if err == gameaccess.ErrGameNotFound {
			return http.StatusNotFound, map[string]interface{}{"error": "Game not found", "code": "GAME_NOT_FOUND"}
		}
		return http.StatusBadRequest, map[string]interface{}{"error": err.Error()}
	}

	var snapshot struct {
		ID      interface{}            `bson:"_id"`
		Doc     map[string]interface{} `bson:"doc"`
		TakenAt string                 `bson:"takenAt"`
	}
	findOpts := options.FindOne().SetSort(bson.M{"_id": -1})
	if err := snapshots.FindOne(ctx, bson.M{"gameId": gameID}, findOpts).Decode(&snapshot); err != nil {
		if err == mongo.ErrNoDocuments {
			return http.StatusNotFound, map[string]interface{}{
				"error": "There is no earlier state to go back to.",
				"code":  "UNDO_NOTHING",
			}
		}
		return http.StatusInternalServerError, map[string]interface{}{"error": "Could not read the previous state.", "code": "UNDO_FAILED"}
	}
	if snapshot.Doc == nil {
		return http.StatusInternalServerError, map[string]interface{}{"error": "The previous state is empty.", "code": "UNDO_FAILED"}
	}

	// The replacement document cannot carry a different _id; deleting it lets Mongo keep the one the game already has.
	restored := make(map[string]interface{}, len(snapshot.Doc))
	for k, v := range snapshot.Doc {
		if k == "_id" {
			continue
		}
		restored[k] = v
	}
	if _, err := coll.ReplaceOne(ctx, bson.M{"gameId": gameID}, restored); err != nil {
		log.Printf("undo: could not restore gameId=%s: %v", gameID, err)
		return http.StatusInternalServerError, map[string]interface{}{"error": "Could not restore the previous state.", "code": "UNDO_FAILED"}
	}
	if _, err := snapshots.DeleteOne(ctx, bson.M{"_id": snapshot.ID}); err != nil {
		log.Printf("undo: restored gameId=%s but could not drop the snapshot: %v", gameID, err)
	}

	conversation := toIfaceSlice(restored["conversation"])
	remaining, _ := snapshots.CountDocuments(ctx, bson.M{"gameId": gameID})
	log.Printf("undo: gameId=%s restored to %s; %d message(s) left, %d more step(s) available",
		gameID, snapshot.TakenAt, len(conversation), remaining)
	if d.Hub != nil {
		d.Hub.NotifyGameStateUpdated(gameID)
	}
	return http.StatusOK, map[string]interface{}{
		"ok":             true,
		"gameId":         gameID,
		"undone":         snapshot.TakenAt,
		"messageCount":   len(conversation),
		"stepsRemaining": remaining,
	}
}
