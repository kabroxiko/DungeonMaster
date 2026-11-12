package gamesession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cbroglie/mustache"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/deckofdmthings/gmai/internal/campaignspec"
	"github.com/deckofdmthings/gmai/internal/promptmgr"
	"github.com/deckofdmthings/gmai/internal/vellaris"
)

// campaignWorld is the world a campaign is generated on: the stored world document plus the settings that render it
// again. Settings is nil when the world was imported without them or when the map service keeps them beside the map,
// which means no map image can be rendered by settings; the map id in the game document names the service's copy.
type campaignWorld struct {
	World    map[string]interface{}
	Settings map[string]interface{}
	Reused   bool
}

// worldSeedKeys are the settings fields Vellaris seeds its generation from. They move together so the graph,
// the background, the regions and the names all belong to the same world.
var worldSeedKeys = []string{"randomSeed", "backgroundRandomSeed", "regionsRandomSeed", "textRandomSeed", "frayedBorderSeed"}

// emptyWorldEdits is the "no edits yet" document Vellaris expects: its parser reads hasIconEdits unconditionally,
// so the object must be present even when nothing has been edited.
func emptyWorldEdits() map[string]interface{} {
	return map[string]interface{}{
		"centerEdits":  []interface{}{},
		"edgeEdits":    []interface{}{},
		"hasIconEdits": false,
		"iconEdits":    []interface{}{},
		"regionEdits":  []interface{}{},
		"roads":        []interface{}{},
		"textEdits":    []interface{}{},
	}
}

// prepareCampaignWorld resolves the world a campaign must be built on.
//
// The order is fixed: a game that already carries a world reuses it, otherwise the map service is asked once and the
// answer is stored on the game before any campaign text is generated. The returned message is empty on success; when
// it is not, no campaign may be produced, because a campaign that is not grounded in its map is exactly what this
// step exists to prevent.
//
// The returned channel carries the player-facing map image. It is started as soon as the settings are known, so the
// render runs while the world description is still being produced; it is nil when there is nothing to render (the
// game already has a stored map image, or the world was imported without settings).
//
// language is the game's session language: the map's place and region names are generated in it, because those names
// are what the story names.
func prepareCampaignWorld(ctx context.Context, d *Deps, gameID string, body map[string]interface{}, language string) (campaignWorld, <-chan campaignWorldMapResult, int, string, string) {
	if d == nil || d.Coll == nil {
		return campaignWorld{}, nil, 500, "DB_ERROR", "database is not available"
	}

	if gameID != "" {
		var doc map[string]interface{}
		if err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc); err == nil && doc != nil {
			spec, _ := doc["campaignSpec"].(map[string]interface{})
			existing := campaignspec.SanitizeVellarisWorld(mapFrom(spec, "vellarisWorld"))
			if existing != nil {
				settings, _ := spec["vellarisSettings"].(map[string]interface{})
				mapID := str(spec["vellarisMapId"])
				var imageCh <-chan campaignWorldMapResult
				if mapFrom(spec, "mapImage") == nil {
					// The world exists but its picture does not; fetch it from the service's stored map, or render it from
					// the stored settings for a world created before the service kept maps.
					imageCh = startCampaignWorldMapDownload(ctx, d, settings, mapID)
				}
				return campaignWorld{World: existing, Settings: settings, Reused: true}, imageCh, 200, "", ""
			}
		}
	}

	settings, explicit, code, problem := worldSettingsForCampaign(d, body)
	if problem != "" {
		return campaignWorld{}, nil, 400, code, problem
	}
	if d.Cfg == nil || strings.TrimSpace(d.Cfg.VellarisURL) == "" {
		return campaignWorld{}, nil, 400, "VELLARIS_NOT_CONFIGURED",
			"no map service is configured (set DM_VELLARIS_URL); a campaign is generated on the map it is played on"
	}
	if !explicit {
		settings = withFreshWorldSeed(settings)
	}
	settings = withCampaignLanguage(settings, language)

	// Ask for a stored map, so later calls can name the world by id instead of sending its megabyte of settings again.
	// A service without a database, or without the shared secret the game server was given, refuses that explicitly;
	// that is a supported deployment, so the world is then generated statelessly and its settings stay on the game.
	client := vellaris.NewWithToken(d.Cfg.VellarisURL, d.Cfg.VellarisToken)
	answer, err := client.WorldStored(ctx, settings)
	if err != nil {
		var apiErr *vellaris.APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusServiceUnavailable || apiErr.StatusCode == http.StatusForbidden) {
			log.Printf("campaign world: the map service cannot store maps (%d: %s); keeping the settings on the game instead", apiErr.StatusCode, apiErr.Message)
			answer, err = client.World(ctx, settings)
		}
	}
	if err != nil {
		return campaignWorld{}, nil, 502, "VELLARIS_WORLD_FAILED", err.Error()
	}
	world, _ := answer["world"].(map[string]interface{})
	stored := campaignspec.SanitizeVellarisWorld(world)
	if stored == nil {
		return campaignWorld{}, nil, 502, "VELLARIS_WORLD_EMPTY",
			"the map service returned a world with no regions and no places"
	}
	canonical, _ := answer["settings"].(map[string]interface{})
	if canonical == nil {
		canonical = settings
	}
	mapID, _ := answer["mapId"].(string)

	if gameID != "" {
		set := bson.M{"campaignSpec.vellarisWorld": stored}
		if mapID != "" {
			// The service keeps the canonical settings beside the stored world, so a second copy on the game would be
			// a second source of truth for the same map.
			set["campaignSpec.vellarisMapId"] = mapID
			set["campaignSpec.vellarisSettings"] = nil
		} else if encoded, err := json.Marshal(canonical); err == nil && len(encoded) <= maxStoredVellarisSettings {
			set["campaignSpec.vellarisSettings"] = canonical
		}
		if _, err := d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": set}); err != nil {
			return campaignWorld{}, nil, 500, "CAMPAIGN_WORLD_PERSIST_FAILED",
				"the generated world could not be saved, so the campaign was not started"
		}
		if mapID != "" {
			log.Printf("campaign world: stored map %s for game %s", mapID, gameID)
		}
	}

	// The image was rendered while the world was generated, so this is a download rather than a second render.
	imageCh := startCampaignWorldMapDownload(ctx, d, canonical, mapID)
	return campaignWorld{World: stored, Settings: canonical}, imageCh, 200, "", ""
}

