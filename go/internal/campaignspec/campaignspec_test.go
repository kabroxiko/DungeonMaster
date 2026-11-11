package campaignspec

import "testing"

// A character generated before the campaign stores the rendered map in campaignSpec. That marker must not make the game
// look like it already has a campaign, or the lobby start answers "already started" and no campaign is ever written.
func TestRenderedMapDoesNotCountAsACampaign(t *testing.T) {
	spec := map[string]interface{}{
		"mapImage": map[string]interface{}{
			"contentType": "image/jpeg",
			"byteLength":  2109050.0,
			"updatedAt":   "2026-11-10T20:27:55.840023Z",
		},
		"mapImageError":    nil,
		"partyMapPosition": map[string]interface{}{"x": 0.42, "y": 0.31, "label": "Aditarie"},
		"vellarisWorld":    map[string]interface{}{"seed": 42.0, "regions": []interface{}{map[string]interface{}{"name": "Giah"}}},
		"vellarisSettings": map[string]interface{}{"randomSeed": 42.0},
	}
	if HasSubstantiveCampaignSpec(spec) {
		t.Fatal("a game that only has a map and no campaign text has no campaign yet")
	}

	// The map marker stays visible to clients: the show/hide control reads it.
	if RedactCampaignSpecForClient(spec)["mapImage"] == nil {
		t.Fatal("the rendered map marker must stay in the client view")
	}
	if RedactCampaignSpecForClient(spec)["vellarisWorld"] != nil {
		t.Fatal("the world itself is DM-only")
	}
	// The party pin is part of the map the table is shown, so clients must receive it and it must not look like story.
	if RedactCampaignSpecForClient(spec)["partyMapPosition"] == nil {
		t.Fatal("the party pin must stay in the client view")
	}
	if RedactCampaignSpecForClient(spec)["mapGeometry"] != nil {
		t.Fatal("the coordinate projection is arithmetic, not something clients need")
	}

	spec["title"] = "La Traición del Mentor"
	if !HasSubstantiveCampaignSpec(spec) {
		t.Fatal("a written campaign is substantive")
	}
}

func TestEmptyCampaignSpecIsNotSubstantive(t *testing.T) {
	if HasSubstantiveCampaignSpec(nil) {
		t.Fatal("a missing spec has no campaign")
	}
	if HasSubstantiveCampaignSpec(map[string]interface{}{"mapImageError": "render failed"}) {
		t.Fatal("a failed map render is not a campaign")
	}
	if HasSubstantiveCampaignSpec(map[string]interface{}{"keyLocations": []interface{}{}}) {
		t.Fatal("empty stage output is not a campaign")
	}
}
