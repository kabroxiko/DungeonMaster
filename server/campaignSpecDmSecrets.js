/**
 * DM-only campaign fields: kept in DB and injected into server-side prompts, never sent to the client.
 */
const DM_ONLY_CAMPAIGN_KEYS = ['dmHiddenAdventureObjective'];

function redactCampaignSpecForClient(spec) {
  if (!spec || typeof spec !== 'object') return spec;
  const out = { ...spec };
  for (const k of DM_ONLY_CAMPAIGN_KEYS) {
    if (Object.prototype.hasOwnProperty.call(out, k)) delete out[k];
  }
  return out;
}

/**
 * When the client persists a campaignSpec snapshot (e.g. bootstrap), preserve DM-only keys already stored server-side.
 */
function mergeCampaignSpecPreservingDmSecrets(existingSpec, incomingSpec) {
  if (!incomingSpec || typeof incomingSpec !== 'object') return incomingSpec;
  const next = { ...incomingSpec };
  for (const k of DM_ONLY_CAMPAIGN_KEYS) {
    const prev = existingSpec && existingSpec[k];
    const inc = incomingSpec[k];
    const prevOk = typeof prev === 'string' && prev.trim();
    const incOk = typeof inc === 'string' && inc.trim();
    if (prevOk && !incOk) {
      next[k] = prev;
    }
  }
  return next;
}

module.exports = {
  DM_ONLY_CAMPAIGN_KEYS,
  redactCampaignSpecForClient,
  mergeCampaignSpecPreservingDmSecrets,
};