// resolveCampaignLanguage returns the language the game is actually played in.
//
// The stored gameSetup is authoritative: the map's names, the campaign text and the table's UI all have to agree, so a
// request that omits (or disagrees with) the language cannot leave the map in a different one. The request value is
// used only when the game has no stored language yet.
func resolveCampaignLanguage(ctx context.Context, d *Deps, gameID, requested string) string {
	requested = strings.TrimSpace(requested)
	if d != nil && d.Coll != nil && strings.TrimSpace(gameID) != "" {
		var doc map[string]interface{}
		if err := d.Coll.FindOne(ctx, bson.M{"gameId": gameID}).Decode(&doc); err == nil {
			if setup, _ := doc["gameSetup"].(map[string]interface{}); setup != nil {
				if stored, _ := setup["language"].(string); strings.TrimSpace(stored) != "" {
					return strings.TrimSpace(stored)
				}
			}
		}
	}
	if requested == "" {
		return "English"
	}
	return requested
}

// worldSettingsForCampaign returns the map settings a world is generated with.
//
// Settings sent with the request are authoritative and keep their seed, which is how a caller asks for one specific
// world. Settings read from DM_VELLARIS_SETTINGS_FILE describe the kind of map instead, so each campaign built from
// the file gets its own seed (see withFreshWorldSeed).
func worldSettingsForCampaign(d *Deps, body map[string]interface{}) (settings map[string]interface{}, explicit bool, code, problem string) {
	if raw, ok := body["worldSettings"].(map[string]interface{}); ok && len(raw) > 0 {
		return cloneMap(raw), true, "", ""
	}

	path := ""
	if d != nil && d.Cfg != nil {
		path = strings.TrimSpace(d.Cfg.VellarisSettingsFile)
	}
	if path == "" {
		return nil, false, "VELLARIS_SETTINGS_REQUIRED",
			"send worldSettings, or set DM_VELLARIS_SETTINGS_FILE to a map-settings document"
	}
	fromFile, err := readWorldSettingsDocument(path)
	if err != nil {
		return nil, false, "VELLARIS_SETTINGS_UNREADABLE", err.Error()
	}
	return fromFile, false, "", ""
}

