/**
 * Prompt trace for AI / LM Studio logs: which conceptual stack was sent (not filenames).
 * Implemented as a separate, short first `system` message so log UIs that truncate long
 * `content` still show the full trace on `messages[0]`. The real instructions stay in
 * `messages[1]` (and following) unchanged.
 */
function sanitizeTrace(s) {
  return String(s || '')
    .replace(/\-\->/g, '')
    .replace(/<\!--/g, '')
    .replace(/\n/g, ' ')
    .trim();
}

const TRACE_LEADER = '[DM prompt stack — log ID only; next system message is the real instructions] ';

/**
 * @param {Array<{role:string,content?:string}>} messages
 * @param {string} conceptualDescription
 * @returns {Array<{role:string,content?:string}>}
 */
function withPromptTrace(messages, conceptualDescription) {
  if (!conceptualDescription || !Array.isArray(messages) || messages.length === 0) {
    return messages;
  }
  const safe = sanitizeTrace(conceptualDescription);
  if (!safe) return messages;
  const traceBody = TRACE_LEADER + safe;
  const copy = messages.map((m) => ({ ...m }));
  copy.unshift({ role: 'system', content: traceBody });
  return copy;
}

/** When false, no extra system message (set DM_PROMPT_DEBUG=0 to disable). */
function isPromptTraceEnabled() {
  const v = process.env.DM_PROMPT_DEBUG;
  if (v === '0' || v === 'false' || v === 'off') return false;
  return true;
}

/** Use before persist + generateResponse so outbound payload and DB match. */
function traceMessages(messages, conceptualDescription) {
  if (!conceptualDescription || !isPromptTraceEnabled() || !Array.isArray(messages)) {
    return messages;
  }
  return withPromptTrace(messages, conceptualDescription);
}

module.exports = { withPromptTrace, isPromptTraceEnabled, sanitizeTrace, traceMessages };
