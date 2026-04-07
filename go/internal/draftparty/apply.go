package draftparty

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
)

// ApplyDraftPartyTtlAfterCharacterGen mirrors server/services/draftPartyTtl.js applyDraftPartyTtlAfterCharacterGen.
func ApplyDraftPartyTtlAfterCharacterGen(ctx context.Context, coll *mongo.Collection, gameID string) error {
	if gameID == "" || DraftPartyTTLMs() <= 0 {
		return nil
	}
	var doc struct {
		MemberUserIDs []primitive.ObjectID `bson:"memberUserIds"`
		CampaignSpec  interface{}          `bson:"campaignSpec"`
	}
	err := coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc)
	if err != nil {
		return nil
	}
	m := len(doc.MemberUserIDs)
	spec, _ := doc.CampaignSpec.(map[string]interface{})
	hasCamp := campaignspec.HasSubstantiveCampaignSpec(spec)
	if m <= 1 && !hasCamp {
		if at := DraftPartyExpiresAtFromNow(); at != nil {
			_, _ = coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{"draftPartyExpiresAt": at}})
		}
	} else {
		_, _ = coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$unset": bson.M{"draftPartyExpiresAt": ""}})
	}
	return nil
}

// ClearDraftPartyTtlIfCampaignNowSubstantive mirrors clearDraftPartyTtlIfCampaignNowSubstantive.
func ClearDraftPartyTtlIfCampaignNowSubstantive(ctx context.Context, coll *mongo.Collection, gameID string) error {
	if gameID == "" {
		return nil
	}
	var doc struct {
		CampaignSpec interface{} `bson:"campaignSpec"`
	}
	err := coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc)
	if err != nil {
		return nil
	}
	spec, _ := doc.CampaignSpec.(map[string]interface{})
	if campaignspec.HasSubstantiveCampaignSpec(spec) {
		_, _ = coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$unset": bson.M{"draftPartyExpiresAt": ""}})
	}
	return nil
}
