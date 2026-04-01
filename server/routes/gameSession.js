//GMAI/server/routes/gameSession.js

const express = require('express');
const router = express.Router();
const { generateResponse, getLastGenerateFailureMessage } = require('../openai-api');
const {
  composeSystemMessages,
  loadPrompt,
  detectMode,
  lastUserText,
  userMessageLooksCombat,
  blocksCombatEntryForAmbiguousWeapon,
} = require('../promptManager');
const { traceMessages } = require('../promptDebug');
const Mustache = require('mustache');
const { persistGameStateFromBody, mergePersistWithAssistantReply } = require('../gameStatePersist');
const { normalizeCoinageObject } = require('../validatePlayerCharacter');
const { redactCampaignSpecForClient } = require('../campaignSpecDmSecrets');

const DEFAULT_MODEL = process.env.DM_OPENAI_MODEL || 'gpt-3.5-turbo';

function dmHiddenAdventureObjectiveForPrompt(spec) {
  const s =
    spec && typeof spec.dmHiddenAdventureObjective === 'string' ? spec.dmHiddenAdventureObjective.trim() : '';
  if (s) return s;
  return 'Not set in campaign core: infer one coherent arc from campaignConcept and majorConflicts. When the player acts off that arc, use mundane, realistic outcomes—do not invent a new adventure or conspiracy.';
}

// Note: Output formatting and presentation should be enforced via prompts.

// (Name generation moved to AI: server will not invent character names)

/**
 * Heuristic token estimation: approximate 1 token ≈ 4 characters.
 * Accepts an array of chat messages or a single string.
 */
function estimateTokenCount(input) {
  try {
    let text = '';
    if (Array.isArray(input)) {
      text = input.map(m => (m.content || '')).join('\n');
    } else {
      text = String(input || '');
    }
    // rough heuristic: 1 token ~= 4 chars
    const chars = text.length || 0;
    return Math.max(1, Math.ceil(chars / 4));
  } catch (e) {
    return 100;
  }
}

/**
 * Consolidate an array of system/assistant messages into a single system-role string.
 * Priority: put strong guards (json output, no-prefatory) first to ensure precedence.
 */
function consolidateSystemMessages(msgs = []) {
  try {
    const guardKeys = ['OUTPUT FORMAT RULE', 'NO PREFATORY TEXT', 'NO PREFATORY', 'OUTPUT FORMAT'];
    const guards = [];
    const others = [];
    for (const m of msgs) {
      const content = typeof m.content === 'string' ? m.content : '';
      const isGuard = guardKeys.some(k => content.includes(k));
      if (isGuard) guards.push(content.trim());
      else if (m.role === 'system') others.push(content.trim());
      // skip assistant-role contents to avoid few-shot priming
    }
    // Remove duplicates while preserving order
    const all = [...guards, ...others];
    const seen = new Set();
    const deduped = [];
    for (const s of all) {
      if (!s) continue;
      if (!seen.has(s)) {
        deduped.push(s);
        seen.add(s);
      }
    }
    return deduped.join('\n\n');
  } catch (e) {
    return (Array.isArray(msgs) ? msgs.map(m => m.content || '').join('\n\n') : String(msgs || ''));
  }
}

/** Appended every /generate from disk (geography + no-menu closers). */
function appendSceneGroundingPolicy(consolidated) {
  if (!consolidated || typeof consolidated !== 'string') return consolidated;
  try {
    const g = loadPrompt('dm_scene_grounding.txt');
    if (g && String(g).trim()) return `${consolidated.trim()}\n\n${String(g).trim()}`;
  } catch (e) {
    console.warn('appendSceneGroundingPolicy:', e);
  }
  return consolidated;
}

/**
 * Extract the first top-level JSON object substring from a string by tracking balanced braces.
 * Returns the substring or null if not found.
 */
function extractFirstJsonObject(text) {
  if (!text || typeof text !== 'string') return null;
  const start = text.indexOf('{');
  if (start === -1) return null;
  let depth = 0;
  let inString = false;
  let escape = false;
  for (let i = start; i < text.length; i++) {
    const ch = text[i];
    if (inString) {
      if (escape) {
        escape = false;
        continue;
      }
      if (ch === '\\') {
        escape = true;
        continue;
      }
      if (ch === '"') {
        inString = false;
        continue;
      }
      continue;
    } else {
      if (ch === '"') {
        inString = true;
        continue;
      }
      if (ch === '{') {
        depth++;
        continue;
      }
      if (ch === '}') {
        depth--;
        if (depth === 0) {
          return text.slice(start, i + 1);
        }
      }
    }
  }
  return null;
}

/** Normalize curly/smart quotes so JSON.parse succeeds on model output. */
function normalizeJsonLikeQuotes(s) {
  if (!s || typeof s !== 'string') return s;
  return s
    .replace(/\u201c/g, '"')
    .replace(/\u201d/g, '"')
    .replace(/\u2018/g, "'")
    .replace(/\u2019/g, "'");
}

/** Strip BOM / zero-width chars some APIs prepend. */
function stripBomAndInvisible(s) {
  if (!s || typeof s !== 'string') return s;
  return s.replace(/^\uFEFF/, '').replace(/^[\u200B-\u200D\uFEFF]+/, '');
}

function jsonParseLenientObject(jsonStr) {
  if (!jsonStr || typeof jsonStr !== 'string') return null;
  const tryParse = (t) => {
    try {
      const o = JSON.parse(t);
      return o && typeof o === 'object' && !Array.isArray(o) ? o : null;
    } catch (e) {
      return null;
    }
  };
  let o = tryParse(jsonStr);
  if (o) return o;
  const trimmedTrailingComma = jsonStr.replace(/,\s*\}\s*$/, '}');
  if (trimmedTrailingComma !== jsonStr) {
    o = tryParse(trimmedTrailingComma);
    if (o) return o;
  }
  return null;
}

/**
 * Coerce model "narration" field to a string (some models emit numbers, arrays of paragraphs, or {markdown}).
 * @returns {string|null} null if the key is missing or shape is unusable
 */
function narrationFromEnvelopeField(raw) {
  if (raw === undefined) return null;
  if (raw === null) return '';
  if (typeof raw === 'string') return raw;
  if (typeof raw === 'number' || typeof raw === 'boolean') return String(raw);
  if (Array.isArray(raw)) {
    const parts = raw.filter((x) => typeof x === 'string');
    return parts.length ? parts.join('\n\n') : '';
  }
  if (typeof raw === 'object') {
    if (typeof raw.markdown === 'string') return raw.markdown;
    if (typeof raw.text === 'string') return raw.text;
    if (typeof raw.content === 'string') return raw.content;
  }
  return null;
}

/**
 * When the model starts a JSON envelope but truncates mid-string or omits closing braces, recover narration.
 * Persists dmInitialEnvelopeSalvagedAt for diagnostics (workspace policy).
 */
