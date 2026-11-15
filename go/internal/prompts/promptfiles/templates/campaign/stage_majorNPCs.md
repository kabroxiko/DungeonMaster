---
name: campaign-stage-major-npcs
description: Campaign stage template producing the major NPCs array.
---

Campaign stage: majorNPCs

Based on this campaignConcept: {{campaignConcept}}

Return **ONLY** YAML (no surrounding text or commentary). Preferred shape:

majorNPCs:
  - name: "..."
    role: "..."
    briefDescription: "..."

Each item must have:
- name (string) — proper name **in the session language** (see the session language policy in the system message), epic or invented, Latin alphabet, so the name reads in the same language as **role** and **briefDescription**. **Every major NPC must have a distinct name** (no duplicates; not the same as another entry ignoring spaces or letter case).
- role (string)
- briefDescription (2 sentences)

**Setting (hard):** pre-industrial medieval fantasy. Roles and descriptions must use medieval means and trades — smith, herbalist, abbot, shipwright, hedge wizard, sergeant, tax-farmer — never technology, science, or a modern profession, and never a device that a medieval world could not have.

A bare YAML sequence at the root is also acceptable.
