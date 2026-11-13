---
name: character-generation
description: Character sheet generation rules with a system section and a rendered user-prompt section.
---

{{{languageInstruction}}}
Character generation rules:
- Generate a detailed player character using D&D 5e conventions.
- Use provided fields when present (Race, Class, Level, Background). For any missing field, choose a sensible random value that fits a classic fantasy setting.
- **LOCKED choices (mandatory — the server overwrites mistakes, but you must still match):** The user message includes JSON `gameSetup` with `race`, `class`, optional `subrace`, optional `subclass` (string ids from the setup). When a key is present and its value is **not** the literal `random`, you **must** use **exactly** that D&D 5e PHB option for the sheet: same race, same subrace if specified, same class, same subclass if specified. **Do not** substitute a different race, subrace, class, or subclass. Human-readable spellings in the session language must describe **that** id only. If `subrace` or `subclass` is absent or `random`, you may choose a legal PHB option; if present and fixed, output must match it.
- Strong instruction: the generated playerCharacter.level MUST be 1. Ensure the JSON includes "level": 1 for the playerCharacter.
- Narrative constraint: **`brief_backstory` = their life so far (a life history), not a single short story or one-scene hook.** Write **~150–400 words** in **one or two** paragraphs, past tense, tracing **childhood or origin → formative years → how they learned what they know → why the chosen PHB background fits in fiction** — so the reader understands **who they were before** they set out. A later episode may appear **inside** that arc, but the whole text must not collapse into a single isolated scene with no prior upbringing, kin, or community. It must read as **the story of their life** up to the start of play, not only a brief episode. Past facts and flavor only; MUST NOT include present objectives, goals, or directives about what they will do next. Goals and decisions belong to the player during play and must not be baked into the background.
- **Diegetic / game-agnostic prose (MANDATORY for `brief_backstory`):** The backstory is **in-world history**, not a player-facing blurb. **Do not** mention **character level**, **level 1 / first level**, **hit points**, **stats**, **saving throws**, **proficiency**, **the Player’s Handbook**, **D&D**, **PHB**, **character sheet**, **class** as a game term, **background** as a game term, **player**, **campaign**, or other **table / rules / app** language. Prose belongs to the setting: people, places, training, faith, trade, and loss; not rules explanations. The YAML still carries `level`, `class`, and `background` keys elsewhere; those are data, not narration.

Background (PHB) — must match the story:
- **`background` display string = localized PHB title:** The JSON value for **`background`** is what the **player reads on the character sheet**. It MUST be the **PHB background name in the session language** (see **languageInstruction**). **Spanish sessions:** Spanish PHB-style titles only — never English PHB titles. **English sessions:** English PHB titles only.
- **`background` and `brief_backstory` are one path:** Pick a **legal PHB background** whose **identity and past** are exactly what the backstory describes (same profession, community, training, or social role—not a random label). Choose the PHB background title in the session language that matches the life described; do not pick a military-themed title unless the narrative clearly supports formal soldiery or comparable service.
- **Workflow:** Draft **`brief_backstory`** as that **life history** (rich prose, past facts only), then set **`background`** to the **localized** PHB title that matches; adjust wording if needed until the title and the story unmistakably describe the same person.

Structured gear (MUST — use these exact keys; do NOT use a single flat "starting_equipment" list):
- **Prompt-only classification (no server repair):** The **server does not** move lines between arrays or repair mistakes. You must output **`armor`**, **`weapons`**, **`ammunition`**, **`tools`**, and **`equipment`** in the correct shape.

**Gear pipeline — follow this order; do not dump the whole PHB pack into `equipment`.**
1. **Merge sources (mental):** one legal class starting package at level 1 + full background + race/subclass items explicitly listed in PHB text at level 1. Rolled gold: buy the same categories the package would have included.
2. **`weapons` first:** Every item the PC uses to make a **`weapons`** attack row goes **only** here — melee, ranged, unarmed if you include it. **Nothing that belongs in `weapons` may appear in `equipment`.**
3. **`armor` second:** Worn body armor and shield only (strings or objects per rules below). **No** weapons.
4. **`ammunition` third:** If a row in **`weapons`** needs ammo, put **quantity lines** here; else `[]`. **No** ammo counts in `equipment` or `weapons`.
5. **`tools` fourth:** PHB **Tools**-chapter entries only (kits, thieves’ tools, artisan’s tools with trade named, granted instruments, etc.). **Not** in `equipment`.
6. **`equipment` last — residual list only:** Build it **after** the four arrays above. It may contain **only**: (a) leftover **general gear** from the merged package that is **not** a weapon, **not** a Tool-chapter item, **not** worn armor, **not** an ammo quantity; (b) **spellbook**, **holy symbol**, **druidic focus**, **component pouch** or **arcane focus**, **formula/spell record** (Artificer), and similar **non-weapon** class gear; (c) **mandatory lines** below. **Treat `equipment` as “everything else + containers,” never as a copy of weapons.**

