const fs = require('fs');
const path = require('path');
const Mustache = require('mustache');

const clientPromptsDir = path.join(__dirname, '../client/dungeonmaster/src/prompts');
const serverPromptsDir = path.join(__dirname, 'prompts');
const cache = {};

/** Explicit violent / attack intent on the latest user line (Spanish + English). */
const LAST_USER_COMBAT_RE =
  /\b(ataco|atacar|ataca|atacáis|golpeo|golpear|golpea|golpeas|disparo|disparar|dispara|apuñalo|apuñalar|puñetazo|pateo|patear|empujo|empujar|arrojo|arrojar|lanzo|lanzar|desenvaino|desenvainar|desenfundo|desenfundar|mato|matar|hiereo|herir|derribo|derribar|acuchillo|acuchillar|corto|rajo|peleo|pelear|lucho|luchar|combate|iniciativa|daño|armadura|clase de armadura|tirada de ataque|tiro para golpear|bonificador de ataque|impacto|ajustar cuentas)\b|\b(attack|attacks|attacking|stab|stabs|shoot|shooting|punch|kick|draw my|i draw|fire at|swing at|hit him|hit her|slash|charge at|grapple|shove|combat|initiative|damage dealt|attack roll|roll to hit)\b/i;

function userMessageLooksCombat(text) {
  if (text == null || text === '') return false;
  return LAST_USER_COMBAT_RE.test(String(text).toLowerCase());
}

/** Strip accents for loose substring match (Spanish sheet names vs player typing). */
function normalizeForWeaponMatch(s) {
  return String(s || '')
    .toLowerCase()
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '');
}

/**
 * True if the player message names at least one weapon from the sheet (substring match on weapon name words).
 */
function userMessageNamesWeaponFromSheet(userText, weapons) {
  if (!userText || !Array.isArray(weapons)) return false;
  const t = normalizeForWeaponMatch(userText);
  for (const w of weapons) {
    const rawName = String((w && w.name) || '').trim();
    if (!rawName) continue;
    const stripped = rawName.replace(/^\d+\s*[x×]\s*/i, '').trim();
    const n = normalizeForWeaponMatch(stripped);
    if (n.length >= 2 && t.includes(n)) return true;
    const words = n.split(/\s+/).filter((x) => x.length >= 3);
    for (const word of words) {
      if (t.includes(word)) return true;
    }
  }
  return false;
}

/**
 * When the PC has more than one weapon row, a vague attack ("ataco", "I attack") must NOT start combat until they name a weapon.
 */
function blocksCombatEntryForAmbiguousWeapon(userText, generatedCharacter) {
  if (!userMessageLooksCombat(userText)) return false;
  const weapons =
    generatedCharacter && typeof generatedCharacter === 'object' && Array.isArray(generatedCharacter.weapons)
      ? generatedCharacter.weapons
      : [];
  if (weapons.length === 0) return true;
  if (weapons.length === 1) return false;
  return !userMessageNamesWeaponFromSheet(String(userText), weapons);
}

function languageInstructionForCompose(language) {
  const langFile = language && String(language).toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
  return loadPrompt(langFile) || '';
}

function renderSkillPrompt(skillContent, language) {
  if (!skillContent || !skillContent.includes('{{')) return skillContent;
  const languageInstruction = languageInstructionForCompose(language);
  return Mustache.render(skillContent, { languageInstruction, language: language || 'English' });
}

function loadPrompt(filename) {
  if (cache[filename]) return cache[filename];
  const serverPath = path.join(serverPromptsDir, filename);
  const clientPath = path.join(clientPromptsDir, filename);
  try {
    // Prefer authoritative prompt files located on the server (keep AI prompt text centralized)
    if (fs.existsSync(serverPath)) {
      const content = fs.readFileSync(serverPath, 'utf8').trim();
      cache[filename] = content;
      return content;
    }
    if (fs.existsSync(clientPath)) {
      const content = fs.readFileSync(clientPath, 'utf8').trim();
      cache[filename] = content;
      return content;
    }
    console.warn('Prompt file missing (both server and client):', filename);
    cache[filename] = '';
    return '';
  } catch (e) {
    console.warn('Error loading prompt file:', filename, e);
    cache[filename] = '';
    return '';
  }
}

/**
 * Compose system messages for the model.
 * - mode: 'exploration' | 'combat' | 'investigation' | 'decision' | 'initial'
 * - sessionSummary: short string (optional)
 * - includeFullSkill: boolean - if true, include the full skill prompt; otherwise include only a short reminder
 */
