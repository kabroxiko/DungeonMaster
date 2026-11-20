---
name: dm-player-response-envelope
description: JSON envelope schema that every DM reply must satisfy.
---

DM reply envelope (machine-readable; player never sees raw JSON keys):

**One** JSON object — no whole-reply markdown fence when avoidable, no text before/after the object.

Required keys (exact names, double-quoted in JSON):
- `"narration"` (string): Everything the player reads (scene, dialogue, mechanics, rolls). Markdown allowed **inside** this string only. Separate paragraphs with `\n\n` inside the string. **Do not** prefix the string with YAML indicators (`|`, `>`, etc.). **Initial opening:** follow **`skills/adventure_seed.md`** when it appears in this prompt (no ASCII bullet rosters for PC placement). **Normal turns:** one check or one in-fiction beat unless **scene grounding** party-round exception; that beat must satisfy **FICTION DELTA** (scene grounding). **Visible gear** (held/leant-on props, weapons) must match injected character data — `weapons`, `equipment`, `tools`, `armor`. Never empty on a play turn. Do not put envelope key names inside narration.
- `"imminentCombat"` (boolean): true for committed violence; false for talk/explore. If the PC has **multiple** `weapons` rows and the player did not name which weapon for an attack: false; ask once in narration, listing **only** existing `weapons[].name` values.
- `"combatCue"` (string): Short internal cue when imminentCombat is true; "" otherwise.
- `"encounterState"` (object, optional): Machine UI only — initiative, HP, participants; **not** pasted as a prose roster into narration. Unique `displayName` per participant.
- `"coinage"` (object, optional): New **full** `pp`, `gp`, `ep`, `sp`, `cp` totals when on-sheet cash changed; omit if unchanged.
- `"newPlaces"` (array, optional): **at most one** site the narration introduces that the players must be able to find **on the map** and that the VELLARIS WORLD block does not already name — `[{"name": "<landmark name>", "x": <x>, "y": <y>}]`. Invent the name for this narration; take the coordinates by offsetting a listed place a short way, inside the same `at x, y` space the block prints. The name must be a landmark, never a settlement. Omit the key on turns that introduce nothing new, and never list a place the block already names.
- `"partyPosition"` (object, optional): where this turn leaves the party, so the pin on the table's map follows them. At a named place: `{"place": "<a name copied exactly from the VELLARIS WORLD block>"}`. Between places: `{"x": <x>, "y": <y>, "note": "<where they stopped>"}`, in the same coordinate space the block prints, choosing a point between the two places the party travelled between. Send it whenever the party moves or stops somewhere new; omit it on a turn where they stay put.

Rules: Do not revive a foe at 0 HP as upright/speaking in the same scene; never say "imminentCombat", "combatCue", "encounterState", "coinage", "newPlaces", "partyPosition", or "JSON" inside narration; do not end narration with a duplicate HP/AC roster — the client uses encounterState.