// readWorldSettingsDocument reads a map-settings document. A relative path is tried from the working directory and
// from its parent, because the API runs both from the repository root and from go/.
func readWorldSettingsDocument(path string) (map[string]interface{}, error) {
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		candidates = append(candidates, filepath.Join("..", path))
	}
	var lastErr error
	for _, candidate := range candidates {
		raw, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		var out map[string]interface{}
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("%s is not a map-settings document: %w", candidate, err)
		}
		return out, nil
	}
	return nil, fmt.Errorf("could not read map settings %s: %w", path, lastErr)
}

// withFreshWorldSeed returns a copy of settings that describes a new world.
//
// The settings file may hold either shape of document. A full one carries a seed per randomised part - the graph, the
// background, the regions and the names - and those seeds move together so the parts all belong to the same world. It
// also carries edits, which address centres and edges of the graph the old seed produced, so they are cleared with it.
// The short document carries neither: it names only the kind of map to invent and Vellaris invents the rest around it,
// so a single seed is enough to say which world comes out.
func withFreshWorldSeed(settings map[string]interface{}) map[string]interface{} {
	if _, full := settings["version"]; !full {
		out := cloneMap(settings)
		out["randomSeed"] = newWorldSeed()
		return out
	}
	out := cloneMap(settings)
	seed := newWorldSeed()
	for _, key := range worldSeedKeys {
		out[key] = seed
	}
	out["edits"] = emptyWorldEdits()
	return out
}

// newWorldSeed picks the seed for a new campaign world. The seed decides which map a table plays on, so it only needs
// to differ between campaigns, not to resist prediction.
func newWorldSeed() int64 {
	return rand.Int64()
}

// withCampaignLanguage stamps the game's language on the map settings, because Vellaris builds place and region names
// from language-specific formats ("Aldea de X" in Spanish, "Village of X" in English). Request settings are
// authoritative for the shape of the map, but the language belongs to the table, so it is always overwritten.
func withCampaignLanguage(settings map[string]interface{}, language string) map[string]interface{} {
	out := cloneMap(settings)
	out["language"] = vellarisLanguageCode(language)
	return out
}

// vellarisLanguageCode maps a session language name to the code Vellaris uses for name generation.
//
// Vellaris supports en, es, fr, de, pt, ru and zh; a game in any other language cannot have its map named in it, so the
// service default (English) is used rather than sending it a locale it may reject.
func vellarisLanguageCode(language string) string {
	normalized := strings.ToLower(strings.TrimSpace(language))
	switch {
	case strings.HasPrefix(normalized, "span") || normalized == "es":
		return "es"
	case strings.HasPrefix(normalized, "fren") || normalized == "fr":
		return "fr"
	case strings.HasPrefix(normalized, "germ") || normalized == "de":
		return "de"
	case strings.HasPrefix(normalized, "port") || normalized == "pt":
		return "pt"
	case strings.HasPrefix(normalized, "russ") || normalized == "ru":
		return "ru"
	case strings.HasPrefix(normalized, "chin") || normalized == "zh":
		return "zh"
	case strings.HasPrefix(normalized, "engl") || normalized == "en":
		return "en"
	default:
		return "en"
	}
}

// campaignWorldMapResult is a rendered map image, or the reason it could not be rendered.
type campaignWorldMapResult struct {
	Data        []byte
	ContentType string
	Err         error
}

// startCampaignWorldMapDownload fetches the player-facing map image.
//
// When the world is stored on the service the image is already rendered, so this is a download by id; otherwise it
// asks the service to render the settings, which is the stateless path.
func startCampaignWorldMapDownload(ctx context.Context, d *Deps, settings map[string]interface{}, mapID string) <-chan campaignWorldMapResult {
	ch := make(chan campaignWorldMapResult, 1)
	if d == nil || d.Cfg == nil || strings.TrimSpace(d.Cfg.VellarisURL) == "" {
		ch <- campaignWorldMapResult{Err: fmt.Errorf("no map service is configured to fetch the map image from")}
		close(ch)
		return ch
	}
	if strings.TrimSpace(mapID) == "" && settings == nil {
		ch <- campaignWorldMapResult{Err: fmt.Errorf("no map settings to render the map image from")}
		close(ch)
		return ch
	}
	go func() {
		client := vellaris.NewWithToken(d.Cfg.VellarisURL, d.Cfg.VellarisToken)
		if strings.TrimSpace(mapID) != "" {
			data, contentType, err := client.StoredWorldImage(ctx, mapID)
			ch <- campaignWorldMapResult{Data: data, ContentType: contentType, Err: err}
			close(ch)
			return
		}
		data, contentType, err := client.WorldMap(ctx, settings)
		ch <- campaignWorldMapResult{Data: data, ContentType: contentType, Err: err}
		close(ch)
	}()
	return ch
}

