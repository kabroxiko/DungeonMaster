const express = require('express');
const router = express.Router();
const GameState = require('../models/GameState');
const { redactCampaignSpecForClient } = require('../campaignSpecDmSecrets');
const { ensurePlayerCharacterSheetDefaults } = require('../validatePlayerCharacter');

function gameStateDocForClient(doc) {
  if (!doc) return doc;
  const o = typeof doc.toObject === 'function' ? doc.toObject() : { ...doc };
  if (o.campaignSpec) o.campaignSpec = redactCampaignSpecForClient(o.campaignSpec);
  if (o.gameSetup && o.gameSetup.generatedCharacter && typeof o.gameSetup.generatedCharacter === 'object') {
    const lang = o.gameSetup.language || 'English';
    o.gameSetup = {
      ...o.gameSetup,
      generatedCharacter: ensurePlayerCharacterSheetDefaults(o.gameSetup.generatedCharacter, { language: lang }),
    };
  }
  return o;
}

// Persistence is server-driven: POST /api/game-session/bootstrap-session (setup shell) and
// POST /api/game-session/generate with a `persist` payload (each successful reply). No public POST /save.

// Load game state
router.get('/load/:gameId', async (req, res) => {
    const { gameId } = req.params;

    try {
        // Find the game state by gameId
        const gameState = await GameState.findOne({ gameId });
        
        if (!gameState) {
            return res.status(404).json({ error: 'No game state found for this game' });
        }

        res.json(gameStateDocForClient(gameState));
    } catch (err) {
        console.error(err);
        res.status(500).json({ error: 'Failed to load game state' });
    }
});

// Debug: return persisted raw request/output and consolidated core for a game (DM-only)
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
        const list = Array.isArray(gameStates) ? gameStates : [];
        res.json(list.map((g) => gameStateDocForClient(g)));
    } catch (err) {
        console.error(err);
        res.status(500).json({ error: 'Failed to load game states' });
    }
});

module.exports = router;
