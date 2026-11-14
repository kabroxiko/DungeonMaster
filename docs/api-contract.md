# HTTP API contract (GameMasterAI backend)

Vue SPA calls these JSON endpoints under `/api`. The backend is **gmai-server** (Go).

## Meta

| Method | Path | Auth | Notes |
|--------|------|------|--------|
| GET | `/api/meta` | No | `{ api, deployMarker, googleClientId }`; `googleClientId` is the public OAuth Web client ID from `DM_GOOGLE_CLIENT_ID` |

## Auth (`/api/auth`)

| Method | Path | Auth | Body / query | Response |
|--------|------|------|--------------|----------|
| POST | `/api/auth/google` | No | `{ idToken }` | `{ token, user: { _id, picture, nickname } }` |
| POST | `/api/auth/join` | Bearer | `{ inviteToken }` | `{ gameId, alreadyMember? }` |
| PATCH | `/api/auth/nickname` | Bearer | `{ nickname }` | `{ user }` |
| GET | `/api/auth/me` | Bearer | — | `{ user }` |

## Game state (`/api/game-state`)

| Method | Path | Auth | Notes |
|--------|------|------|--------|
| POST | `/append-player-message` | Bearer | Party round + broadcast |
| GET | `/events/:gameId` | Bearer **or** `?access_token=` | SSE |
| GET | `/load/:gameId` | Bearer | Full state (redacted) |
| GET | `/map/:gameId` | Bearer **or** `?access_token=` | Rendered map image (`image/jpeg`) |
| GET | `/map-json/:gameId` | Bearer, owner only | World JSON download (`{ success, world, settings }`, same shape as `POST /api/world`) |
| GET | `/debug/:gameId/prompts` | Bearer | Debug fields |
| POST | `/create-party` | Bearer | 201 + lobby `gameSetup` |
| POST | `/party-ready` | Bearer | |
| PATCH | `/party-premise` | Bearer | Owner only |
| GET | `/mine` | Bearer only (no query token) | Summary rows |
| DELETE | `/mine/:gameId` | Bearer | Owner delete |

## Game session (`/api/game-session`)

| Method | Path | Auth |
|--------|------|------|
| POST | `/generate` | Bearer |
| POST | `/generate-campaign` | Bearer |
| POST | `/generate-campaign-core` | Bearer |
| POST | `/preview-character-name` | Bearer |
| POST | `/generate-character` | Bearer |
| POST | `/start-party-adventure` | Bearer |
| POST | `/bootstrap-session` | Bearer |
| POST | `/create-invite` | Bearer |
| POST | `/import-world` | Bearer |
| POST | `/measure-travel` | Bearer |

### Campaign generation and the map

`generate-campaign`, `generate-campaign-core` and `start-party-adventure` resolve the campaign's world **before**
generating any text:

1. A game that already carries `campaignSpec.vellarisWorld` reuses it.
2. Otherwise the map service (`DM_VELLARIS_URL`) is asked once (`POST /api/world?persist=true`) with map settings from
   the request's `worldSettings` or from `DM_VELLARIS_SETTINGS_FILE`. Request settings keep their seed; settings read
   from the file are given a fresh seed per campaign.
3. When the service keeps maps, its answer carries `mapId`. The game stores `campaignSpec.vellarisMapId`; the service
   holds the canonical settings beside the map, so the game no longer stores a megabyte-sized `vellarisSettings` copy.
   A service with no database answers 503 to `persist`, and the game then keeps `campaignSpec.vellarisSettings` as
   before (the world is still generated, just statelessly).
4. The world is stored on the game, then campaign text is generated from it (regions feed the generator prompt).
5. The player-facing map image is fetched by `mapId` when there is one (it was rendered during generation and is already
   stored), otherwise rendered from the settings, and stored for `GET /api/game-state/map/:gameId`.

Route measurement (`/measure-travel`) and the map JSON download follow the same split: by `mapId` when the game has
one, otherwise by the stored settings. Deleting a game deletes its stored map from the service.

A failed world step fails the campaign (a campaign is never written without its map). A failed image render does not:
the campaign still succeeds and the response carries `mapImageWarning`, with `campaignSpec.mapImageError` on the game.

`GET /api/game-state/map/:gameId` serves the rendered map to any game member. The chat header's map button is
per-browser: each player opens and closes the campaign map on their own screen, so opening it never affects anyone
else's view and no owner permission is involved.

Two renditions are stored for each game: the picture exactly as rendered (2.7 MB for a 4096×2304 world) and a
pre-scaled 2048px copy (about 530 KB) built once by `storeWorldMapImage`. The route answers the preview, and the render
only when the client asks for `?full=1` — which it does as soon as the preview is on screen, and immediately when the
view is zoomed past 1.25, fading it in over the preview. Both carry the version in the URL
(`v=<campaignSpec.mapImage.updatedAt>`), so both are served `Cache-Control: private, max-age=31536000, immutable`. The
client preloads the preview as soon as game state announces a map, so opening the modal paints without a download.

The version is only meaningful on the game that wrote it: the picture is stored per game, and the client keys the URL
on both, so a marker left over from the game a browser was in before never asks this game for that picture. A marker
whose picture is not stored — the two are written in that order, so this is a marker with nothing behind it — is
dropped when the route is asked for it, with a game-state push, so the map control disappears instead of staying
broken and the next world step renders the picture again.

The modal zooms by resizing the picture's box rather than by CSS-scaling it: a scaled layer is rasterised once at the
fitted size, so every zoom level enlarged those pixels and the map looked blocky. With a resized box the browser
samples the stored picture at the size it is drawn at.

