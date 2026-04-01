const GameState = require('./models/GameState');
const { mergeCampaignSpecPreservingDmSecrets } = require('./campaignSpecDmSecrets');
const { normalizeCoinageObject, ensurePlayerCharacterSheetDefaults } = require('./validatePlayerCharacter');

/**
 * Upsert full play snapshot (conversation, setup, counters, encounter). Used only from server routes — not exposed as /save.
 */
async function persistGameStateFromBody(body) {
  const gameId = body && body.gameId;
  if (!gameId) {
    throw new Error('persistGameStateFromBody: gameId is required');
  }

  let gameSetup = body.gameSetup || {};
  if (gameSetup.generatedCharacter && typeof gameSetup.generatedCharacter === 'object') {
    const lang = gameSetup.language || 'English';
    gameSetup = {
      ...gameSetup,
      generatedCharacter: ensurePlayerCharacterSheetDefaults(gameSetup.generatedCharacter, { language: lang }),
    };
  }

  const update = {
    gameId,
    gameSetup,
    conversation: body.conversation,
    summaryConversation: body.summaryConversation,
    summary: body.summary,
    totalTokenCount: body.totalTokenCount,
    userAndAssistantMessageCount: body.userAndAssistantMessageCount,
    systemMessageContentDM: body.systemMessageContentDM,
  };
  if (body.campaignSpec !== undefined) {
    const existing = await GameState.findOne({ gameId }).select('campaignSpec').lean();
    update.campaignSpec = mergeCampaignSpecPreservingDmSecrets(
      existing && existing.campaignSpec,
      body.campaignSpec
    );
  }
  if (body.mode != null && body.mode !== '') {
    update.mode = body.mode;
  }
  if (Object.prototype.hasOwnProperty.call(body, 'encounterState')) {
    update.encounterState = body.encounterState;
  }

  const gameState = await GameState.findOneAndUpdate({ gameId }, update, { new: true, upsert: true });

  try {
    const { maybeTriggerSummaryAfterSave } = require('./summaryWorker');
    setImmediate(() => {
      maybeTriggerSummaryAfterSave(gameId, gameState).catch((err) =>
        console.warn('maybeTriggerSummaryAfterSave error', err)
      );
    });
  } catch (e) {
    console.warn('Failed to schedule conditional summary after persist:', e);
  }

  return gameState;
}

/**
 * Client sends persist snapshot with conversation ending on the latest user (or system-only for opening).
 * Server appends assistant narration and aligns counters / encounter for DB write.
 */
function mergePersistWithAssistantReply(persistBase, envelope, { finalUsedCombatStack = false } = {}) {
  if (!persistBase || typeof persistBase !== 'object') return null;
  const narration = String((envelope && envelope.narration) || '');
  const aiMsg = { role: 'assistant', content: narration };
  const conv = Array.isArray(persistBase.conversation) ? [...persistBase.conversation] : [];
  const sumConv = Array.isArray(persistBase.summaryConversation) ? [...persistBase.summaryConversation] : [];
  conv.push(aiMsg);
  sumConv.push(aiMsg);

  const beforeLast = conv.length >= 2 ? conv[conv.length - 2] : null;
  const countInc = beforeLast && beforeLast.role === 'user' ? 1 : 0;
  const extraNarrationTokens = Math.max(0, Math.ceil(narration.length / 4));

  const encounterState =
    envelope && Object.prototype.hasOwnProperty.call(envelope, 'encounterState')
      ? envelope.encounterState
      : persistBase.encounterState;

  const mode = finalUsedCombatStack ? 'combat' : persistBase.mode;

  let gameSetup = persistBase.gameSetup;
  if (
    envelope &&
    envelope.coinage != null &&
    typeof envelope.coinage === 'object' &&
    !Array.isArray(envelope.coinage)
  ) {
    const gs = { ...(persistBase.gameSetup || {}) };
    const gc = { ...(gs.generatedCharacter || {}) };
    gc.coinage = normalizeCoinageObject(envelope.coinage);
    gs.generatedCharacter = gc;
    gameSetup = gs;
  }

  return {
    ...persistBase,
    gameId: persistBase.gameId,
    conversation: conv,
    summaryConversation: sumConv,
    encounterState,
    mode,
    gameSetup,
    userAndAssistantMessageCount: (persistBase.userAndAssistantMessageCount || 0) + countInc,
    totalTokenCount: (persistBase.totalTokenCount || 0) + extraNarrationTokens,
  };
}

module.exports = {
  persistGameStateFromBody,
  mergePersistWithAssistantReply,
};
