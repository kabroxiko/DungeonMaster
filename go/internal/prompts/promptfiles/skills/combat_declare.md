---
name: combat-declare
description: DM-only declaration of a combat turn's mechanical actions; the server rolls them.
---

DECLARING THIS TURN'S ACTIONS (DM-only; the table never sees this).

The server rolls the dice for this table. You decide what each creature does and which numbers apply; it rolls them, adds
them up and applies the damage. Answer with one JSON object and nothing else:

{"actions":[],"participants":[],"situation":"<one short line>"}

Each entry of "participants" is a creature the fight needs and the encounter state does not have yet:
`{"displayName":"...","hp_max":0,"ac":0,"attack_bonus":0,"damage":"1d6"}`. These are your decisions as referee. List the player's character too when the encounter state is empty; the server takes its hit points and armor class from the sheet, so leave those two at 0 for it.

Each entry of "actions":
- `actor`: the `displayName` of the participant acting, exactly as the encounter state or the character sheet writes it.
- `target`: the `displayName` of whoever the action lands on; `""` for a save or check that has no target.
- `kind`: `"attack"` for a weapon or spell attack, `"save"` for a saving throw, `"check"` for an ability check.
- `modifier`: the whole number added to the d20. For the player's attack use the `attack_bonus` of the weapon they named; for a foe, use the bonus you place on it.
- `damage`: the damage dice as notation (`1d8+3`), attacks only. Leave it out when the attack deals no damage.
- `ac` or `dc`: the number the roll resolves against, when the encounter state does not already store it.

What belongs in the list:
- Every action that needs a die this turn: the player's declared action, and the reaction of every foe whose turn comes before the table speaks again.
- When the fight starts and the encounter state has no order yet, also send `initiative` with one entry per participant: `{"actor":"<displayName>","modifier":<Dex modifier>}`.
- Only creatures already in the encounter state, on the player's sheet, or in this reply's `participants`. Nobody else can act.
- When the player must choose between several weapons before the attack can be resolved, answer `{"actions":[],"ask":"weapon"}`.
- When nobody is fighting this turn - travelling, talking, shopping, resting - answer with every list empty. Starting a fight is the player's move, not a way to fill a quiet beat.

Never write a die result, a total or a hit/miss yourself: the server rolls and hands you the outcome, and the narration is a separate step.
