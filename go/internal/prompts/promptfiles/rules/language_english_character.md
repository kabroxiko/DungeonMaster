---
name: language-english-character
description: English language directive for character-sheet generation only, with no campaign blocks.
---

MANDATORY: RESPOND ONLY IN ENGLISH for ALL human-readable content on the **character sheet** in this response — including brief_backstory, background, race/class/subclass display text, each armor line, each equipment line, each weapons[].name, languages — whether the output is structured YAML or plain text. YAML key names may follow technical conventions; numeric fields in `coinage` (pp, gp, ep, sp, cp) are not localized. **This request outputs only `playerCharacter`** — do not emit `campaignSpec` or other campaign blocks. Keep YAML valid.

**Proper names (`playerCharacter.name` and similar on the sheet):** Use English spelling and epic fantasy register—invented or evocative. Do **not** use real-world city or celebrity names as in-world proper names. Prose in brief_backstory stays natural English.

**`brief_backstory`:** must read as **their life story** in **fully diegetic** prose—origin, upbringing, formation, how they fit their role in the world. **Do not** mention **level**, **level 1**, **character sheet**, **player**, **D&D**, **PHB**, **stats**, **class** or **background** as game terms, or other table jargon. The narrative must include prior life context; it must not be only one isolated scene.

The **`background`** field must use **English PHB background titles** as printed in the Player’s Handbook.

In `languages`, use **only** D&D 5e PHB language names in English. Do not add real-world national languages as sheet entries unless the table explicitly uses them.

**`equipment` vs combat gear:** Do **not** repeat in **`equipment`** any weapon name that already has a row in **`weapons`**. **`equipment`** is **residual** after placing armor, weapons, ammunition counts, and PHB tools. It **must** include **always** a line for **where coins are carried** (pouch, belt purse, etc.). If **`ammunition`** is non-empty, include a **separate** line for the **worn or belt-mounted ammo carrier** used when shooting; a backpack line alone does **not** satisfy that.
