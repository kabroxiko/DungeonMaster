---
name: npc-turn
description: DM-only decision for one creature's own turn; the server rolls it.
---

DECIDING ONE CREATURE'S TURN (DM-only; the table never sees this).

It is {{actor}}'s turn, and only that creature acts now. Decide what it does knowing what has already happened in this
fight: a creature that just saw its ally fall does not fight as if nothing happened, and one that is badly hurt looks
for a way out before it looks for a throat.

Answer with one JSON object and nothing else:

{"actions":[{"actor":"<the creature whose turn it is>","target":"<who the action lands on, or empty>","kind":"attack|heal|save|check","modifier":0,"damage":"<dice notation, or empty>","dc":0}]}

- Exactly one entry, for the creature whose turn it is. Nobody else acts in this answer.
- The target is a participant the record has standing. A creature never targets itself, and never targets someone the
  record has already put down.
- `modifier` is the whole number added to the d20. `damage` is the creature's own damage, as the record writes it.
- `kind` is `attack` for a weapon attack, `heal` for something that mends, `save` or `check` for a throw with no target.
- A creature that can reach a standing enemy attacks it. `check` is for what the creature is trying to accomplish that
  is not a blow — forcing a door, keeping its footing, reading the room — never a way to spend a turn of a fight without
  risking anything. A creature that truly cannot reach anyone may still act, and then its `check` carries the difficulty
  in `dc`.

Never write a die result, a total or a hit or miss: the server rolls and applies it, and the narration is a separate
step. Never decide for the player's character.
