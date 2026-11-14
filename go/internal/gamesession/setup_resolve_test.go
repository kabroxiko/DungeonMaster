package gamesession

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// A draft is what makes a random character random: the same draft must resolve identically (the name preview and the
// generation ask twice), and two different drafts must not resolve to the same character.
func TestRandomSetupIsStablePerDraftAndDifferentAcrossDrafts(t *testing.T) {
	first := ResolveCharacterSetupForGeneration(map[string]interface{}{
		"race": "random", "class": "random", "subclass": "random", "subrace": "random", "background": "random", "draftId": "a",
	})
	firstAgain := ResolveCharacterSetupForGeneration(map[string]interface{}{
		"race": "random", "class": "random", "subclass": "random", "subrace": "random", "background": "random", "draftId": "a",
	})
	for _, key := range []string{"race", "class", "subrace", "subclass", "background"} {
		if first[key] != firstAgain[key] {
			t.Fatalf("one draft resolved two different characters: %v changed from %v to %v", key, first[key], firstAgain[key])
		}
	}

	sameCharacter := true
	for _, draft := range []string{"b", "c", "d", "e", "f", "g"} {
		other := ResolveCharacterSetupForGeneration(map[string]interface{}{
			"race": "random", "class": "random", "subclass": "random", "subrace": "random", "background": "random", "draftId": draft,
		})
		if other["race"] != first["race"] || other["class"] != first["class"] {
			sameCharacter = false
			break
		}
	}
	if sameCharacter {
		t.Fatal("every draft resolved to the same race and class, which is what the draft id exists to prevent")
	}

	if _, leaked := first["draftId"]; leaked {
		t.Fatal("the draft id seeds the resolution; it must not be carried into the character")
	}
}

// The table plays one adventure at one level, so a guest's sheet is written at the host's level.
func TestPartyLevelComesFromTheHostSheet(t *testing.T) {
	owner := primitive.NewObjectID()
	gameRow := map[string]interface{}{
		"ownerUserId": owner,
		"gameSetup": map[string]interface{}{
			"playerCharacters": map[string]interface{}{
				owner.Hex(): map[string]interface{}{"level": float64(5)},
			},
		},
	}
	if got := partyLevelFor(gameRow, map[string]interface{}{"level": 2}); got != 5 {
		t.Fatalf("a guest must be written at the host's level, got %d", got)
	}
	if got := partyLevelFor(nil, map[string]interface{}{"level": 3}); got != 3 {
		t.Fatalf("with no host sheet the requested level is used, got %d", got)
	}
	if got := partyLevelFor(nil, nil); got != 1 {
		t.Fatalf("with no level anywhere the sheet is level 1, got %d", got)
	}
}
