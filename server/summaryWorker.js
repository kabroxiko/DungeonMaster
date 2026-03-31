const { generateResponse } = require('./openai-api');
const GameState = require('./models/GameState');
const { loadPrompt } = require('./promptManager');
const { traceMessages } = require('./promptDebug');

// One in-flight summary per gameId so rapid saves do not stack parallel LLM calls (looks like "AI keeps running").
const summaryLocks = new Set();

function estimateConversationTokens(conversation) {
  if (!Array.isArray(conversation)) return 0;
  let chars = 0;
  for (const m of conversation) {
    if (!m || (m.role !== 'user' && m.role !== 'assistant')) continue;
    chars += String(m.content || '').length;
  }
  return Math.max(1, Math.ceil(chars / 4));
}

/**
 * Decide whether to run a background summary after a save.
 * Uses env-tunable thresholds: adventure advance (exchange delta), client token delta, and full-transcript size vs assumed context.
 *
 * DM_SUMMARY_FIRST_AT_EXCHANGES (default 2) — first summary once this many user+assistant turns exist and there is no summary yet.
 * DM_SUMMARY_EVERY_N_EXCHANGES (default 4) — summarize again after this many new exchanges since last run.
 * DM_SUMMARY_MIN_TOKEN_DELTA (default 1200) — summarize if client totalTokenCount grew by at least this much since last run.
 * DM_SUMMARY_MODEL_CONTEXT (default 4000) — assumed prompt context window for pressure check.
 * DM_SUMMARY_CONTEXT_PRESSURE (default 0.55) — run if estimated transcript tokens >= MODEL_CONTEXT * this (capacity pressure).
 */
function shouldRunSummary(gs) {
  if (!gs || !gs.gameId) return false;

  const count = Number(gs.userAndAssistantMessageCount) || 0;
  const totalTok = Number(gs.totalTokenCount) || 0;
  const hasSummary = !!(gs.summary && String(gs.summary).trim());
  // Missing baselines = treat as “just summarized” (no phantom deltas).
  const lastEx = Number.isFinite(Number(gs.summaryLastRunAtExchangeCount))
    ? Number(gs.summaryLastRunAtExchangeCount)
    : count;
  const lastTok = Number.isFinite(Number(gs.summaryLastRunTokenApprox))
    ? Number(gs.summaryLastRunTokenApprox)
    : totalTok;

  const minFirst = parseInt(process.env.DM_SUMMARY_FIRST_AT_EXCHANGES || '2', 10);
  const everyN = parseInt(process.env.DM_SUMMARY_EVERY_N_EXCHANGES || '4', 10);
  const tokenDelta = parseInt(process.env.DM_SUMMARY_MIN_TOKEN_DELTA || '1200', 10);
  const modelCtx = parseInt(process.env.DM_SUMMARY_MODEL_CONTEXT || '4000', 10);
  const pressure = parseFloat(process.env.DM_SUMMARY_CONTEXT_PRESSURE || '0.55');

  if (!hasSummary && count >= minFirst) return true;

  const deltaEx = count - lastEx;
  if (deltaEx >= everyN) return true;

  const deltaTok = totalTok - lastTok;
  if (deltaTok >= tokenDelta) return true;

  const estConv = estimateConversationTokens(gs.conversation);
  if (estConv >= modelCtx * pressure) return true;

  return false;
}

async function maybeTriggerSummaryAfterSave(gameId, gameState) {
  if (!gameId || !gameState) return;
  if (shouldRunSummary(gameState)) {
    triggerSummaryForGame(gameId);
  }
}

async function generateSummaryForGame(gameId) {
  if (!gameId || summaryLocks.has(gameId)) return null;
  summaryLocks.add(gameId);
  try {
    const gs = await GameState.findOne({ gameId }).select('+conversation +summary +gameSetup');
    if (!gs) return null;

    const language = (gs.gameSetup && gs.gameSetup.language) || 'English';

    // Build inbound messages (exclude system)
    const inbound = (gs.conversation || []).filter(m => m.role !== 'system');

    // Do NOT use composeSystemMessages here: it pulls full DM systemCore (imminentCombat JSON, combat rules, length guard),
    // which conflicts with a short session summary and forces tiny max_tokens → truncation, meta chatter, and duplicate work
    // alongside the main /generate call after each save.
    const langFile = language && language.toLowerCase().startsWith('span') ? 'language_spanish.txt' : 'language_english.txt';
    let langPrompt = '';
    try {
      langPrompt = loadPrompt(langFile) || '';
    } catch (e) {
      /* ignore */
    }

    const summaryInstruction =
      language && language.toLowerCase().startsWith('span')
        ? 'Todo lo siguiente es una transcripción de una partida de rol. Resume los eventos de forma concisa (menos de 75 palabras), objetiva, en tercera persona. Menciona personajes, lugares u objetos importantes si aplica. Responde solo con el resumen en prosa: sin JSON al final, sin metacommentarios, sin prefijos del tipo "Aquí está el resumen".'
        : 'Everything below is a TTRPG session transcript. Summarize events concisely (under 75 words), third person, noting important NPCs, locations, and unresolved threads. Reply with summary prose only: no trailing JSON, no meta-commentary, no "Here is the summary" preface.';

    const systemChunks = [];
    if (langPrompt) systemChunks.push(langPrompt);
    if (gs.summary && String(gs.summary).trim()) {
      systemChunks.push(
        'Existing summary for this session (update or replace to reflect the full transcript; do not merely prepend):\n' +
          String(gs.summary).trim()
      );
    }
    systemChunks.push(summaryInstruction);

    const messagesToSend = [{ role: 'system', content: systemChunks.join('\n\n') }, ...inbound];
    const summaryOutbound = traceMessages(
      messagesToSend,
      'session memory summary; language policy; optional prior summary; full transcript to condense'
    );

    // Reasoning models may consume completion budget before visible text; keep headroom beyond 75 words of output.
    const aiSummary = await generateResponse({ messages: summaryOutbound }, { max_tokens: 400, temperature: 0.5, gameId });
    if (!aiSummary) return null;

    const now = new Date().toISOString();
    const ex = Number(gs.userAndAssistantMessageCount) || 0;
    const tok = Number(gs.totalTokenCount) || 0;
    // Persist summary and baselines so the next run waits for real adventure advance or capacity pressure
    await GameState.findOneAndUpdate(
      { gameId },
      {
        $set: {
          summary: String(aiSummary),
          summaryUpdatedAt: now,
          summaryLastRunAtExchangeCount: ex,
          summaryLastRunTokenApprox: tok,
        },
      },
      { upsert: true }
    );
    return String(aiSummary);
  } catch (e) {
    console.error('summaryWorker: failed to generate summary for', gameId, e);
    return null;
  } finally {
    summaryLocks.delete(gameId);
  }
}

function triggerSummaryForGame(gameId) {
  if (!gameId) return;
  setImmediate(() => generateSummaryForGame(gameId).catch(err => console.warn('triggerSummaryForGame error', err)));
}

module.exports = {
  generateSummaryForGame,
  triggerSummaryForGame,
  shouldRunSummary,
  maybeTriggerSummaryAfterSave,
};

