---
name: gamemasterai-automation
description: >-
  Automate common GameMasterAI project tasks: maintain and update prompt
  templates, enforce campaign-first flow (campaign → character → adventure),
  add GM-only injections, update server/client code for prompt/template changes,
  and create follow-up patches that implement requested feature work.
---

# GameMasterAI Automation Skill

## Purpose & When to Use

Use this skill to automate edits and maintenance tasks for the GameMasterAI
project. Typical triggers:

- The user asks to add or modify prompt templates under `server/prompts/`.
- The user asks to change DM behavior in `server/routes/gameSession.js`.
- The user requests new client behavior in `client/gamemasterai/src/...`.
- The user requests new Cursor rules or skills for persistent conventions.
 - The user requests enforcement of campaign-first generation (campaign -> character -> adventure).

Apply this skill when you will:

- Create or update prompt files (text templates used by Mustache).
- Implement server-side logic to load/persist/inject `campaignSpec`.
- Update API endpoints or the client flow to follow the campaign-first order
  (campaign → character → adventure).
- Add tests, logging, or env-configurable limits (token budgets, slice sizes).

## Quick Design Summary (current project state)

The repository currently contains the following notable items and conventions
that this skill must respect and extend:

- Server prompts (editable): `server/prompts/` including:
  - `campaign_generator_prompt.txt` (policy + generator instructions)
  - `json_output_guard.txt`, `no_prefatory_guard.txt` (output guards)
  - `gm_inject_explore.txt`, `gm_inject_combat.txt` (GM injections)
- Server routes: `server/routes/gameSession.js`
  - Endpoints: `/generate`, `/generate-campaign-core`, `/generate-character`
  - Uses Mustache for rendering prompt templates via `loadPrompt(...)`
  - Persists `campaignSpec` and `rawModelOutput` in `GameState`
- Server model: `server/models/GameState.js` has `campaignSpec` and
  `rawModelOutput` fields (GM-only selectors on some)
  - Client: `client/gamemasterai/src/components/SetupForm.vue`
  - Calls `/generate-campaign-core` first, then `/generate-character`
  - Stores `campaignSpec` in initial saved state and uses campaign starter content (campaignConcept, majorConflicts, keyLocations) when composing the opening to compose the DM system message
- Utilities:
  - `extractFirstJsonObject` robust JSON extraction for model output
  - `estimateTokenCount` and dynamic completion budgeting in `gameSession.js`
  - Mustache templating for prompt rendering

## Objectives this Skill Automates

When invoked, perform the following, depending on the user's request:

1. Create or update prompt templates under `server/prompts/` using Mustache
   variables (e.g., `{{#factions}}...{{/factions}}`, `{{languageInstruction}}`).
2. Modify `server/routes/gameSession.js` to:
   - Load `campaignSpec` for a given `gameId`.
   - Prepend rendered GM-only prompt templates to consolidated system messages
     (for modes: `initial`, `exploration`, `combat/decision/investigation`).
   - Enforce campaign-first generation (require `campaignSpec` before
     character generation) or optionally auto-generate campaign if user prefers.
3. Update client flow (`SetupForm.vue`, `ChatRoom.vue`) to:
   - Request campaign core first, persist returned `campaignSpec` to server.
   - Pass `gameId` with subsequent `/generate-character` requests.
  - Use campaign starter content (e.g., `campaignSpec.campaignConcept` or a majorConflict/keyLocation) when composing `systemMessageContentDM`.
4. Add/adjust guards and prompt precedence to ensure JSON-only outputs for
   campaign-core endpoints and avoid prefatory commentary.
5. Persist raw model outputs in `GameState.rawModelOutput` for debugging.
6. Add logging to show consolidated system messages (redact sensitive fields).
7. Add configuration options (env vars) for slice limits and token budgets:
   - e.g., `GM_INJECT_FACTIONS_LIMIT`, `GM_INJECT_HOOKS_LIMIT`, `MODEL_MAX_TOKENS`.

## Implementation Steps (authoring patches)

Follow this process when making edits:

1. Discovery: Re-read the user's instruction and identify target files and
   templates. Confirm if changes should be global (`alwaysApply`) or scoped to
   certain files.
2. Design: Draft the prompt/template text using Mustache. Keep templates short
   (concise, under ~20 lines) and avoid exposing GM-only content to the client.
3. Implementation:
   - Use `ApplyPatch` to add/update prompt files under `server/prompts/`.
   - Use `ApplyPatch` to update `server/routes/gameSession.js`, preferring small,
     well-scoped changes. Always `Read` the target file before patching.
   - Update client files similarly (`client/gamemasterai/src/...`).
4. Verification:
   - Run lint checks (`ReadLints`) on changed server files and fix any lints.
   - Add server-side console logs showing the rendered injection (redacted).
   - Optionally run the dev server to smoke test endpoints.

## Templates & Examples

Example GM injection template (Mustache):

```
GM-ONLY EXPLORATION GUIDANCE (do NOT reveal to players):

Relevant factions (brief):
{{#factions}}
{{/factions}}

```

Example server-side injection snippet (pseudo):

```js
const spec = gs.campaignSpec;
const tpl = loadPrompt('gm_inject_explore.txt');
const injected = Mustache.render(tpl, { factions: spec.factions.slice(0,3) });
systemMsgs.unshift({ role: 'system', content: injected });
```

<!-- removed deprecated snippet referencing hooks -->

## Checklist before finishing a patch

- [ ] Read the target file(s) before editing.
- [ ] Keep patches focused and minimal.
- [ ] Add or update prompt files under `server/prompts/` rather than hardcoding.
- [ ] Update server logging to surface consolidated system messages (redacted).
- [ ] Run `ReadLints` on server files edited.
- [ ] Do not commit secrets or expose GM-only data to clients.
- [ ] Ensure character generation requires `gameId` with `campaignSpec` present (unless user opts to auto-generate campaign).

## Follow-ups & Automation Capabilities

This skill can be extended to:

- Add utility scripts (under `scripts/`) for bulk prompt refactors.
- Create `.cursor/rules/` entries for persistent project conventions (use `create-rule` skill).
- Add unit/integration tests for API endpoints.
- Provide a template for adding new GM injection templates and wire them into routes.

## Safety & Privacy

 - Always treat `campaignSpec` as GM-only. When logging, redact or truncate long fields.
 - Never write secrets into prompt files or skill files.

## Example Triggers (for agent)

- "Add a GM injection template for negotiation scenes and wire it into /generate."
- "Require campaign core before generating characters; return helpful 400 when missing."
- "Merge campaign policy into the campaign generator prompt and remove the old file."

--- End of SKILL.md