**Mandatory lines in `equipment` (every character, session language):**
- **Exactly one** line naming **where coins are carried** (purse, belt pouch, coin bag — not optional). The server fills numeric **`coinage`**; this line is required anyway.
- **Exactly one** line for **backpack or a PHB pack** that includes one (Explorer’s Pack, Dungeoneer’s Pack, etc., localized). If the merged package names such a pack, that single line satisfies both pack and implied storage where PHB bundles items.
- **If `ammunition` is not `[]`:** **exactly one** additional line for the **projectile carrier** worn or on the belt for shooting (not the backpack alone).

**`armor`:** Worn armor and shield only. Use `[]` when none. **No** `ac_bonus` on objects. Body armor: **`name`** + **`base_ac`** and/or **`ac`** per PHB. Shield: **plain string** with AC contribution in text. **No** weapons.

**`weapons`:** Rows: `{ "name", optional "quantity", "attack_bonus", "damage", optional "ability" }`. **No** `ac_bonus`. **No** ammunition quantity strings here.

**`ammunition`:** PHB-legal ammo for ranged **`weapons`** only; localized. **Every** non-empty entry **must** include an **explicit positive integer count in Arabic digits** in the same string (e.g. `20× longbow arrows`, `Flecha × 20`, `Crossbow bolts (12)`). **No** ammo-only label without a number. Empty when no such weapon.

**`tools`:** PHB Tools-chapter only. `[]` when none.

**Clothes:** If **no** body armor (or only shield), add **one** everyday-garment line in **`equipment`**; outerwear alone is insufficient.

**Class/background lines the server does not inject (still your job):** Wizard → spellbook in **`equipment`**; Artificer → thieves’ tools in **`tools`** + formula record in **`equipment`**; Cleric/Paladin → holy symbol in **`equipment`**; Druid → druidic focus in **`equipment`**; spellcasters with components → component pouch **or** arcane focus in **`equipment`**; Bard → instrument in **`tools`**.

**Final sweep before you output YAML (MUST):**
- For **each** string in **`equipment`**, ask: does it name the **same weapon** as any **`weapons[].name`** (any wording, plural, quantity prefix)? If **yes**, **delete** that `equipment` line.
- Confirm **`equipment`** still has the **coin** line, **pack/backpack** line, and **ammo carrier** line if **`ammunition`** is non-empty.
- Confirm **no** Tool-chapter item remains in **`equipment`**; move to **`tools`**.
- Confirm **no** ammo **quantity** text remains in **`equipment`**; counts belong in **`ammunition`** only.
- For **each** string in **`ammunition`**, confirm it contains a **digit sequence** for the count; if not, **fix** the line before output.

- **languages** (REQUIRED array of strings): **D&D 5e Player’s Handbook** only — list every language this PC **knows at level 1** from **race + class + subclass (if any) + background**, using PHB rules (no extra tongues, no omissions).
  - **Race / subrace (PHB):** Include each language the race and chosen subrace grant per printed PHB rules; apply optional racial language choices only where the rules allow; do not invent extra languages. If `gameSetup` includes **subrace**, use that PHB subrace for traits and languages.
  - **Class (PHB, 1st level):** Add every language the class and subclass **explicitly** grant at level 1, including fixed and chosen picks from the printed lists. Do **not** grant languages from higher levels or from features that are not language grants.
  - **Background:** If the chosen PHB-style background grants languages, include those picks; names must be **PHB-standard** for the pool used above.
  - **Standard pool:** Use only languages from the **printed PHB** for race, class, subclass, and background with correct quantities; **deduplicate** (each entry once).
  - **Localization:** Human-readable language names MUST follow **languageInstruction** for PHB-style naming. No ad‑hoc spellings and **not** real-world national languages as sheet entries unless the table explicitly uses them.
- **Coinage:** Do **not** output **`coinage`** (or **`currency`**). The **server** sets starting coins from the PHB Chapter 4 background pouch that matches **`background`**.

Spellcasting (server-assigned — do **not** output `spells` or `spell_slots`):
- **Spells and spell slots** are **computed on the server** from class, stats, and PHB/SRD-style lists (deterministic random picks). The user message includes **`serverSpellsAuthoritativeJSON`** with the final **`spells`** and **`spell_slots`** for this PC — **do not** emit those keys in your YAML.
- Narration and flavor may still mention spellcasting in general terms; sheet spell names and preparation flags come **only** from the server JSON.

