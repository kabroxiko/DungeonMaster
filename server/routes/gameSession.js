//GMAI/server/routes/gameSession.js

const express = require('express');
const router = express.Router();
const { generateResponse } = require('../openai-api');
const { composeSystemMessages, loadPrompt } = require('../promptManager');
const Mustache = require('mustache');

const DEFAULT_MODEL = process.env.GM_OPENAI_MODEL || 'gpt-3.5-turbo';

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

// Route to generate AI Dungeon Master and campaign generating responses
router.post('/generate', async (req, res) => {
    // Extract parameters from the request body
    const { messages = [], mode = 'exploration', sessionSummary = '', includeFullSkill = false, language = 'English', gameId = null } = req.body;

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

    // If mode not provided, try to auto-detect from the conversation
    let resolvedMode = mode;
    if (!resolvedMode || resolvedMode === 'exploration') {
        const { detectMode } = require('../promptManager');
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

    // Load persisted GameState early when available so we can reuse stored consolidated system core
    let persistedSystemCore = null;
    let persistedGameState = null;
    if (gameId) {
      try {
        const GameState = require('../models/GameState');
        const gs = await GameState.findOne({ gameId }).select('+campaignSpec +systemCore +rawModelOutput +gameSetup +summary');
        if (gs) {
          persistedGameState = gs;
          if (gs.systemCore) persistedSystemCore = gs.systemCore;
        }
      } catch (e) {
        console.warn('Failed to load GameState for generate:', e);
      }
    }

    // Build system messages. If a persisted consolidated system core exists, do not recompose core messages;
    // only include the dynamic skill/adventure seed and GM injections below. Otherwise compose the full core.
    let systemMsgs = [];
    if (!persistedSystemCore) {
      systemMsgs = composeSystemMessages({ mode: resolvedMode, sessionSummary: sessionSummaryToUse, includeFullSkill, language }).filter(m => m.role === 'system');
    }

    // Ensure the full adventure-skill prompt is included for initial scenes so the model receives the seed template
    if (resolvedMode === 'initial') {
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
          // push as a dynamic system message (these will be combined with persisted core if present)
          systemMsgs.push({ role: 'system', content: advSeed });
        }
      } catch (e) {
        console.warn('Failed to load skill_adventureSeed.txt for initial mode:', e);
      }
    }

    // If a campaignSpec is available from persisted GameState, render GM-only injections from it
    if (persistedGameState && persistedGameState.campaignSpec) {
      const spec = persistedGameState.campaignSpec;
      // Helper to robustly take the first N items from a field that may be an array, object, or string.
      const takeItems = (field, n) => {
        if (!field) return [];
        if (Array.isArray(field)) return field.slice(0, n);
        if (typeof field === 'object') {
          try {
            return Object.values(field).slice(0, n);
          } catch (e) {
            return [];
          }
        }
        // primitive (string/number) -> wrap
        return [field].slice(0, n);
      };
      try {
        let injectTemplate = null;
        let renderData = {};
        if (resolvedMode === 'initial' && spec) {
          injectTemplate = loadPrompt('gm_inject_initial.txt');
          renderData = {
            campaignConcept: spec.campaignConcept || '',
            factions: takeItems(spec.factions, 3),
            majorConflicts: takeItems(spec.majorConflicts, 4),
            majorNPCs: takeItems(spec.majorNPCs, 4),
            keyLocations: takeItems(spec.keyLocations, 4),
            campaignSpecJson: JSON.stringify(spec || {}, null, 2),
            rawModelOutput: String(persistedGameState.rawModelOutput || '').slice(0, 20000),
            gameSetup: JSON.stringify(persistedGameState.gameSetup || {}, null, 2),
            sessionSummary: persistedGameState.summary || sessionSummary || ''
          };
        } else if ((resolvedMode === 'exploration' || resolvedMode === 'explore') && spec) {
          injectTemplate = loadPrompt('gm_inject_explore.txt');
          renderData = { factions: takeItems(spec.factions, 3) };
        } else if ((resolvedMode === 'combat' || resolvedMode === 'decision' || resolvedMode === 'investigation') && spec) {
          injectTemplate = loadPrompt('gm_inject_combat.txt');
          renderData = {
            majorNPCs: takeItems(spec.majorNPCs, 4),
            majorConflicts: takeItems(spec.majorConflicts, 4),
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
    // Consolidate all system messages into one system-role prompt to ensure guard precedence
    const consolidatedSystem = consolidateSystemMessages(systemMsgs);
    const messagesToSend = [{ role: 'system', content: consolidatedSystem }, ...inboundMessages];
    // Debug: log the consolidated system message and outbound messages
    try {
      console.log('DEBUG: consolidated system (generate):', consolidatedSystem);
      console.log('DEBUG: messagesToSend (generate):', JSON.stringify(messagesToSend, null, 2));
    } catch (e) {
      console.log('DEBUG: messagesToSend (generate) - could not stringify', e);
    }

    // Use central generateResponse (handles model selection and fallbacks)
    try {
        // Dynamically estimate prompt size and compute a safe completion token budget.
        // Conservative model context window assumption:
        const MODEL_MAX_TOKENS = 4000;
        const promptTokens = estimateTokenCount(messagesToSend);
        let completionBudget = resolvedMode === 'initial' ? 800 : 500;
        // Determine available tokens for completion (leave a small margin)
        const available = MODEL_MAX_TOKENS - promptTokens - 50;
        if (available <= 0) {
          completionBudget = 100;
        } else {
          // Aim for a reasonable completion but don't exceed available. Use a higher minimum to reduce truncation.
          completionBudget = Math.min(Math.max(completionBudget, 500), Math.min(1500, available));
        }
        console.log(`Estimated prompt tokens: ${promptTokens}, using completion budget: ${completionBudget}`);
    // Persist outgoing request for debugging if gameId supplied (trim to cap)
    try {
      if (gameId) {
        const GameState = require('../models/GameState');
        await GameState.findOneAndUpdate({ gameId }, { rawModelRequest: JSON.stringify(messagesToSend).slice(0, 200000) }, { upsert: true });
      }
    } catch (e) {
      console.warn('Failed saving rawModelRequest for generate:', e);
    }

    // Log clearly that we are about to call the LLM for the generate route and persist timestamps
    try {
      console.log('OUTGOING (generate) messagesToSend:', JSON.stringify(messagesToSend, null, 2));
    } catch (e) {
      console.log('OUTGOING (generate) messagesToSend (could not stringify)', messagesToSend);
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
      aiMessage = await generateResponse({ messages: messagesToSend }, { max_tokens: completionBudget, temperature: 0.8, gameId });
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

    // Persist raw model output for debugging if gameId supplied
    try {
      if (gameId && typeof aiMessage !== 'undefined' && aiMessage !== null) {
        const GameState = require('../models/GameState');
        await GameState.findOneAndUpdate({ gameId }, { rawModelOutput: String(aiMessage).slice(0, 200000), $set: { llmCallCompletedAt: new Date().toISOString() } }, { upsert: true });
      }
    } catch (e) {
      console.warn('Failed saving rawModelOutput for generate:', e);
    }

    if (!aiMessage) {
        console.error('LLM returned no content for generate');
        return res.status(500).json({ error: 'AI response was empty or failed (see server logs).' });
    }
        // If the model likely truncated mid-sentence, try a short continuation prompt (up to 2 retries).
        function isLikelyTruncated(text) {
          if (!text || typeof text !== 'string') return false;
          const trimmed = text.trim();
          if (!trimmed) return false;
          // If it ends with an ellipsis or with an incomplete last character, consider it truncated.
          if (/\.\.\.$/.test(trimmed)) return true;
          // If last character is not a strong sentence terminator, consider truncated.
          const last = trimmed.slice(-1);
          if (!/[\.!\?\"'”\)\]\}]/.test(last)) return true;
          return false;
        }
        let finalMessage = aiMessage;
        if (isLikelyTruncated(aiMessage)) {
          try {
            // Build a continuation user message (no prefatory text).
            const contUser = { role: 'user', content: 'Continue the previous narrative from where it stopped. Output only the continuation.' };
            // Use a modest token budget for continuation.
            const contResp = await generateResponse({ messages: [ { role: 'system', content: consolidatedSystem }, contUser ] }, { max_tokens: 600, temperature: 0.8, gameId });
            if (contResp) {
              finalMessage = String(aiMessage) + '\n' + String(contResp);
            }
          } catch (e) {
            console.warn('Continuation attempt failed:', e);
          }
        }
        // Return raw model output; formatting should be handled by prompts
        console.log('AI DM processed:', finalMessage);
        res.json(finalMessage);
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
    // Debug: log the consolidated system message and outbound messages
    try {
      console.log('DEBUG: consolidated system (generate-campaign):', consolidatedCampaignSystem);
      console.log('DEBUG: messagesToSend (generate-campaign):', JSON.stringify(messagesToSend, null, 2));
    } catch (e) {
      console.log('DEBUG: messagesToSend (generate-campaign) - could not stringify', e);
    }

    try {
        // Estimate prompt size and choose a completion budget so output won't be truncated.
        const MODEL_MAX_TOKENS = 4000;
        const promptTokensCampaign = estimateTokenCount(messagesToSend);
        let completionBudgetCampaign = 700;
        const availableCampaign = MODEL_MAX_TOKENS - promptTokensCampaign - 50;
        if (availableCampaign <= 0) {
          completionBudgetCampaign = 100;
        } else {
          // Use a higher minimum to reduce risk of truncation for campaign generation.
          completionBudgetCampaign = Math.min(
            1500,
            Math.max(600, Math.min(availableCampaign, completionBudgetCampaign + (hiddenPlotline ? 200 : 0)))
          );
        }
        console.log(`Campaign generator: prompt tokens ${promptTokensCampaign}, completion budget ${completionBudgetCampaign}`);
        const aiMessage = await generateResponse({ messages: messagesToSend }, { max_tokens: completionBudgetCampaign, temperature: 0.8, gameId });
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

        // Return parsed campaign JSON (campaignConcept) or raw AI output
        res.json(parsed || aiMessage);
    } catch (error) {
        console.error('Error generating text:', error);
        res.status(500).json({ error: `Error generating text: ${String(error)}` });
    }
});

// New: generate only core campaign spec (small, reliable output)
router.post('/generate-campaign-core', async (req, res) => {
  const { gameSetup = {}, sessionSummary = '', language = 'English', gameId = null, waitForStages = true } = req.body;
  console.log('Campaign core generator called');

  // Prepare system messages (guards + core guidance)
  // Load campaign-core policy from prompt file so this behavior is editable (do NOT hardcode)
  const baseSystemMsgs = composeSystemMessages({ mode: 'initial', sessionSummary, includeFullSkill: false, language }).filter(m => m.role === 'system');
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
    userPromptRendered = Mustache.render(template, { sessionSummary: sessionSummary || '', languageInstruction: languageInstructionForTemplate, language });
  } catch (e) {
    console.error('Failed rendering campaign generator prompt template:', e);
    return res.status(500).json({ error: 'Failed rendering campaign generator prompt' });
  }
  const messagesToSend = [{ role: 'system', content: consolidated }, { role: 'user', content: userPromptRendered }];
  // Log the exact messages being sent to the model for debugging (redactable if needed).
  try {
    console.log('OUTGOING (campaign-core) messagesToSend:', JSON.stringify(messagesToSend, null, 2));
  } catch (e) {
    console.log('OUTGOING (campaign-core) messagesToSend (could not stringify):', messagesToSend);
  }

  try {
    // Persist outgoing request for debugging if gameId supplied (trim to cap)
    try {
      if (gameId) {
        const GameState = require('../models/GameState');
        await GameState.findOneAndUpdate({ gameId }, { rawModelRequest: JSON.stringify(messagesToSend).slice(0, 200000) }, { upsert: true });
      }
    } catch (e) {
      console.warn('Failed saving rawModelRequest for campaign-core:', e);
    }

    const aiMessage = await generateResponse({ messages: messagesToSend }, { max_tokens: 900, temperature: 0.8, gameId });
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

    // If caller requested to wait for background stages (default true), run them synchronously and fail fast on error.
    if (gameId && waitForStages) {
      // Helper to run a stage with timeout and stop waiting on first failure.
      const STAGE_TIMEOUT = process.env.GM_STAGE_TIMEOUT_MS ? parseInt(process.env.GM_STAGE_TIMEOUT_MS, 10) : 15000;
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
          return res.json(combined);
        } catch (e) {
          console.warn('Failed persisting combined campaignSpec after stages:', e);
          return res.json(parsed);
        }
      } catch (e) {
        console.error('Synchronous campaign stages failed:', e);
        return res.status(500).json({ error: 'Background campaign stages failed' });
      }
    }
    // Default: respond immediately and generate stages asynchronously (not used when waitForStages=true)
    res.json(parsed);

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
        // fallback to inline prompts (legacy)
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
    // Log the exact messages being sent to the model for debugging (redactable if needed).
    try {
      console.log(`OUTGOING (stage:${stage}) messagesToSend:`, JSON.stringify(messagesToSend, null, 2));
    } catch (e) {
      console.log(`OUTGOING (stage:${stage}) messagesToSend (could not stringify):`, messagesToSend);
    }
    const aiMessage = await generateResponse({ messages: messagesToSend }, { max_tokens: 800, temperature: 0.8, gameId });
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

    // Persist into campaignSpec.<stage>
    try {
      const update = {};
      update[`campaignSpec.${stage}`] = parsed;
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

// Route to generate game session summary   
router.post('/generate-summary', async (req, res) => {
    try {
        // Extract the messages and the existing summary from the request body
        const { messages = [], sessionSummary = '', language = 'English' } = req.body;

        // Compose a concise system message to drive summarization
        const inboundSummaryMessages = (messages || []).filter(m => m.role !== 'system');
    const systemMsgs = composeSystemMessages({ mode: 'exploration', sessionSummary, includeFullSkill: false, language });
    // Add a concise summarization instruction according to language
    const summaryInstruction = language === 'Spanish'
      ? '*Todo lo anterior fue una transcripción de una partida de rol. Resume los eventos descritos en esta transcripción. Sé conciso (menos de 75 palabras) y objetivo. Toma nota de personajes, lugares y objetos importantes. Usa tercera persona.*'
      : '*Everything above is a transcript of a TTRPG session. Summarize the events concisely (under 75 words), noting important NPCs, locations, and unresolved threads. Use third person.*';
    systemMsgs.push({ role: 'system', content: summaryInstruction });
    const messagesToSend = [...systemMsgs, ...inboundSummaryMessages];

        const aiSummary = await generateResponse({ messages: messagesToSend }, { max_tokens: 150, temperature: 0.8, gameId });
        if (!aiSummary) {
            return res.status(500).json({ error: 'AI summary was empty or failed (see server logs).' });
        }
        // Return raw summary from the model; prompts must enforce style and language
        res.json(aiSummary);
    } catch (error) {
        console.error('Error generating text:', error);
        res.status(500).json({ error: `Error generating text: ${String(error)}` });
    }

});

// Export the router to be used in other files
module.exports = router;

// Route to generate only a playerCharacter (separate from campaign generation)
router.post('/generate-character', async (req, res) => {
  const { gameSetup = {}, sessionSummary = '', language = 'English', gameId = null } = req.body;

  // Enforce campaign-first policy: require an existing campaignSpec for this gameId.
  if (!gameId) {
    return res.status(400).json({ error: 'generate-character requires gameId. Generate campaign first and provide gameId.' });
  }
  try {
    const GameState = require('../models/GameState');
    const gsCheck = await GameState.findOne({ gameId }).select('+campaignSpec');
    if (!gsCheck || !gsCheck.campaignSpec) {
      return res.status(400).json({ error: 'No campaignSpec found for this gameId. Please generate the campaign core before generating characters.' });
    }
  } catch (e) {
    console.warn('Failed to verify campaignSpec for generate-character:', e);
    return res.status(500).json({ error: 'Failed to verify campaign state before character generation.' });
  }

  try {
    // Compose system messages and include the character-generation skill prompt
    let systemMsgs = composeSystemMessages({ mode: 'initial', sessionSummary, includeFullSkill: false, language });
    // Remove assistant-role entries
    systemMsgs = (systemMsgs || []).filter(m => m.role === 'system');

    // Add the character-generation prompt, rendered with language instructions via Mustache.
    const charPrompt = loadPrompt('skill_character.txt');
    if (charPrompt) {
      // Use existing language prompt templates (do not hardcode language strings).
      let langInstruction = '';
      try {
        if (language && language.toLowerCase().startsWith('span')) {
          const lp = loadPrompt('language_spanish.txt');
          if (lp) langInstruction = lp;
        }
      } catch (e) {
        console.warn('Failed to load language prompt for character generation:', e);
      }
      const renderedCharPrompt = Mustache.render(charPrompt, { languageInstruction: langInstruction, language });
      systemMsgs.push({ role: 'system', content: renderedCharPrompt });
    }

    // Consolidate into one system message
    const consolidated = consolidateSystemMessages(systemMsgs);

    // Build user instruction by extracting the USER section from skill_character.txt (keeps phrasing editable)
    let userContent = '';
    try {
      const fullCharTpl = loadPrompt('skill_character.txt');
      const marker = '--- USER PROMPT BELOW (render this section as the user message) ---';
      let userTpl = null;
      if (fullCharTpl && fullCharTpl.includes(marker)) {
        userTpl = fullCharTpl.split(marker)[1].trim();
      } else {
        // fallback to separate file if present
        userTpl = loadPrompt('skill_character_user.txt');
      }
      const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
      let languageInstructionForTemplate = '';
      try {
        const langPrompt = loadPrompt(langFile);
        if (langPrompt) languageInstructionForTemplate = langPrompt;
      } catch (e) {}
      if (userTpl) {
        userContent = Mustache.render(userTpl, { gameSetup: JSON.stringify(gameSetup), languageInstruction: languageInstructionForTemplate, language });
      } else {
        const langPrompt = languageInstructionForTemplate ? languageInstructionForTemplate + '\n\n' : '';
        userContent = `${langPrompt}Using the following partial character info (may be empty): ${JSON.stringify(gameSetup)}\n\nReturn ONLY valid JSON with a top-level key "playerCharacter" whose value is an object containing: name, race, class, subclass (optional), level (set to 1), background, brief_backstory (2-3 sentences), stats {STR,DEX,CON,INT,WIS,CHA}, max_hp, ac, starting_equipment (array). Do not include any other keys or commentary.`;
      }
    } catch (e) {
      console.warn('Failed to render character user prompt, falling back:', e);
      userContent = `Using the following partial character info (may be empty): ${JSON.stringify(gameSetup)}\n\nReturn ONLY valid JSON with a top-level key "playerCharacter"...`;
    }
    const messagesToSend = [{ role: 'system', content: consolidated }, { role: 'user', content: userContent }];

    // Use a fixed, conservative completion budget for character generation (no token estimation needed)
    const completionBudget = 400;
    const aiMessage = await generateResponse({ messages: messagesToSend }, { max_tokens: completionBudget, temperature: 0.8, gameId });
    if (!aiMessage) return res.status(500).json({ error: 'AI response empty' });

    // Parse JSON (use balanced-brace extractor to avoid greedy regex issues)
    let parsed = null;
    try {
      const raw = String(aiMessage || '');
      console.log('Character generator raw output preview:', raw.slice(0, 2000));
      const jsonText = extractFirstJsonObject(raw);
      if (jsonText) {
        try {
          parsed = JSON.parse(jsonText);
        } catch (pe) {
          console.warn('JSON.parse failed on extracted JSON from character generator:', pe);
          parsed = null;
        }
      } else {
        console.warn('No JSON object found in AI response for character generator.');
      }
    } catch (e) {
      console.warn('Failed to parse JSON from character generator (unexpected):', e);
    }

    // Do NOT perform automatic retries. If the initial response cannot be parsed into a valid
    // playerCharacter object, persist raw output for debugging and fail fast.
    if (parsed && parsed.playerCharacter) {
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
      return res.json(parsed);
    } else {
      try {
        if (gameId) {
          await require('../models/GameState').findOneAndUpdate(
            { gameId },
            { rawModelOutput: String(aiMessage).slice(0, 200000) },
            { upsert: true, new: true }
          );
        }
      } catch (pe) {
        console.warn('Failed to persist rawModelOutput after character parse failure:', pe);
      }
      return res.status(500).json({ error: 'Failed to generate valid playerCharacter', raw: aiMessage });
    }
  } catch (e) {
    console.error('Error generating character:', e);
    res.status(500).json({ error: String(e) });
  }
});