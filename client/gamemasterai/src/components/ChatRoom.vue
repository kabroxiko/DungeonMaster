// GMAI/client/gamemasterai/src/components/ChatRoom.vue

<template>
    <h1 class="chat-room-title">GameMaster.AI - Chat</h1>
    <h4 class="chat-room-subtitle">{{ language === 'Spanish' ? 'Ahora puedes chatear con un Maestro de Juego de IA. ¡Diviértete!' : 'You can now chat with an AI Game Master. Have fun!' }}</h4>
    <div v-if="errorMessage" class="error-message">
        <p>Error: {{ errorMessage }}</p>
        <button @click="tryAgain">Try again</button>
    </div>
    <div class="chat-room">
        <div class="chat-messages">
            <div v-for="(message, index) in messages" :key="index" class="chat-message">
                <strong>{{ message.user }}:</strong>
                <div class="message-content" v-html="renderMarkdown(message.text)"></div>
            </div>
        </div>
        <form @submit.prevent="submitMessage">
            <input type="text" v-model="newMessage" :placeholder="language === 'Spanish' ? 'Escribe tu mensaje aquí...' : 'Type your message here...'" />
            <button type="submit">{{ language === 'Spanish' ? 'Enviar' : 'Send' }}</button>
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
    import NotePanel from './NotePanel.vue';
    import summaryPrompt from '@/prompts/summaryPrompt.txt';

    export default {
        components: {
            NotePanel
        },
        data() {
            return {
                summaryPrompt,
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
                errorMessage: null // add error message data property


            };
        },
        created() {
            console.log('this.$route.params.id:', this.$route.params.id); // This should log the gameId or undefined

            // Initialize language from store (set during setup)
            this.language = (this.$store.state.gameSetup && this.$store.state.gameSetup.language) || 'English';

            // check if a gameId is provided in the route
            if (this.$route.params.id) {
                // Load the existing game
                this.loadGameState(this.$route.params.id);
                this.systemMessageContentDM = this.$store.state.systemMessageContentDM;
                const systemMessageDM = {
                    role: 'system',
                    content: this.systemMessageContentDM,
                };

                // Push the system message to the conversation
                this.conversation.push(systemMessageDM);
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

                    // Build the summary instruction; use a Spanish version when language is Spanish
                    const summaryInstruction = this.language === 'Spanish'
                        ? '*Todo lo anterior fue una transcripción de una partida de rol. Resume los eventos descritos en esta transcripción. Sé conciso (menos de 75 palabras) y objetivo. Toma nota de personajes, lugares y objetos importantes. Usa tercera persona.*'
                        : this.summaryPrompt;

                    const summaryRequest = {
                        role: 'system',
                        content: summaryInstruction,
                    };

                    const messagesToSend = [...lastSummaryMessages, summaryRequest];

                    const response = await axios.post('http://localhost:5001/api/game-session/generate-summary', {
                        messages: messagesToSend,
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

                        // Prepend language system message when Spanish is selected
                        const messagesToSend = lastMessages.slice();
                        if (this.language === 'Spanish') {
                            messagesToSend.unshift({ role: 'system', content: 'Por favor responde en español. Responde todas las interacciones en español.' });
                        }

                        const response = await axios.post('http://localhost:5001/api/game-session/generate', {
                            messages: messagesToSend
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
                    }

                    this.newMessage = "";
                }
            },
            
            // Generate an initial AI message using the current conversation (used when entering a new game)
            async generateInitialMessage() {
                try {
                    const messagesToSend = this.conversation.slice(-this.ContextLength * 2);
                    if (this.language === 'Spanish') {
                        messagesToSend.unshift({ role: 'system', content: 'Por favor responde en español. Responde todas las interacciones en español.' });
                    }
                    const response = await axios.post('http://localhost:5001/api/game-session/generate', {
                        messages: messagesToSend
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
        margin-bottom: 0.5rem;
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
    .message-content {
        text-align: justify;
        white-space: pre-wrap; /* preserve newlines */
        word-break: break-word;
    }
</style>