Consistency:
- PHB-style class packages only; no impossible extra weapons. **Incomplete gear** (wrong array, missing mandatory class lines, or missing mandatory **`equipment`** coin/pack/carrier lines) is unacceptable — fix before output.
- **Cross-array (MUST):** **`equipment`** must never repeat **`weapons`**. **Ammunition quantities** only in **`ammunition`**; **carrier** only as the dedicated line in **`equipment`**. Run the **Final sweep** above every time.
- All human-readable strings in playerCharacter (background, brief_backstory, armor, equipment, **ammunition**, tools, weapons[].name, languages, clothing line in equipment, spell names) MUST match languageInstruction.
- **Ability scores, HP, AC, coinage:** Do **not** output **`stats`**, **`max_hp`**, **`ac`**, **`coinage`**, or **`skills`**. The **server** assigns ability scores (**PHB 4d6 drop lowest**, deterministic; highest rolls go to the class’s primary and secondary abilities, then CON, then the rest) + racial modifiers, hit points, armor class, background starting gp, and skill modifiers — not the model.
- **Skills:** Do **not** output a **`skills`** key. The **server** computes PHB skill modifiers from the computed **`stats`**, **`class`**, **`background`**, **`race`/`subrace`**, and level.
- **Spells:** Do **not** output **`spells`** or **`spell_slots`**. The **server** assigns cantrips, known or prepared leveled spells (per class), wizard spellbook **`prepared`** flags, and slot counts.

Do NOT output "starting_equipment" as a flat list. The client renders armor, weapons, **ammunition**, equipment, **tools**, and languages when present; **stats**, **coinage**, **HP/AC**, **skills**, **spells**, and **spell_slots** are computed by the server **before** this call (see JSON above) and merged after your output — do not emit those keys.

YAML structure (MANDATORY — follow before you output):
- **Shape:** One top-level key `playerCharacter` whose value is the full character mapping. No other top-level keys.
- **Indentation:** Use consistent spaces for nesting. Close every mapping and sequence; do not stop mid-document.
- **Self-check:** Mentally validate that your output is parseable YAML. Fix structure until it would succeed.

- Output format: RETURN ONLY valid YAML: one document with top-level key `playerCharacter` whose value includes the fields above. No markdown fences or commentary. The document MUST be **syntactically complete** and MUST NOT be followed by any prose, prefixes, or channel tags.

--- USER PROMPT BELOW (render this section as the user message) ---
{{{languageInstruction}}}
pcDisplayName: {{pcDisplayName}}

Partial character info (JSON, may be empty): {{gameSetup}}

**Server-authoritative stats, coinage, max_hp, ac (JSON — already computed; do not output these keys in your YAML):** {{serverCombatAuthoritativeJSON}}

**Server-authoritative spells and spell_slots (JSON — already computed; do not output these keys in your YAML):** {{serverSpellsAuthoritativeJSON}}

**Ids in that JSON lock character build:** non-`random` `race` / `class` / `subrace` / `subclass` are authoritative — your YAML must reflect those PHB choices only.

Return ONLY valid YAML with top-level key `playerCharacter` whose value is a mapping containing:
- race, subrace (optional when the table chose one), class, subclass (optional), level (1), background, brief_backstory (~150–400 words: **life story**, past only, **diegetic** — no level/sheet/rules jargon; see rules above) — **omit** `stats`, `max_hp`, `ac`, `coinage`, `skills` (server-computed)
- armor (array of strings and/or { "name", optional "base_ac", "ac" } — **no** `ac_bonus`; shields as plain strings per rules above; may be empty)
- equipment (array of strings; **gear pipeline** + mandatory coin + pack + ammo carrier; **zero** weapon names that match **`weapons`**)
- ammunition (array of strings; per **ammunition** rules — **each** entry with a **numeric count** in the text; **`[]`** when no ranged weapon needs ammo; **never** put **quantity** lines in `equipment` or `weapons`)
- tools (array of strings; per **tools** rules and class checklist; use `[]` if none)
- weapons (array of { name, quantity optional, attack_bonus, damage, ability } — **no** `ac_bonus`; may be [] but include attacks when possible — **no ammunition lines** here)
- languages (array of strings; PHB-derived from race, subrace when present, class, subclass, background at level 1 — see rules above)

Do not include "starting_equipment" as a key. **Do not** include **`skills`**, **`stats`**, **`max_hp`**, **`ac`**, **`coinage`**, **`spells`**, or **`spell_slots`** (server-computed). **Include** separate **`equipment`**, **`ammunition`**, and **`tools`** sequences as specified (do not merge tool-type items into `equipment`; do not put ammunition **quantities** in `equipment` — the **carrier** line belongs in **`equipment`**). Do not include any other keys or commentary. Output YAML only. **Close every open structure** (see “YAML structure” above — especially when the last property is `languages`).
