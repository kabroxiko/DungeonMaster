//GMAI/server/routes/gameSession.js

const express = require('express');
const router = express.Router();
const { generateResponse } = require('../openai-api');
const { composeSystemMessages, loadPrompt } = require('../promptManager');

const DEFAULT_MODEL = process.env.OPENAI_MODEL || 'gpt-3.5-turbo';

// Note: Output formatting and presentation should be enforced via prompts.

// (Name generation moved to AI: server will not invent character names)

// Route to generate AI Dungeon Master and campaign generating responses
router.post('/generate', async (req, res) => {
    // Extract parameters from the request body
    const { messages = [], mode = 'exploration', sessionSummary = '', includeFullSkill = false, language = 'English' } = req.body;

    console.log('AI DM Processing the following messages (mode:', mode, ')');
    console.log(messages);

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

    // Strip any client-sent system messages to avoid conflicting system-level instructions
    const inboundMessages = (messages || []).filter(m => m.role !== 'system');

    // Compose system messages based on resolved mode/sessionSummary
    const systemMsgs = composeSystemMessages({ mode: resolvedMode, sessionSummary, includeFullSkill, language });
    const messagesToSend = [...systemMsgs, ...inboundMessages];

    // Use central generateResponse (handles model selection and fallbacks)
    try {
        // Increase token budget for initial-mode openings which are multi-paragraph
        const maxTokens = resolvedMode === 'initial' ? 800 : 300;
        const aiMessage = await generateResponse({ messages: messagesToSend }, { max_tokens: maxTokens, temperature: 0.8 });
        if (!aiMessage) {
            return res.status(500).json({ error: 'AI response was empty or failed (see server logs).' });
        }
        // Return raw model output; formatting should be handled by prompts
        console.log('AI DM processed:', aiMessage);
        res.json(aiMessage);
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

    // For campaign generation, include the initial/adventure seed skill fully
    const systemMsgs = composeSystemMessages({ mode: 'initial', sessionSummary, includeFullSkill: true, language });
    // Load character generation prompt if available
    const charPrompt = loadPrompt('skill_character.txt');
    if (charPrompt) systemMsgs.push({ role: 'system', content: charPrompt });
    // Load assistant few-shot examples for character generation, language-specific
    try {
      const exFile = language && language.toLowerCase() === 'spanish' ? 'skill_character_examples_es.txt' : 'skill_character_examples_en.txt';
      const charExamples = loadPrompt(exFile);
      if (charExamples) systemMsgs.push({ role: 'assistant', content: charExamples });
    } catch (e) {
      // ignore
    }

    // Create a single user instruction that provides the gameSetup and requests a JSON output
    const userInstruction = {
        role: 'user',
        content:
            `Using the following partial character info (may be empty): ${JSON.stringify(gameSetup)}\n` +
            `Also generate a short campaign concept (2 sentences). Output MUST be valid JSON with keys "campaignConcept" (string) and "playerCharacter" (object with name, race, class, subclass, level, background, brief_backstory, stats, max_hp, ac, starting_equipment). Fill any missing character fields with sensible random choices. Do not include any text outside the JSON.`,
    };

    // Language is handled via prompt files loaded by promptManager; no hardcoded language rules here.

    const messagesToSend = [...systemMsgs, userInstruction];

    try {
        const aiMessage = await generateResponse({ messages: messagesToSend }, { max_tokens: 700, temperature: 0.8 });
        if (!aiMessage) {
            return res.status(500).json({ error: 'AI response was empty or failed (see server logs).' });
        }

        // Try to parse JSON from the response; if it fails, return raw string
        let parsed = null;
        try {
            // Some models return JSON inside markdown; extract the first JSON object
            const jsonMatch = aiMessage.match(/(\{[\s\S]*\})/);
            const jsonText = jsonMatch ? jsonMatch[0] : aiMessage;
            parsed = JSON.parse(jsonText);

            // Leave level enforcement to the character-generation prompt (do not hardcode here).
        } catch (e) {
            console.warn('Failed to parse JSON from campaign generator:', e);
        }
        // If parsed exists but lacks playerCharacter, attempt a focused retry to obtain the character JSON
        if ((!parsed || !parsed.playerCharacter) && charPrompt) {
            try {
                const retrySystem = composeSystemMessages({ mode: 'initial', sessionSummary, includeFullSkill: false, language });
                // include the character-generation spec prompt
                retrySystem.push({ role: 'system', content: charPrompt });
                const retryUser = {
                    role: 'user',
                    content:
                        'The previous response did not include a "playerCharacter" object. Please RETURN ONLY valid JSON with a top-level key "playerCharacter" whose value is an object containing: name, race, class, subclass (optional), level (set to 1), background, brief_backstory (2-3 sentences), stats {STR,DEX,CON,INT,WIS,CHA}, max_hp, ac, starting_equipment (array). Fill any missing fields sensibly based on the campaign concept.'
                };
                const retryResp = await generateResponse({ messages: [...retrySystem, retryUser] }, { max_tokens: 400, temperature: 0.8 });
                if (retryResp) {
                    const jsonMatch2 = retryResp.match(/(\{[\s\S]*\})/);
                    const jsonText2 = jsonMatch2 ? jsonMatch2[0] : retryResp;
                    const parsed2 = JSON.parse(jsonText2);
                    if (parsed2 && parsed2.playerCharacter) {
                        parsed = parsed || {};
                        parsed.playerCharacter = parsed2.playerCharacter;
                    }
                }
            } catch (e) {
                console.warn('Retry to obtain playerCharacter failed:', e);
            }
        }

        // Do not modify or invent character names here; return parsed model output as-is.

        res.json(parsed || aiMessage);
    } catch (error) {
        console.error('Error generating text:', error);
        res.status(500).json({ error: `Error generating text: ${String(error)}` });
    }
});

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

        const aiSummary = await generateResponse({ messages: messagesToSend }, { max_tokens: 150, temperature: 0.8 });
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