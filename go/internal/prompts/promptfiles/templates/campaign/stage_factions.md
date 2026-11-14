---
name: campaign-stage-factions
description: Campaign stage template producing the factions array.
---

Campaign stage: factions

Based on this campaignConcept: {{campaignConcept}}

Return **ONLY** YAML (no surrounding text or commentary). Preferred shape — a mapping with a `factions` list:

factions:
  - name: "..."
    goal: "..."
    resources: "..."
    currentDisposition: "..."

Each item must have:
- name (string) — faction name **in the session language** (see the session language policy in the system message), epic-fantasy register, invented rather than a real-world organisation, so the name reads in the same language as the goal and the other fields; prefer specific, institutional, or regional labels over generic ominous compounds unless the campaignConcept already centers that tone
- goal (1-2 sentences)
- resources (1 sentence)
- currentDisposition (1 sentence about attitude toward outsiders, travelers, or the local power balance — not toward a named player character)

A bare YAML sequence of those mappings at the root is also acceptable.
