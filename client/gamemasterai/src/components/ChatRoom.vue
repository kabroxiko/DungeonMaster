// GMAI/client/gamemasterai/src/components/ChatRoom.vue

<template>
    <h1 class="chat-room-title">GameMaster.AI - Chat</h1>
    <h4 class="chat-room-subtitle">{{ language === 'Spanish' ? 'Ahora puedes chatear con un Maestro de Juego de IA. ¡Diviértete!' : 'You can now chat with an AI Game Master. Have fun!' }}</h4>
    <div v-if="errorMessage" class="error-message">
        <p>Error: {{ errorMessage }}</p>
        <button @click="tryAgain">Try again</button>
    </div>
    <div class="chat-room">
        
        <div v-if="playerCharacter" class="character-sheet">
            <h3 class="cs-title">{{ language === 'Spanish' ? 'Ficha del Personaje' : 'Character Sheet' }}</h3>
            <div class="cs-grid">
                <div><strong>{{ playerCharacter.name }}</strong> — {{ playerCharacter.race }}</div>
                <div>{{ playerCharacter.class }} {{ playerCharacter.subclass ? ('(' + playerCharacter.subclass + ')') : '' }} — {{ language === 'Spanish' ? 'Nivel' : 'Level' }} {{ playerCharacter.level }}</div>
                <div>{{ language === 'Spanish' ? 'PV Máx' : 'Max HP' }}: {{ playerCharacter.max_hp }} • AC: {{ playerCharacter.ac }}</div>
                <div class="cs-backstory">{{ playerCharacter.brief_backstory }}</div>
                <div class="cs-stats">
                    <strong>STR</strong>: {{ playerCharacter.stats.STR }} &nbsp;
                    <strong>DEX</strong>: {{ playerCharacter.stats.DEX }} &nbsp;
                    <strong>CON</strong>: {{ playerCharacter.stats.CON }} &nbsp;
                    <strong>INT</strong>: {{ playerCharacter.stats.INT }} &nbsp;
                    <strong>WIS</strong>: {{ playerCharacter.stats.WIS }} &nbsp;
                    <strong>CHA</strong>: {{ playerCharacter.stats.CHA }}
                </div>
                <div class="cs-equipment"><strong>{{ language === 'Spanish' ? 'Equipo' : 'Equipment' }}:</strong>
                    <ul>
                        <li v-for="(it, i) in playerCharacter.starting_equipment" :key="i">{{ it }}</li>
                    </ul>
                </div>
            </div>
        </div>
        <div class="chat-messages">
            <div v-for="(message, index) in messages" :key="index" class="chat-message">
                <strong>{{ message.user }}:</strong>
                <div class="message-content" v-html="renderMarkdown(message.text)"></div>
            </div>
        </div>
        <form @submit.prevent="submitMessage">
            <input type="text" v-model="newMessage" :placeholder="language === 'Spanish' ? 'Escribe tu mensaje aquí...' : 'Type your message here...'" :disabled="isSending" />
            <button type="submit" :disabled="isSending" aria-busy="isSending">
              {{ isSending ? (language === 'Spanish' ? 'Enviando...' : 'Sending...') : (language === 'Spanish' ? 'Enviar' : 'Send') }}
            </button>
        </form>
        <h1 class="notetaker-title">Notetaker.AI</h1>
        <h4 class="notetaker-subtitle">{{ language === 'Spanish' ? 'Un resumen de tu aventura se actualizará aquí automáticamente.' : 'A summary of your adventure will update here automatically!' }}</h4>
        <h4 class="notetaker-subtitle-editing">{{ language === 'Spanish' ? 'Puedes editar este resumen para ajustar lo que GameMaster.AI toma en cuenta con el tiempo. Estos cambios surtirán efecto la próxima vez que se actualice el resumen.' : 'You may edit this summary to adjust what GameMaster.AI takes into consideration over time. These edits will take effect the next time the summary updates.' }}</h4>

        <NotePanel :summary="summary" @update-summary="updateSummaryInChatRoom" />
    </div>
</template>

