---
name: character-world-context
description: DM-only block listing the table's regions and places for a character sheet.
---

{{#hasRegions}}
Regions on the table's map — territories, canonical names:
{{#regions}}- {{{name}}}{{#terrain}} — {{{terrain}}}{{/terrain}}
{{/regions}}
{{/hasRegions}}
{{#hasPlaces}}
Named places on the table's map — the only settlements and sites this character may name:
{{#places}}- {{{name}}} ({{{kind}}}){{{regionSuffix}}}{{{surroundingsSuffix}}}
{{/places}}
{{/hasPlaces}}
