const axios = require('axios');

const DEFAULT_MODEL = process.env.GM_OPENAI_MODEL || 'gpt-3.5-turbo';
const USE_LM_STUDIO = String(process.env.GM_USE_LM_STUDIO || 'false').toLowerCase() === 'true';
// LM Studio commonly listens on :1234 in logs; default to that
const LM_STUDIO_URL = process.env.GM_LM_STUDIO_URL || 'http://localhost:1234';
const LM_STUDIO_MODEL = process.env.GM_LM_STUDIO_MODEL || process.env.GM_OPENAI_MODEL || 'gpt-3.5-turbo';

/**
 * generateResponse accepts either:
 *  - { messages: [...] } (array of chat messages), or
 *  - { prompt: "string" } (simple prompt)
 *
 * Supports two backends:
 *  - LM Studio (local) when USE_LM_STUDIO=true
 *  - OpenAI REST API otherwise
 */
async function generateResponse(input = {}, options = {}) {
  const model = process.env.GM_OPENAI_MODEL || DEFAULT_MODEL;
  const messages = Array.isArray(input.messages)
    ? input.messages
    : [{ role: 'user', content: input.prompt || '' }];
  const { gameId = null } = options;

  // Helper: persist fallback/diagnostic info to GameState when gameId is available
  async function persistDiagnostic(data = {}) {
    if (!gameId) return;
    try {
      const GameState = require('./models/GameState');
      await GameState.findOneAndUpdate({ gameId }, { $set: data }, { upsert: true });
    } catch (e) {
      console.warn('Failed to persist LLM diagnostic for gameId', gameId, e);
    }
  }

  // Convert chat messages to a single prompt for LM Studio
  const messagesToPrompt = (msgs) =>
    msgs.map((m) => `${m.role.toUpperCase()}: ${m.content}`).join('\n\n');

  if (USE_LM_STUDIO) {
    // Prefer OpenAI-compatible chat completions endpoint that LM Studio exposes
    const base = LM_STUDIO_URL.replace(/\/$/, '');
    const headers = { 'Content-Type': 'application/json' };

    // 1) Try OpenAI-compatible /v1/chat/completions
    try {
      const payload = {
        model: LM_STUDIO_MODEL,
        messages,
        max_tokens: options.max_tokens || 500,
        temperature: options.temperature ?? 1.0,
      };
      const resp = await axios.post(`${base}/v1/chat/completions`, payload, { headers });
      const data = resp.data || {};
      const content = data?.choices?.[0]?.message?.content ?? data?.choices?.[0]?.text ?? null;
      if (content) return content;
      // fallthrough if unexpected shape
    } catch (err) {
      console.error('LM Studio /v1/chat/completions failed:', err?.response?.data ?? err.message ?? err);
      await persistDiagnostic({ llmCallError: String(err?.response?.data ?? err.message ?? err).slice(0, 200000), llmCallFallbackAt: new Date().toISOString() });
    }

    // 2) Try LM Studio native chat endpoint /api/v1/chat
    try {
      // LM Studio native endpoint often expects an `input` string rather than OpenAI-style `messages`.
      const payload = {
        model: LM_STUDIO_MODEL,
        input: messagesToPrompt(messages),
        max_tokens: options.max_tokens || 500,
        temperature: options.temperature ?? 1.0,
      };
      const resp = await axios.post(`${base}/api/v1/chat`, payload, { headers });
      const data = resp.data || {};
      // Common LM Studio shapes: data.output, data.result, data.generated_text, data.choices
      const result =
        data.output ||
        data.result ||
        data.generated_text ||
        (Array.isArray(data.results) && (data.results[0]?.text || data.results[0]?.output)) ||
        (data.choices && data.choices[0]?.text) ||
        (data?.response && data.response?.generated_text) ||
        null;
      if (result) return result;
      // If still nothing, return stringified response for debugging
      return JSON.stringify(data);
    } catch (err) {
      console.error('LM Studio /api/v1/chat failed:', err?.response?.data ?? err.message ?? err);
      await persistDiagnostic({ llmCallError: String(err?.response?.data ?? err.message ?? err).slice(0, 200000), llmCallFallbackAt: new Date().toISOString() });
      return null;
    }
  }

  // Fallback to OpenAI REST API
  const callOpenAI = async (useModel) => {
    const payload = {
      model: useModel,
      messages,
      max_tokens: options.max_tokens || 500,
      temperature: options.temperature ?? 1.0,
    };
    const headers = {
      Authorization: `Bearer ${process.env.GM_OPENAI_API_KEY}`,
      'Content-Type': 'application/json',
    };
    const resp = await axios.post('https://api.openai.com/v1/chat/completions', payload, { headers });
    return resp.data?.choices?.[0]?.message?.content ?? null;
  };

  try {
    return await callOpenAI(model);
  } catch (err) {
    console.error('Error generating text (primary):', err?.response?.data ?? err.message ?? err);
    await persistDiagnostic({ llmCallError: String(err?.response?.data ?? err.message ?? err).slice(0, 200000), llmCallStartedAt: new Date().toISOString() });
    const code = err?.response?.data?.error?.code || '';
    const msg = String(err?.response?.data?.error?.message || err.message || '').toLowerCase();

    // If model unavailable or insufficient quota, try fallback to gpt-3.5-turbo
    if ((code === 'model_not_found' || code === 'insufficient_quota' || /model not found|insufficient_quota|quota/i.test(msg)) && model !== 'gpt-3.5-turbo') {
      try {
        // record that we're attempting a fallback
        await persistDiagnostic({ llmFallbackModel: 'gpt-3.5-turbo', llmFallbackAttemptedAt: new Date().toISOString() });
        const fallbackResp = await callOpenAI('gpt-3.5-turbo');
        if (fallbackResp) {
          await persistDiagnostic({ llmFallbackSucceededAt: new Date().toISOString(), llmModelUsed: 'gpt-3.5-turbo' });
        } else {
          await persistDiagnostic({ llmFallbackSucceededAt: null });
        }
        return fallbackResp;
      } catch (e2) {
        console.error('Fallback to gpt-3.5-turbo failed:', e2?.response?.data ?? e2.message ?? e2);
        await persistDiagnostic({ llmFallbackError: String(e2?.response?.data ?? e2.message ?? e2).slice(0, 200000) });
      }
    }
    return null;
  }
}

module.exports = { generateResponse };
