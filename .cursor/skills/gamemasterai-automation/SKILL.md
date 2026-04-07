---
name: dungeonmaster-automation
description: >-
  Automate common Dungeon Master project tasks: maintain and update prompt
  templates, enforce lobby-first multiplayer flow (party lobby → character sheets
  + ready → campaign + opening), add DM-only injections, update Go API / client
  code for prompt/template changes, and create follow-up patches that implement requested feature work.
---

# Dungeon Master Automation Skill

## Purpose & When to Use

Use this skill to automate edits and maintenance tasks for the Dungeon Master
project. Typical triggers:

- The user asks to add or modify prompt templates under `go/internal/prompts/promptfiles/`.
- The user asks to change DM behavior in `go/internal/gamesession/` or HTTP handlers in `go/internal/httpserver/`.
- The user requests new client behavior in `client/dungeonmaster/src/...`.
- The user requests new Cursor rules or skills for persistent conventions.
 - The user requests enforcement of lobby-first flow (characters + ready → automated campaign + opening).

Apply this skill when you will:

- Create or update prompt files (text templates used by Mustache).
- Implement server-side logic to load/persist/inject `campaignSpec`.
- Update API endpoints or the client flow to follow the lobby-first order
  (create-party → character per member → ready → `start-party-adventure`).
- Add tests, logging, or env-configurable limits (token budgets, slice sizes).

## Quick Design Summary (current project state)

The repository currently contains the following notable items and conventions
that this skill must respect and extend:

- Prompts (embedded at build): `go/internal/prompts/promptfiles/` grouped as `core/`, `rules/`, `skills/`, `templates/dm/`, `templates/campaign/`.
  - Campaign core: `templates/campaign/generator.txt` (build block + `---` + Mustache user slice); load via `promptmgr.LoadCampaignGeneratorParts()` in Go.
  - Guards: `rules/json_output_guard.txt`, `rules/no_prefatory_guard.txt`, etc.
  - DM injections: `templates/dm/inject_explore.txt`, `templates/dm/inject_combat.txt`, etc.
- API implementation: `go/internal/httpserver/` (routes), `go/internal/gamesession/` (LLM flows)
  - Endpoints: `/generate`, `/generate-campaign-core`, `/generate-character`, `/start-party-adventure`
  - Game state: `POST /api/game-state/create-party`, `POST /api/game-state/party-ready`, `PATCH /api/game-state/party-premise`
  - Uses Mustache for rendering prompt templates via `promptmgr.LoadPrompt(...)`
  - Persists `campaignSpec` and `rawModelOutput` in `GameState`
- MongoDB `GameState` documents have `campaignSpec` and
  `rawModelOutput` fields (DM-only selectors on some)
  - Client: `App.vue` “New game” → `POST /api/game-state/create-party` → `ChatRoom` lobby (premise, ready, character link); `SetupForm` is character-only when joining or when `gameSetup.party.phase` is `lobby`/`starting`.
  - Optional host seed: `gameSetup.party.hostPremise` → Mustache `{{hostPremise}}` in `templates/campaign/generator.txt`.
  - Late join after play: `gameSetup.party.pendingNarrativeIntroductionUserIds` + DM block `templates/dm/party_arrival.txt` (alias `dm_party_arrival.txt`); cleared on next successful `/generate` persist (`go/internal/persist/`).
  - Party context: `templates/dm/party_context.txt` (alias `dm_party_context.txt`).
- Utilities:
  - `parseModelStructuredObject` / `parseCampaignStageModelOutput` (YAML via `yaml` package) for model output
  - `estimateTokenCount` and dynamic completion budgeting in `gameSession.js`
  - Mustache templating for prompt rendering

## Objectives this Skill Automates

When invoked, perform the following, depending on the user's request:

1. Create or update prompt templates under `go/internal/prompts/promptfiles/` using Mustache
   variables (e.g., `{{#factions}}...{{/factions}}`, `{{languageInstruction}}`).
2. Modify `go/internal/gamesession/` (and handlers in `go/internal/httpserver/`) to:
   - Load `campaignSpec` for a given `gameId`.
  - Prepend rendered DM-only prompt templates to consolidated system messages
     (for modes: `initial`, `exploration`, `combat/decision/investigation`).
   - Keep `/generate` initial mode requiring persisted `campaignSpec` (lobby start path runs core + stages server-side before the first opening).
3. Update client flow (`App.vue`, `SetupForm.vue`, `ChatRoom.vue`) to:
   - Prefer `create-party` + lobby UI over host-driven `/generate-campaign-core` from setup.
   - Pass `gameId` with `/generate-character`; use `/setup?joinGame=` for sheet editing from the lobby.