<script>import axios from 'axios';
import MarkdownIt from 'markdown-it';
const md = new MarkdownIt({ html: false, linkify: true, typographer: true });
// Force markdown renderer to emit paragraphs with zero margin to avoid CSS spacing issues
// Use renderer overrides instead of relying on global CSS (more robust)
// Render plain <p> tags; spacing handled via CSS for consistency
md.renderer.rules.paragraph_open = function() {
    return '<p>';
};
md.renderer.rules.paragraph_close = function() {
    return '</p>';
};
// Also ensure list items are rendered compactly
// Render plain <li>; spacing handled via CSS
md.renderer.rules.list_item_open = function() {
    return '<li>';
};
md.renderer.rules.heading_open = function(tokens, idx) {
    return `<${tokens[idx].tag}>`;
};
md.renderer.rules.heading_close = function(tokens, idx) {
    return `</${tokens[idx].tag}>`;
};
    import NotePanel from './NotePanel.vue';

    export default {
        components: {
            NotePanel
        },
        data() {
            return {
                // summaryPrompt removed from client; server composes summary instruction
                // Initial state for the component
                newMessage: "", // Holds the current message being typed
                language: 'English',
                messages: [], // Array to hold all the chat messages
                conversation: [], // Array to hold all conversation data
                summaryConversation: [], // Array to hold all summary conversation data
                summary: "", // Holds the summary of the game session
                systemMessageContentDM: "", //Holds the prompt for the AI DM
                ContextLength: 3, // The number of most recent messages to consider for generating a response
                userAndAssistantMessageCount: 0, // initialize the counter here
                totalTokenCount: 0,
                errorMessage: null, // add error message data property
                localPlayerCharacter: null,
                isSending: false,


            };
        },
        // removed duplicate data() 
        async created() {
            console.log('this.$route.params.id:', this.$route.params.id); // This should log the gameId or undefined

            // Initialize language from store (set during setup)
            this.language = (this.$store.state.gameSetup && this.$store.state.gameSetup.language) || 'English';

            // check if a gameId is provided in the route
            if (this.$route.params.id) {
                // Load the existing game and wait for it so character sheet is available immediately
                await this.loadGameState(this.$route.params.id);
                this.systemMessageContentDM = this.$store.state.systemMessageContentDM;
                const systemMessageDM = {
                    role: 'system',
                    content: this.systemMessageContentDM,
                };

                // Push the system message to the conversation if not already present
                if (!this.conversation.find(m => m.role === 'system' && m.content === this.systemMessageContentDM)) {
                    this.conversation.unshift(systemMessageDM);
                }
            }
        },

        computed: {
            playerCharacter() {
                return this.localPlayerCharacter || (this.$store.state.gameSetup && this.$store.state.gameSetup.generatedCharacter) || null;
            }
        },

        methods: {
            renderMarkdown(text) {
                if (!text) return '';
                try {
                    return md.render(text);
                } catch (e) {
                    console.error('Markdown render error:', e);
                    return text;
                }
            },

            incrementTokenCount(message) {
                const tokenCountForMessage = Math.ceil(message.length / 4);
                this.totalTokenCount += tokenCountForMessage;
                console.log("Total tokens processed by AI: ", this.totalTokenCount);

            },
            async updateSummary() {
                try {
                    this.errorMessage = null; // Clear the error message

                    // Prepare an array of last ContextLength number of messages
                    let lastSummaryMessages = this.summaryConversation.slice(-(this.ContextLength * 2));

                    // Filter out system messages to avoid including long system prompts in the summary input
                    lastSummaryMessages = lastSummaryMessages.filter(m => m.role !== 'system');

                    // Increment token count based on the messages read by the AI
                    lastSummaryMessages.forEach(message => {
                        this.incrementTokenCount(message.content);
                    });

                    // Send only the recent user/assistant messages; server will add summarization instruction
                    const response = await axios.post('http://localhost:5001/api/game-session/generate-summary', {
                        messages: lastSummaryMessages,
                        language: this.language,
                    });

                    this.summary += "\n" + response.data;
                    this.incrementTokenCount(response.data); // Increment token count

                    // Remove the used messages from summaryConversation
                    this.summaryConversation = this.summaryConversation.slice(0, -(this.ContextLength));
                } catch (error) {
                    console.error('Error generating summary:', error);
                    this.errorMessage = "Failed to generate summary. Please try again."; // Set the error message

                }
            },

        updateSummaryInChatRoom(updatedSummary) {
        this.summary = updatedSummary;
        // Call the setSummary mutation to update the summary in the Vuex store
        this.$store.commit('setSummary', updatedSummary);
        this.saveGameState();
    },

            async submitMessage() {
                //const gameSetup = this.$store.state.gameSetup;

                if (this.newMessage.trim() !== "") {
                    this.isSending = true;
                    this.messages.push({ user: "Player", text: this.newMessage.trim() });
    /*
                    const userMessageWithHidden = {
                        role: 'user',
                        content: this.appendHiddenMessage(this.newMessage.trim()),
                    };
                    this.conversation.push(userMessageWithHidden);
    */
                    const userMessage = {
                        role: 'user',
                        content: this.newMessage.trim(),
                    };
                    this.conversation.push(userMessage);
                    this.summaryConversation.push(userMessage);

                    try {
                        this.errorMessage = null; // Clear the error message

                        // Prepare an array of last ContextLength number of messages
                        const lastMessages = this.conversation.slice(-this.ContextLength * 2);

                        // Increment token count based on the messages read by the AI 
                        lastMessages.forEach(message => {
                            this.incrementTokenCount(message.content);
                        });

                        // Send only user/assistant messages; server will enforce language and system prompts
                        const messagesToSend = lastMessages.slice();
                        const response = await axios.post('http://localhost:5001/api/game-session/generate', {
                            messages: messagesToSend,
                            language: this.language,
                        });
                        const aiMessageContent = response.data;



                        this.incrementTokenCount(aiMessageContent); // Increment token count

                        const aiMessage = {
                            role: 'assistant',
                            content: aiMessageContent,
                        };
                        this.conversation.push(aiMessage);
                        this.summaryConversation.push(aiMessage);
                        this.messages.push({ user: "GameMaster.AI", text: aiMessageContent });

                        // Increment the counter only if the message is from the user or the assistant
                        if (userMessage.role === 'user' || aiMessage.role === 'assistant') {
                            this.userAndAssistantMessageCount++;
                            console.log('User and assistant message count:');
                            console.log(this.userAndAssistantMessageCount);
                            // Call saveGameState after each message
                            this.saveGameState();
                        }

                        if (this.userAndAssistantMessageCount % this.ContextLength === 0) {
                            await this.updateSummary();

                            const reminderMessage = {
                                role: 'system',
                                content: '(this is a reminder of your role, do not respond directly: ' + this.systemMessageContentDM + '. ' + 'This is also a summary of what has transpired in this game so far. Ensure continuity and consistency using this summary:' + this.summary + ')',
                            };
                            this.conversation.push(reminderMessage);
                        }
                    } catch (error) {
                        console.error('Error generating AI message:', error);
                        this.errorMessage = "Failed to send message. Please try again."; // Set the error message
                    } finally {
                        this.isSending = false;
                    }

                    this.newMessage = "";
                }
            },
            
            // Generate an initial AI message using the current conversation (used when entering a new game)
            async generateInitialMessage() {
                try {
                    const messagesToSend = this.conversation.slice(-this.ContextLength * 2);
                    const sessionSummary = (this.$store.state.gameSetup && this.$store.state.gameSetup.generatedCharacter)
                        ? JSON.stringify({ playerCharacter: this.$store.state.gameSetup.generatedCharacter })
                        : '';

                    const response = await axios.post('http://localhost:5001/api/game-session/generate', {
                        messages: messagesToSend,
                        mode: 'initial',
                        includeFullSkill: true,
                        language: this.language,
                        sessionSummary
                    });
                    const aiMessageContent = typeof response.data === 'string' ? response.data : (response.data?.text || JSON.stringify(response.data));

                    const aiMessage = {
                        role: 'assistant',
                        content: aiMessageContent,
                    };
                    this.conversation.push(aiMessage);
                    this.summaryConversation.push(aiMessage);
                    this.messages.push({ user: "GameMaster.AI", text: aiMessageContent });

                    // Save the updated state
                    this.saveGameState();
                } catch (err) {
                    console.error('Error generating initial AI message:', err);
                }
            },
            tryAgain() {
                this.errorMessage = null;
                this.submitMessage(); // Retry sending the message
            },

    /*
    appendHiddenMessage(message) {
                // Add the hidden message to the end of the user input
                const hiddenMessage = "Keep response under 75 words."; // Replace with your hidden message
                return message + hiddenMessage;
            }, */

            async saveGameState() {
                // Game state to save
                const gameState = {
                    gameId: this.$store.state.gameId,
                    userId: this.$store.state.userId,
                    gameSetup: this.$store.state.gameSetup,
                    conversation: this.conversation,
                    summaryConversation: this.summaryConversation,
                    summary: this.summary,
                    totalTokenCount: this.totalTokenCount,
                    userAndAssistantMessageCount: this.userAndAssistantMessageCount,
                    systemMessageContentDM: this.systemMessageContentDM
                };

                try {
                    // Save game state to backend
                    await axios.post('/api/game-state/save', gameState);
                    console.log('Game saved');
                    console.log(gameState);

                } catch (error) {
                    console.error('Error saving game state:', error);
                }
            },
            async loadGameState(gameId) {
                try {
                    const response = await axios.get(`/api/game-state/load/${gameId}`);
                    const gameState = response.data;

                    // Restore the game state
                    this.$store.commit('setGameId', gameState.gameId);
                    this.$store.commit('setGameSetup', gameState.gameSetup);
                    // set local language from loaded game setup
                    this.language = (gameState.gameSetup && gameState.gameSetup.language) || this.language;
                    this.$store.commit('setUserId', gameState.userId);
                    this.conversation = gameState.conversation;
                    this.summaryConversation = gameState.summaryConversation;
                    this.summary = gameState.summary;
                    this.totalTokenCount = gameState.totalTokenCount;
                    this.userAndAssistantMessageCount = gameState.userAndAssistantMessageCount;
                    this.systemMessageContentDM = gameState.systemMessageContentDM;
                    

                    // If generatedCharacter missing in gameSetup, try to parse it from systemMessageContentDM
                    if ((!this.$store.state.gameSetup || !this.$store.state.gameSetup.generatedCharacter) && this.systemMessageContentDM) {
                        const m = this.systemMessageContentDM;
                        // Try to find any JSON object in the system message
                        const jsonMatch = m.match(/(\{[\s\S]*\})/);
                        if (jsonMatch) {
                            let parsed = null;
                            try {
                                parsed = JSON.parse(jsonMatch[0]);
                            } catch (err) {
                                // permissive fallback: replace single quotes with double quotes
                                try {
                                    parsed = JSON.parse(jsonMatch[0].replace(/'/g, '"'));
                                } catch (err2) {
                                    console.warn('Failed to parse JSON from systemMessageContentDM', err2);
                                }
                            }
                            if (parsed && typeof parsed === 'object') {
                                // Heuristic: does this object look like a character? check for stats or name
                                const maybePC = parsed.playerCharacter || parsed;
                                if (maybePC && (maybePC.name || maybePC.stats || maybePC.max_hp)) {
                                    const newSetup = { ...(this.$store.state.gameSetup || {}), generatedCharacter: maybePC };
                                    this.$store.commit('setGameSetup', newSetup);
                                    console.log('Recovered generatedCharacter from systemMessageContentDM:', maybePC);
                                    // also set local player character for immediate rendering
                                    this.localPlayerCharacter = maybePC;
                                }
                            }
                        }
                    }
                    // If store already contains generatedCharacter, ensure localPlayerCharacter is set
                    if (this.$store.state.gameSetup && this.$store.state.gameSetup.generatedCharacter && !this.localPlayerCharacter) {
                        this.localPlayerCharacter = this.$store.state.gameSetup.generatedCharacter;
                        console.log('Set localPlayerCharacter from store:', this.localPlayerCharacter);
                    }

                    // Map the conversation array to match the structure needed by this.messages
                    // And only include the messages that are not of role 'system'
                    this.messages = this.conversation
                        .filter(({ role }) => role !== 'system')  // Filter out the 'system' role messages
                        .map(({ role, content }) => ({
                            user: role === 'assistant' ? 'GameMaster.AI' : role.charAt(0).toUpperCase() + role.slice(1),
                            text: content,
                        }));

                    // If there are no visible messages (only a system prompt exists), generate an initial AI message
                    const hasOnlySystem = this.conversation.length === 1 && this.conversation[0]?.role === 'system';
                    if (this.messages.length === 0 && hasOnlySystem) {
                        // fire-and-forget initial generation
                        this.generateInitialMessage();
                    }

                } catch (error) {
                    console.error('Error loading game state:', error);
                }
            }

        }
    };</script>

<style scoped>
  .chat-room {
    width: 100%;
    max-width: 600px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
  }

  .chat-messages {
    height: 400px;
    overflow-y: auto;
    border: 1px solid #ccc;
    padding: 1rem;
    margin-bottom: 1rem;
  }

  .chat-message {
    margin-bottom: 0.75rem;
  }

  input {
    width: 100%;
    padding: 0.5rem;
    margin-bottom: 1rem;
    box-sizing: border-box;
  }

  .error-message {
    color: red;
    margin: 1rem 0;
  }

  /* Minimal, clean message styling */
  .message-content {
    text-align: justify;
    white-space: pre-wrap;
    word-break: break-word;
    line-height: 1.6;
    margin-bottom: 0.9rem;
  }

  .message-content p {
    margin: 0 0 0.6rem 0;
  }

  /* Compact list items: reduce extra gaps inside lists */
  .message-content li {
    padding: 0.15rem 0;
  }
  .message-content li p {
    margin: 0;
  }

  .character-sheet {
    border: 1px solid #ddd;
    padding: 0.75rem;
    margin-bottom: 1rem;
    background: #fafafa;
  }

  .character-sheet .cs-title {
    margin: 0 0 0.5rem 0;
  }

  .character-sheet .cs-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 0.5rem;
  }

  .character-sheet .cs-stats {
    font-family: monospace;
  }

  .character-sheet ul {
    margin: 0;
    padding-left: 1.25rem;
  }
</style>
