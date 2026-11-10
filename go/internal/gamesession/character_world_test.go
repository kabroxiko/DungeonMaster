package gamesession

import (
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
)

// A character sheet names a homeland, so the sheet has to be written on the table's map instead of on invented places.
func TestCharacterWorldSystemBlockKeepsTheCharacterOnTheMap(t *testing.T) {
	world, _ := sampleVellarisSpec()["vellarisWorld"].(map[string]interface{})

	block := characterWorldSystemBlock(world, 12)
	for _, want := range []string{"THE TABLE'S MAP", "Homeland", "Venetzaddana", "Suste", "Azureenci", "Aditarie", "settlement"} {
		if !strings.Contains(block, want) {
			t.Fatalf("character world block is missing %q\n---\n%s", want, block)
		}
	}
	// Coordinates are the DM's working numbers; a character sheet never sees them.
	if strings.Contains(block, "1773.46") || strings.Contains(block, "1794.97") {
		t.Fatalf("the character world block must not carry coordinates\n---\n%s", block)
	}
	if got := characterWorldSystemBlock(nil, 12); got != "" {
		t.Fatalf("a game with no world must not get a world block, got %q", got)
	}
}

func TestCharacterWorldSystemBlockCapsThePlacesItLists(t *testing.T) {
	world, _ := sampleVellarisSpec()["vellarisWorld"].(map[string]interface{})
	places, _ := campaignspec.NormalizeVellarisWorld(world)["places"].([]interface{})
	if len(places) < 2 {
		t.Fatalf("the fixture needs two places, got %d", len(places))
	}
	firstName, _ := places[0].(map[string]interface{})["name"].(string)
	secondName, _ := places[1].(map[string]interface{})["name"].(string)

	one := characterWorldSystemBlock(world, 1)
	if !strings.Contains(one, firstName) {
		t.Fatalf("the first place must survive the cap\n---\n%s", one)
	}
	if strings.Contains(one, secondName) {
		t.Fatalf("places beyond the cap must not be listed\n---\n%s", one)
	}
	regionsOnly := characterWorldSystemBlock(world, -1)
	if strings.Contains(regionsOnly, firstName) {
		t.Fatalf("-1 must skip places entirely\n---\n%s", regionsOnly)
	}
	if !strings.Contains(regionsOnly, "Venetzaddana") {
		t.Fatalf("regions are the homeland vocabulary, so they stay\n---\n%s", regionsOnly)
	}
}
