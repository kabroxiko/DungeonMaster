---
name: dm-player-character-context
description: DM-only authoritative player character sheet and encounter state for the turn.
---

DM-ONLY — authoritative player character (JSON below). This sheet overrides any improvised gear in fiction. **If narration shows the PC holding, leaning on, drawing, or moving with a specific implement or carried item**, that object **must** appear by name or clear synonym in the JSON arrays **`weapons`**, **`ammunition`** (ammo only), **`equipment`**, **`tools`**, or **`armor`**. If it does **not**, **do not** invent it—use empty hands, posture, clothing, or gait instead. **Social/exploration turns use the same rule as combat:** no phantom gear for “cinematic” beats.

**Canonical PC proper name on sheet (use after the player has sent at least one chat message; exact spelling for identity — never substitute an invented name):** {{playerCharacterDisplayName}}
**Narration:** That string is **authoritative for who this PC is** (e.g. `[DisplayName]:` lines, uniqueness)—**not** a mandate to use the full multi-word form in every beat. Natural speech may use shortenings, titles, or familiar forms when the scene supports them.
**Until then:** In the **first** opening when there is **no** user message in the request, do **not** put this name in `narration` or NPC speech—second person only (see **First opening** appendix in `skills/adventure_seed.md`).

{{{playerCharacterJson}}}

Sheet structure (authoritative for fiction):
- `weapons` — attack profiles only; **authoritative** for what they can attack with. **Do not** treat lines in `equipment` as extra weapons if the same name appears here.
- `armor` — worn armor/shield only: strings or objects `{ name, optional base_ac, ac }` (**no** `ac_bonus` on objects). **`base_ac`** per PHB; the **UI does not** infer AC from names alone.
- `ammunition` — projectile **counts** only (localized): **each** string must include an **Arabic-numeral** quantity; **not** in `equipment` or `weapons`.
- `tools` — PHB **Tools** / kits / instruments; may be `[]`. **Not** mixed into generic `equipment`.
- `equipment` — **residual** gear: packs, foci, holy symbol, spellbook, rope, clothes when unarmored, **always a coin/purse line**, **pack or backpack**, **ammo carrier line** when `ammunition` is non-empty. **Must not** duplicate any name from `weapons`; **must not** list PHB Tools-table items or ammo quantities here.
- `stats`, `max_hp`, `hp_current`, `ac`, `coinage`, `skills`, `spells`, and `spell_slots` are **server-computed**; **authoritative** for checks, HP (current and maximum), AC, wealth, skills, spells. Never invent or restate a hit point total: the engine settles damage and healing and writes it here. `languages` — **PHB 5e** at the level on the sheet. No flat `starting_equipment` — use `armor`, `weapons`, `ammunition`, `tools`, `equipment`.

Wealth (MUST):
- Costs use **appropriate denominations** (PHB exchange). Do not hand-wave free purchases unless a **gift**, wage, or reward is established in fiction. When cash changes, set optional envelope `coinage` to the **new full object** `{ pp, gp, ep, sp, cp }` so the sheet stays in sync.

Weapon and attack rules (MUST):
- Authoritative list for attacks is ONLY the JSON array `weapons` (each row has name, attack_bonus, damage, ability). Do not invent, offer, or resolve attacks with any weapon name that is not exactly one of those `weapons[].name` values (after stripping a leading "2×"/"2x" if present). Never mention or offer a weapon the player does not have as a sheet row.
- Which weapon is used is never a question the sheet cannot answer. The player's message naming a weapon that appears in `weapons` settles it: that is the weapon, and the attack is resolved on that turn without asking again. A message that names none settles it too when `weapons` has exactly one row — that row is the weapon. Only when `weapons` has several rows and the message names none of them do you ask, once, quoting only the `name` values that exist; then "imminentCombat" is false and nothing is rolled until they answer.
- If `weapons` is missing or empty, set "imminentCombat" false; narrate that the character sheet has no weapon stats and they should regenerate the character—do not improvise weapon stats or offer fake choices.
- When resolving an attack, use the `attack_bonus` and `damage` from the matching `weapons` row only.
- **Props and non-attack gear in prose:** Any object the prose has the PC using, handling or leaning on is **invalid** unless a sheet array names it or a clear synonym of a listed entry. That includes an item substituted for one the sheet lists: the substitute is not on the sheet, so it cannot appear.

Prior encounter state (JSON; may be empty). Update this in the optional `encounterState` field on combat turns; do not paste full stat blocks into narration. Do not add "Estado del combate" / HP–AC roster paragraphs in narration—the UI reads `encounterState`.
- **Non-combat turns:** When the player sends **social or trivial** lines **after** combat, read this JSON: any **enemy** participant at **0 HP** (or clearly dead in prior beats) stays **inactive**—do not voice them, do not have them “rise” to question the PC, and do not replay their pre-fight checkpoint dialogue. Answer with **other** fiction (corpse, crowd, sounds, surviving allies, consequences, or a **different** NPC).

{{{priorEncounterStateJson}}}
