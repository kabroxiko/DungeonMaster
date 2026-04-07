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

## WebSocket

- Upgrade: `GET /api/game-state/ws/:gameId?access_token=<jwt>`

## Client references

- [`client/dungeonmaster/src/store.js`](../client/dungeonmaster/src/store.js) — auth
- [`client/dungeonmaster/src/utils/apiBase.js`](../client/dungeonmaster/src/utils/apiBase.js) — SSE URL
- [`client/dungeonmaster/src/utils/fetchGameStateLoad.js`](../client/dungeonmaster/src/utils/fetchGameStateLoad.js) — load
- [`client/dungeonmaster/src/components/ChatRoom.vue`](../client/dungeonmaster/src/components/ChatRoom.vue) — generate, invites, append
- [`client/dungeonmaster/src/components/SetupForm.vue`](../client/dungeonmaster/src/components/SetupForm.vue) — campaign, character, bootstrap

## Environment (see [`env.example`](../env.example))

- `DM_MONGODB_URI`, `DM_JWT_SECRET`, `DM_FRONTEND_URL`, `DM_PUBLIC_URL`, `DM_GOOGLE_CLIENT_ID`, `DM_OPENAI_MODEL`, `DM_USE_LM_STUDIO`, `DM_LM_STUDIO_URL`, `PORT`, `DM_BIND_HOST`, `DM_TRUST_PROXY`, etc.
