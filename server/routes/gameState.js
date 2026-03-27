const express = require('express');
const router = express.Router();
const GameState = require('../models/GameState');
const { generateResponse } = require('../openai-api');
const { loadPrompt } = require('../promptManager');

// Save game state 
router.post('/save', async (req, res) => {
    const {
        gameId,
        gameSetup,
        conversation,
        summaryConversation,
        summary,
        totalTokenCount,
        userAndAssistantMessageCount,
        systemMessageContentDM
    } = req.body;
    // Do NOT auto-extract generatedCharacter from systemMessageContentDM.
    // Persist only what the client explicitly provided in `gameSetup`.
    let finalGameSetup = gameSetup || {};

    const update = {
        gameId,
        gameSetup: finalGameSetup,
        conversation,
        summaryConversation,
        summary,
        totalTokenCount,
        userAndAssistantMessageCount,
        systemMessageContentDM,
        mode: req.body.mode || undefined,
        campaignSpec: req.body.campaignSpec || undefined,
    };

    try {
        console.log('Received save request for gameId:', gameId);
        // Ensure consolidated system core is persisted once per game so we don't need to render it repeatedly.
        try {
          if (gameId) {
            const existing = await GameState.findOne({ gameId }).select('+systemCore');
            if (!existing || !existing.systemCore) {
              // Build core system messages (mode=initial as canonical) and persist consolidated string.
              const { composeSystemMessages } = require('../promptManager');
              const coreMsgs = composeSystemMessages({ mode: 'initial', sessionSummary: '', includeFullSkill: false, language: req.body.language || 'English' }).filter(m => m.role === 'system');
              const consolidatedCore = coreMsgs.map(m => m.content).join('\n\n');
              update.systemCore = consolidatedCore;
            }
          }
        } catch (e) {
          console.warn('Failed to persist consolidated system core during save:', e);
        }
        // Log a short preview of the payload (avoid spamming full conversation in prod)
        console.log('Save payload preview:', {
            gameId,
            summary: summary ? summary.slice(0, 200) : '',
            totalTokenCount,
            userAndAssistantMessageCount,
        });

        // Find and update the game state by gameId, or create a new one if it doesn't exist
        let gameState = await GameState.findOneAndUpdate({ gameId }, update, { new: true, upsert: true });

        console.log('Saved game state _id:', gameState?._id);
        res.json(gameState);
    } catch (err) {
        console.error(err);
        res.status(500).json({ error: 'Failed to save game state' });
    }
});

// Load game state
router.get('/load/:gameId', async (req, res) => {
    const { gameId } = req.params;

    try {
        // Find the game state by gameId
        const gameState = await GameState.findOne({ gameId });
        
        if (!gameState) {
            return res.status(404).json({ error: 'No game state found for this game' });
        }

        res.json(gameState);
    } catch (err) {
        console.error(err);
        res.status(500).json({ error: 'Failed to load game state' });
    }
});

// Debug: return persisted raw request/output and consolidated core for a game (GM-only)
router.get('/debug/:gameId/prompts', async (req, res) => {
  const { gameId } = req.params;
  try {
    // include diagnostics and raw fields for debugging
    const gameState = await GameState.findOne({ gameId }).select('+rawModelRequest +rawModelOutput +systemCore +campaignSpec +gameSetup +llmCallError +llmFallbackError');
    if (!gameState) return res.status(404).json({ error: 'No game state found' });
    // Return only debugging fields including LLM diagnostics
    const debug = {
      rawModelRequest: gameState.rawModelRequest || null,
      rawModelOutput: gameState.rawModelOutput || null,
      systemCore: gameState.systemCore || null,
      campaignSpec: gameState.campaignSpec || null,
      gameSetup: gameState.gameSetup || null,
      diagnostics: {
        llmCallEnteredAt: gameState.llmCallEnteredAt || null,
        llmCallStartedAt: gameState.llmCallStartedAt || null,
        llmCallCompletedAt: gameState.llmCallCompletedAt || null,
        llmCallError: gameState.llmCallError || null,
        llmCallFallbackAt: gameState.llmCallFallbackAt || null,
        llmFallbackModel: gameState.llmFallbackModel || null,
        llmFallbackAttemptedAt: gameState.llmFallbackAttemptedAt || null,
        llmFallbackSucceededAt: gameState.llmFallbackSucceededAt || null,
        llmFallbackError: gameState.llmFallbackError || null,
        llmModelUsed: gameState.llmModelUsed || null,
      }
    };
    res.json(debug);
  } catch (err) {
    console.error('Failed to load debug prompts for gameId', gameId, err);
    res.status(500).json({ error: 'Failed to load debug data' });
  }
});

// Get all game states
router.get('/all', async (req, res) => {
    try {
        // Find all game states
        const gameStates = await GameState.find({});
        
        if (!gameStates || gameStates.length === 0) {
            return res.status(404).json({ error: 'No game states found' });
        }

        res.json(gameStates);
    } catch (err) {
        console.error(err);
        res.status(500).json({ error: 'Failed to load game states' });
    }
});

module.exports = router;
