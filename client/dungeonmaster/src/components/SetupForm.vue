// GMAI/client/dungeonmaster/src/components/SetupForm.vue


<template>
    <div class="setup-page">
    <UIPanel>
      <form @submit.prevent="submitForm" aria-labelledby="setup-title" class="setup-form">
        <h1 id="setup-title" class="form-title">{{ $i18n.setup_title }}</h1>
        <h4 class="form-description">{{ $i18n.setup_desc }}</h4>

        <div v-if="setupPhase === 'confirm_character'" class="character-confirm-block" role="region" :aria-label="$i18n.confirm_character_continue">
          <p class="confirm-hint">{{ $i18n.character_ready_confirm }}</p>
          <div class="form-actions confirm-actions">
            <button type="button" class="ui-button secondary" @click="backToForm" :disabled="isStarting">{{ $i18n.back_to_edit }}</button>
            <button type="button" class="ui-button secondary" @click="regenerateCharacter" :disabled="isStarting">{{ $i18n.regenerate_character }}</button>
            <button type="button" class="ui-button" @click="confirmCharacterAndWorld" :disabled="isStarting" :aria-busy="isStarting">{{ $i18n.confirm_character_continue }}</button>
          </div>
        </div>

        <template v-if="setupPhase === 'form'">
        <!-- Game system selection removed; D&D 5e is the default -->
        <!-- Adventure setting removed; Classic Fantasy is used by default -->
        <!-- Language selection removed from setup; language is global via header -->
        <div class="form-row">
          <label for="character-gender" class="form-label">{{$i18n.character_gender}}</label>
          <select id="character-gender" v-model="formData.gender" class="control" :aria-label="$i18n.character_gender">
            <option value="Male">{{ $i18n.gender_male_short }}</option>
            <option value="Female">{{ $i18n.gender_female_short }}</option>
          </select>
        </div>

        <div class="form-row">
          <label for="character-name" class="form-label">{{$i18n.character_name}}</label>
          <input id="character-name" class="control" v-model="formData.characterName" type="text" :placeholder="$i18n.random" :aria-label="$i18n.character_name" />
        </div>

        <div class="form-row">
          <label for="character-race" class="form-label">{{$i18n.character_race}}</label>
          <select id="character-race" class="control" v-model="formData.characterRace" :aria-label="$i18n.character_race" @change="onRaceChange">
            <option v-for="r in $i18n.races" :key="r.id" :value="r.id">{{ r.label }}</option>
          </select>
        </div>

        <div class="form-row">
          <label for="character-class" class="form-label">{{$i18n.character_class}}</label>
          <select id="character-class" class="control" v-model="formData.characterClass" :aria-label="$i18n.character_class">
            <option v-for="c in availableClasses" :key="c.id" :value="c.id">{{ c.label }}</option>
          </select>
        </div>

        <div class="form-row" v-if="availableSubclasses.length">
          <label for="character-subclass" class="form-label">{{$i18n.subclass}}</label>
          <select id="character-subclass" class="control" v-model="formData.subclass" :aria-label="$i18n.subclass">
            <option v-for="s in availableSubclasses" :key="s.id" :value="s.id">{{ s.label }}</option>
          </select>
        </div>

        <div class="form-row">
          <label for="character-level" class="form-label">{{$i18n.character_level}}</label>
          <select id="character-level" class="control" v-model.number="formData.characterLevel" :aria-label="$i18n.character_level">
            <option v-for="n in 20" :key="n" :value="n">{{ n }}</option>
          </select>
        </div>
        
        <div class="form-actions">
          <button v-show="!isStarting" class="ui-button" type="submit">{{ $i18n.start_game }}</button>
        </div>
        </template>

        <!-- Progress message (prominent) -->
        <div v-if="progressMessage" class="progress-message" role="status" aria-live="polite">
            {{ progressMessage }}
        </div>
      </form>
    </UIPanel>
    <!-- :key bumps after each successful generate/regenerate so the sheet remounts with the new prop -->
    <!-- confirmSheetCharacter is set only from the latest API response or restore — never from a stale store merge -->
    <FloatingCard
      v-if="setupPhase === 'confirm_character' && confirmSheetCharacter"
      :key="'setup-pc-' + characterPreviewKey"
      :character="confirmSheetCharacter"
      :defaultOpen="true"
    />
    </div>
