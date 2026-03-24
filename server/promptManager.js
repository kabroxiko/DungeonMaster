const fs = require('fs');
const path = require('path');

const clientPromptsDir = path.join(__dirname, '../client/gamemasterai/src/prompts');
const serverPromptsDir = path.join(__dirname, 'prompts');
const cache = {};

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
    if (includeFullSkill && skillContent) {
      msgs.push({ role: 'system', content: skillContent });
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

module.exports = { composeSystemMessages, loadPrompt };
/*
 * Simple heuristic to detect current mode from recent conversation messages.
 * Prioritizes combat detection, then investigation, then decision. Defaults to exploration.
 */
function detectMode(conversation = []) {
  if (!Array.isArray(conversation)) return 'exploration';

  const recent = conversation.slice(-12).map(m => (m.content || '').toLowerCase()).join('\n');

  // Combat keywords
  const combatRe = /\b(attack|attacks|attack roll|initiative|combat|hit for|damage|hit|miss|armor class|ac|attack roll|critical)\b/;
  if (combatRe.test(recent)) return 'combat';

  // Investigation keywords
  const investRe = /\b(investigat|search|clue|examine|inspect|percept|forensic|evidence|trace)\b/;
  if (investRe.test(recent)) return 'investigation';

  // Decision / choice keywords
  const decisionRe = /\b(choose|choose one|option|which do you|do you want to|what do you do|decide)\b/;
  if (decisionRe.test(recent)) return 'decision';

  // Initial/adventure seed detection (rare)
  const initialRe = /\b(adventure|hook|seed|begin|start of your adventure|two-sentence)\b/;
  if (initialRe.test(recent)) return 'initial';

  return 'exploration';
}

module.exports = { composeSystemMessages, loadPrompt, detectMode };

