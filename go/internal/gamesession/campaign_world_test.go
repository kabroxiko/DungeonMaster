package gamesession

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deckofdmthings/gmai/internal/config"
)

// A generated world is only usable if its graph, background, regions and names all come from one seed, and old edits
// (which address the previous graph's centres) must not survive the reseed.
func TestWithFreshWorldSeedMovesEverySeedAndClearsEdits(t *testing.T) {
	base := map[string]interface{}{
		"randomSeed":           1.0,
		"backgroundRandomSeed": 1.0,
		"regionsRandomSeed":    1.0,
		"textRandomSeed":       1.0,
		"frayedBorderSeed":     1.0,
		"worldSize":            10000.0,
		"edits": map[string]interface{}{
			"centerEdits":  []interface{}{map[string]interface{}{"centerIndex": 3.0}},
			"hasIconEdits": true,
		},
	}
	got := withFreshWorldSeed(base)

	var seed interface{}
	for _, key := range worldSeedKeys {
		if got[key] == nil {
			t.Fatalf("%s was not seeded", key)
		}
		if seed == nil {
			seed = got[key]
			continue
		}
		if got[key] != seed {
			t.Fatalf("%s carries a different seed (%v) than %v", key, got[key], seed)
		}
	}
	if seed == 1.0 {
		t.Fatal("the settings file seed must not be reused for a new campaign world")
	}
	if got["worldSize"] != 10000.0 {
		t.Fatalf("settings unrelated to the seed must be kept, got worldSize=%v", got["worldSize"])
	}
	edits, _ := got["edits"].(map[string]interface{})
	if edits == nil {
		t.Fatal("Vellaris requires an edits object even when nothing is edited")
	}
	if edits["hasIconEdits"] != false {
		t.Fatalf("old edits must be cleared with the seed, got hasIconEdits=%v", edits["hasIconEdits"])
	}
	if base["randomSeed"] != 1.0 {
		t.Fatal("the caller's settings must not be mutated")
	}
}

func TestWorldSettingsForCampaignPrefersTheRequest(t *testing.T) {
	sent := map[string]interface{}{"randomSeed": 99.0, "worldSize": 5000.0}
	settings, explicit, code, problem := worldSettingsForCampaign(&Deps{Cfg: &config.Config{}}, map[string]interface{}{
		"worldSettings": sent,
	})
	if problem != "" || code != "" {
		t.Fatalf("unexpected rejection: %s %s", code, problem)
	}
	if !explicit {
		t.Fatal("request settings are an explicit world choice")
	}
	if settings["randomSeed"] != 99.0 {
		t.Fatalf("request seed must be kept, got %v", settings["randomSeed"])
	}
}

func TestWorldSettingsForCampaignReportsMissingConfiguration(t *testing.T) {
	_, _, code, problem := worldSettingsForCampaign(&Deps{Cfg: &config.Config{}}, nil)
	if code != "VELLARIS_SETTINGS_REQUIRED" || problem == "" {
		t.Fatalf("a server with no settings source must say so, got %q %q", code, problem)
	}

	missing := filepath.Join(t.TempDir(), "not-there.json")
	_, _, code, problem = worldSettingsForCampaign(&Deps{Cfg: &config.Config{VellarisSettingsFile: missing}}, nil)
	if code != "VELLARIS_SETTINGS_UNREADABLE" || !strings.Contains(problem, "not-there.json") {
		t.Fatalf("an unreadable settings file must name the file, got %q %q", code, problem)
	}
}