// finishCampaignWorldMapDownload waits for the image and stores it with the game.
//
// Returns a warning that describes a failure, or "" on success. A missing map image does not fail the campaign: the
// world, and therefore the campaign, exists already; the warning is reported to the caller so the host knows the
// map cannot be shown instead of discovering it when the players ask.
func finishCampaignWorldMapDownload(ctx context.Context, d *Deps, gameID string, ch <-chan campaignWorldMapResult) string {
	if ch == nil {
		return ""
	}
	result := <-ch
	if result.Err != nil || len(result.Data) == 0 {
		warning := "the map image could not be rendered"
		if result.Err != nil {
			warning = result.Err.Error()
		}
		log.Printf("campaign world map image failed for gameId=%s: %s", gameID, warning)
		if gameID != "" && d != nil && d.Coll != nil {
			_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
				"campaignSpec.mapImageError": warning,
				"campaignSpec.mapImage":      nil,
			}})
		}
		return warning
	}
	if gameID == "" {
		return ""
	}
	contentType := result.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}
	if err := storeWorldMapImage(ctx, d, gameID, contentType, result.Data); err != nil {
		log.Printf("campaign world map image store failed for gameId=%s: %v", gameID, err)
		_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
			"campaignSpec.mapImageError": err.Error(),
			"campaignSpec.mapImage":      nil,
		}})
		return err.Error()
	}
	_, _ = d.Coll.UpdateOne(ctx, bson.M{"gameId": gameID}, bson.M{"$set": bson.M{
		"campaignSpec.mapImage": map[string]interface{}{
			"contentType": contentType,
			"byteLength":  len(result.Data),
			// Clients key the image URL on this so a regenerated map is never served from a stale browser cache.
			"updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
		"campaignSpec.mapImageError": nil,
	}})
	return ""
}

// renderCampaignWorldContext renders the world block the campaign generator reads.
func renderCampaignWorldContext(world map[string]interface{}) string {
	normalized := campaignspec.NormalizeVellarisWorld(world)
	if normalized == nil {
		return ""
	}
	template := promptmgr.LoadPrompt("templates/campaign/world_context.txt")
	if template == "" {
		return ""
	}
	rendered, err := mustache.Render(template, normalized)
	if err != nil {
		return ""
	}
	return rendered
}

// campaignWorldSystemBlock returns the authoritative-geography block for a campaign stage's system stack.
//
// maxPlaces caps the place list; -1 skips places entirely. The keyLocations stage passes -1 because its user message
// already carries the exact region and place lists, and spending context twice on the same names buys nothing.
func campaignWorldSystemBlock(world map[string]interface{}, maxPlaces int) string {
	if world == nil {
		return ""
	}
	block := cloneMap(world)
	if maxPlaces < 0 {
		block["places"] = nil
		block["hasPlaces"] = false
		block["placeCount"] = 0
		block["placesNotice"] = ""
	} else if maxPlaces > 0 {
		block["places"] = limitWorldPlaces(block["places"], maxPlaces)
	}
	context := renderCampaignWorldContext(block)
	if context == "" {
		return ""
	}
	rules := promptmgr.LoadPrompt("rules/vellaris_world.txt")
	if rules != "" {
		context = rules + "\n\n" + context
	}
	return context
}

// limitWorldPlaces keeps the first n places, which NormalizeVellarisWorld has already ranked by significance.
func limitWorldPlaces(raw interface{}, n int) []interface{} {
	list, _ := raw.([]interface{})
	if len(list) <= n {
		return list
	}
	return list[:n]
}

// campaignWorldTemplateData is the Mustache data for the campaign generator's region and place lists.
func campaignWorldTemplateData(world map[string]interface{}) map[string]interface{} {
	normalized := campaignspec.NormalizeVellarisWorld(world)
	if normalized == nil {
		return map[string]interface{}{"hasVellarisWorld": false}
	}
	return map[string]interface{}{
		"hasVellarisWorld": true,
		"vellarisRegions":  normalized["regions"],
		"vellarisPlaces":   limitWorldPlaces(normalized["places"], 24),
	}
}

// mapFrom returns a nested map field, or nil when it is absent or another type.
func mapFrom(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	out, _ := m[key].(map[string]interface{})
	return out
}
