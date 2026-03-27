// GMAI/client/gamemasterai/src/components/SetupForm.vue


<template>
    <form @submit.prevent="submitForm">
        <h1 class="form-title">The Start of Your Adventure</h1>
        <h4 class="form-description">Select the building blocks of your character and story. Allow up to 30 seconds after clicking "Start Game".</h4>
        <!-- Game system selection removed; D&D 5e is the default -->
        <!-- Adventure setting removed; Classic Fantasy is used by default -->
        <div>
            <label for="language-select">Language:</label>
            <select id="language-select" v-model="formData.language">
                <option value="English">English</option>
                <option value="Spanish">Spanish</option>
            </select>
        </div>
        <div>
            <label for="character-gender">Character Gender:</label>
            <select id="character-gender" v-model="formData.gender">
                <option value="Male">Male</option>
                <option value="Female">Female</option>
                <option value="Non-binary">Non-binary</option>
            </select>
        </div>
        <div>
            <label for="character-name">Character Name:</label>
            <input id="character-name" v-model="formData.characterName" type="text">
        </div>
        <div>
            <label for="Character Class">Character Class:</label>
            <input id="character-class" v-model="formData.characterClass" type="text">
        </div>
        <div>
            <label for="Character Race">Character Race:</label>
            <input id="character-race" v-model="formData.characterRace" type="text">
        </div>
        <div>
            <label for="Character Level">Character Level:</label>
            <input id="character-level" v-model="formData.characterLevel" type="text">
        </div>
        
        <button type="submit" :disabled="isStarting">{{ isStarting ? (formData.language === 'Spanish' ? 'Iniciando...' : 'Starting...') : (formData.language === 'Spanish' ? 'Iniciar Juego' : 'Start Game') }}</button>

        <!-- Progress message (prominent) -->
        <div v-if="progressMessage" class="progress-message" role="status" aria-live="polite">
            {{ progressMessage }}
        </div>
    </form>
    
</template>

<script>
    import axios from 'axios';


    export default {
        data() {
            return {
                isStarting: false,
                progressMessage: '',
                formData: {
                    gameSystem: 'Dungeons and Dragons 5th Edition',
                    characterName: '',
                    characterClass: '',
                    characterRace: '',
                    characterLevel: 1,
                    language: 'English',
                    gender: 'Male'
                }
            };
        },
        methods: {

        async generateCampaignConcept(gameId = null) {
        // Request campaign concept and generated player character from server.
        try {
            const response = await axios.post('/api/game-session/generate-campaign-core', {
                gameId,
                gameSetup: {
                    name: this.formData.characterName,
                    class: this.formData.characterClass,
                    race: this.formData.characterRace,
                    level: this.formData.characterLevel,
                    background: this.formData.characterBackground,
                    language: this.formData.language
                },
                sessionSummary: '',
                language: this.formData.language,
                waitForStages: true
            });

            return response.data;
        } catch (error) {
            console.error('Error generating campaign concept:', error);
        }
    },

    async submitForm() {
            this.isStarting = true;
            this.progressMessage = this.formData.language === 'Spanish' ? 'Generando campaña...' : 'Generating campaign...';
            this.$store.commit('createNewGame');
            this.$store.commit('setGameSetup', this.formData);

            let systemMessageContentDM;

            // Generate the campaign concept (structured campaignSpec). Character generation is separate.
            const gen = await this.generateCampaignConcept(this.$store.state.gameId);
            this.progressMessage = this.formData.language === 'Spanish' ? 'Campaña generada. Generando personaje...' : 'Campaign generated. Generating character...';
            let campaignConcept = '';
            let campaignSpec = null;
            let playerCharacter = null;
            if (gen && typeof gen === 'object' && gen.campaignConcept) {
                campaignConcept = gen.campaignConcept;
                campaignSpec = gen;
                // playerCharacter will be generated separately below if missing
            } else {
                // fallback: treat gen as plain string campaign text
                campaignConcept = typeof gen === 'string' ? gen : '';
            }

            // If campaign generation did not include a playerCharacter, request character generation separately
            if (!playerCharacter) {
                try {
                    this.progressMessage = this.formData.language === 'Spanish' ? 'Generando personaje...' : 'Generating character...';
                    const charResp = await axios.post('/api/game-session/generate-character', {
                        gameId: this.$store.state.gameId,
                        gameSetup: {
                            name: this.formData.characterName,
                            class: this.formData.characterClass,
                            race: this.formData.characterRace,
                            level: this.formData.characterLevel,
                            background: this.formData.characterBackground,
                            language: this.formData.language
                        },
                        sessionSummary: '',
                        language: this.formData.language
                    });
                    if (charResp && charResp.data && charResp.data.playerCharacter) {
                        playerCharacter = charResp.data.playerCharacter;
                    }
                } catch (e) {
                    console.error('Error generating player character separately:', e);
                    this.progressMessage = this.formData.language === 'Spanish' ? 'Error generando personaje' : 'Error generating character';
                }
            }

            // Build the system DM content including the campaign starter and generated player character (if present)
            // Use the campaign concept as the player-facing starter.
            const entry = (campaignSpec && campaignSpec.campaignConcept) ? campaignSpec.campaignConcept : campaignConcept;
            systemMessageContentDM = entry + ' Assume the player knows nothing. Allow for an organic introduction of information.';
            if (playerCharacter) {
                systemMessageContentDM += '\n\nPlayer Character:\n' + JSON.stringify(playerCharacter, null, 2);
                // save generated character into game setup for persistence
                this.$store.commit('setGameSetup', { ...this.formData, generatedCharacter: playerCharacter });
            }

            // If language is Spanish, instruct the AI to respond in Spanish
            if (this.formData.language === 'Spanish') {
                systemMessageContentDM = systemMessageContentDM + '\n\nPor favor responde en español. Responde todas las interacciones en español.';
            }

            // Set the system message content DM
            this.$store.commit('setSystemMessageContentDM', systemMessageContentDM);

            const gameId = this.$store.state.gameId;
            // Save initial game state to backend so the new game is persisted immediately
            const initialState = {
                gameId: gameId,
                userId: this.$store.state.userId || null,
                gameSetup: this.$store.state.gameSetup,
                conversation: [{ role: 'system', content: systemMessageContentDM }],
                summaryConversation: [],
                summary: '',
                totalTokenCount: 0,
                userAndAssistantMessageCount: 0,
                systemMessageContentDM: systemMessageContentDM
            };
            // Attach the structured campaignSpec to the initial state so it is persisted and injected on load
            if (campaignSpec) {
                initialState.campaignSpec = campaignSpec;
            }

            try {
                this.progressMessage = this.formData.language === 'Spanish' ? 'Guardando partida...' : 'Saving game...';
                await axios.post('/api/game-state/save', initialState);
                console.log('Initial game saved', initialState);
                this.$router.push({ name: 'ChatRoom', params: { id: gameId } });
            } catch (err) {
                console.error('Error saving initial game state:', err);
                this.progressMessage = this.formData.language === 'Spanish' ? 'Error guardando partida' : 'Error saving game';
            } finally {
                this.isStarting = false;
                // clear progress after a short delay so the user sees completion
                setTimeout(() => { this.progressMessage = ''; }, 1200);
            }
        }
    }
    };</script>


<style scoped>
.progress-message {
  margin-top: 14px;
  padding: 12px 16px;
  background: linear-gradient(180deg, #eef6ff 0%, #dfefff 100%);
  color: #0b3d91;
  border-radius: 8px;
  font-weight: 700;
  text-align: center;
  box-shadow: 0 2px 6px rgba(11,61,145,0.08);
}
</style>
