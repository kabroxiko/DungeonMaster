//GMAI/server/routes/gameSession.js

const express = require('express');
const router = express.Router();
const { generateResponse } = require('../openai-api');

const DEFAULT_MODEL = process.env.OPENAI_MODEL || 'gpt-3.5-turbo';

// Route to generate AI Dungeon Master and campaign generating responses
router.post('/generate', async (req, res) => {
    // Extract the messages from the request body  
    const messages = req.body.messages;

    console.log('AI DM Processing the following messages');
    console.log(messages);

    // Use central generateResponse (handles model selection and fallbacks)
    try {
        const aiMessage = await generateResponse({ messages }, { max_tokens: 300, temperature: 0.8 });
        if (!aiMessage) {
            return res.status(500).json({ error: 'AI response was empty or failed (see server logs).' });
        }
        console.log('AI DM processed:', aiMessage);
        res.json(aiMessage);
    } catch (error) {
        console.error('Error generating text:', error);
        res.status(500).json({ error: `Error generating text: ${String(error)}` });
    }
});

// Route to generate campaign generating responses 
router.post('/generate-campaign', async (req, res) => {
    // Extract the messages from the request body  
    const messages = req.body.messages;

    console.log('Prepper is Processing the following messages');
    console.log(messages);

    try {
        const aiMessage = await generateResponse({ messages }, { max_tokens: 400, temperature: 0.8 });
        if (!aiMessage) {
            return res.status(500).json({ error: 'AI response was empty or failed (see server logs).' });
        }
        res.json(aiMessage);
    } catch (error) {
        console.error('Error generating text:', error);
        res.status(500).json({ error: `Error generating text: ${String(error)}` });
    }
});

// Route to generate game session summary   
router.post('/generate-summary', async (req, res) => {
    try {
        // Extract the messages and the existing summary from the request body   
        const messages = req.body.messages;
        //console.log("AI notetaker is processing the following:");
        //console.log(messages);

        const aiSummary = await generateResponse({ messages }, { max_tokens: 150, temperature: 0.8 });
        if (!aiSummary) {
            return res.status(500).json({ error: 'AI summary was empty or failed (see server logs).' });
        }
        res.json(aiSummary);
    } catch (error) {
        console.error('Error generating text:', error);
        res.status(500).json({ error: `Error generating text: ${String(error)}` });
    }

});

// Export the router to be used in other files
module.exports = router;