### Marking new places on the map

A DM turn may introduce a site the players must be able to find that the world does not already name. The reply's
optional `newPlaces` array (`[{ "name", "x", "y" }]`, at most one entry per turn) carries it in the world's own
resolution-invariant coordinates. After the turn is stored, `ApplyMapPlaces` (`gamesession/map_places.go`) asks the map
service for the mark — `POST /api/maps/{id}/places` when the game has a stored map, `POST /api/world/places` when it
keeps the settings instead — and stores the updated world, the canonical settings (stateless path only) and the
re-rendered image together. `campaignSpec.mapImage.updatedAt` changes, which is the cache key the client uses for the
image URL, and a game-state push makes every open client reload.

The render takes seconds, so the update runs in the background: the narration is never delayed by it. A failure — the
map service is down, the name already exists, the point is open water — is logged and leaves the turn and the existing
map untouched. The added place comes back as a `Landmark`, and `campaignspec.PlaceKind` reports it as `landmark` so it
is never treated as a settlement.

### The party pin

A reply may also carry `partyPosition`, either `{ "place": "<name from the world block>" }` or
`{ "x": .., "y": .., "note": "on the road to …" }` for an encounter between places. `ApplyPartyPosition` resolves the
name against `campaignSpec.vellarisWorld`, projects the point onto the picture, and stores
`campaignSpec.partyMapPosition` as `{ x, y, label, place?, note?, updatedAt }` where `x`/`y` are **fractions of the
image** (0–1, top-left origin). A place's `x`/`y` in the world is where the map draws it — the centre of its pictogram
for a settlement, its label for anything with no drawing of its own — so the pin lands on the place, not beside its
name. No render and no image download is involved: the client draws the dot inside the same layer as the picture it
already shows, at those fractions, so travel and encounters move the pin for free and it cannot drift from the map
while that layer is panned or zoomed.

The pin exists before play starts: when the campaign's opening frame is written, `openingSceneFrame.startPlace` names
the map place the first scene happens in and the server resolves it into `partyMapPosition` straight away. A start
the map does not have — a region, or an invented name — falls through to the campaign's key locations, which are
settlement names, so the party opens on an exact spot and never on a territory.

### Asking the map questions

The map service is a general map: it knows terrain, water and coast, and nothing about the game. Two of its answers
keep the DM off guesswork, and both are stored with the world so the DM has them every turn without a request:

- `POST /api/world` carries `terrain`, a coarse raster of the land (one character per patch about a town wide, capitals
  for land that touches the sea) plus its legend, cell size and origin, and every place and region reports the cell it
  occupies as `gridColumn`/`gridRow`. That is what lets the DM put "the nearest beach" on the map instead of naming the
  nearest town that happens to be on a coast.
- `POST /api/find` (or `POST /api/maps/{id}/find` for a stored map) answers "where is the nearest ground like this"
  from a point or a place name: `terrain` names, `coast`, `river`, `lake` and `water`, with `limit`, `maxDistance` and
  `spacing`. Each match carries coordinates, distance, compass `direction`, terrain, region and the nearest named place,
  and can be handed straight back to `/api/route` for a travel time. `POST /api/nearest` describes the ground at one
  point, including whether it is on the coast at all.

Nothing about these is game-specific: they are terrain, distance and direction, so a world is playable by anything that
can read a map. Vocabulary that belongs to the game — a beach, a harbour, a camp site — is the caller's, mapped onto
`coastal`, `river` and `terrain` rather than asked for as its own endpoint.

The projection lives in `campaignSpec.mapGeometry` (DM-only): `xPerUnit`, `yPerUnit`, `xOffset`, `yOffset`. It is
derived once from the map's settings — `resolution`, `generatedWidth`/`generatedHeight`, and the border padding when
`drawBorder` is on and `borderPosition` is `Outside map` — and then reused, so a game does not refetch the settings each
turn. Because the fractions are relative, they stay correct if the image is ever served downscaled.

`partyMapPosition` is delivered with `/load` and over SSE like the rest of the game state, and it does not count as
campaign content: `campaignspec.mapMarkerKeys` keeps it out of the readiness check.

## WebSocket

- Upgrade: `GET /api/game-state/ws/:gameId?access_token=<jwt>`

## Client references

- [`client/dungeonmaster/src/store.js`](../client/dungeonmaster/src/store.js) — auth
- [`client/dungeonmaster/src/utils/apiBase.js`](../client/dungeonmaster/src/utils/apiBase.js) — SSE URL
- [`client/dungeonmaster/src/utils/fetchGameStateLoad.js`](../client/dungeonmaster/src/utils/fetchGameStateLoad.js) — load
- [`client/dungeonmaster/src/components/ChatRoom.vue`](../client/dungeonmaster/src/components/ChatRoom.vue) — generate, invites, append
- [`client/dungeonmaster/src/components/SetupForm.vue`](../client/dungeonmaster/src/components/SetupForm.vue) — campaign, character, bootstrap

## Environment (see [`env.example`](../env.example))

- `DM_MONGODB_URI`, `DM_JWT_SECRET`, `DM_FRONTEND_URL`, `DM_PUBLIC_URL`, `DM_GOOGLE_CLIENT_ID`, `DM_AI_PROVIDER` (`openai` | `deepseek` | `gemini` | `lmstudio`), `DM_OPENAI_MODEL`, `DM_GEMINI_MODEL`, `DM_LM_STUDIO_URL`, `PORT`, `DM_BIND_HOST`, `DM_TRUST_PROXY`, etc.
