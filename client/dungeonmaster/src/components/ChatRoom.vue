// GMAI/client/dungeonmaster/src/components/ChatRoom.vue

<template>
    <div v-if="errorMessage" class="error-message">
        <p>Error: {{ errorMessage }}</p>
        <button @click="tryAgain">Try again</button>
    </div>
    <div class="chat-room two-column">
        <h2 v-if="campaignTitle" class="campaign-heading">{{ campaignTitle }}</h2>
        <!-- floating card handled globally -->
        <div class="chat-messages chat-messages-container" style="flex:1">
            <ChatMessage
              v-for="(message, index) in messages"
              :key="index"
              :message="renderMarkdown(message.text)"
              :sender="message.user"
              :role="message.user === 'Player' ? 'player' : 'system'"
            />
        </div>
        <form @submit.prevent="submitMessage" class="chat-input-form">
            <input class="chat-input" type="text" v-model="newMessage"
              :placeholder="$i18n.chat_placeholder"
              :disabled="isSending" />
            <button class="ui-button chat-send-button" type="submit" :disabled="isSending" :aria-busy="isSending">
              {{ isSending ? $i18n.sending : $i18n.send }}
            </button>
        </form>
        <!-- Notetaker UI removed -->

    <!-- Floating character sheet: PC HP shown here; enemy HP is not shown to the player -->
    <FloatingCard v-if="playerCharacter" :character="playerCharacter" :hp-snapshot="playerHitPoints" :defaultOpen="false" />
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
    import ChatMessage from '@/ui/ChatMessage.vue';
    import FloatingCard from '@/ui/FloatingCard.vue';

    export default {
        components: {
            ChatMessage,
            FloatingCard
        },
        data() {
            return {
                // summaryPrompt removed from client; server composes summary instruction
                // Initial state for the component
                newMessage: "", // Holds the current message being typed
                // language moved to global store; use computed property
                // language: 'English',
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
                lastEncounterState: null,
                isSending: false,
                campaignTitle: '',


            };
        },
        // removed duplicate data() 
        async created() {
            console.log('this.$route.params.id:', this.$route.params.id); // This should log the gameId or undefined

            // language is now global in the store; no local initialization needed
            

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
            },
            /** Current/max HP for the sheet: encounterState PC row when present, else full max_hp. */
            playerHitPoints() {
                const c = this.playerCharacter;
                if (!c) return null;
                let max = c.max_hp != null ? Number(c.max_hp) : null;
                let current = max;
                const es = this.lastEncounterState;
                if (es && Array.isArray(es.participants)) {
                    const lower = (x) => String(x || '').toLowerCase();
                    const pc = es.participants.find(
                        (p) =>
                            p &&
                            (lower(p.kind) === 'pc' || p.id === 'pc' || lower(p.id) === 'player')
                    );
                    if (pc) {
                        if (pc.hp_max != null && !Number.isNaN(Number(pc.hp_max))) max = Number(pc.hp_max);
                        if (pc.hp_current != null && !Number.isNaN(Number(pc.hp_current))) current = Number(pc.hp_current);
                    }
                }
                if (max == null || Number.isNaN(max)) return null;
                if (current == null || Number.isNaN(current)) current = max;
                return { current, max };
            },
            language: {
                get() {
                    return (this.$store.state && this.$store.state.language) || 'English';
                },
                set(val) {
                    this.$store.commit('setLanguage', val);
                }
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

            narrationFromGenerateResponse(data) {
                if (data == null) return '';
                if (typeof data === 'object' && typeof data.narration === 'string') return data.narration;
                if (typeof data === 'string') return data;
                return '';
            },

            incrementTokenCount(message) {
                const tokenCountForMessage = Math.ceil(message.length / 4);
                this.totalTokenCount += tokenCountForMessage;
                console.log("Total tokens processed by AI: ", this.totalTokenCount);

            },
            // summary generation is handled server-side; frontend will not request summaries

        updateSummaryInChatRoom(updatedSummary) {
                this.summary = updatedSummary;
                this.$store.commit('setSummary', updatedSummary);
            },

            /** Snapshot for server-side persist (conversation must include the latest user line before the assistant reply). */
            buildPersistPayload() {
                return {
                    gameId: this.$store.state.gameId,
                    gameSetup: this.$store.state.gameSetup,
                    conversation: this.conversation,
                    summaryConversation: this.summaryConversation,
                    summary: this.summary,
                    totalTokenCount: this.totalTokenCount,
                    userAndAssistantMessageCount: this.userAndAssistantMessageCount,
                    systemMessageContentDM: this.systemMessageContentDM,
                    encounterState: this.lastEncounterState,
                };
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
                        const response = await axios.post('/api/game-session/generate', {
                            messages: messagesToSend,
                            language: this.language,
                            gameId: this.$store.state.gameId,
                            persist: this.buildPersistPayload(),
                        });
                        if (response.data && Object.prototype.hasOwnProperty.call(response.data, 'encounterState')) {
                            this.lastEncounterState = response.data.encounterState || null;
                        }
                        const aiMessageContent = this.narrationFromGenerateResponse(response.data);
                        if (!aiMessageContent) {
                            throw new Error(response.data?.error || 'Empty narration from server');
                        }

                        this.incrementTokenCount(aiMessageContent); // Increment token count

                        const aiMessage = {
                            role: 'assistant',
                            content: aiMessageContent,
                        };
                        this.conversation.push(aiMessage);
                        this.summaryConversation.push(aiMessage);
                        this.messages.push({ user: "Dungeon Master", text: aiMessageContent });

                        // Increment the counter only if the message is from the user or the assistant
                        if (userMessage.role === 'user' || aiMessage.role === 'assistant') {
                            this.userAndAssistantMessageCount++;
                            console.log('User and assistant message count:');
                            console.log(this.userAndAssistantMessageCount);
                        }

                        // Do not request frontend-generated summaries. Server handles summarization.
                    } catch (error) {
                        console.error('Error generating AI message:', error);
                        const apiErr = error.response?.data?.error;
                        this.errorMessage = apiErr || error.message || 'Failed to send message. Please try again.';
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

                        const response = await axios.post('/api/game-session/generate', {
                        gameId: this.$store.state.gameId,
                        messages: messagesToSend,
                        mode: 'initial',
                        includeFullSkill: true,
                        language: this.language,
                        sessionSummary,
                        persist: this.buildPersistPayload(),
                    });
                    if (response.data && Object.prototype.hasOwnProperty.call(response.data, 'encounterState')) {
                        this.lastEncounterState = response.data.encounterState || null;
                    }
                    const aiMessageContent = this.narrationFromGenerateResponse(response.data);
                    if (!aiMessageContent) {
                        throw new Error(response.data?.error || 'Empty opening narration');
                    }

                    const aiMessage = {
                        role: 'assistant',
                        content: aiMessageContent,
                    };
                    this.conversation.push(aiMessage);
                    this.summaryConversation.push(aiMessage);
                    this.messages.push({ user: "Dungeon Master", text: aiMessageContent });
                } catch (err) {
                    console.error('Error generating initial AI message:', err);
                    const apiErr = err.response?.data?.error;
                    const preview = err.response?.data?.rawPreview;
                    this.errorMessage =
                        apiErr ||
                        (preview ? `${err.message || 'Request failed'}. ${String(preview).slice(0, 280)}` : null) ||
                        err.message ||
                        'Could not start the adventure.';
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
                    this.lastEncounterState = gameState.encounterState || null;
                    const spec = gameState.campaignSpec;
                    this.campaignTitle =
                        spec && typeof spec.title === 'string' && spec.title.trim() ? spec.title.trim() : '';

                    // Note: Do NOT auto-extract generatedCharacter from systemMessageContentDM.
                    // The server no longer relies on this fallback; gameSetup.generatedCharacter must be provided explicitly.
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
                        user: role === 'assistant' ? this.$i18n.dm_label : role.charAt(0).toUpperCase() + role.slice(1),
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
  .two-column {
    display: flex;
    flex-direction: column;
    gap: 12px;
    align-items: stretch;
    width: 100%;
    max-width: 720px;
    margin: 0 auto;
  }

  .campaign-heading {
    margin: 0 0 4px 0;
    font-size: 1.15rem;
    font-weight: 700;
    color: var(--gm-text, #e8e4dc);
    line-height: 1.3;
  }

  .chat-message {
    margin-bottom: 0.75rem;
  }

  /* Chat input form: wider text box and aligned button */
  .chat-input-form {
    display: flex;
    gap: 10px;
    align-items: center;
    width: 100%;
    margin-top: 12px;
  }
  .chat-input {
    flex: 1 1 auto;
    min-height: 48px;
    padding: 0.8rem 1rem;
    border-radius: 10px;
    border: 1px solid rgba(255,255,255,0.08);
    background: rgba(10,9,8,0.92); /* darker, high-contrast input */
    color: var(--gm-text);
    box-sizing: border-box;
    font-family: var(--gm-font-sans);
    font-size: 1rem;
    outline: none;
    box-shadow: 0 6px 18px rgba(0,0,0,0.5) inset;
    position: relative;
    z-index: 6;
  }
  .chat-input::placeholder {
    color: rgba(230,225,216,0.65);
    opacity: 1;
  }
  .chat-send-button {
    flex: 0 0 auto;
    height: 44px;
    padding: 0 14px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  /* ensure form sits above any background overlays */
  .chat-input-form { position: relative; z-index: 6; }

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
    border: 1px solid rgba(255,255,255,0.03);
    padding: 0.75rem;
    margin-bottom: 1rem;
    background: rgba(255,255,255,0.02);
    color: var(--gm-text);
    border-radius: 8px;
    box-shadow: var(--gm-shadow);
  }

  .character-sheet .cs-title {
    margin: 0 0 0.5rem 0;
    color: var(--gm-text);
  }

  .character-sheet .cs-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 0.5rem;
  }

  .character-sheet .cs-stats {
    font-family: monospace;
    color: var(--gm-text);
  }

  .character-sheet ul {
    margin: 0;
    padding-left: 1.25rem;
  }
  .right-sidebar {
    position: relative;
  }
  @media (max-width: 920px) {
    .two-column { flex-direction: column; }
    .right-sidebar { width: 100%; margin-left: 0; }
  }
</style>