</template>

<script>
    import axios from 'axios';
    import UIPanel from '@/ui/Panel.vue';
    import FloatingCard from '@/ui/FloatingCard.vue';
    import { SESSION_SETUP_GAME_ID } from '@/setupSession.js';

    export default {
        data() {
            return {
                setupPhase: 'form',
                /** Last character sheet shown on confirm; source of truth for the floating card (avoids stale Vuex overwrites). */
                confirmSheetCharacter: null,
                characterPreviewKey: 0,
                isStarting: false,
                progressMessage: '',
                formData: {
                    gameSystem: 'Dungeons and Dragons 5th Edition',
                    characterName: '',
                    characterClass: '',
                    characterRace: '',
                    characterLevel: 1,
                    gender: 'Male',
                    subclass: 'random'
                }
            ,
            classes: [],
            races: [],
            subclasses: []
            };
        },
        components: { UIPanel, FloatingCard },
        created() {
          // Provide DnD 5e class and race lists for selects
          // initial values (labels come from $i18n.classes / $i18n.races)
          // Ensure defaults (use ids matching localized lists)
          if (!this.formData.characterClass) this.formData.characterClass = 'random';
          if (!this.formData.characterRace) this.formData.characterRace = 'random';
          // allowed classes mapping by race id (permits most combos, but restricts some unreasonable ones)
          this.allowedClassesByRace = {
            'random': null, // null means all allowed
            'human': null,
            'elf': null,
            'dwarf': null,
            'halfling': ['random','bard','cleric','druid','fighter','rogue','wizard','sorcerer','warlock'],
            'half-elf': null,
            'half-orc': ['random','barbarian','fighter','paladin','ranger','rogue','cleric'],
            'gnome': ['random','bard','cleric','druid','wizard','sorcerer','warlock','rogue'],
            'tiefling': null
          };
          // Subclasses mapping by class id: array of {id,label,minLevel}
          this.subclassesByClass = {
            'random': [],
            'barbarian': [{id:'berserker', label:'Berserker'}, {id:'totem', label:'Path of the Totem'}],
            'bard': [{id:'lore', label:'College of Lore'}, {id:'valor', label:'College of Valor'}],
            'cleric': [{id:'life', label:'Life Domain'}, {id:'war', label:'War Domain'}],
            'druid': [{id:'land', label:'Circle of the Land'}, {id:'moon', label:'Circle of the Moon'}],
            'fighter': [{id:'champion', label:'Champion'}, {id:'battle_master', label:'Battle Master'}],
            'monk': [{id:'way_of_open_hand', label:'Way of the Open Hand'}],
            'paladin': [{id:'devotion', label:'Oath of Devotion'}],
            'ranger': [{id:'hunter', label:'Hunter'}],
            'rogue': [{id:'thief', label:'Thief'}, {id:'assassin', label:'Assassin'}],
            'sorcerer': [{id:'draconic', label:'Draconic Bloodline'}],
            'warlock': [{id:'fiend', label:'The Fiend'}],
            'wizard': [{id:'evocation', label:'School of Evocation'}]
          };
          // classMinLevel: minimum class level at which subclass choice is available
          this.classMinLevel = {
            'random': 1,
            'barbarian': 3,
            'bard': 3,
            'cleric': 1,
            'druid': 2,
            'fighter': 3,
            'monk': 3,
            'paladin': 3,
            'ranger': 3,
            'rogue': 3,
            'sorcerer': 1,
            'warlock': 1,
            'wizard': 2,
            'artificer': 3
          };
        },
        computed: {
            availableClasses() {
              return (this.$i18n.classes || []).filter(c => this.isClassAllowed(c.id));
            }
            ,
            availableSubclasses() {
              return this.getAvailableSubclassesForClass(this.formData.characterClass, this.formData.characterLevel).map(s => ({ id: s.id, label: s.label, minLevel: s.minLevel }));
            },
        },
        mounted() {
            this.tryRestoreSetupFromServer();
        },
        methods: {
            tryPersistSetupSessionGameId() {
                const id = this.$store.state.gameId;
                if (!id) return;
                try {
                    sessionStorage.setItem(SESSION_SETUP_GAME_ID, id);
                } catch (e) {
                    /* ignore */
                }
            },
            clearSetupSessionGameId() {
                try {
                    sessionStorage.removeItem(SESSION_SETUP_GAME_ID);
                } catch (e) {
                    /* ignore */
                }
            },
            async tryRestoreSetupFromServer() {
                let sid = null;
                try {
                    sid = sessionStorage.getItem(SESSION_SETUP_GAME_ID);
                } catch (e) {
                    return;
                }
                if (!sid) return;
                if (this.$store.state.gameId && this.$store.state.gameId !== sid) return;
                try {
                    const { data } = await axios.get(
                        `/api/game-state/load/${encodeURIComponent(sid)}`,
                        { params: { _t: Date.now() } }
                    );
                    let sessionStill = null;
                    try {
                        sessionStill = sessionStorage.getItem(SESSION_SETUP_GAME_ID);
                    } catch (e) {
                        return;
                    }
                    if (sessionStill !== sid) return;
                    const gid = this.$store.state.gameId;
                    if (gid && gid !== sid) return;
                    const gc = data?.gameSetup?.generatedCharacter;
                    if (!gc || typeof gc !== 'object') return;
                    let clone;
                    try {
                        clone = JSON.parse(JSON.stringify(gc));
                    } catch (e) {
                        clone = gc;
                    }
                    this.confirmSheetCharacter = clone;
                    this.$store.commit('setGameId', sid);
                    const gs = data.gameSetup && typeof data.gameSetup === 'object' ? { ...data.gameSetup } : {};
                    this.$store.commit('setGameSetup', { ...gs, language: this.$store.state.language });
                    if (gc.name != null && String(gc.name).trim()) this.formData.characterName = String(gc.name).trim();
                    if (gc.level != null && !Number.isNaN(Number(gc.level))) {
                        this.formData.characterLevel = Number(gc.level);
                    }
                    this.setupPhase = 'confirm_character';
                    await this.$nextTick();
                    this.characterPreviewKey += 1;
                } catch (e) {
                    this.clearSetupSessionGameId();
                }
            },
            commitGameSetupWithCharacter(playerCharacter) {
                let pc = playerCharacter;
                try {
                    pc = JSON.parse(JSON.stringify(playerCharacter));
                } catch (e) {
                    /* use as-is */
                }
                this.confirmSheetCharacter = pc;
                this.$store.commit('setGameSetup', {
                    ...this.$store.state.gameSetup,
                    ...this.formData,
                    generatedCharacter: pc,
                    language: this.$store.state.language,
                });
                this.tryPersistSetupSessionGameId();
            },
            isClassAllowed(classId) {
                const raceId = this.formData.characterRace || 'random';
                const allowed = this.allowedClassesByRace[raceId];
                if (!allowed) return true;
                return allowed.includes(classId);
            },
            onRaceChange() {
                // If the currently selected class is not allowed for the new race, reset to 'random'
                if (!this.isClassAllowed(this.formData.characterClass)) {
                    this.formData.characterClass = 'random';
                }
            },
            // Choose a random race id (excluding 'random' id)
            chooseRandomRace() {
                const races = (this.$i18n.races || []).map(r => r.id).filter(id => id && id !== 'random');
                return races[Math.floor(Math.random() * races.length)];
            },
            // Choose a random class id allowed for a given race
            chooseRandomClassForRace(raceId) {
                const allowed = this.allowedClassesByRace[raceId];
                const classList = (this.$i18n.classes || []).map(c => c.id);
                let pool;
                if (!allowed) {
                    // all classes except 'random' are valid choices
                    pool = classList.filter(id => id !== 'random');
                } else {
                    // allowed may include 'random' or ids; filter to available class ids excluding 'random'
                    pool = allowed.filter(id => id !== 'random' && classList.includes(id));
                }
                if (!pool || pool.length === 0) {
                    // fallback to any class available
                    pool = classList.filter(id => id !== 'random');
                }
                return pool[Math.floor(Math.random() * pool.length)];
            },
            // get subclasses allowed for current class and level
            getAvailableSubclassesForClass(classId, level) {
                const minLevel = this.classMinLevel[classId] || 1;
                if (level < minLevel) return [];
                const list = this.subclassesByClass[classId] || [];
                return list;
            },
            // Resolve 'random' selections so final formData contains concrete ids respecting rules
            resolveRandomSelections() {
                // If both race and class are random: pick race then class compatible with it
                if ((this.formData.characterRace === 'random' || !this.formData.characterRace) &&
                    (this.formData.characterClass === 'random' || !this.formData.characterClass)) {
                    const race = this.chooseRandomRace();
                    const cls = this.chooseRandomClassForRace(race);
                    this.formData.characterRace = race;
                    this.formData.characterClass = cls;
                    // resolve subclass
                    const subs = this.getAvailableSubclassesForClass(cls, this.formData.characterLevel);
                    this.formData.subclass = subs.length ? subs[Math.floor(Math.random()*subs.length)].id : 'random';
                    return;
                }
                // If race is random but class is concrete: pick a race that allows that class
                if (this.formData.characterRace === 'random' && this.formData.characterClass && this.formData.characterClass !== 'random') {
                    // find races whose allowedClassesByRace includes the class (or null)
                    const candidateRaces = (this.$i18n.races || []).map(r => r.id).filter(rid => {
                        const allowed = this.allowedClassesByRace[rid];
                        if (!allowed) return true;
                        return allowed.includes(this.formData.characterClass);
                    }).filter(id => id !== 'random');
                    this.formData.characterRace = candidateRaces.length ? candidateRaces[Math.floor(Math.random() * candidateRaces.length)] : this.chooseRandomRace();
                    return;
                }
                // If class is random but race is concrete: pick a class allowed for that race
                if (this.formData.characterClass === 'random' && this.formData.characterRace && this.formData.characterRace !== 'random') {
                    const cls = this.chooseRandomClassForRace(this.formData.characterRace);
                    this.formData.characterClass = cls;
                    const subs = this.getAvailableSubclassesForClass(cls, this.formData.characterLevel);
                    this.formData.subclass = subs.length ? subs[Math.floor(Math.random()*subs.length)].id : 'random';
                    return;
                }
                // If subclass is random or invalid, resolve now if possible
                if (!this.formData.subclass || this.formData.subclass === 'random') {
                    const subs = this.getAvailableSubclassesForClass(this.formData.characterClass, this.formData.characterLevel);
                    this.formData.subclass = subs.length ? subs[Math.floor(Math.random()*subs.length)].id : 'random';
                }
                // otherwise both concrete — nothing to do
            },

        /** World-only campaign core + stages; does not receive character data (server ignores it anyway). */
        async generateCampaignConcept(gameId = null) {
            try {
                const response = await axios.post('/api/game-session/generate-campaign-core', {
                    gameId,
                    language: this.$store.state.language,
                    waitForStages: true,
                });
                return response.data;
            } catch (error) {
                console.error('Error generating campaign concept:', error);
            }
        },

        characterGenerationErrorMessage(err) {
            const d = err && err.response && err.response.data;
            if (d && typeof d.error === 'string' && d.error.trim()) {
                let msg = d.error.trim();
                if (d.detail && String(d.detail).trim()) {
                    msg += ` — ${String(d.detail).trim()}`;
                }
                if (d.hint && String(d.hint).trim()) {
                    msg += ` ${String(d.hint).trim()}`;
                }
                if (d.code === 'INVALID_MODEL_JSON' && d.preview && String(d.preview).trim()) {
                    msg += ` [model output preview: ${String(d.preview).trim().slice(0, 600)}${String(d.preview).length > 600 ? '…' : ''}]`;
                }
                if (d.code && d.code !== 'INVALID_MODEL_JSON') {
                    msg += ` (${d.code})`;
                }
                return msg;
            }
            return this.$i18n.error_generating_character;
        },

        async generatePlayerCharacter(gameId = null) {
            const response = await axios.post('/api/game-session/generate-character', {
                gameId,
                gameSetup: {
                    name: this.formData.characterName,
                    class: this.formData.characterClass,
                    race: this.formData.characterRace,
                    level: this.formData.characterLevel,
                    background: this.formData.characterBackground,
                    language: this.$store.state.language,
                },
                language: this.$store.state.language,
            });
            return response.data;
        },

    async submitForm() {
            if (this.setupPhase === 'confirm_character') return;
            this.resolveRandomSelections();
            this.isStarting = true;
            this.progressMessage = this.$i18n.generating_character;
            if (!this.$store.state.gameId) {
                this.clearSetupSessionGameId();
                this.$store.commit('createNewGame');
            }
            this.$store.commit('setGameSetup', {
                ...this.$store.state.gameSetup,
                ...this.formData,
                language: this.$store.state.language,
            });

            try {
                const charData = await this.generatePlayerCharacter(this.$store.state.gameId);
                if (charData && charData.playerCharacter) {
                    this.commitGameSetupWithCharacter(charData.playerCharacter);
                    await this.$nextTick();
                    this.characterPreviewKey += 1;
                } else {
                    throw new Error('No playerCharacter in response');
                }
            } catch (e) {
                console.error('Error generating player character:', e);
                if (e && e.response && e.response.data) {
                    console.error(
                        'generate-character response:',
                        e.response.status,
                        typeof e.response.data === 'string' ? e.response.data : JSON.stringify(e.response.data).slice(0, 2500)
                    );
                }
                this.progressMessage = this.characterGenerationErrorMessage(e);
                this.isStarting = false;
                setTimeout(() => {
                    this.progressMessage = '';
                }, 5200);
                return;
            }

            this.setupPhase = 'confirm_character';
            this.progressMessage = this.$i18n.character_ready_confirm;
            this.isStarting = false;
        },

        backToForm() {
            this.setupPhase = 'form';
            this.confirmSheetCharacter = null;
            const gs = { ...this.$store.state.gameSetup };
            delete gs.generatedCharacter;
            this.$store.commit('setGameSetup', gs);
            this.progressMessage = '';
        },

        async regenerateCharacter() {
            this.isStarting = true;
            this.progressMessage = this.$i18n.generating_character;
            try {
                const charData = await this.generatePlayerCharacter(this.$store.state.gameId);
                if (charData && charData.playerCharacter) {
                    this.commitGameSetupWithCharacter(charData.playerCharacter);
                    await this.$nextTick();
                    this.characterPreviewKey += 1;
                    this.progressMessage = this.$i18n.character_ready_confirm;
                } else {
                    throw new Error('No playerCharacter in response');
                }
            } catch (e) {
                console.error('Error regenerating player character:', e);
                if (e && e.response && e.response.data) {
                    console.error(
                        'generate-character response:',
                        e.response.status,
                        typeof e.response.data === 'string' ? e.response.data : JSON.stringify(e.response.data).slice(0, 2500)
                    );
                }
                this.progressMessage = this.characterGenerationErrorMessage(e);
                setTimeout(() => {
                    this.progressMessage = this.$i18n.character_ready_confirm;
                }, 5200);
            } finally {
                this.isStarting = false;
            }
        },

        async confirmCharacterAndWorld() {
            if (this.setupPhase !== 'confirm_character') return;
            const playerCharacter =
                this.confirmSheetCharacter || this.$store.state.gameSetup?.generatedCharacter;
            if (!playerCharacter) {
                this.progressMessage = this.$i18n.error_generating_character;
                return;
            }

            this.isStarting = true;
            this.progressMessage = this.$i18n.generating_campaign;
            const gen = await this.generateCampaignConcept(this.$store.state.gameId);
            let campaignConcept = '';
            let campaignSpec = null;
            if (gen && typeof gen === 'object' && gen.campaignConcept) {
                campaignConcept = gen.campaignConcept;
                campaignSpec = gen;
                const t = typeof gen.title === 'string' && gen.title.trim() ? gen.title.trim() : '';
                this.progressMessage = t ? `${this.$i18n.campaign_generated} «${t}».` : this.$i18n.campaign_generated;
            } else {
                console.error('Campaign generation failed or returned no campaignConcept', gen);
                this.progressMessage = this.$i18n.error_generating_campaign;
                this.isStarting = false;
                setTimeout(() => {
                    this.progressMessage = this.$i18n.character_ready_confirm;
                }, 2200);
                return;
            }

            const entry = (campaignSpec && campaignSpec.campaignConcept) ? campaignSpec.campaignConcept : campaignConcept;
            let systemMessageContentDM = entry + ' Assume the player knows nothing. Allow for an organic introduction of information.';
            systemMessageContentDM += '\n\nPlayer Character:\n' + JSON.stringify(playerCharacter, null, 2);

            if (this.$store.state.language === 'Spanish') {
                systemMessageContentDM = systemMessageContentDM + '\n\nPor favor responde en español. Responde todas las interacciones en español.';
            }

            this.$store.commit('setSystemMessageContentDM', systemMessageContentDM);

            const gameId = this.$store.state.gameId;
            let pcSave;
            try {
                pcSave = JSON.parse(JSON.stringify(playerCharacter));
            } catch (e) {
                pcSave = playerCharacter;
            }
            const gameSetupForSave = {
                ...this.$store.state.gameSetup,
                generatedCharacter: pcSave,
                language: this.$store.state.language,
            };
            const initialState = {
                gameId: gameId,
                userId: this.$store.state.userId || null,
                gameSetup: gameSetupForSave,
                conversation: [{ role: 'system', content: systemMessageContentDM }],
                summaryConversation: [],
                summary: '',
                totalTokenCount: 0,
                userAndAssistantMessageCount: 0,
                systemMessageContentDM: systemMessageContentDM
            };
            if (campaignSpec) {
                initialState.campaignSpec = campaignSpec;
            }

            try {
                this.progressMessage = this.$i18n.saving_game;
                await axios.post('/api/game-session/bootstrap-session', initialState);
                console.log('Initial game saved', initialState);
                this.clearSetupSessionGameId();
                this.setupPhase = 'form';
                this.$router.push({ name: 'ChatRoom', params: { id: gameId } });
            } catch (err) {
                console.error('Error saving initial game state:', err);
                this.progressMessage = this.$i18n.error_saving_game;
            } finally {
                this.isStarting = false;
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
/* Layout for setup form: aligned controls, consistent sizing */
.setup-form {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 18px 20px;
}
.form-row {
  display: flex;
  gap: 16px;
  align-items: center;
}
.control {
  flex: 1 1 auto;
  min-height: 48px;
  padding: 0.75rem 1rem;
  border-radius: 10px;
  border: 1px solid rgba(255,255,255,0.06);
  background: rgba(255,255,255,0.02);
  color: var(--gm-text);
  box-sizing: border-box;
}
.control::placeholder {
  color: rgba(230,225,216,0.45);
  opacity: 1;
}
.form-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
.form-label {
  width: 160px;
  color: var(--gm-text);
  font-weight: 600;
  padding-left: 6px;
}
.form-row select.control, .form-row input.control {
  background: rgba(255,255,255,0.03);
}
.character-confirm-block {
  margin-bottom: 8px;
}
.confirm-hint {
  margin: 0 0 14px;
  color: rgba(230, 225, 216, 0.88);
  line-height: 1.45;
}
.confirm-actions {
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}
.ui-button.secondary {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.12);
  box-shadow: none;
}
</style>