func TestWorldSettingsForCampaignReadsTheConfiguredFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world-settings.json")
	if err := os.WriteFile(path, []byte(`{"worldSize": 8000, "randomSeed": 5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, explicit, code, problem := worldSettingsForCampaign(&Deps{Cfg: &config.Config{VellarisSettingsFile: path}}, nil)
	if problem != "" {
		t.Fatalf("unexpected rejection: %s %s", code, problem)
	}
	if explicit {
		t.Fatal("settings from the file are a shape, not a chosen world")
	}
	if settings["worldSize"] != 8000.0 {
		t.Fatalf("settings were not read, got %v", settings)
	}
}

func TestCampaignWorldPromptDataCarriesTheRegions(t *testing.T) {
	world, _ := sampleVellarisSpec()["vellarisWorld"].(map[string]interface{})

	data := campaignWorldTemplateData(world)
	if data["hasVellarisWorld"] != true {
		t.Fatal("a world must be announced to the generator")
	}
	regions, _ := data["vellarisRegions"].([]interface{})
	if len(regions) == 0 {
		t.Fatal("the generator needs the region list to place conflicts")
	}

	context := renderCampaignWorldContext(world)
	for _, want := range []string{"DM-ONLY VELLARIS WORLD", "Venetzaddana", "Suste", "Azureenci"} {
		if !strings.Contains(context, want) {
			t.Fatalf("campaign world block is missing %q\n---\n%s", want, context)
		}
	}
	if strings.Contains(context, "1773.46") {
		t.Fatalf("campaign world block must not carry coordinates\n---\n%s", context)
	}
}

func TestCampaignWorldPromptDataCarriesPlacesForTheGenerator(t *testing.T) {
	world, _ := sampleVellarisSpec()["vellarisWorld"].(map[string]interface{})

	data := campaignWorldTemplateData(world)
	places, _ := data["vellarisPlaces"].([]interface{})
	if len(places) == 0 {
		t.Fatal("the generator needs the map's named places to write a story on that map")
	}
	first, _ := places[0].(map[string]interface{})
	if first["name"] == "" {
		t.Fatalf("a listed place has no name: %v", first)
	}
}

// Every campaign stage gets the map in its system stack, so a stage that names a settlement cannot drift off the map.
func TestCampaignWorldSystemBlockCarriesRulesRegionsAndPlaces(t *testing.T) {
	world, _ := sampleVellarisSpec()["vellarisWorld"].(map[string]interface{})

	block := campaignWorldSystemBlock(world, 10)
	for _, want := range []string{"VELLARIS WORLD", "Never add geography", "DM-ONLY VELLARIS WORLD", "Venetzaddana", "Azureenci"} {
		if !strings.Contains(block, want) {
			t.Fatalf("stage world block is missing %q\n---\n%s", want, block)
		}
	}
	if campaignWorldSystemBlock(nil, 10) != "" {
		t.Fatal("a game with no world must inject nothing")
	}

	// The keyLocations stage lists places in its user message; the system block only needs the regions then.
	withoutRoutes := map[string]interface{}{}
	for k, v := range world {
		withoutRoutes[k] = v
	}
	withoutRoutes["routes"] = nil
	regionsOnly := campaignWorldSystemBlock(withoutRoutes, -1)
	if !strings.Contains(regionsOnly, "Venetzaddana") {
		t.Fatal("regions must survive the places cap")
	}
	if strings.Contains(regionsOnly, "Named places (") {
		t.Fatal("places should be skipped when they are already in the user message")
	}
}

// The opening turn must be written from the stored campaign, with location names reconciled against the map. Before
// this injection existed the model saw only the world and the sheet, so the opening was a story of its own.
func TestRenderInitialCampaignInjectGroundsTheOpeningInTheCampaignAndMap(t *testing.T) {
	spec := sampleVellarisSpec()
	spec["title"] = "La Red de Sombras"
	spec["campaignConcept"] = "War over the salt road."
	spec["majorConflicts"] = []interface{}{"guilds against the crown"}
	spec["toneAndStyle"] = "somber"
	spec["keyLocations"] = []interface{}{
		map[string]interface{}{"name": "Ironspire Vault", "type": "ruin", "significance": "an old vault"},
	}
	spec["openingSceneFrame"] = map[string]interface{}{
		"id": "temple_alarm",
		"directive": strings.Repeat(
			"The PCs stand at the gates of the Ironspire Vault while an alarm bell tolls over the roofs. ", 2),
	}

	got := renderInitialCampaignInject(spec)
	for _, want := range []string{
		"DM-ONLY INITIAL ADVENTURE CONTEXT",
		"War over the salt road.",
		"guilds against the crown",
		"an old vault",
		"Aditarie",
		"Settlements named on the players' map",
		"Mandatory opening frame",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("initial campaign inject is missing %q\n---\n%s", want, got)
		}
	}
	if strings.Contains(got, "Ironspire Vault") {
		t.Fatalf("a key location the map does not have must be reconciled before the opening\n---\n%s", got)
	}
}

func TestRenderInitialCampaignInjectKeepsNamesWithoutAMap(t *testing.T) {
	spec := map[string]interface{}{
		"title":           "No map here",
		"campaignConcept": "A campaign with no imported world.",
		"keyLocations": []interface{}{
			map[string]interface{}{"name": "Ironspire Vault", "type": "ruin", "significance": "an old vault"},
		},
	}
	got := renderInitialCampaignInject(spec)
	if !strings.Contains(got, "Ironspire Vault") {
		t.Fatalf("a campaign with no world keeps its location names\n---\n%s", got)
	}
	if strings.Contains(got, "Settlements named on the players' map") {
		t.Fatal("no map means no place list")
	}
}

// The map's place and region names are the names the story uses, so they are generated in the game's language.
func TestWithCampaignLanguageStampsTheSessionLanguage(t *testing.T) {
	base := map[string]interface{}{"language": "en", "worldSize": 10000.0}
	got := withCampaignLanguage(base, "Spanish")
	if got["language"] != "es" {
		t.Fatalf("a Spanish game must generate Spanish map names, got %v", got["language"])
	}
	if got["worldSize"] != 10000.0 {
		t.Fatalf("the rest of the settings must be kept, got %v", got["worldSize"])
	}
	if base["language"] != "en" {
		t.Fatal("the caller's settings must not be mutated")
	}
	if withCampaignLanguage(base, "English")["language"] != "en" {
		t.Fatal("an English game must generate English map names")
	}
}

func TestVellarisLanguageCodeMapsSessionLanguages(t *testing.T) {
	cases := map[string]string{
		"Spanish":    "es",
		"spanish":    "es",
		"es":         "es",
		"English":    "en",
		"French":     "fr",
		"German":     "de",
		"Portuguese": "pt",
		"Russian":    "ru",
		"Chinese":    "zh",
		"":           "en",
		"Klingon":    "en",
	}
	for language, want := range cases {
		if got := vellarisLanguageCode(language); got != want {
			t.Fatalf("vellarisLanguageCode(%q) = %q, want %q", language, got, want)
		}
	}
}

func TestResolveCampaignLanguageFallsBackToTheRequestWithoutAStoredGame(t *testing.T) {
	if got := resolveCampaignLanguage(context.Background(), nil, "", "Spanish"); got != "Spanish" {
		t.Fatalf("requested language should be used when nothing is stored, got %q", got)
	}
	if got := resolveCampaignLanguage(context.Background(), nil, "", ""); got != "English" {
		t.Fatalf("an empty request must default to English, got %q", got)
	}
}

func TestReconcileDirectiveNamesRewritesOldPlaceNames(t *testing.T) {
	stored := []interface{}{
		map[string]interface{}{"name": "Eldermist Keep"},
		map[string]interface{}{"name": "Templo del Viento Oscuro"},
	}
	aligned := []interface{}{
		map[string]interface{}{"name": "Aldea de Alequettenie"},
		map[string]interface{}{"name": "Aldea de Dius"},
	}
	text := "At eldermist keep the guard waits; later, the templo del viento oscuro burns."
	got := reconcileDirectiveNames(text, stored, aligned)
	if strings.Contains(strings.ToLower(got), "eldermist") || strings.Contains(strings.ToLower(got), "viento oscuro") {
		t.Fatalf("old names must be rewritten, got %q", got)
	}
	for _, want := range []string{"Aldea de Alequettenie", "Aldea de Dius"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewritten directive is missing %q: %q", want, got)
		}
	}
}

// The download has to look like the map service's own answer so the file can be posted straight back to import-world.
func TestBuildWorldJSONMatchesTheMapServiceAnswer(t *testing.T) {
	spec := map[string]interface{}{
		"vellarisWorld": map[string]interface{}{
			"regions": []interface{}{map[string]interface{}{"id": "0", "name": "Cae"}},
			"places":  []interface{}{map[string]interface{}{"name": "Aldea de Dius", "type": "City"}},
		},
		"vellarisSettings": map[string]interface{}{"randomSeed": 1.0, "language": "es"},
	}
	got := buildWorldJSON(spec)
	if got == nil || got["success"] != true {
		t.Fatalf("unexpected payload: %v", got)
	}
	if got["world"] == nil || got["settings"] == nil {
		t.Fatalf("world and settings must both be present: %v", got)
	}
	if buildWorldJSON(map[string]interface{}{}) != nil {
		t.Fatal("a game with no world has nothing to download")
	}
	if buildWorldJSON(nil) != nil {
		t.Fatal("nil spec must not produce a payload")
	}
}

func TestCampaignWorldPromptDataIsEmptyWithoutAWorld(t *testing.T) {
	data := campaignWorldTemplateData(nil)
	if data["hasVellarisWorld"] != false {
		t.Fatalf("no world must not be announced, got %v", data["hasVellarisWorld"])
	}
	if renderCampaignWorldContext(nil) != "" {
		t.Fatal("no world must render no block")
	}
}