4. Add/adjust guards and prompt precedence to ensure JSON-only outputs for
   campaign-core endpoints and avoid prefatory commentary.
5. Persist raw model outputs in `GameState.rawModelOutput` for debugging.
6. Add logging to show consolidated system messages (redact sensitive fields).
7. Add configuration options (env vars) for slice limits and token budgets:
-- e.g., `DM_INJECT_FACTIONS_LIMIT`, `DM_INJECT_HOOKS_LIMIT`, `MODEL_MAX_TOKENS`.

## Implementation Steps (authoring patches)

Follow this process when making edits:

1. Discovery: Re-read the user's instruction and identify target files and
   templates. Confirm if changes should be global (`alwaysApply`) or scoped to
   certain files.
2. Design: Draft the prompt/template text using Mustache. Keep templates short
   (concise, under ~20 lines) and avoid exposing DM-only content to the client.
3. Implementation:
   - Use `ApplyPatch` to add/update prompt files under `go/internal/prompts/promptfiles/`.
   - Use `ApplyPatch` to update `go/internal/gamesession/` or `go/internal/httpserver/`, preferring small,
     well-scoped changes. Always `Read` the target file before patching.
   - Update client files similarly (`client/dungeonmaster/src/...`).
4. Verification:
   - Run lint checks (`ReadLints`) on changed server files and fix any lints.
   - Add server-side console logs showing the rendered injection (redacted).
   - Optionally run the dev server to smoke test endpoints.

## Templates & Examples

Example DM injection template (Mustache):

```
DM-ONLY EXPLORATION GUIDANCE (do NOT reveal to players):

Relevant factions (brief):
{{#factions}}
{{/factions}}

```

Example server-side injection snippet (pseudo):

```js
const spec = gs.campaignSpec;
const tpl = loadPrompt('templates/dm/inject_explore.txt');
const injected = Mustache.render(tpl, { factions: spec.factions.slice(0,3) });
systemMsgs.unshift({ role: 'system', content: injected });
```

## Checklist before finishing a patch

- [ ] Read the target file(s) before editing.
- [ ] Keep patches focused and minimal.
- [ ] Add or update prompt files under `go/internal/prompts/promptfiles/` rather than hardcoding.
- [ ] Update server logging to surface consolidated system messages (redacted).
- [ ] Run `ReadLints` on server files edited.
- [ ] Do not commit secrets or expose DM-only data to clients.
- [ ] For `/generate-character`, respect current policy: joiners need `gameId` or `newParty`; `campaignSpec` is optional (legacy games / in-progress setup).

## Follow-ups & Automation Capabilities

This skill can be extended to:

- Add utility scripts (under `scripts/`) for bulk prompt refactors.
- Create `.cursor/rules/` entries for persistent project conventions (use `create-rule` skill).
- Add unit/integration tests for API endpoints.
- Provide a template for adding new DM injection templates and wire them into routes.

## Safety & Privacy

- Always treat `campaignSpec` as DM-only. When logging, redact or truncate long fields.
 - Never write secrets into prompt files or skill files.

## Authentication and multi-player

### Investigation protocol (do not guess on join / wrong character / wrong buttons)

When reports say **invitee sees the host’s character**, **solo “generate campaign” instead of Join party**, or **stale sheet after a new invite**:

1. **Inspect the API contract first** — In DevTools (or logs), confirm `GET /api/game-state/load/:gameId` for the invitee’s session:
   - `viewerIsGameOwner` must be **`false`** for non-host members when `ownerUserId` is set on the document.
   - **`gameSetup.generatedCharacter`** is **not part of the API** (API omits it from load; sheets are only under `gameSetup.playerCharacters[userId]`). If a stray field appears, fix `go/internal/gamesession/` / `go/internal/httpserver/` / client `setGameSetup` before changing Vue heuristics.
2. **Inspect persist rules** — `POST /api/game-session/generate-character` writes the sheet under **`gameSetup.playerCharacters[requestingUserId]`** only. `persistGameStateFromBody` strips any `generatedCharacter` on save.
3. **Only then** adjust client `SetupForm.vue` / `ChatRoom.vue` if the payload is already correct.

Add or extend **tests** in `go/...` when changing this behavior (load never exposes `generatedCharacter`).

### Rules summary

