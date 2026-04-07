package gamesession

import (
	"github.com/deckofdmthings/gmai/internal/config"
	"github.com/deckofdmthings/gmai/internal/realtime"
	"go.mongodb.org/mongo-driver/mongo"
)

// Deps holds shared services for game session handlers.
type Deps struct {
	Cfg  *config.Config
	Coll *mongo.Collection
	Hub  *realtime.Hub
}