function salvageTruncatedInitialEnvelope(raw, gameId) {
  const s = stripBomAndInvisible(stripLlmChannelNoise(stripMarkdownJsonFence(String(raw || '')))).trim();
  if (!s || s.length < 12) return null;
  const m = s.match(/"narration"\s*:\s*"/);
  if (!m) return null;
  const start = m.index + m[0].length;
  let i = start;
  let out = '';
  while (i < s.length) {
    const ch = s[i];
    if (ch === '\\' && i + 1 < s.length) {
      const n = s[i + 1];
      if (n === 'n') {
        out += '\n';
        i += 2;
        continue;
      }
      if (n === 't') {
        out += '\t';
        i += 2;
        continue;
      }
      if (n === 'r') {
        out += '\r';
        i += 2;
        continue;
      }
      if (n === '"' || n === '\\' || n === '/') {
        out += n;
        i += 2;
        continue;
      }
      if (n === 'u' && i + 5 < s.length) {
        const hex = s.slice(i + 2, i + 6);
        if (/^[0-9a-fA-F]{4}$/.test(hex)) {
          out += String.fromCharCode(parseInt(hex, 16));
          i += 6;
          continue;
        }
      }
      out += n;
      i += 2;
      continue;
    }
    if (ch === '"') break;
    out += ch;
    i++;
  }
  const narration = out.trim();
  if (narration.length < 8) return null;
  console.warn(
    '[DM] Initial opening: salvaged narration from truncated/partial envelope JSON.',
    JSON.stringify({ gameId: gameId || null, length: narration.length, preview: narration.slice(0, 120) })
  );
  try {
    if (gameId) {
      const GameState = require('../models/GameState');
      GameState.findOneAndUpdate(
        { gameId },
        {
          $set: {
            dmInitialEnvelopeSalvagedAt: new Date().toISOString(),
            dmInitialEnvelopeSalvagedChars: Math.min(narration.length, 500000),
          },
        },
        { upsert: true }
      ).catch((e) => console.warn('Failed to persist dmInitialEnvelopeSalvagedAt:', e));
    }
  } catch (e) {
    /* ignore */
  }
  return {
    narration,
    imminentCombat: false,
    combatCue: '',
    encounterState: null,
  };
}

function takeCampaignFieldItems(field, n) {
  if (!field) return [];
  if (Array.isArray(field)) return field.slice(0, n);
  if (typeof field === 'object') {
    try {
      return Object.values(field).slice(0, n);
    } catch (e) {
      return [];
    }
  }
  return [field].slice(0, n);
}

const STAGE_ALTERNATE_KEYS = {
  factions: ['faction', 'factions_list', 'relevant_factions'],
  majorNPCs: ['major_npcs', 'npcs', 'NPCs', 'majorNpcs'],
  keyLocations: ['locations', 'key_locations', 'places', 'sites', 'keyPlaces', 'lugares_clave'],
};

/** Mongo / some serializers turn arrays into { "0": item, "1": item }. */
function arrayLikeObjectToArray(obj) {
  if (obj == null || typeof obj !== 'object' || Array.isArray(obj)) return null;
  const keys = Object.keys(obj);
  if (keys.length === 0) return null;
  if (!keys.every((k) => /^\d+$/.test(k))) return null;
  return keys
    .sort((a, b) => Number(a) - Number(b))
    .map((k) => obj[k])
    .filter((x) => x != null);
}

function asObjectArray(val) {
  if (val == null) return null;
  if (Array.isArray(val)) return val;
  return arrayLikeObjectToArray(val);
}

/**
 * Coerce a campaign stage LLM payload to a plain array for persistence and dm_inject_*.txt.
 * Models often return { keyLocations: [...] } or use alternate property names; Mongo Mixed can also
 * store odd shapes — this keeps Mustache sections populated.
 */
function coerceCampaignStageToArray(stage, parsed) {
  if (parsed == null) return [];
  let p = parsed;
  if (typeof p === 'string') {
    try {
      p = JSON.parse(p);
    } catch (e) {
      return [];
    }
  }
  if (Array.isArray(p)) return p.filter((x) => x != null);
  if (typeof p !== 'object') return [];

  const topNumeric = arrayLikeObjectToArray(p);
  if (
    topNumeric &&
    topNumeric.length > 0 &&
    typeof topNumeric[0] === 'object' &&
    !Array.isArray(topNumeric[0])
  ) {
    return topNumeric;
  }

  let fromStage = asObjectArray(p[stage]);
  if (fromStage) return fromStage.filter((x) => x != null);

  const alts = STAGE_ALTERNATE_KEYS[stage] || [];
  for (const k of alts) {
    const a = asObjectArray(p[k]);
    if (a) return a.filter((x) => x != null);
  }

  const objectArrays = Object.values(p).filter(
    (v) =>
      Array.isArray(v) &&
      v.length > 0 &&
      v[0] != null &&
      typeof v[0] === 'object' &&
      !Array.isArray(v[0])
  );
  if (objectArrays.length === 1) return objectArrays[0];

  // Single object mistaken for one-element list (wrap) — stage-specific to avoid false positives
  if (objectArrays.length === 0) {
    if (
      stage === 'keyLocations' &&
      typeof p.name === 'string' &&
      (p.type != null || p.significance != null)
    ) {
      return [p];
    }
    if (
      stage === 'factions' &&
      typeof p.name === 'string' &&
      (p.goal != null || p.resources != null || p.currentDisposition != null)
    ) {
      return [p];
    }
    if (
      stage === 'majorNPCs' &&
      typeof p.name === 'string' &&
      (p.role != null || p.briefDescription != null)
    ) {
      return [p];
    }
  }

  return [];
}

/** Ensure campaign core JSON has player-facing `title`; model may omit — synthesize from campaignConcept and log. */
function ensureCampaignCoreTitle(core) {
  if (!core || typeof core !== 'object') return core;
  const raw = core.title;
  if (typeof raw === 'string' && raw.trim()) {
    core.title = raw.trim().slice(0, 200);
    return core;
  }
  const cc = String(core.campaignConcept || '').replace(/\s+/g, ' ').trim();
  let fallback = 'Untitled campaign';
  if (cc) {
    const m = cc.match(/^[^.!?]+[.!?]?/);
    const chunk = (m ? m[0] : cc).trim();
    fallback = chunk.slice(0, 100) || cc.slice(0, 80);
  }
  console.warn('Campaign core JSON missing non-empty title; using fallback.', { fallback: fallback.slice(0, 100) });
  core.title = fallback;
  return core;
}

function stripMarkdownJsonFence(s) {
  if (!s || typeof s !== 'string') return s;
  let t = s.trim();
  if (/^```/.test(t)) {
    t = t.replace(/^```(?:json)?\s*\n?/i, '').replace(/\n?```\s*$/i, '').trim();
  }
  return t;
}

/** Some local models prefix JSON with <|message|> or similar; strip so extractFirstJsonObject finds the envelope. */
function stripLlmChannelNoise(s) {
  if (!s || typeof s !== 'string') return s;
  let t = s.trim();
  const msgIdx = t.search(/<\|message\|\>\s*/i);
  if (msgIdx !== -1) {
    t = t.slice(msgIdx).replace(/^<\|message\|\>\s*/i, '').trim();
  }
  t = t.replace(/^<\|channel\|\>[^\n]*\n?/gi, '').trim();
  return t;
}

/** Player-visible text is envelope.narration; imminentCombat never sent to client. */
function parseDmPlayerEnvelope(raw) {
  if (raw == null) return null;
  let s = stripBomAndInvisible(stripLlmChannelNoise(stripMarkdownJsonFence(String(raw))));
  s = normalizeJsonLikeQuotes(s);
  const extracted = extractFirstJsonObject(s);
  const trimmed = s.trim();
  const jsonStr = extracted || (trimmed.startsWith('{') ? trimmed : null);
  if (!jsonStr) return null;
  const obj = jsonParseLenientObject(jsonStr);
  if (!obj) return null;
  const narration = narrationFromEnvelopeField(obj.narration);
  if (narration === null) return null;
  let encounterState = null;
  if (obj.encounterState != null && typeof obj.encounterState === 'object' && !Array.isArray(obj.encounterState)) {
    encounterState = obj.encounterState;
  }
  const base = {
    narration,
    imminentCombat: Boolean(obj.imminentCombat),
    combatCue: typeof obj.combatCue === 'string' ? obj.combatCue : '',
    encounterState,
  };
  if (obj.coinage != null && typeof obj.coinage === 'object' && !Array.isArray(obj.coinage)) {
    base.coinage = normalizeCoinageObject(obj.coinage);
  }
  return base;
}

/**
 * Some models (especially local) return markdown prose for the opening scene instead of DM envelope JSON.
 * Only stackMode === 'initial': wrap as narration and persist a diagnostic timestamp (see GameState.dmInitialEnvelopeCoercedAt).
 */
function coerceInitialProseToEnvelope(raw, stackMode, gameId) {
  if (stackMode !== 'initial') return null;
  const s = stripBomAndInvisible(stripLlmChannelNoise(stripMarkdownJsonFence(String(raw || '')))).trim();
  if (s.length < 24) return null;
  if (/^\s*[\[{]/.test(s)) return null;
  console.warn(
    '[DM] Initial opening: non-JSON prose coerced into envelope.narration.',
    JSON.stringify({ gameId: gameId || null, length: s.length, preview: s.slice(0, 140) })
  );
  try {
    if (gameId) {
      const GameState = require('../models/GameState');
      GameState.findOneAndUpdate(
        { gameId },
        {
          $set: {
            dmInitialEnvelopeCoercedAt: new Date().toISOString(),
            dmInitialEnvelopeCoercedChars: Math.min(s.length, 500000),
          },
        },
        { upsert: true }
      ).catch((e) => console.warn('Failed to persist dmInitialEnvelopeCoercedAt:', e));
    }
  } catch (e) {
    /* ignore */
  }
  return {
    narration: s,
    imminentCombat: false,
    combatCue: '',
    encounterState: null,
  };
}

/** When the latest user line is clearly an attack, force server-side combat routing even if the model omits imminentCombat. */
function applyUserCombatToEnvelope(envelope, inboundMessages, generatedCharacter) {
  if (!envelope || !Array.isArray(inboundMessages)) return;
  const lu = lastUserText(inboundMessages);
  if (!lu || !userMessageLooksCombat(lu)) return;
  if (blocksCombatEntryForAmbiguousWeapon(lu, generatedCharacter)) return;
  envelope.imminentCombat = true;
  if (!String(envelope.combatCue || '').trim()) envelope.combatCue = 'player attack intent';
}

/** Model sometimes sets imminentCombat too eagerly; keep exploration until the player names a weapon when the sheet has several. */
function enforceAmbiguousWeaponNoCombat(envelope, inboundMessages, generatedCharacter) {
  if (!envelope || !Array.isArray(inboundMessages)) return;
  const lu = lastUserText(inboundMessages);
  if (!lu || !blocksCombatEntryForAmbiguousWeapon(lu, generatedCharacter)) return;
  envelope.imminentCombat = false;
  envelope.combatCue = '';
}

/** DM-only block so every play turn sees authoritative gear and optional prior encounterState. */
function renderPlayerCharacterContextBlock(persistedGameState, language) {
  try {
    const gc = persistedGameState && persistedGameState.gameSetup && persistedGameState.gameSetup.generatedCharacter;
    if (!gc || typeof gc !== 'object') return null;
    const tpl = loadPrompt('dm_player_character_context.txt');
    if (!tpl) return null;
    const langFile = language && String(language).toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
    let languageInstruction = '';
    try {
      languageInstruction = loadPrompt(langFile) || '';
    } catch (e) {
      /* ignore */
    }
    const prior = (persistedGameState && persistedGameState.encounterState) || {};
    return Mustache.render(tpl, {
      languageInstruction,
      language: language || 'English',
      playerCharacterJson: JSON.stringify(gc).slice(0, 120000),
      priorEncounterStateJson: JSON.stringify(prior).slice(0, 32000),
    });
  } catch (e) {
    console.warn('renderPlayerCharacterContextBlock failed:', e);
    return null;
  }
}

// Route to generate AI Dungeon Master and campaign generating responses
router.post('/generate', async (req, res) => {
    // Extract parameters from the request body
    const {
      messages = [],
      mode = 'exploration',
      sessionSummary = '',
      includeFullSkill = false,
      language = 'English',
      gameId = null,
      persist = null,
    } = req.body;

    console.log('AI DM Processing the following messages (mode:', mode, ')');
    console.log(messages);
    // Early instrumentation: record entry to generate route for easier tracing
    try {
      console.log(`ENTER /generate - gameId=${gameId} mode=${mode} messagesCount=${(messages || []).length}`);
      if (gameId) {
        const GameState = require('../models/GameState');
        // note: do not overwrite other fields; just stamp a started timestamp for tracing
        await GameState.findOneAndUpdate({ gameId }, { $set: { llmCallEnteredAt: new Date().toISOString() } }, { upsert: true });
      }
    } catch (e) {
      console.warn('Failed to persist generate-entry instrumentation:', e);
    }

    // Normalize client alias so skillMap and campaign inject branches match.
    let resolvedMode = mode === 'explore' ? 'exploration' : mode;
    // If mode not provided, infer from the conversation.
    if (!resolvedMode || resolvedMode === 'exploration') {
        try {
            const inferred = detectMode(messages);
            if (inferred) resolvedMode = inferred;
        } catch (e) {
            console.warn('Mode detection failed, defaulting to exploration', e);
            resolvedMode = 'exploration';
        }
    }

    // Enforce campaign-first for initial/adventure generation: require an existing campaignSpec
    if (resolvedMode === 'initial') {
      if (!gameId) {
        return res.status(400).json({ error: 'Initial adventure generation requires a gameId with an existing campaign. Generate the campaign core first.' });
      }
      try {
        const GameState = require('../models/GameState');
        const gsCheck = await GameState.findOne({ gameId }).select('+campaignSpec');
        if (!gsCheck || !gsCheck.campaignSpec) {
          return res.status(400).json({ error: 'No campaignSpec found for this gameId. Please generate the campaign core before generating the initial adventure.' });
        }
      } catch (e) {
        console.warn('Failed to verify campaignSpec before initial generation:', e);
        return res.status(500).json({ error: 'Failed to verify campaign state before initial generation.' });
      }
    }

    // Strip any client-sent system messages to avoid conflicting system-level instructions
    const inboundMessages = (messages || []).filter(m => m.role !== 'system');

    // If this is the initial scene and a campaignSpec exists for this game, ignore client-provided sessionSummary
    // (prevents player-character data from steering world-level entrypoint generation).
    let sessionSummaryToUse = sessionSummary;
    if (resolvedMode === 'initial' && gameId) {
      try {
        const GameState = require('../models/GameState');
        const gsCheck = await GameState.findOne({ gameId }).select('+campaignSpec');
        if (gsCheck && gsCheck.campaignSpec) {
          sessionSummaryToUse = '';
        }
      } catch (e) {
        console.warn('Failed to check GameState for sessionSummary override:', e);
      }
    }

    // Load persisted GameState for campaignSpec, character, mode, etc. DM rules always composed from prompt files each request.
    let persistedGameState = null;
    if (gameId) {
      try {
        const GameState = require('../models/GameState');
        const gs = await GameState.findOne({ gameId }).select('+campaignSpec +rawModelOutput +gameSetup +summary');
        if (gs) {
          persistedGameState = gs;
        }
      } catch (e) {
        console.warn('Failed to load GameState for generate:', e);
      }
    }

    // Two-phase combat entry: from exploration, a declared attack uses an empty-narration handoff turn, then
    // the server re-runs with skill_combat. When GameState.mode is already 'combat', skip the handoff.
    const activeCombat = Boolean(persistedGameState && persistedGameState.mode === 'combat');
    const generatedCharacterEarly = persistedGameState && persistedGameState.gameSetup && persistedGameState.gameSetup.generatedCharacter;
    const lastUtterance = lastUserText(messages);
    const userCombatEntry =
      resolvedMode !== 'initial' &&
      !activeCombat &&
      userMessageLooksCombat(lastUtterance) &&
      !blocksCombatEntryForAmbiguousWeapon(lastUtterance, generatedCharacterEarly);
    const stackMode = userCombatEntry ? 'exploration' : resolvedMode;

    // Compose full core from prompt files (sessionSummary, mode, skills). Combat turns need full skill_combat when stackMode is combat.
    const includeFullSkillThisTurn = includeFullSkill || stackMode === 'combat';
    let systemMsgs = composeSystemMessages({
      mode: stackMode,
      sessionSummary: sessionSummaryToUse,
      includeFullSkill: includeFullSkillThisTurn,
      language,
    }).filter((m) => m.role === 'system');

    // Ensure the full adventure-skill prompt is included for initial scenes so the model receives the seed template
    if (stackMode === 'initial') {
      try {
        let advSeed = loadPrompt('skill_adventureSeed.txt');
        if (advSeed) {
          // Render languageInstruction into the advSeed template so placeholders are resolved
          try {
            const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
            const langPrompt = loadPrompt(langFile);
            const renderData = { languageInstruction: langPrompt || '', language };
            advSeed = Mustache.render(advSeed, renderData);
          } catch (re) {
            // fallback: use advSeed unrendered
            console.warn('Failed to render languageInstruction into advSeed:', re);
          }
          systemMsgs.push({ role: 'system', content: advSeed });
        }
      } catch (e) {
        console.warn('Failed to load skill_adventureSeed.txt for initial mode:', e);
      }
    }

    // If a campaignSpec is available from persisted GameState, render DM-only injections from it
    let campaignInjectionKind = null;
    if (persistedGameState && persistedGameState.campaignSpec) {
      const spec = persistedGameState.campaignSpec;
      try {
        let injectTemplate = null;
        let renderData = {};
        if (stackMode === 'initial' && spec) {
          injectTemplate = loadPrompt('dm_inject_initial.txt');
          campaignInjectionKind = 'opening scene';
          renderData = {
            campaignTitle: typeof spec.title === 'string' ? spec.title.trim() : '',
            campaignConcept: spec.campaignConcept || '',
            factions: coerceCampaignStageToArray('factions', spec.factions).slice(0, 3),
            majorConflicts: takeCampaignFieldItems(spec.majorConflicts, 4),
            majorNPCs: coerceCampaignStageToArray('majorNPCs', spec.majorNPCs).slice(0, 4),
            keyLocations: coerceCampaignStageToArray('keyLocations', spec.keyLocations).slice(0, 4),
            dmHiddenAdventureObjective: dmHiddenAdventureObjectiveForPrompt(spec),
            // Compact JSON saves prompt tokens; do not inject rawModelOutput here (stale/truncated LLM blobs poison opening turns).
            campaignSpecJson: JSON.stringify(spec || {}),
            sessionSummary: persistedGameState.summary || sessionSummary || '',
          };
        } else if ((stackMode === 'exploration' || stackMode === 'explore') && spec) {
          injectTemplate = loadPrompt('dm_inject_explore.txt');
          campaignInjectionKind = 'exploration';
          renderData = {
            factions: coerceCampaignStageToArray('factions', spec.factions).slice(0, 3),
            dmHiddenAdventureObjective: dmHiddenAdventureObjectiveForPrompt(spec),
          };
        } else if ((stackMode === 'combat' || stackMode === 'decision' || stackMode === 'investigation') && spec) {
          injectTemplate = loadPrompt('dm_inject_combat.txt');
          campaignInjectionKind = 'combat or tense encounter';
          renderData = {
            majorNPCs: coerceCampaignStageToArray('majorNPCs', spec.majorNPCs).slice(0, 4),
            majorConflicts: takeCampaignFieldItems(spec.majorConflicts, 4),
            dmHiddenAdventureObjective: dmHiddenAdventureObjectiveForPrompt(spec),
          };
        }

        if (injectTemplate) {
          try {
            const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
            const langPrompt = loadPrompt(langFile);
            if (langPrompt) renderData.languageInstruction = langPrompt;
          } catch (e) {}
          const injected = Mustache.render(injectTemplate, renderData);
          systemMsgs.unshift({ role: 'system', content: injected });
        }
      } catch (e) {
        console.warn('Failed to render campaignSpec injection for generate:', e);
      }
    }

    try {
      const jsonGuardDm = loadPrompt('json_output_guard.txt');
      if (jsonGuardDm) systemMsgs.unshift({ role: 'system', content: jsonGuardDm });
      const envelopeInstr = loadPrompt('dm_player_response_envelope.txt');
      if (envelopeInstr) systemMsgs.unshift({ role: 'system', content: envelopeInstr });
    } catch (e) {
      console.warn('Failed to load DM JSON envelope / guard:', e);
    }

    if (userCombatEntry) {
      try {
        const handoffTpl = loadPrompt('dm_combat_entry_handoff.txt');
        if (handoffTpl) {
          const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
          let languageInstruction = '';
          try {
            languageInstruction = loadPrompt(langFile) || '';
          } catch (e) {
            /* ignore */
          }
          systemMsgs.push({
            role: 'system',
            content: Mustache.render(handoffTpl, { languageInstruction, language }),
          });
        }
      } catch (e) {
        console.warn('Failed to load dm_combat_entry_handoff.txt:', e);
      }
    }

    const pcContextBlock = renderPlayerCharacterContextBlock(persistedGameState, language);
    if (pcContextBlock) {
      systemMsgs.push({ role: 'system', content: pcContextBlock });
    }

    // Consolidate all system messages into one system-role prompt to ensure guard precedence
    let consolidatedSystem = appendSceneGroundingPolicy(consolidateSystemMessages(systemMsgs));
    const messagesToSend = [{ role: 'system', content: consolidatedSystem }, ...inboundMessages];
    const dmPromptTraceParts = [
      `composed DM rules core (play mode: ${stackMode})`,
      ...(sessionSummaryToUse ? ['session recap in system stack'] : []),
      ...(stackMode === 'initial' ? ['opening scene seed'] : []),
      ...(campaignInjectionKind ? [`campaign world injection (${campaignInjectionKind})`] : []),
      ...(userCombatEntry ? ['combat entry handoff (empty narration)'] : []),
    ];
    const outboundMessages = traceMessages(messagesToSend, dmPromptTraceParts.join('; '));
    // Debug: log the consolidated system message and outbound messages
    try {
      console.log('DEBUG: consolidated system (generate):', consolidatedSystem);
      console.log('DEBUG: messagesToSend (generate):', JSON.stringify(outboundMessages, null, 2));
    } catch (e) {
      console.log('DEBUG: messagesToSend (generate) - could not stringify', e);
    }

    // Use central generateResponse (handles model selection and fallbacks)
    try {
        // Dynamically estimate prompt size and compute a safe completion token budget.
        // Default 16k matches common chat models; override with DM_MODEL_CONTEXT_TOKENS for your deployment.
        const MODEL_MAX_TOKENS = Math.min(
          128000,
          Math.max(4096, parseInt(process.env.DM_MODEL_CONTEXT_TOKENS || '16384', 10) || 16384)
        );
        const promptTokens = estimateTokenCount(outboundMessages);
        let completionBudget = stackMode === 'initial' ? 2200 : userCombatEntry ? 200 : 550;
        const available = MODEL_MAX_TOKENS - promptTokens - 80;
        if (available <= 0) {
          console.warn(
            `DM generate: estimated prompt tokens (${promptTokens}) exceed MODEL_MAX_TOKENS (${MODEL_MAX_TOKENS}); using completion floor (reply may still fail at the API if the real context is smaller).`
          );
          completionBudget = resolvedMode === 'initial' ? 2200 : 650;
        } else {
          const cap = stackMode === 'initial' ? Math.min(4500, available) : Math.min(2000, available);
          completionBudget = Math.min(Math.max(completionBudget, 500), cap);
        }
        console.log(
          `Estimated prompt tokens: ${promptTokens}, model context cap: ${MODEL_MAX_TOKENS}, completion budget: ${completionBudget}`
        );
    // Persist outgoing request for debugging if gameId supplied (trim to cap)
    try {
      if (gameId) {
        const GameState = require('../models/GameState');
        await GameState.findOneAndUpdate({ gameId }, { rawModelRequest: JSON.stringify(outboundMessages).slice(0, 200000) }, { upsert: true });
      }
    } catch (e) {
      console.warn('Failed saving rawModelRequest for generate:', e);
    }

    // Log clearly that we are about to call the LLM for the generate route and persist timestamps
    try {
      console.log('OUTGOING (generate) messagesToSend:', JSON.stringify(outboundMessages, null, 2));
    } catch (e) {
      console.log('OUTGOING (generate) messagesToSend (could not stringify)', outboundMessages);
    }

    // Stamp LLM call start time
    try {
      if (gameId) {
        const GameState = require('../models/GameState');
        await GameState.findOneAndUpdate({ gameId }, { $set: { llmCallStartedAt: new Date().toISOString() } }, { upsert: true });
      }
    } catch (e) {
      console.warn('Failed to persist llmCallStartedAt for generate:', e);
    }

    let aiMessage = null;
    try {
      aiMessage = await generateResponse({ messages: outboundMessages }, { max_tokens: completionBudget, temperature: 0.8, gameId });
    } catch (llmErr) {
      console.error('LLM call failed for generate:', llmErr);
      try {
        if (gameId) {
          const GameState = require('../models/GameState');
          await GameState.findOneAndUpdate({ gameId }, { $set: { llmCallError: String(llmErr).slice(0, 200000) } }, { upsert: true });
        }
      } catch (ee) {
        console.warn('Failed to persist llmCallError for generate:', ee);
      }
      return res.status(500).json({ error: 'LLM call failed (see server logs).' });
    }

    if (!aiMessage) {
        console.error('LLM returned no content for generate');
        return res.status(500).json({ error: 'AI response was empty or failed (see server logs).' });
    }

        let finalRaw = String(aiMessage);
        let envelope = parseDmPlayerEnvelope(finalRaw);
        if (!envelope && stackMode === 'initial') {
          envelope = salvageTruncatedInitialEnvelope(finalRaw, gameId);
        }
        if (!envelope) {
          try {
            const repairUser = {
              role: 'user',
              content:
                'Your previous assistant output was missing or invalid. Output ONLY one JSON object, no markdown fences, no other text. Keys: "narration" (string, markdown for the player), "imminentCombat" (boolean), "combatCue" (string, use "" when imminentCombat is false), optional "encounterState" (object) for combat tracking only.',
            };
            const repairMsgs = [{ role: 'system', content: consolidatedSystem }, ...inboundMessages, repairUser];
            const repairOutbound = traceMessages(repairMsgs, 'DM JSON envelope repair');
            const repairResp = await generateResponse(
              { messages: repairOutbound },
              { max_tokens: stackMode === 'initial' ? 1600 : 900, temperature: 0.6, gameId }
            );
            if (repairResp) {
              finalRaw = String(repairResp);
              envelope = parseDmPlayerEnvelope(finalRaw);
              if (!envelope && stackMode === 'initial') {
                envelope = salvageTruncatedInitialEnvelope(finalRaw, gameId);
              }
            }
          } catch (e) {
            console.warn('JSON envelope repair failed:', e);
          }
        }

        applyUserCombatToEnvelope(envelope, inboundMessages, persistedGameState && persistedGameState.gameSetup && persistedGameState.gameSetup.generatedCharacter);
        enforceAmbiguousWeaponNoCombat(envelope, inboundMessages, persistedGameState && persistedGameState.gameSetup && persistedGameState.gameSetup.generatedCharacter);

        console.log('AI DM envelope parsed:', !!envelope);

        const combatStackNeeded =
          envelope && envelope.imminentCombat && (userCombatEntry || stackMode !== 'combat');

        let didCombatRedo = false;

        // Imminent combat: rebuild system stack as a real combat turn (dm_inject_combat + skill_combat),
        // not exploration inject + ad-hoc skill append (which left wrong campaign context).
        if (combatStackNeeded) {
          try {
            console.log('Detected imminentCombat=true; re-running with combat campaign injection + skill_combat.');
            const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
            let langPromptCombat = '';
            try {
              langPromptCombat = loadPrompt(langFile) || '';
            } catch (e) {
              /* ignore */
            }

            let redoSystemMsgs = composeSystemMessages({
              mode: 'combat',
              sessionSummary: sessionSummaryToUse,
              includeFullSkill: true,
              language,
            }).filter((m) => m.role === 'system');

            if (persistedGameState && persistedGameState.campaignSpec) {
              const spec = persistedGameState.campaignSpec;
              const injectCombat = loadPrompt('dm_inject_combat.txt');
              if (injectCombat) {
                const renderData = {
                  majorNPCs: takeCampaignFieldItems(spec.majorNPCs, 4),
                  majorConflicts: takeCampaignFieldItems(spec.majorConflicts, 4),
                  languageInstruction: langPromptCombat,
                };
                redoSystemMsgs.unshift({
                  role: 'system',
                  content: Mustache.render(injectCombat, renderData),
                });
              }
            }

            const pcRedoCtx = renderPlayerCharacterContextBlock(persistedGameState, language);
            if (pcRedoCtx) {
              redoSystemMsgs.push({ role: 'system', content: pcRedoCtx });
            }

            try {
              const jsonGuardRedo = loadPrompt('json_output_guard.txt');
              if (jsonGuardRedo) redoSystemMsgs.unshift({ role: 'system', content: jsonGuardRedo });
              const envelopeRedo = loadPrompt('dm_player_response_envelope.txt');
              if (envelopeRedo) redoSystemMsgs.unshift({ role: 'system', content: envelopeRedo });
            } catch (e) {
              console.warn('Failed to prepend JSON guard / envelope to combat redo stack:', e);
            }

            const consolidatedWithCombat = appendSceneGroundingPolicy(consolidateSystemMessages(redoSystemMsgs));

            const combatHandoffText =
              language && String(language).toLowerCase().startsWith('span')
                ? '[DM / interno] Instrucciones de combate D&D 5e cargadas. Devuelve SOLO un objeto JSON (mismo esquema que el sobre DM): narration (markdown, todo lo visible al jugador), imminentCombat, combatCue, y "encounterState" (objeto) con participantes/HP/iniciativa según el sobre DM. ' +
                  'Inicia el combate formal: sorpresa si aplica, luego iniciativa (d20 + Des), orden de turnos, y resuelve el ataque que declaró el jugador en su turno con tirada vs CA y daño si procede. Mantén la misma escena y personajes.'
                : '[DM / internal] D&D 5e combat instructions are now loaded. Output ONLY one JSON object (same DM envelope schema): narration (markdown, all player-visible text), imminentCombat, combatCue, and "encounterState" (object) with participants/HP/initiative per the envelope doc. ' +
                  'Begin formal combat: surprise if it applies, then initiative (d20 + Dex), establish turn order, then resolve the player’s declared attack on the correct turn with attack roll vs AC and damage if appropriate. Keep the same scene and cast.';
            const combatHandoffUser = { role: 'user', content: combatHandoffText };
            const messagesToSendCombat = [
              { role: 'system', content: consolidatedWithCombat },
              ...inboundMessages,
              combatHandoffUser,
            ];
            const combatOutbound = traceMessages(
              messagesToSendCombat,
              'DM imminent-combat redo; dm_inject_combat + skill_combat; internal handoff user message'
            );
            const combatResp = await generateResponse({ messages: combatOutbound }, { max_tokens: 1200, temperature: 0.8, gameId });
            if (combatResp) {
              finalRaw = String(combatResp);
              let combatEnv = parseDmPlayerEnvelope(finalRaw);
              if (!combatEnv) {
                console.warn('Combat redo: model output was not a valid DM envelope; refusing empty first-pass narration.');
                return res.status(502).json({
                  error:
                    'Combat pass failed: model output was not valid DM envelope JSON. Retry the message or check server logs / rawModelOutput.',
                  rawPreview: String(finalRaw).slice(0, 1500),
                });
              }
              envelope = combatEnv;
              applyUserCombatToEnvelope(envelope, inboundMessages, persistedGameState && persistedGameState.gameSetup && persistedGameState.gameSetup.generatedCharacter);
              enforceAmbiguousWeaponNoCombat(envelope, inboundMessages, persistedGameState && persistedGameState.gameSetup && persistedGameState.gameSetup.generatedCharacter);
              didCombatRedo = true;

              if (!String(envelope.narration || '').trim()) {
                const narrFixUser = {
                  role: 'user',
                  content:
                    language && String(language).toLowerCase().startsWith('span')
                      ? 'Tu último JSON tenía "narration" vacía. Las reglas de combate ya están activas. Devuelve UN solo objeto JSON (mismo esquema DM). "narration" debe ser texto markdown NO vacío para el jugador: iniciativa, tiradas y resolución del ataque. No uses narration "".'
                      : 'Your last JSON had empty "narration". Combat rules are active. Return ONE JSON object (same DM envelope). "narration" must be non-empty markdown for the player: initiative, rolls, and attack resolution. Do not use an empty narration string.',
                };
                const narrFixOutbound = traceMessages(
                  [...messagesToSendCombat, narrFixUser],
                  'DM combat narration must be non-empty (repair)'
                );
                try {
                  const narrFixResp = await generateResponse(
                    { messages: narrFixOutbound },
                    { max_tokens: 1200, temperature: 0.55, gameId }
                  );
                  if (narrFixResp) {
                    finalRaw = String(narrFixResp);
                    const fixedEnv = parseDmPlayerEnvelope(finalRaw);
                    if (fixedEnv && String(fixedEnv.narration || '').trim()) {
                      envelope = fixedEnv;
                      applyUserCombatToEnvelope(envelope, inboundMessages, persistedGameState && persistedGameState.gameSetup && persistedGameState.gameSetup.generatedCharacter);
                      enforceAmbiguousWeaponNoCombat(envelope, inboundMessages, persistedGameState && persistedGameState.gameSetup && persistedGameState.gameSetup.generatedCharacter);
                    }
                  }
                } catch (nfe) {
                  console.warn('Combat narration repair pass failed:', nfe);
                }
              }

              try {
                if (gameId) {
                  const GameState = require('../models/GameState');
                  await GameState.findOneAndUpdate(
                    { gameId },
                    { rawModelOutput: String(finalRaw).slice(0, 200000) },
                    { upsert: true }
                  );
                }
              } catch (pe) {
                console.warn('Failed to persist combat-aware rawModelOutput:', pe);
              }
            }
          } catch (e) {
            console.warn('Failed to re-run generate with combat stack:', e);
          }
        }

        if (!envelope) {
          const coerced = coerceInitialProseToEnvelope(finalRaw, stackMode, gameId);
          if (coerced) envelope = coerced;
        }

        if (!envelope) {
          return res.status(502).json({
            error: 'DM reply was not valid envelope JSON',
            rawPreview: String(finalRaw).slice(0, 1200),
          });
        }

        if (userCombatEntry && combatStackNeeded && !didCombatRedo && !String(envelope.narration || '').trim()) {
          return res.status(502).json({
            error:
              'Combat handoff did not complete: the model returned empty narration but the combat pass failed. Retry or check server logs.',
            rawPreview: String(finalRaw).slice(0, 1200),
          });
        }

        if (!String(envelope.narration || '').trim()) {
          return res.status(502).json({
            error:
              'The model returned empty narration. Combat handoff must be followed by non-empty player-visible text; retry your last message.',
            encounterState: envelope.encounterState != null ? envelope.encounterState : null,
            rawPreview: String(finalRaw).slice(0, 1500),
          });
        }

        const finalUsedCombatStack = stackMode === 'combat' || didCombatRedo;
        try {
          if (gameId) {
            const GameState = require('../models/GameState');
            const $set = { llmCallCompletedAt: new Date().toISOString() };
            if (finalUsedCombatStack) $set.mode = 'combat';
            if (envelope.encounterState != null) {
              $set.encounterState = envelope.encounterState;
            }
            await GameState.findOneAndUpdate(
              { gameId },
              { rawModelOutput: String(finalRaw).slice(0, 200000), $set },
              { upsert: true }
            );
          }
        } catch (e) {
          console.warn('Failed saving rawModelOutput for generate:', e);
        }

        if (gameId && persist && typeof persist === 'object' && String(persist.gameId || '') === String(gameId)) {
          try {
            const merged = mergePersistWithAssistantReply(persist, envelope, { finalUsedCombatStack });
            if (merged) await persistGameStateFromBody(merged);
          } catch (pe) {
            console.warn('Post-generate persistGameState failed:', pe);
          }
        }

        const responseBody = {
          narration: envelope.narration,
          encounterState: envelope.encounterState != null ? envelope.encounterState : null,
          activeCombat: Boolean(finalUsedCombatStack),
        };
        if (envelope.coinage != null && typeof envelope.coinage === 'object') {
          responseBody.coinage = normalizeCoinageObject(envelope.coinage);
        }
        res.json(responseBody);
    } catch (error) {
        console.error('Error generating text:', error);
        res.status(500).json({ error: `Error generating text: ${String(error)}` });
    }
});

// Route to generate campaign generating responses 
router.post('/generate-campaign', async (req, res) => {
    // Extract parameters from the request body; accept gameSetup for character details
    const { messages = [], sessionSummary = '', gameSetup = {}, language = 'English' } = req.body;

    console.log('Prepper is Processing the following messages (campaign generation)');
    // Ensure gender default
    gameSetup.gender = gameSetup.gender || gameSetup.characterGender || 'Male';
    console.log('gameSetup:', gameSetup);

    // Campaign core is the authoritative kickstarter.

    // For campaign generation, compose base system messages but ensure strict no-preface/json guards
    const baseSystemMsgs = composeSystemMessages({ mode: 'initial', sessionSummary, includeFullSkill: true, language });
    // Remove any assistant-role few-shot examples to avoid priming explanatory text
    const filteredBaseSystem = baseSystemMsgs.filter(m => m.role !== 'assistant');

    // Load guards and ensure they appear first (highest priority)
    const systemMsgs = [];
    try {
      const jsonGuard = loadPrompt('json_output_guard.txt');
      if (jsonGuard) systemMsgs.push({ role: 'system', content: jsonGuard });
    } catch (e) {
      console.warn('json_output_guard.txt missing or failed to load:', e);
    }
    try {
      const noPreface = loadPrompt('no_prefatory_guard.txt');
      if (noPreface) systemMsgs.push({ role: 'system', content: noPreface });
    } catch (e) {
      console.warn('no_prefatory_guard.txt missing or failed to load:', e);
    }
    // Insert language prompt at high priority so it overrides formatting expectations.
    try {
      const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
      const langPrompt = loadPrompt(langFile);
      if (langPrompt) systemMsgs.push({ role: 'system', content: langPrompt });
    } catch (e) {
      // ignore
    }

    // Then append the filtered base system messages
    systemMsgs.push(...filteredBaseSystem);


    // Load the templated user instruction prompt and render it with dynamic values.
    const template = loadPrompt('campaign_generator_prompt.txt');
    if (!template) {
      console.error('Missing required prompt template: server/prompts/campaign_generator_prompt.txt');
      return res.status(500).json({ error: 'Server misconfiguration: campaign_generator_prompt.txt is required' });
    }
    // Render campaign prompt template (include language instruction from prompts so templates can adapt)
    let languageInstructionForTemplate = '';
    try {
      const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
      const langPrompt = loadPrompt(langFile);
      if (langPrompt) languageInstructionForTemplate = langPrompt;
    } catch (e) {
      // ignore
    }
    const rendered = Mustache.render(template, {
      gameSetup: JSON.stringify(gameSetup),
      sessionSummary: sessionSummary || '',
      languageInstruction: languageInstructionForTemplate,
      language
    });
    const userInstruction = { role: 'user', content: rendered };

    // Language is handled via prompt files loaded by promptManager; no hardcoded language rules here.

    // Consolidate system messages into a single system-role prompt to reduce role drift and priming
    const consolidatedCampaignSystem = consolidateSystemMessages(systemMsgs);
    const messagesToSend = [{ role: 'system', content: consolidatedCampaignSystem }, userInstruction];
    const campaignOutbound = traceMessages(
      messagesToSend,
      'full campaign JSON generator; JSON output guards; no-preface guard; language policy; DM core slice; user: campaign premise JSON'
    );
    // Debug: log the consolidated system message and outbound messages
    try {
      console.log('DEBUG: consolidated system (generate-campaign):', consolidatedCampaignSystem);
      console.log('DEBUG: messagesToSend (generate-campaign):', JSON.stringify(campaignOutbound, null, 2));
    } catch (e) {
      console.log('DEBUG: messagesToSend (generate-campaign) - could not stringify', e);
    }

    try {
        // Estimate prompt size and choose a completion budget so output won't be truncated.
        const MODEL_MAX_TOKENS = 4000;
        const promptTokensCampaign = estimateTokenCount(campaignOutbound);
        let completionBudgetCampaign = 700;
        const availableCampaign = MODEL_MAX_TOKENS - promptTokensCampaign - 50;
        if (availableCampaign <= 0) {
          completionBudgetCampaign = 100;
        } else {
          // Use a higher minimum to reduce risk of truncation for campaign generation.
          completionBudgetCampaign = Math.min(
            1500,
            Math.max(600, Math.min(availableCampaign, completionBudgetCampaign))
          );
        }
        console.log(`Campaign generator: prompt tokens ${promptTokensCampaign}, completion budget ${completionBudgetCampaign}`);
        const aiMessage = await generateResponse({ messages: campaignOutbound }, { max_tokens: completionBudgetCampaign, temperature: 0.8, gameId });
        if (!aiMessage) {
          return res.status(500).json({ error: 'AI response was empty or failed (see server logs).' });
        }
        // Debug: log the raw AI response received for campaign generation
        try {
          console.log('DEBUG: raw AI response (generate-campaign):', aiMessage);
        } catch (e) {
          console.log('DEBUG: raw AI response (generate-campaign) - could not stringify', e);
        }

        // Try to parse JSON from the response; extract first balanced JSON object then parse; if it fails, do not retry (avoid token waste)
        let parsed = null;
        let rawJsonText = null;
        try {
            // Extract first balanced JSON object substring to avoid trailing non-JSON text
            rawJsonText = extractFirstJsonObject(aiMessage) || aiMessage;
            parsed = JSON.parse(rawJsonText);
        } catch (e) {
            console.warn('Failed to parse JSON from campaign generator (first attempt):', e, 'raw snippet:', rawJsonText ? rawJsonText.slice(0, 2000) : 'none');
            // Do not call the model again to "repair" — avoid wasting tokens.
            // Leave parsed as null; rawModelOutput will be saved for inspection.
        }

        if (parsed) ensureCampaignCoreTitle(parsed);

        // Persist campaignSpec and raw AI output into GameState if gameId supplied in request body
        try {
          const gameIdToPersist = req.body.gameId || req.query.gameId || null;
          if (gameIdToPersist) {
            const GameState = require('../models/GameState');
            const update = {};
            if (parsed) update.campaignSpec = parsed;
            update.rawModelOutput = String(aiMessage).slice(0, 200000); // cap size
            await GameState.findOneAndUpdate({ gameId: gameIdToPersist }, update, { upsert: true, new: true });
            console.log('Persisted campaignSpec/rawModelOutput to GameState for gameId:', gameIdToPersist);
          }
        } catch (e) {
          console.warn('Failed to persist campaignSpec/rawModelOutput to GameState:', e);
        }

        // Return parsed campaign JSON (campaignConcept) or raw AI output (strip DM-only fields for clients)
        res.json(parsed ? redactCampaignSpecForClient(parsed) : aiMessage);
    } catch (error) {
        console.error('Error generating text:', error);
        res.status(500).json({ error: `Error generating text: ${String(error)}` });
    }
});

// New: generate only core campaign spec (small, reliable output)
router.post('/generate-campaign-core', async (req, res) => {
  const { gameId = null, waitForStages = true, language = 'English' } = req.body;
  console.log('Campaign core generator called');
  // Product flow: campaign core runs only after a character exists for this game (persisted by generate-character).
  if (gameId) {
    try {
      const GameState = require('../models/GameState');
      const gs = await GameState.findOne({ gameId }).select('gameSetup.generatedCharacter').lean();
      if (!gs?.gameSetup?.generatedCharacter) {
        return res.status(400).json({
          error:
            'Generate your character first and confirm it before generating the campaign. The server has no saved character for this game.',
        });
      }
    } catch (e) {
      console.warn('generate-campaign-core: failed to verify generatedCharacter:', e);
      return res.status(500).json({ error: 'Could not verify game state before campaign generation.' });
    }
  }
  // World-only: never use client gameSetup, sessionSummary, or character data for core generation.
  const sessionSummaryForCore = '';

  // Prepare system messages (guards + core guidance)
  // Load campaign-core policy from prompt file so this behavior is editable (do NOT hardcode)
  const baseSystemMsgs = composeSystemMessages({
    mode: 'initial',
    sessionSummary: sessionSummaryForCore,
    includeFullSkill: false,
    language,
  }).filter(m => m.role === 'system');
  const systemMsgs = [];
  try {
    const jsonGuard = loadPrompt('json_output_guard.txt');
    if (jsonGuard) systemMsgs.push({ role: 'system', content: jsonGuard });
  } catch (e) {}
  try {
    const noPreface = loadPrompt('no_prefatory_guard.txt');
    if (noPreface) systemMsgs.push({ role: 'system', content: noPreface });
  } catch (e) {}
  // Ensure language prompt is high-priority for core campaign generation as well
  try {
    const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
    const langPrompt = loadPrompt(langFile);
    if (langPrompt) systemMsgs.push({ role: 'system', content: langPrompt });
  } catch (e) {
    // ignore
  }
  // Insert campaign-core policy prompt to ensure core generation ignores player/session data
  try {
    const corePolicy = loadPrompt('campaign_generator_prompt.txt');
    if (corePolicy) systemMsgs.push({ role: 'system', content: corePolicy });
  } catch (e) {
    console.warn('campaign_generator_prompt.txt missing or failed to load:', e);
  }
  systemMsgs.push(...baseSystemMsgs);

  // If there is a stored campaignSpec for this game, prioritize system guards and base messages.
  if (gameId) {
    try {
      const GameState = require('../models/GameState');
      const gs = await GameState.findOne({ gameId }).select('+campaignSpec');
      // Campaign core is the authoritative kickstarter.
    } catch (e) {
      console.warn('Failed to load GameState for core generation:', e);
    }
  }

  // Use the campaign_generator_prompt.txt template as the user instruction so prompts live in files (not hardcoded)
  const consolidated = consolidateSystemMessages(systemMsgs);
  let userPromptRendered = null;
  try {
    const template = loadPrompt('campaign_generator_prompt.txt');
    if (!template) {
      console.error('Missing required prompt template: server/prompts/campaign_generator_prompt.txt');
      return res.status(500).json({ error: 'Server misconfiguration: campaign_generator_prompt.txt is required' });
    }
    // provide sessionSummary and languageInstruction to the template
    const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
    let languageInstructionForTemplate = '';
    try {
      const langPrompt = loadPrompt(langFile);
      if (langPrompt) languageInstructionForTemplate = langPrompt;
    } catch (e) { /* ignore */ }
    userPromptRendered = Mustache.render(template, {
      sessionSummary: sessionSummaryForCore,
      languageInstruction: languageInstructionForTemplate,
      language,
    });
  } catch (e) {
    console.error('Failed rendering campaign generator prompt template:', e);
    return res.status(500).json({ error: 'Failed rendering campaign generator prompt' });
  }
  const messagesToSend = [{ role: 'system', content: consolidated }, { role: 'user', content: userPromptRendered }];
  const coreOutbound = traceMessages(
    messagesToSend,
    'campaign core JSON only; JSON guards; language policy; world premise policy; DM core slice; user: campaign template request'
  );
  // Log the exact messages being sent to the model for debugging (redactable if needed).
  try {
    console.log('OUTGOING (campaign-core) messagesToSend:', JSON.stringify(coreOutbound, null, 2));
  } catch (e) {
    console.log('OUTGOING (campaign-core) messagesToSend (could not stringify):', coreOutbound);
  }

  try {
    // Persist outgoing request for debugging if gameId supplied (trim to cap)
    try {
      if (gameId) {
        const GameState = require('../models/GameState');
        await GameState.findOneAndUpdate({ gameId }, { rawModelRequest: JSON.stringify(coreOutbound).slice(0, 200000) }, { upsert: true });
      }
    } catch (e) {
      console.warn('Failed saving rawModelRequest for campaign-core:', e);
    }

    const aiMessage = await generateResponse({ messages: coreOutbound }, { max_tokens: 1000, temperature: 0.8, gameId });
    if (!aiMessage) return res.status(500).json({ error: 'AI response empty' });

    const rawJson = extractFirstJsonObject(aiMessage) || aiMessage;
    let parsed = null;
    try {
      parsed = JSON.parse(rawJson);
    } catch (e) {
      // persist raw for debugging if gameId
      if (gameId) {
        try {
          const GameState = require('../models/GameState');
          await GameState.findOneAndUpdate({ gameId }, { rawModelOutput: String(aiMessage).slice(0, 200000) }, { upsert: true });
        } catch (ee) {
          console.warn('Failed saving rawModelOutput:', ee);
        }
      }
      return res.status(500).json({ error: 'Failed to parse campaign core JSON', raw: aiMessage });
    }

    ensureCampaignCoreTitle(parsed);

    // If caller requested to wait for background stages (default true), run them synchronously and fail fast on error.
    if (gameId && waitForStages) {
      // Helper to run a stage with timeout and stop waiting on first failure.
      const STAGE_TIMEOUT = process.env.DM_STAGE_TIMEOUT_MS ? parseInt(process.env.DM_STAGE_TIMEOUT_MS, 10) : 60000; // default 60s
      console.log(`Stage timeout is ${STAGE_TIMEOUT}ms`);
      async function runStageWithTimeout(stageName) {
        try {
          const stagePromise = generateCampaignStage({ gameId, stage: stageName, campaignCore: parsed, systemMsgs, language });
          const timeoutPromise = new Promise((_, reject) => setTimeout(() => reject(new Error('stage_timeout')), STAGE_TIMEOUT));
          const result = await Promise.race([stagePromise, timeoutPromise]);
          return result;
        } catch (err) {
          console.error(`Stage ${stageName} failed or timed out:`, err);
          return false;
        }
      }

      try {
        const ok1 = await runStageWithTimeout('factions');
        if (!ok1) return res.status(500).json({ error: 'Failed generating factions stage' });
        const ok2 = await runStageWithTimeout('majorNPCs');
        if (!ok2) return res.status(500).json({ error: 'Failed generating majorNPCs stage' });
        const ok3 = await runStageWithTimeout('keyLocations');
        if (!ok3) return res.status(500).json({ error: 'Failed generating keyLocations stage' });
        console.log('Completed synchronous campaign stages for', gameId);
        // At this point stages persisted their outputs. Now persist the full campaignSpec (core + stages) atomically.
        try {
          const GameState = require('../models/GameState');
          const existing = await GameState.findOne({ gameId }).lean();
          const combined = Object.assign({}, parsed, (existing && existing.campaignSpec) ? existing.campaignSpec : {});
          await GameState.findOneAndUpdate({ gameId }, { campaignSpec: combined, rawModelOutput: String(aiMessage).slice(0, 200000) }, { upsert: true });
          return res.json(redactCampaignSpecForClient(combined));
        } catch (e) {
          console.warn('Failed persisting combined campaignSpec after stages:', e);
          return res.json(redactCampaignSpecForClient(parsed));
        }
      } catch (e) {
        console.error('Synchronous campaign stages failed:', e);
        return res.status(500).json({ error: 'Background campaign stages failed' });
      }
    }
    // Default: respond immediately and generate stages asynchronously (not used when waitForStages=true)
    res.json(redactCampaignSpecForClient(parsed));

    if (gameId) {
      setImmediate(() => {
        (async () => {
          try {
            await generateCampaignStage({ gameId, stage: 'factions', campaignCore: parsed, systemMsgs, language });
            await generateCampaignStage({ gameId, stage: 'majorNPCs', campaignCore: parsed, systemMsgs, language });
            await generateCampaignStage({ gameId, stage: 'keyLocations', campaignCore: parsed, systemMsgs, language });
            console.log('Completed background campaign stages for', gameId);
          } catch (e) {
            console.error('Background campaign stages failed:', e);
          }
        })();
      });
    }
  } catch (err) {
    console.error('Error in generate-campaign-core:', err);
    res.status(500).json({ error: String(err) });
  }
});

// Removed separate plot endpoint — campaign generation is the single kickstarter

// Helper to generate and persist a campaign stage (background, not blocking response)
async function generateCampaignStage({ gameId, stage, campaignCore, systemMsgs, language }) {
  try {
    console.log(`Starting campaign stage generation: ${stage} for gameId=${gameId}`);
    // Build a focused user prompt per stage, preferring prompt files under server/prompts/.
    let userPrompt = '';
    let templateFile = null;
    if (stage === 'factions') templateFile = 'campaign_stage_factions.txt';
    else if (stage === 'majorNPCs') templateFile = 'campaign_stage_majorNPCs.txt';
    else if (stage === 'keyLocations') templateFile = 'campaign_stage_keyLocations.txt';
    else {
      console.warn('Unknown campaign stage:', stage);
      return;
    }

    // Attempt to load the stage template; fall back to an inline prompt if missing.
    try {
      const tpl = loadPrompt(templateFile);
      if (tpl) {
        // include languageInstruction for template rendering
        const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
        let languageInstructionForTemplate = '';
        try {
          const langPrompt = loadPrompt(langFile);
          if (langPrompt) languageInstructionForTemplate = langPrompt;
        } catch (e) {}
        userPrompt = Mustache.render(tpl, { campaignConcept: campaignCore.campaignConcept, languageInstruction: languageInstructionForTemplate, language });
      } else {
        // inline fallback if template file missing
        if (stage === 'factions') {
          userPrompt =
            `Based on this campaignConcept: ${campaignCore.campaignConcept}\n` +
            `Return ONLY a JSON array named "factions" where each item has: name (string), goal (1-2 sentences), resources (1 sentence), currentDisposition (1 sentence). Return the array (not wrapped) as JSON.`;
        } else if (stage === 'majorNPCs') {
          userPrompt =
            `Based on this campaignConcept: ${campaignCore.campaignConcept}\n` +
            `Return ONLY a JSON array named "majorNPCs" where each item has: name (string), role (string), briefDescription (2 sentences). Return the array as JSON.`;
        } else if (stage === 'keyLocations') {
          userPrompt =
            `Based on this campaignConcept: ${campaignCore.campaignConcept}\n` +
            `Return ONLY a JSON array named "keyLocations" where each item has: name (string), type (string), significance (1-2 sentences). Return the array as JSON.`;
        }
      }
    } catch (e) {
      console.warn('Failed to load/render campaign stage template:', e);
      return;
    }

    const consolidated = consolidateSystemMessages(systemMsgs);
    const messagesToSend = [{ role: 'system', content: consolidated }, { role: 'user', content: userPrompt }];
    const stageTrace =
      stage === 'factions'
        ? 'campaign build stage: factions JSON; inherits core-generation system stack'
        : stage === 'majorNPCs'
          ? 'campaign build stage: major NPCs JSON; inherits core-generation system stack'
          : 'campaign build stage: key locations JSON; inherits core-generation system stack';
    const stageOutbound = traceMessages(messagesToSend, stageTrace);
    // Log the exact messages being sent to the model for debugging (redactable if needed).
    try {
      console.log(`OUTGOING (stage:${stage}) messagesToSend:`, JSON.stringify(stageOutbound, null, 2));
    } catch (e) {
      console.log(`OUTGOING (stage:${stage}) messagesToSend (could not stringify):`, stageOutbound);
    }
    const aiMessage = await generateResponse({ messages: stageOutbound }, { max_tokens: 800, temperature: 0.8, gameId });
    if (!aiMessage) {
      console.warn(`Stage ${stage} returned empty response`);
      return false;
    }

    const rawJson = extractFirstJsonObject(aiMessage) || aiMessage;
    let parsed = null;
    try {
      parsed = JSON.parse(rawJson);
    } catch (e) {
      console.warn(`Failed to parse JSON for stage ${stage}:`, e);
      // persist raw for debugging
      try {
        await require('../models/GameState').findOneAndUpdate(
          { gameId },
          { rawModelOutput: String(aiMessage).slice(0, 200000) },
          { upsert: true, new: true }
        );
      } catch (pe) {
        console.warn('Failed to persist rawModelOutput for stage parse failure:', pe);
      }
      return false;
    }

    const coerced = coerceCampaignStageToArray(stage, parsed);
    if (!coerced.length) {
      console.warn(`Stage ${stage}: empty array after coercion; raw snippet:`, String(rawJson).slice(0, 400));
      try {
        await require('../models/GameState').findOneAndUpdate(
          { gameId },
          { rawModelOutput: String(aiMessage).slice(0, 200000) },
          { upsert: true, new: true }
        );
      } catch (pe) {
        console.warn('Failed to persist rawModelOutput for empty coerced stage:', pe);
      }
      return false;
    }

    // Persist into campaignSpec.<stage>
    try {
      const update = {};
      update[`campaignSpec.${stage}`] = coerced;
      update.rawModelOutput = String(aiMessage).slice(0, 200000);
      await require('../models/GameState').findOneAndUpdate({ gameId }, update, { upsert: true, new: true });
      console.log(`Persisted campaign stage ${stage} for gameId=${gameId}`);
    } catch (e) {
      console.warn(`Failed to persist stage ${stage}:`, e);
      return false;
    }
    return true;
  } catch (e) {
    console.error('Error generating campaign stage', stage, e);
    return false;
  }
}

// Note: summary generation is now handled server-side as part of campaign/session flows.

// Route to generate only a playerCharacter (separate from campaign generation)
function safeJsonStringifyForPrompt(obj) {
  try {
    return JSON.stringify(obj != null && typeof obj === 'object' ? obj : {});
  } catch (e) {
    console.warn('generate-character: gameSetup JSON.stringify failed:', e?.message || e);
    return '{}';
  }
}

router.post('/generate-character', async (req, res) => {
  const body = req.body && typeof req.body === 'object' ? req.body : {};
  const { gameSetup = {}, language = 'English', gameId = null } = body;

  if (!gameId) {
    return res.status(400).json({ error: 'generate-character requires gameId.' });
  }

  try {
    // Character generation is NOT an in-play DM turn. Do not use composeSystemMessages(mode: initial):
    // systemCore.txt describes the narration JSON envelope and conflicts with { "playerCharacter": ... }.
    const systemMsgs = [];
    try {
      const jsonGuardDm = loadPrompt('json_output_guard.txt');
      if (jsonGuardDm) systemMsgs.push({ role: 'system', content: jsonGuardDm });
    } catch (e) {
      /* ignore */
    }
    systemMsgs.push({
      role: 'system',
      content:
        'CHARACTER GENERATION ONLY (not a play turn). Ignore any "DM reply envelope", "narration", or in-scene JSON shape. Your entire reply MUST be one JSON object whose only top-level key is "playerCharacter", exactly as specified in the following skill. No markdown fences, no extra keys, no prose outside JSON.',
    });
    try {
      const langFile = language && language.toLowerCase().startsWith('span') ? 'language_spanish.txt' : 'language_english.txt';
      const langPrompt = loadPrompt(langFile);
      if (langPrompt) systemMsgs.push({ role: 'system', content: langPrompt });
    } catch (e) {
      /* ignore */
    }

    const charPrompt = loadPrompt('skill_character.txt');
    if (charPrompt) {
      let langInstruction = '';
      try {
        if (language && language.toLowerCase().startsWith('span')) {
          const lp = loadPrompt('language_spanish.txt');
          if (lp) langInstruction = lp;
        }
      } catch (e) {
        console.warn('Failed to load language prompt for character generation:', e);
      }
      try {
        const renderedCharPrompt = Mustache.render(charPrompt, { languageInstruction: langInstruction, language });
        systemMsgs.push({ role: 'system', content: renderedCharPrompt });
      } catch (e) {
        console.error('generate-character: Mustache render failed for skill_character system section:', e);
        return res.status(500).json({
          error: 'generate-character: failed to render skill_character.txt (check Mustache placeholders).',
          details: String(e && e.message ? e.message : e),
          code: 'CHARACTER_PROMPT_RENDER_FAILED',
        });
      }
    }

    const consolidated = consolidateSystemMessages(systemMsgs);

    // User message comes only from the marked section in skill_character.txt (no alternate files or inline fallbacks).
    const fullCharTpl = loadPrompt('skill_character.txt');
    const userMarker = '--- USER PROMPT BELOW (render this section as the user message) ---';
    if (!fullCharTpl || !fullCharTpl.includes(userMarker)) {
      return res.status(500).json({
        error: 'generate-character: skill_character.txt must contain the USER PROMPT marker; no fallback prompt is used.',
      });
    }
    const userTpl = fullCharTpl.split(userMarker)[1].trim();
    if (!userTpl) {
      return res.status(500).json({
        error: 'generate-character: skill_character.txt user section after marker is empty.',
      });
    }
    const langFileUser = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
    let languageInstructionForTemplate = '';
    try {
      const lp = loadPrompt(langFileUser);
      if (lp) languageInstructionForTemplate = lp;
    } catch (e) {
      /* ignore */
    }
    let userContent;
    try {
      userContent = Mustache.render(userTpl, {
        gameSetup: safeJsonStringifyForPrompt(gameSetup),
        languageInstruction: languageInstructionForTemplate,
        language,
      });
    } catch (e) {
      console.error('generate-character: Mustache render failed for user template:', e);
      return res.status(500).json({ error: 'generate-character: failed to render user prompt template.', details: String(e) });
    }

    const messagesToSend = [{ role: 'system', content: consolidated }, { role: 'user', content: userContent }];
    const charOutbound = traceMessages(
      messagesToSend,
      'player character sheet JSON; DM core slice; character generation skill; user: partial build + structured stats'
    );

    const completionBudget = Math.min(2000, Math.max(800, parseInt(process.env.DM_CHARACTER_MAX_TOKENS || '1400', 10) || 1400));
    let aiMessage = await generateResponse({ messages: charOutbound }, { max_tokens: completionBudget, temperature: 0.75, gameId });
    if (!aiMessage) {
      const detail = getLastGenerateFailureMessage() || 'No text from model.';
      const useLm = String(process.env.DM_USE_LM_STUDIO || '').toLowerCase() === 'true';
      const hint = useLm
        ? `LM Studio mode (DM_USE_LM_STUDIO=true). Start LM Studio, load a model, and check ${process.env.DM_LM_STUDIO_URL || 'http://localhost:1234'} — try Server Settings → API → OpenAI-compatible /v1/chat/completions.`
        : 'Using OpenAI: set DM_OPENAI_API_KEY in server/.env and ensure the key is valid.';
      return res.status(503).json({
        error: 'Character generation failed: the model returned no usable text.',
        code: 'AI_RESPONSE_EMPTY',
        detail,
        hint,
      });
    }

    function tryParsePlayerCharacterBlob(text) {
      const cleaned = normalizeJsonLikeQuotes(stripMarkdownJsonFence(String(text || '')));
      const jsonText = extractFirstJsonObject(cleaned);
      if (!jsonText) return null;
      try {
        return JSON.parse(jsonText);
      } catch (pe) {
        console.warn('JSON.parse failed on character generator blob:', pe?.message || pe);
        return null;
      }
    }

    const parsed = tryParsePlayerCharacterBlob(aiMessage);
    console.log('Character generator raw output preview:', String(aiMessage).slice(0, 2000));

    if (parsed && parsed.playerCharacter) {
      const {
        validateGeneratedPlayerCharacter,
        normalizeGeneratedPlayerCharacter,
      } = require('../validatePlayerCharacter');
      parsed.playerCharacter = normalizeGeneratedPlayerCharacter(parsed.playerCharacter, { language });
      const charCheck = validateGeneratedPlayerCharacter(parsed.playerCharacter);
      if (!charCheck.ok) {
        console.warn('generate-character: validation failed:', charCheck.error);
        try {
          if (gameId) {
            await require('../models/GameState').findOneAndUpdate(
              { gameId },
              { rawModelOutput: String(aiMessage).slice(0, 200000) },
              { upsert: true, new: true }
            );
          }
        } catch (pe) {
          console.warn('Failed to persist rawModelOutput after character validation failure:', pe);
        }
        return res.status(422).json({ error: charCheck.error, code: 'INVALID_PLAYER_CHARACTER' });
      }
      // Persist generated character into GameState so clients loading the game will see it.
      try {
        if (gameId) {
          const GameState = require('../models/GameState');
          await GameState.findOneAndUpdate(
            { gameId },
            { $set: { 'gameSetup.generatedCharacter': parsed.playerCharacter } },
            { upsert: true }
          );
        }
      } catch (pe) {
        console.warn('Failed to persist generatedCharacter to GameState:', pe);
      }
      try {
        return res.json(parsed);
      } catch (serErr) {
        console.error('generate-character: res.json(parsed) failed:', serErr);
        return res.status(500).json({
          error: 'Generated character could not be sent as JSON (non-serializable values?).',
          code: 'CHARACTER_RESPONSE_SERIALIZE_FAILED',
          detail: String(serErr && serErr.message ? serErr.message : serErr),
        });
      }
    } else {
      const rawStr = String(aiMessage || '');
      const preview = rawStr.slice(0, 8000);
      try {
        if (gameId) {
          await require('../models/GameState').findOneAndUpdate(
            { gameId },
            { rawModelOutput: rawStr.slice(0, 200000) },
            { upsert: true, new: true }
          );
        }
      } catch (pe) {
        console.warn('Failed to persist rawModelOutput after character parse failure:', pe);
      }
      return res.status(422).json({
        error:
          'The model did not return valid JSON with a top-level playerCharacter object. Check server logs for a preview.',
        code: 'INVALID_MODEL_JSON',
        preview,
        previewLength: rawStr.length,
      });
    }
  } catch (e) {
    console.error('Error generating character:', e);
    res.status(500).json({
      error: String(e && e.message ? e.message : e),
      code: 'GENERATE_CHARACTER_EXCEPTION',
      stack: process.env.NODE_ENV === 'development' || process.env.NODE_ENV === 'dev' ? String(e && e.stack) : undefined,
    });
  }
});

/** One-shot persist after setup (system message + campaign shell). Not named /save — clients must not POST /api/game-state/save. */
router.post('/bootstrap-session', async (req, res) => {
  try {
    const gs = await persistGameStateFromBody(req.body);
    const o = typeof gs.toObject === 'function' ? gs.toObject() : { ...gs };
    if (o.campaignSpec) o.campaignSpec = redactCampaignSpecForClient(o.campaignSpec);
    res.json(o);
  } catch (e) {
    console.error('bootstrap-session failed:', e);
    res.status(400).json({ error: String(e && e.message ? e.message : e) });
  }
});

module.exports = router;