- **One party = one `gameId` string**: The host and every invitee use the **same** `GameState.gameId` (the value returned by `POST /api/auth/join` and shown in `/chat-room/:id`). There is no separate “party id” per player. If the client shows a different id (often a `Date.now()` string), **`createNewGame()` was called by mistake** while the store lost the real id — fix recovery from `?joinGame=`, `dm_join_ui_game_id`, or `dm_setup_game_id`, and **never** invent a new id in join context (`SetupForm.vue` `recoverSetupGameIdFromSession` / `isPartyJoinSetupContext`).
- **Server-enforced access**: Every route that accepts a `gameId` must verify the caller is a member (`ownerUserId` or `memberUserIds` on `GameState`). Return **404** for non-members (avoid gameId enumeration). Do not rely on the UI hiding `gameId`.
- **Explicit identity fields**: Pass `userId` (Mongo ObjectId string) and optional `displayName` in API payloads where needed—e.g. user chat lines as `{ role, content, userId?, displayName? }`. Do not infer player identity from free-text system messages (see workspace rule `avoid-fallback-workarounds.mdc`).
- **Multi-client push (SSE)**: `GET /api/game-state/events/:gameId` is a **text/event-stream** (JWT via `?access_token=` because `EventSource` cannot always send `Authorization`). After `persistGameStateFromBody`, invite **join**, or **generate-character** sheet persist, the API emits `game-state-updated`; **`ChatRoom.vue`** (2+ party members) listens and **debounced `GET /load`**. Manual **Sync** and **visibility** debounced sync remain as fallbacks. **Nginx Proxy Manager:** step-by-step + paste-ready **Custom Nginx Configuration** in `env.example` (SSE `location` + AI timeouts). Same ideas apply to plain nginx. Full **WebSocket** is still optional if you need duplex or lower latency.
- **Invite → character → chat**: Invite links are **`/join-party/:opaqueToken`** (public). Unauthenticated visitors get `dm_pending_invite` in `sessionStorage`; after sign-in, `App.vue` sends them to `JoinParty`, which sets `SESSION_CONSUME_INVITE` and `replace`s to `/setup` (no token in the URL). `SetupForm` consumes the session key, calls `POST /api/auth/join` with `{ inviteToken }`, then replaces the URL with `/setup?joinGame=:gameId`, runs join setup, then after confirm the client goes to `ChatRoomWithId`. `POST /api/game-session/generate-character` may merge `existingCampaignWorld` from DB when `campaignSpec` exists. `ChatRoom` redirects members without `playerCharacters[theirId]` to `/setup?joinGame=…` when `memberUserIds.length >= 2`.
- **Multiplayer UI (chat)**: When **2+** party members are known, the chat header shows a **party roster** (character names from `playerCharacters`, “Host”, “you”, pending if no sheet yet) plus a short tip about sync / tab visibility.

### Multiplayer roadmap (optional next steps)

- **Done in repo**: Google auth, `ownerUserId` / `memberUserIds`, invites, join, per-user `playerCharacters`, DM party injection (`templates/dm/party_context.txt`), load omits `generatedCharacter`, **SSE** `GET /api/game-state/events/:gameId`, manual + visibility-based sync, party roster strip in `ChatRoom`.
- **Later**: WebSocket (duplex / presence); **leave party** / owner kick; optional **privacy** (names-only for other PCs on load); “who is typing” if desired.
- **Shared vs per-player data**: `campaignSpec` is shared and DM-only (use existing `redactCampaignSpecForClient`). Per-player sheets live under **`gameSetup.playerCharacters[userId]`** only.
- **Campaign-first**: Unchanged for new games: campaign core still follows a saved character for that user; in multi-player, “character exists” means the **current user** has an entry in `playerCharacters`.
- **Typical touch files** for auth/multi-player work: `go/internal/auth/`, `go/internal/store/`, `go/internal/httpserver/`, `go/internal/gamesession/`, `go/internal/persist/`, `go/internal/prompts/promptfiles/templates/dm/party_context.txt` (party injection), `client/dungeonmaster/src/store.js`, `main.js`, `router.js`, `App.vue`, `ChooseNickname.vue`, `googleSignIn.js`, `LoadGame.vue`, `ChatRoom.vue`, `SetupForm.vue`.
- **Nickname**: New accounts must set `nickname` (`PATCH /api/auth/nickname`); router enforces `/choose-nickname` until set. Optional `/change-nickname` and **Edit name** in the header; `refreshSessionUser` (`GET /api/auth/me`) syncs profile on app load.
- **Docs & checks**: README “Google Sign-In and session JWT”; `env.example` and `client/dungeonmaster/env.example`; run `go test ./...` from `go/` for API tests.

## Example Triggers (for agent)

- "Add a DM injection template for negotiation scenes and wire it into /generate."
- "Require campaign core before generating characters; return helpful 400 when missing."
- "Merge campaign policy into the campaign generator prompt and remove the old file."

--- End of SKILL.md

