---
name: prompt-writing
description: Rules and audit procedure for the model-facing prompts in go/internal/prompts/promptfiles. Load before adding or editing any prompt, and whenever generation repeats a stock scene, prop, name or phrasing, or refuses something the setting actually allows.
---

# Writing prompts without example poisoning

A prompt is text a model continues. Anything concrete in it — an item list, a sample sentence, a sample JSON value, a
proper name, a coordinate — is text the model may reuse as if it were content.

## The three failures this prevents

1. **Examples become inventories.** Listing the acceptable outcomes of a field turns them into the checklist that every
   generation must satisfy. Three permitted kinds quietly become the only three ever produced.
2. **False bans.** Banning a noun also deletes everything else that noun names. The model then refuses scenes the
   setting allows, and the refusal looks like disobedience rather than an over-broad rule.
3. **Quoted bad prose gets imitated.** A fluent negative sample inside instructions is still fluent text in the middle
   of the prompt.

## Rules

1. **State the bound, not the catalogue.** Phrase the limit as a property that decides membership, so the model can
   judge a case nobody listed.
2. **Never enumerate acceptable outputs.** "May use X, Y or Z" reads as a checklist of what each generation contains.
3. **Ban a word only when the word is the leak.** Before banning a noun, ask what else in this game that noun names; if
   it also names something allowed, ban the mechanism instead of the word.
4. **No sample values in schemas.** Payload and YAML examples use placeholders only; real names, coordinates or numbers
   taken from the data become copied output.
5. **Never quote prose the model might imitate.** Describe the defect abstractly instead of showing a bad example line.
6. **Keep a rule next to the field it governs.** A requirement for one mustache section belongs inside that section.
7. **Duplication is drift.** The same rule copied into several templates gets edited in one place and contradicted in
   another. State the bound once, in the highest-level file that always loads, and let lower files refer to it.

## Audit procedure

1. `rg -n 'e\.g\.|for example|such as|etc\.' go/internal/prompts/promptfiles` — every hit is a candidate.
2. Apply the **copy test** to each candidate: if the model copied that text into its answer, would it be correct, or a
   bug? Lists of permitted things, sample payloads with real values and quoted prose fail; statements of limit pass.
3. `rg -n '\{"' go/internal/prompts/promptfiles/templates` for literal JSON payloads, and read every one for real names,
   real coordinates and plausible-looking numbers.
4. For every banned noun, say what else it names in a medieval D&D game. A collision means the ban must be rewritten
   around the mechanism.
5. Re-run `go test ./internal/prompts/... ./internal/promptmgr/... ./internal/gamesession/...` afterwards: the prompts
   are embedded, and those tests catch frontmatter and prompt-order regressions.

## Do not

- Do not paste lines from this skill into a prompt file; that would create the next catalogue.
- Do not add "illustrative only" samples — nothing inside a prompt is ignored.
- Do not fix a repeated scene by adding another permitted example of a different scene; change the bound.
- Do not add a rule to `templates/dm/*` when it is a world-level bound every turn needs; it belongs in `core/system.md`,
  which is the first system message and outranks stored campaign text.
