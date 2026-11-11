# HTTP API contract (GameMasterAI backend)

Vue SPA calls these JSON endpoints under `/api`. The backend is **gmai-server** (Go).

## Meta

| Method | Path | Auth | Notes |
|--------|------|------|--------|
| GET | `/api/meta` | No | `{ api, deployMarker }` deploy probe |

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
| PATCH | `/map-visibility` | Bearer, owner only | `{ gameId, visible }` — show/hide the map for every player |
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

`gameSetup.mapVisible` is the shared show/hide flag the host toggles through `PATCH /api/game-state/map-visibility`;
every client reads it from `/load` and from SSE push. The push payload
`{ type: "world-map-visibility", gameId, visible }` is delivered immediately (not debounced) and carries the value, so
every client at the table applies the host's choice without waiting for a state reload.

## WebSocket

- Upgrade: `GET /api/game-state/ws/:gameId?access_token=<jwt>`

## Client references

- [`client/dungeonmaster/src/store.js`](../client/dungeonmaster/src/store.js) — auth
- [`client/dungeonmaster/src/utils/apiBase.js`](../client/dungeonmaster/src/utils/apiBase.js) — SSE URL
- [`client/dungeonmaster/src/utils/fetchGameStateLoad.js`](../client/dungeonmaster/src/utils/fetchGameStateLoad.js) — load
- [`client/dungeonmaster/src/components/ChatRoom.vue`](../client/dungeonmaster/src/components/ChatRoom.vue) — generate, invites, append
- [`client/dungeonmaster/src/components/SetupForm.vue`](../client/dungeonmaster/src/components/SetupForm.vue) — campaign, character, bootstrap

## Environment (see [`env.example`](../env.example))

- `DM_MONGODB_URI`, `DM_JWT_SECRET`, `DM_FRONTEND_URL`, `DM_PUBLIC_URL`, `DM_GOOGLE_CLIENT_ID`, `DM_AI_PROVIDER` (`openai` | `deepseek` | `lmstudio`), `DM_OPENAI_MODEL`, `DM_LM_STUDIO_URL`, `PORT`, `DM_BIND_HOST`, `DM_TRUST_PROXY`, etc.
