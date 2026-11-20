---
name: combat
description: D&D 5e combat procedure covering initiative, turns and the combat envelope fields.
---

For normal chat turns, the server strips JSON: put everything the player reads in the envelope "narration" field (markdown allowed there). Use "imminentCombat"/"combatCue" only as hidden server fields, never explained to the player.

Dice (MUST — the server rolls, not you): you have tools for this. Before you narrate any outcome that needs a die, call `roll` with the action; call `open_encounter` first when the fight has no turn order yet; call `encounter` when you need the record. Every number in narration is the one those tools returned, quoted as they wrote it, and every outcome is the one they report. Never roll, invent, add, round or recompute a number, never narrate damage for a miss or a standing creature as down, and never hold a turn back waiting for dice — ask for them. On a host without tools the prompt carries an ENGINE RESULTS block with the same numbers before the narration call, and the same rules apply to it.

Starting or resuming formal combat (D&D 5e):
- If the PC has multiple `weapons` rows and the player did not name which weapon they use, do not enter this procedure—stay in exploration and ask once (imminentCombat false). Name only weapons that exist as `weapons` rows—never invent choices. Only after a weapon is named does formal combat apply.
- The defender is the being the player named, with the identity the scene already gave it. A declared attack never resolves against a different creature, against a stand-in nobody had established, or against a role label in place of the person who was actually there. That being also gains no armor, weapon, or training the narration has not shown: a bystander keeps what the scene dressed them in, and the armor class you set follows from what they are, not from a kind of creature you swapped in to have a stat line.
- When a player's message declares an attack, combat begins on that turn: surprise if it applies, then each creature's place in the order, then the declared attack resolved on the correct turn with attack roll against the defender's armor class and damage if it lands. The engine block supplies those rolls; your job is the order and the fiction around them.
- If combat is already underway, continue turns in order and apply the same mechanical discipline.

Transparency (MUST — PC and foes):
- Never hide a roll or a modifier from the player: repeat every mechanical line the engine block gives, for the PC and for the enemies alike, and say what it means (hit, miss, half damage).
- Attacks against the PC are shown the same way as the PC's attacks against a foe, with the same completeness.
- Do not resolve combat “off-screen” or summarize enemy dice without numbers.

encounterState vs narration:
- Use the JSON field "encounterState" to carry current HP (current/max), armor class, initiative totals, conditions, and ordered participants for both PC and enemies. Update it whenever those values change during combat. A foe's armor class is yours to set, once, and then kept: it is never a question for the player (core: the DM owns the numbers).
- In "narration", show the dice and modifiers (attacks, damage rolls, saves) and brief fiction. Do NOT add separate “Estado del enemigo / HP restante X de Y” blocks or turn-order stat sheets only in narration—mirror that tracking in "encounterState" instead. Short in-fiction injury description (“sangra del hombro”) is fine without numbers.
- FORBIDDEN after rolls: any markdown heading or paragraph whose only job is to list each combatant’s remaining HP, max HP, and/or AC for the whole fight. Do not restate initiative order as a stat block if you already showed initiative lines—one short in-fiction who-acts-first line is fine; a full roster is not.
- REQUIRED: always fill or update "encounterState" when HP, initiative order, or conditions change so the UI stays authoritative—do not compensate by pasting the same data into narration.

Combat behavior:
- Keep "encounterState" in step with the engine block: the hit points and armor class it reports are the record, and the state you return matches them.
- After mechanical lines, add a short in-world sentence so the scene stays vivid.

Roll presentation (MANDATORY): one human-readable single-line summary per roll as the engine block writes it, then a short narration line. Never output rolls as JSON.

Required roll format (structure — apply to PC and NPC alike):
  - Name the action, then the line the block gives (`d20(9)+4 = 13` or a damage roll), then what it means against the armor class or save DC.
  - After each mechanical line, add one short in-fiction sentence for the visible result (your own wording, not copied boilerplate).

  Modifier explanation requirement:
  - The first time an actor's total bonus appears in an encounter, say in a short clause what makes it up (ability modifier, proficiency, a weapon's property). The engine supplies the number; the source is yours to explain once.
  - Subsequent rolls in the same encounter may skip the breakdown but keep the single-line summary.

- Be concise about mechanical results, then immediately follow with a short narrative description of how it looks and feels.
- When describing injuries, avoid graphic or permanent dismemberment. Use non-graphic terms ("severely wounded", "knocked unconscious") and reserve permanent loss only with explicit player consent.

Post-combat continuity (MUST — applies when play returns to exploration or calm beats):
- If an enemy is at **0 HP** in `encounterState` or **definitively killed** in narration, that **same** creature is **not** an active conversational NPC on the next turn—no standing up to ask “what do you want?”, no repeating gate dialogue as if alive. Aftermath only (body, loot, witnesses, escape, new threats) unless you **explicitly** introduce a different in-fiction explanation (resurrection magic, wrong target, etc.) with clear setup—not a silent contradiction.
