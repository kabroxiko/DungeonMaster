---
name: dm-world-vellaris
description: Rendered Vellaris world block with regions, named places and measured travel legs.
---

DM-ONLY VELLARIS WORLD — fixed geography for this campaign. Never print this block, its coordinates, or its numbers to players. Follow the VELLARIS WORLD rules above.

World: seed {{{seed}}}, {{{worldSize}}} units across, player map {{{imageWidth}}}x{{{imageHeight}}} pixels{{{mapImageSuffix}}}
{{#hasRegions}}
Regions ({{{regionCount}}}) — territories, not settlements:
{{#regions}}- {{{name}}} (region, id {{{id}}}){{#terrain}}, {{{terrain}}}{{/terrain}} — {{{centerCount}}} cells, centre {{{centerX}}}, {{{centerY}}}
{{/regions}}
{{/hasRegions}}
{{#hasPlaces}}
Named places ({{{placeCount}}}){{{placesNotice}}}:
{{#places}}- {{{name}}} ({{{kind}}}) at {{{x}}}, {{{y}}}{{{gridSuffix}}}{{{regionSuffix}}}{{{surroundingsSuffix}}}
{{/places}}
{{/hasPlaces}}
{{#hasTerrain}}
Terrain grid ({{{terrain.columns}}} columns x {{{terrain.rows}}} rows, about {{{terrain.cellWidth}}} x {{{terrain.cellHeight}}} world units
per cell; row 0 is the north edge, column 0 the west edge):
Legend: {{{terrain.legend}}}
{{{terrain.grid}}}
Reading it: a letter is land, `.` is sea and `~` is a lake. A capital letter is the same ground as its lower-case letter
but on the coast — land that touches the sea — so the nearest capital to a place's cell is the nearest shore to it,
whether or not anything is built there. A place's line ends with its cell as "grid column, row".
{{/hasTerrain}}
{{^hasTerrain}}
No terrain picture came with this world, so its land, water and coastlines are unknown: keep geography vague.
{{/hasTerrain}}
Kind legend: a **settlement** is a built-up place with streets, gates, markets and docks; a **region** is a territory
that contains settlements (say "in the region of X", never "the city of X"); **lake**, **river** and **mountains**
are landforms with no streets or markets. Use each name as the kind it is.

Surroundings after a place describe the ground it stands on: its terrain (forest, hills, plains, marsh, desert,
arctic), the terrain bordering it when another one is close, and whether the coast, a river reaching it ("on a
river") or only passing near it ("near a river"), a lake, mountains or an island are at hand. "On the coast" means
the place itself touches the sea and has docks; "sea within reach, not on it" means the shore is a short walk away
and the place has no harbour, salt air or fishing boats of its own. The same holds of "beside a lake" against
"near a lake". A place "on an island" is within reach of that island's shore; "inland on an island" is not, and has
no harbour. Treat them as map truth alongside the name.

Marking a new site: a landmark the players must be able to find that this block does not name goes in the reply's
`newPlaces`, with coordinates derived from a listed place's `at x, y` (a short offset, so it stays on that land). The
server adds it to the map and to this list for later turns, so mark it once and never repeat the key for the same name.
{{#hasRoutes}}
Travel already measured for this world — use these numbers verbatim when the party makes these journeys:
{{#routes}}- {{{from}}} to {{{to}}}: {{{distance}}} units, about {{{days}}} days on foot{{{terrainSuffix}}}{{{waterSuffix}}}; through {{{regionsText}}}
{{/routes}}
{{/hasRoutes}}
{{^hasRoutes}}
No journeys have been measured yet: describe travel in fiction and keep it vague ("the better part of three days") rather than inventing a distance.
{{/hasRoutes}}
