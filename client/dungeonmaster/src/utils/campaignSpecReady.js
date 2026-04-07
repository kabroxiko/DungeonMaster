/**
 * Same rules as `go/internal/campaignspec` (HasSubstantiveCampaignSpec) — keep in sync.
 * @param {unknown} spec
 * @returns {boolean}
 */
export function hasSubstantiveCampaignSpec(spec) {
  if (spec == null || typeof spec !== 'object' || Array.isArray(spec)) return false;
  const skip = new Set(['dmHiddenAdventureObjective']);
  for (const key of Object.keys(spec)) {
    if (skip.has(key)) continue;
    const v = spec[key];
    if (v == null) continue;
    if (typeof v === 'string' && !String(v).trim()) continue;
    if (Array.isArray(v)) {
      if (v.length === 0) continue;
      return true;
    }
    if (typeof v === 'object') {
      if (Object.keys(v).length === 0) continue;
      return true;
    }
    return true;
  }
  return false;
}
