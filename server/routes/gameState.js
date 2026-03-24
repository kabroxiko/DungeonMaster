const express = require('express');
const router = express.Router();
const GameState = require('../models/GameState');

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
    // Ensure generatedCharacter is persisted. If missing, try to extract from systemMessageContentDM.
    let finalGameSetup = gameSetup || {};
    try {
        if ((!finalGameSetup.generatedCharacter) && systemMessageContentDM && typeof systemMessageContentDM === 'string') {
            const jsonMatch = systemMessageContentDM.match(/(\{[\s\S]*\})/);
            if (jsonMatch) {
                let parsed = null;
                try {
                    parsed = JSON.parse(jsonMatch[0]);
                } catch (e1) {
                    try {
                        parsed = JSON.parse(jsonMatch[0].replace(/'/g, '"'));
                    } catch (e2) {
                        parsed = null;
                    }
                }
                if (parsed && typeof parsed === 'object') {
                    const maybePC = parsed.playerCharacter || parsed;
                    if (maybePC && (maybePC.name || maybePC.stats || maybePC.max_hp)) {
                        finalGameSetup = { ...finalGameSetup, generatedCharacter: maybePC };
                        console.log('Extracted generatedCharacter for save from systemMessageContentDM');
                    }
                }
            }
        }
    } catch (e) {
        console.warn('Error extracting generatedCharacter during save:', e);
    }

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
    };

    try {
        console.log('Received save request for gameId:', gameId);
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
