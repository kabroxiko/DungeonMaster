package gamesession

import (
	"context"
	"errors"
	"testing"
)

// A game with nothing to render must not leave its caller waiting on the map.
func TestStartCampaignWorldMapSaveWithoutARenderFinishesImmediately(t *testing.T) {
	warnings := startCampaignWorldMapSave(context.Background(), nil, "game", nil)

	select {
	case warning := <-warnings:
		if warning != "" {
			t.Fatalf("a nil render reported %q, want no warning", warning)
		}
	default:
		t.Fatal("the caller would block forever: a game with no render has nothing to wait for")
	}
}

// The reason a picture could not be stored has to reach the caller, because the campaign still succeeds and only the map
// is missing.
func TestStoreCampaignWorldMapReportsTheRenderError(t *testing.T) {
	warning := storeCampaignWorldMap(context.Background(), nil, "game", campaignWorldMapResult{Err: errors.New("map service unreachable")})

	if warning != "map service unreachable" {
		t.Fatalf("warning is %q, want the render error", warning)
	}
}

func TestStoreCampaignWorldMapReportsAnEmptyRender(t *testing.T) {
	warning := storeCampaignWorldMap(context.Background(), nil, "game", campaignWorldMapResult{})

	if warning != "the map image could not be rendered" {
		t.Fatalf("warning is %q, want the generic render failure", warning)
	}
}
