/**
 * Same rules as `go/internal/campaignspec` (HasSubstantiveCampaignSpec) — keep in sync.
 * @param {unknown} spec
 * @returns {boolean}
 */
export function hasSubstantiveCampaignSpec(spec) {
  if (spec == null || typeof spec !== 'object' || Array.isArray(spec)) return false;
  // A rendered map is a marker on the game, not a written campaign: `mapImage` / `mapImageError` can be stored by
  // character generation, long before the campaign exists.
  const skip = new Set([
    'dmHiddenAdventureObjective',
    'openingSceneFrame',
    'creativeSeed',
    'vellarisWorld',
    'vellarisSettings',
    'mapImage',
    'mapImageError',
  ]);
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