function composeSystemMessages({ mode = 'exploration', sessionSummary = '', includeFullSkill = false, language = 'English' } = {}) {
  const msgs = [];
  // core system always first
  const core = loadPrompt('systemCore.txt');
  if (core) msgs.push({ role: 'system', content: core });

  // style/story
  const style = loadPrompt('styleStory.txt');
  if (style) msgs.push({ role: 'system', content: style });

  // session memory (short)
  if (sessionSummary) {
    const memTemplate = loadPrompt('memory_summary.txt');
    const mem = `${memTemplate}\n\nSession summary: ${sessionSummary}`;
    msgs.push({ role: 'system', content: mem });
  }

  // If we're generating the initial scene and a playerCharacter was provided in sessionSummary,
  // instruct the model NOT to include the character sheet in its reply. The server will display the sheet separately.
  if (mode === 'initial' && sessionSummary) {
    msgs.push({
      role: 'system',
      content:
        'Note: Character data is available to the server. DO NOT include a character sheet or full character stats in your response. Output only the narrative opening (1–2 sentence context + 1–2 sentence hook). The client will render the Character Sheet separately.',
    });
  }

  // skill prompts: include only relevant one
  const skillMap = {
    combat: 'skill_combat.txt',
    investigation: 'skill_investigation.txt',
    decision: 'skill_decision.txt',
    initial: 'skill_adventureSeed.txt',
  };

  const skillFile = skillMap[mode];
  // No special pre-push for initial here; skill prompts are handled in the skillFile block below.
  if (skillFile) {
    const skillContent = loadPrompt(skillFile);
    // Opening adventure seed is merged in gameSession /generate with Mustache (languageInstruction).
    // Embedding skill_adventureSeed here duplicates it and leaves {{{languageInstruction}}} unreplaced.
    const adventureSeedDeferred = mode === 'initial' && skillFile === 'skill_adventureSeed.txt';
    if (adventureSeedDeferred) {
      msgs.push({
        role: 'system',
        content:
          'Mode: initial. Follow the opening-scene instructions in the dedicated adventure-seed system block supplied by the server (do not assume they appear here).',
      });
    } else if (includeFullSkill && skillContent) {
      msgs.push({ role: 'system', content: renderSkillPrompt(skillContent, language) });
    } else {
      // short reminder
      msgs.push({ role: 'system', content: `Mode: ${mode}. Follow the ${mode} guidelines concisely.` });
    }
    // If this is decision-related, include assistant few-shot examples to bias style
    try {
      const decisionExamples = loadPrompt('skill_decision_examples.txt');
      if (decisionExamples && (skillFile === 'skill_decision.txt' || mode === 'initial' || mode === 'decision')) {
        msgs.push({ role: 'assistant', content: decisionExamples });
      }
    } catch (e) {
      // ignore
    }
  }

  // language-specific prompt (e.g., language_spanish.txt or language_english.txt) - add last to ensure it overrides
  try {
    const langFile = language && language.toLowerCase() === 'spanish' ? 'language_spanish.txt' : 'language_english.txt';
    const langPrompt = loadPrompt(langFile);
    if (langPrompt) msgs.push({ role: 'system', content: langPrompt });
  } catch (e) {
    // ignore
  }
  // Append a general length guard to avoid overly long single replies
  try {
    const guard = loadPrompt('length_guard.txt');
    if (guard) msgs.push({ role: 'system', content: guard });
  } catch (e) {
    // ignore
  }
  // decision behavior is enforced globally in systemCore.txt (no explicit option lists)

  // Note: global language and core rules live in systemCore.txt. Skill prompts contain focused guidance.

  return msgs;
}

/*
 * Simple heuristic to detect current mode from recent conversation messages.
 * Prioritizes the latest user line for combat verbs (ES/EN), then recent window.
 */
function lastUserText(conversation = []) {
  if (!Array.isArray(conversation)) return '';
  for (let i = conversation.length - 1; i >= 0; i--) {
    const m = conversation[i];
    if (m && m.role === 'user' && m.content) return String(m.content);
  }
  return '';
}

function detectMode(conversation = []) {
  if (!Array.isArray(conversation)) return 'exploration';

  const lastUser = lastUserText(conversation).toLowerCase();
  if (lastUser && LAST_USER_COMBAT_RE.test(lastUser)) return 'combat';

  const recent = conversation.slice(-12).map(m => (m.content || '').toLowerCase()).join('\n');

  const combatRe =
    /\b(attack|attacks|attack roll|initiative|combat|hit for|damage|hit|miss|armor class|\bac\b|critical|ataco|atacar|combate|daño|golpe|tirada|clase de armadura)\b/;
  if (combatRe.test(recent)) return 'combat';

  const investRe = /\b(investigat|search|clue|examine|inspect|percept|forensic|evidence|trace)\b/;
  if (investRe.test(recent)) return 'investigation';

  const decisionRe = /\b(choose|choose one|option|which do you|do you want to|what do you do|decide)\b/;
  if (decisionRe.test(recent)) return 'decision';

  const initialRe = /\b(adventure|hook|seed|begin|start of your adventure|two-sentence)\b/;
  if (initialRe.test(recent)) return 'initial';

  return 'exploration';
}

module.exports = {
  composeSystemMessages,
  loadPrompt,
  detectMode,
  lastUserText,
  userMessageLooksCombat,
  blocksCombatEntryForAmbiguousWeapon,
};

