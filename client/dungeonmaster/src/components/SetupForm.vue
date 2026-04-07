// GMAI/client/dungeonmaster/src/components/SetupForm.vue


<template>
    <div
        class="setup-page"
        :class="{
            'setup-page--confirm': setupPhase === 'confirm_character',
            'setup-page--lobby-inline': lobbyInline,
        }"
    >
    <UIPanel :class="{ 'ui-panel--setup-confirm': setupPhase === 'confirm_character' }">
      <form @submit.prevent="submitForm" aria-labelledby="setup-title" class="setup-form">
        <h1 id="setup-title" class="form-title">
          {{ setupHeadingTitle }}
        </h1>
        <p v-if="joinTokenConsumeBusy" class="join-flow-status" role="status">{{ $i18n.join_redirect_character_setup }}</p>
        <p v-if="joinInitBusy && joinGameIdFromRoute" class="join-flow-status" role="status">{{ $i18n.join_loading_campaign }}</p>
        <p v-if="joinFlowError" class="join-flow-error" role="alert">{{ joinFlowError }}</p>
        <h4 v-if="setupPhase === 'form' && !(joinGameIdFromRoute && joinInitBusy)" class="form-description">{{ setupHeadingDesc }}</h4>

        <template v-if="setupPhase === 'confirm_character'">
          <div class="character-confirm-block" role="region" :aria-label="$i18n.setup_confirm_title">
            <FloatingCard
              v-if="confirmSheetCharacter"
              :key="'setup-pc-' + characterPreviewKey"
              :character="confirmSheetCharacter"
              :default-open="true"
              embedded
            />
            <p class="confirm-hint">{{ characterReadyConfirmText }}</p>
            <div class="confirm-actions" :class="{ 'confirm-actions--join-party': usePartyRoomConfirmUi }">
              <template v-if="usePartyRoomConfirmUi && partyLateJoinInviteePlaying">
                <button
                  type="button"
                  class="ui-button confirm-btn confirm-btn--primary confirm-btn--join-party"
                  @click="joinPartyRoomInProgress"
                  :disabled="setupActionDisabled"
                  :aria-busy="isStarting"
                >
                  <span v-if="isStarting" class="setup-submit-busy confirm-btn-busy">{{ $i18n.starting }}</span>
                  <span v-else>{{ $i18n.confirm_character_join_game }}</span>
                </button>
                <div
                  class="confirm-actions__secondary-row confirm-actions__secondary-row--single"
                  role="group"
                  :aria-label="$i18n.confirm_regenerate_action_aria"
                >
                  <button
                    type="button"
                    class="ui-button secondary confirm-btn"
                    @click="regenerateCharacter"
                    :disabled="setupActionDisabled"
                    :aria-busy="setupBusyAction === 'regenerate'"
                  >
                    <span v-if="setupBusyAction === 'regenerate'" class="setup-submit-busy confirm-btn-busy">{{ $i18n.generating_character }}</span>
                    <span v-else>{{ $i18n.regenerate_character }}</span>
                  </button>
                </div>
              </template>
              <template v-else-if="usePartyRoomConfirmUi">
                <button
                  v-if="!lobbyInline"
                  type="button"
                  class="ui-button confirm-btn confirm-btn--primary confirm-btn--join-party"
                  @click="confirmPartyLobbyCharacter"
                  :disabled="setupActionDisabled"
                  :aria-busy="isStarting"
                >
                  <span v-if="isStarting" class="setup-submit-busy confirm-btn-busy">{{ confirmPartyLobbyBusyLabel }}</span>
                  <span v-else>{{ $i18n.confirm_character_mark_ready }}</span>
                </button>
                <div
                  class="confirm-actions__secondary-row confirm-actions__secondary-row--single"
                  role="group"
                  :aria-label="$i18n.confirm_regenerate_action_aria"
                >
                  <button
                    type="button"
                    class="ui-button secondary confirm-btn"
                    @click="regenerateCharacter"
                    :disabled="setupActionDisabled"
                    :aria-busy="setupBusyAction === 'regenerate'"
                  >
                    <span v-if="setupBusyAction === 'regenerate'" class="setup-submit-busy confirm-btn-busy">{{ $i18n.generating_character }}</span>
                    <span v-else>{{ $i18n.regenerate_character }}</span>
                  </button>
                </div>
              </template>
              <template v-else>
                <div class="confirm-actions__secondary-row" role="group" :aria-label="$i18n.confirm_secondary_actions_aria">
                  <button type="button" class="ui-button secondary confirm-btn" @click="backToForm" :disabled="setupActionDisabled">
                    {{ $i18n.back_to_edit }}
                  </button>
                  <button
                    type="button"
                    class="ui-button secondary confirm-btn"
                    @click="regenerateCharacter"
                    :disabled="setupActionDisabled"
                    :aria-busy="setupBusyAction === 'regenerate'"
                  >
                    <span v-if="setupBusyAction === 'regenerate'" class="setup-submit-busy confirm-btn-busy">{{ $i18n.generating_character }}</span>
                    <span v-else>{{ $i18n.regenerate_character }}</span>
                  </button>
                </div>
                <button
                  type="button"
                  class="ui-button confirm-btn confirm-btn--primary"
                  @click="confirmCharacterAndWorld"
                  :disabled="setupActionDisabled"
                  :aria-busy="setupBusyAction === 'campaign' || setupBusyAction === 'save'"
                >
                  <span v-if="setupBusyAction === 'campaign'" class="setup-submit-busy confirm-btn-busy">{{ $i18n.generating_campaign }}</span>
                  <span v-else-if="setupBusyAction === 'save'" class="setup-submit-busy confirm-btn-busy">{{ $i18n.saving_game }}</span>
                  <span v-else>{{ confirmCharacterButtonLabel }}</span>
                </button>
              </template>
            </div>
          </div>
        </template>

        <template v-if="setupPhase === 'form' && !(joinGameIdFromRoute && joinFlowError) && !joinTokenConsumeBusy && !(joinGameIdFromRoute && joinInitBusy)">
        <!-- Game system selection removed; D&D 5e is the default -->
        <!-- Adventure setting removed; Classic Fantasy is used by default -->
        <!-- Language selection removed from setup; language is global via header -->
        <div
          class="setup-form-fields"
          :class="{ 'setup-form-fields--lobby-compact': lobbyInline }"
        >
        <div class="setup-form-row-group setup-form-row-group--2">
          <div class="form-row">
            <label for="character-name" class="form-label">{{$i18n.character_name}}</label>
            <input
              id="character-name"
              class="control"
              v-model="formData.characterName"
              type="text"
              autocomplete="off"
              :placeholder="$i18n.character_name_placeholder"
              :aria-label="$i18n.character_name"
              @input="onCharacterNameInput"
            />
          </div>
          <div class="form-row">
            <label for="character-gender" class="form-label">{{$i18n.character_gender}}</label>
            <select id="character-gender" v-model="formData.gender" class="control" :aria-label="$i18n.character_gender">
              <option value="Male">{{ $i18n.gender_male_short }}</option>
              <option value="Female">{{ $i18n.gender_female_short }}</option>
            </select>
          </div>
        </div>

        <div class="setup-form-row-group setup-form-row-group--2">
          <div class="form-row">
            <label for="character-race" class="form-label">{{$i18n.character_race}}</label>
            <select id="character-race" class="control" v-model="formData.characterRace" :aria-label="$i18n.character_race" @change="onRaceChange">
              <option v-for="r in availableRaces" :key="r.id" :value="r.id">{{ r.label }}</option>
            </select>
          </div>
          <div class="form-row">
            <label for="character-subrace" class="form-label">{{$i18n.subrace}}</label>
            <select
              v-if="hasSubraceOptions"
              id="character-subrace"
              class="control"
              v-model="formData.subrace"
              :aria-label="$i18n.subrace"
              @change="onSubraceChange"
            >
              <option v-for="s in availableSubraces" :key="s.id" :value="s.id">{{ s.label }}</option>
            </select>
            <select v-else id="character-subrace" class="control" disabled :aria-label="$i18n.subrace">
              <option value="">—</option>
            </select>
          </div>
        </div>

        <div class="setup-form-row-group setup-form-row-group--3">
          <div class="form-row">
            <label for="character-class" class="form-label">{{$i18n.character_class}}</label>
            <select id="character-class" class="control" v-model="formData.characterClass" :aria-label="$i18n.character_class" @change="onClassChange">
              <option v-for="c in availableClasses" :key="c.id" :value="c.id">{{ c.label }}</option>
            </select>
          </div>
          <div class="form-row">
            <label for="character-subclass" class="form-label">{{$i18n.subclass}}</label>
            <select
              v-if="hasSubclassOptions"
              id="character-subclass"
              class="control"
              v-model="formData.subclass"
              :aria-label="$i18n.subclass"
              @change="onSubclassChange"
            >
              <option v-for="s in availableSubclasses" :key="s.id" :value="s.id">{{ s.label }}</option>
            </select>
            <select v-else id="character-subclass" class="control" disabled :aria-label="$i18n.subclass">
              <option value="">—</option>
            </select>
          </div>
          <div class="form-row">
            <label for="character-level" class="form-label">{{$i18n.character_level}}</label>
            <select id="character-level" class="control" v-model.number="formData.characterLevel" :aria-label="$i18n.character_level" @change="onLevelChange">
              <option v-for="n in 20" :key="n" :value="n">{{ n }}</option>
            </select>
          </div>
        </div>
        </div>
        
        <div class="form-actions">
          <button
            class="ui-button"
            type="submit"
            :disabled="setupActionDisabled"
            :aria-busy="isStarting"
            :aria-label="isStarting ? $i18n.generating_character : $i18n.create_character"
          >
            <span v-if="isStarting" class="setup-submit-busy">{{ $i18n.generating_character }}</span>
            <span v-else>{{ $i18n.create_character }}</span>
          </button>
        </div>
        </template>

        <!-- Errors / rare notices only; confirm-step loading is on the buttons -->
        <div v-if="progressMessage && showFullProgressBanner" class="progress-message" role="status" aria-live="polite">
            {{ progressMessage }}
        </div>
      </form>
    </UIPanel>
    </div>
</template>

<script>
    import axios from 'axios';
    import { fetchGameStateLoad } from '@/utils/fetchGameStateLoad.js';
    import UIPanel from '@/ui/Panel.vue';
    import FloatingCard from '@/ui/FloatingCard.vue';
    import {
        SESSION_SETUP_GAME_ID,
        SESSION_JOIN_UI_GAME_ID,
        SESSION_CONSUME_INVITE,
        SESSION_CHAT_TOAST,
    } from '@/setupSession.js';

    /**
     * Character / regenerate tracing — logs use prefix `[GMAI character:` via console.log (not the server terminal).
     * Enable any ONE of:
     * - URL: `?gmaiDebugCharacter=1` (same tab; no reload needed if you add it and navigate)
     * - localStorage: `gmDebugCharacterFlow` = `1` for this origin, then reload
     * - DevTools console: `window.__GMAI_DEBUG_CHARACTER__ = true` then generate/regenerate again
     * - `npm run serve` dev bundle: also on when `process.env.NODE_ENV === 'development'`
     * Server (Go) logs are separate: `DM_DEBUG_CHARACTER_FLOW=true` in the API process.
     */
    const GM_DEBUG_CHARACTER_FLOW_LS = 'gmDebugCharacterFlow';
    const SESSION_DEBUG_HINT_LS = 'gmaiCharDebugHintShown';
    const SESSION_DEBUG_ON_ACK_LS = 'gmaiCharDebugOnAck';

    function characterFlowDebugUrlOn() {
        try {
            if (typeof window === 'undefined' || !window.location) return false;
            const search = window.location.search || '';
            if (search) {
                const q = new URLSearchParams(search);
                if (q.get('gmaiDebugCharacter') === '1' || q.get('gmDebugCharacterFlow') === '1') {
                    return true;
                }
            }
            const hash = window.location.hash || '';
            const qi = hash.indexOf('?');
            if (qi >= 0) {
                const q = new URLSearchParams(hash.slice(qi + 1));
                if (q.get('gmaiDebugCharacter') === '1' || q.get('gmDebugCharacterFlow') === '1') {
                    return true;
                }
            }
        } catch (e) {
            /* ignore */
        }
        return false;
    }

    export default {
        props: {
            /** Rendered inside ChatRoom party lobby: same flow as /setup?joinGame without leaving chat. */
            lobbyInline: {
                type: Boolean,
                default: false,
            },
            /** Set by ChatRoom when everyone is ready or adventure is starting — disables lobby inline actions. */
            lobbyInteractionsLocked: {
                type: Boolean,
                default: false,
            },
            /**
             * After /generate-character, optional hook (e.g. ChatRoom loadGameState) so transcript + lobby mirror refresh.
             * SetupForm always follows with GET /game-state/load to align Vuex, confirm sheet, and join flags.
             * @param {string} gameId
             * @returns {Promise<void>}
             */
            afterCharacterPersisted: {
                type: Function,
                default: null,
            },
        },
        emits: [
            'lobby-character-done',
            'lobby-inline-wizard-hold',
            'lobby-inline-confirm-sheet',
            /** Party lobby: after /generate-character, parent closes “edit” so the embedded sheet (same as refresh) shows instead of the confirm step. */
            'lobby-inline-after-character-saved',
        ],
        data() {
            return {
                setupPhase: 'form',
                /** Invited player: create PC for an existing campaign (no new campaign generation). */
                joinMode: false,
                joinFlowError: '',
                joinInitBusy: false,
                joinTokenConsumeBusy: false,
                /** Mirrors SESSION_JOIN_UI_GAME_ID when it matches store gameId (join UX after ?joinGame= drops). */
                joinFlowPersistedGameId: null,
                /** From GET /game-state/load; when false, always use party join UI (Join party) even if joinMode dropped. */
                setupViewerIsGameOwner: null,
                /** Last character sheet shown on confirm; source of truth for the floating card (avoids stale Vuex overwrites). */
                confirmSheetCharacter: null,
                characterPreviewKey: 0,
                isStarting: false,
                /** Party lobby confirm: 'party' (POST party-ready) vs 'adventure' (POST start-party-adventure). */
                lobbyConfirmBusyStep: null,
                /** confirm step: which long action is running (button labels + spinners). */
                setupBusyAction: null,
                progressMessage: '',
                /** True after the user types in the name field; false when name came from preview/sheet. Used on regenerate. */
                characterNameIsManual: false,
                /** Persisted intent: user explicitly chose random (do not lose on refresh/prefill). */
                wizardRandomIntent: {
                    characterRace: false,
                    characterClass: false,
                    subrace: false,
                    subclass: false,
                },
                formData: {
                    characterName: '',
                    characterClass: '',
                    characterRace: '',
                    characterLevel: 1,
                    gender: 'Male',
                    subclass: 'random',
                    subrace: 'random',
                },
            };
        },
        components: { UIPanel, FloatingCard },
        created() {
          if (!this.formData.characterClass) this.formData.characterClass = 'random';
          if (!this.formData.characterRace) this.formData.characterRace = 'random';
          try {
            const j = sessionStorage.getItem(SESSION_JOIN_UI_GAME_ID);
            const gid = this.$store.state.gameId;
            if (j && gid && String(j) === String(gid)) {
              this.joinFlowPersistedGameId = j;
            }
          } catch (e) {
            /* ignore */
          }
        },
        computed: {
            /** BCP 47 tag for list sorting; matches App.vue Spanish detection (not only exact `Spanish`). */
            i18nListSortLocaleTag() {
              const lang = (this.$store && this.$store.state && this.$store.state.language) || 'English';
              return String(lang).toLowerCase().startsWith('span') ? 'es' : 'en';
            },
            i18nListSortCollator() {
              return new Intl.Collator(this.i18nListSortLocaleTag, {
                sensitivity: 'base',
                numeric: true,
              });
            },
            /** DnD 5e setup rules from GET /api/meta/character-options (Vuex). */
            setupRules() {
                const c = this.$store.state.characterCatalog;
                if (!c) {
                    return {
                        allowedClassesByRace: {},
                        subclassesByClass: { random: [] },
                        subracesByRace: {},
                        classMinLevel: { random: 1 },
                    };
                }
                return {
                    allowedClassesByRace: c.allowedClassesByRace || {},
                    subclassesByClass: c.subclassesByClass || { random: [] },
                    subracesByRace: c.subracesByRace || {},
                    classMinLevel: c.classMinLevel || { random: 1 },
                };
            },
            allowedClassesByRace() {
                return this.setupRules.allowedClassesByRace;
            },
            subclassesByClass() {
                return this.setupRules.subclassesByClass;
            },
            subracesByRace() {
                return this.setupRules.subracesByRace;
            },
            classMinLevel() {
                return this.setupRules.classMinLevel;
            },
            availableRaces() {
              const list = this.$i18n.races || [];
              const cmp = this.i18nListSortCollator;
              const randomOpt = list.find((r) => r.id === 'random');
              const rest = list
                .filter((r) => r.id !== 'random')
                .sort((a, b) => cmp.compare(a.label, b.label));
              return randomOpt ? [randomOpt, ...rest] : rest;
            },
            availableClasses() {
              const list = (this.$i18n.classes || []).filter((c) => this.isClassAllowed(c.id));
              const cmp = this.i18nListSortCollator;
              const randomOpt = list.find((c) => c.id === 'random');
              const rest = list
                .filter((c) => c.id !== 'random')
                .sort((a, b) => cmp.compare(a.label, b.label));
              return randomOpt ? [randomOpt, ...rest] : rest;
            },
            availableSubclasses() {
              const labels = (this.$i18n && this.$i18n.subclass_labels) || {};
              const cmp = this.i18nListSortCollator;
              const raw = this.getAvailableSubclassesForClass(this.formData.characterClass, this.formData.characterLevel).map((s) => ({
                id: s.id,
                label: labels[s.id] || s.label,
                minLevel: s.minLevel,
              }));
              if (!raw.length) return [];
              const randomLabel = (this.$i18n && this.$i18n.random) ? String(this.$i18n.random) : 'Random';
              const rest = [...raw].sort((a, b) => cmp.compare(a.label, b.label));
              return [{ id: 'random', label: randomLabel }, ...rest];
            },
            availableSubraces() {
              const labels = (this.$i18n && this.$i18n.subrace_labels) || {};
              const cmp = this.i18nListSortCollator;
              const raw = this.getAvailableSubracesForRace(this.formData.characterRace).map((s) => ({
                id: s.id,
                label: labels[s.id] || s.label,
              }));
              if (!raw.length) return [];
              const randomLabel = (this.$i18n && this.$i18n.random) ? String(this.$i18n.random) : 'Random';
              const rest = [...raw].sort((a, b) => cmp.compare(a.label, b.label));
              return [{ id: 'random', label: randomLabel }, ...rest];
            },
            /** True when the current race has PHB subrace options (row always shows; otherwise subrace select is disabled). */
            hasSubraceOptions() {
                return this.getAvailableSubracesForRace(this.formData.characterRace).length > 0;
            },
            /** True when class+level allow a subclass pick (row always shows; otherwise subclass select is disabled). */
            hasSubclassOptions() {
                return this.getAvailableSubclassesForClass(this.formData.characterClass, this.formData.characterLevel).length > 0;
            },
            showFullProgressBanner() {
                if (!this.progressMessage) return false;
                if (this.setupPhase === 'confirm_character' && this.isStarting) return false;
                return true;
            },
            setupActionDisabled() {
                return this.isStarting || (this.lobbyInline && this.lobbyInteractionsLocked);
            },
            /** Busy text on “Mark ready” in inline lobby (avoids generic “Starting…” while only saving ready). */
            confirmPartyLobbyBusyLabel() {
                if (this.lobbyConfirmBusyStep === 'adventure') return this.$i18n.lobby_starting;
                return this.$i18n.lobby_saving_ready_then_start;
            },
            joinGameIdFromRoute() {
                if (this.lobbyInline) {
                    const g = this.$store.state.gameId;
                    return g != null && String(g).trim() !== '' ? String(g).trim() : '';
                }
                const q = this.$route && this.$route.query && this.$route.query.joinGame;
                return q != null && String(q).trim() !== '' ? String(q).trim() : '';
            },
            /** Opened from ChatRoom lobby (character flow); keeps party UI even if Vuex lost `gameSetup.party`. */
            fromLobbySetupQuery() {
                if (this.lobbyInline) return true;
                return String(this.$route.query && this.$route.query.fromLobby) === '1';
            },
            /** joinGame in query, or invite token handoff from /join-party (session) before query updates. */
            inJoinPartyFlow() {
                return !!(this.joinGameIdFromRoute || this.joinTokenConsumeBusy);
            },
            joinUiActive() {
                const gid = this.$store.state.gameId;
                const persisted = this.joinFlowPersistedGameId;
                const byPersist = !!(persisted && gid && String(persisted) === String(gid));
                if (gid && this.setupViewerIsGameOwner === false) return true;
                if (this.setupViewerIsGameOwner === true) {
                    return !!(this.joinMode && (this.inJoinPartyFlow || byPersist));
                }
                return this.joinMode || this.inJoinPartyFlow || byPersist;
            },
            /** Party table before play: lobby UI for character + confirm + ready (not host campaign wizard). */
            setupLobbyFlowActive() {
                const p = this.$store.state.gameSetup && this.$store.state.gameSetup.party;
                if (p && typeof p === 'object' && !Array.isArray(p)) {
                    const ph = p.phase != null && String(p.phase).trim() !== '' ? String(p.phase) : '';
                    if (ph === 'playing') return false;
                    if (ph === 'lobby' || ph === 'starting') return true;
                }
                if (this.fromLobbySetupQuery) return true;
                const gid = this.$store.state.gameId;
                const jg = this.joinGameIdFromRoute;
                return !!(gid && jg && String(gid) === String(jg));
            },
            usePartyRoomConfirmUi() {
                return this.joinUiActive || this.setupLobbyFlowActive;
            },
            /** Party `phase` from store (e.g. lobby | starting | playing). */
            partyPhaseFromStore() {
                const p = this.$store.state.gameSetup && this.$store.state.gameSetup.party;
                return p && p.phase != null && String(p.phase).trim() !== '' ? String(p.phase).trim() : '';
            },
            partyPhaseLower() {
                return String(this.partyPhaseFromStore || '').toLowerCase();
            },
            /**
             * True only when the server explicitly reports in-play phase (case-insensitive).
             * Missing/unknown phase is treated as pre-play so invitees get Mark ready, not Join party.
             */
            setupPartyIsExplicitlyPlaying() {
                return this.partyPhaseLower === 'playing';
            },
            /**
             * Invitee, invite flow, and table explicitly in play — Join party (late join).
             * Requires joinMode so hosts never hit this path; requires strict non-owner.
             */
            partyLateJoinInviteePlaying() {
                return (
                    this.usePartyRoomConfirmUi &&
                    this.joinMode === true &&
                    this.setupViewerIsGameOwner === false &&
                    this.setupPartyIsExplicitlyPlaying
                );
            },
            setupHeadingTitle() {
                if (this.partyLateJoinInviteePlaying && this.setupPhase === 'form') {
                    return this.$i18n.setup_join_title;
                }
                if (this.usePartyRoomConfirmUi && this.setupPhase === 'form') {
                    return this.$i18n.setup_party_lobby_form_title;
                }
                if (this.joinUiActive && this.setupPhase === 'form') return this.$i18n.setup_join_title;
                return this.setupPhase === 'confirm_character' ? this.$i18n.setup_confirm_title : this.$i18n.setup_title;
            },
            setupHeadingDesc() {
                if (this.partyLateJoinInviteePlaying && this.setupPhase === 'form') {
                    return this.$i18n.setup_party_late_join_desc;
                }
                if (this.usePartyRoomConfirmUi && this.setupPhase === 'form') {
                    return this.$i18n.setup_party_lobby_form_desc;
                }
                if (this.joinUiActive && this.setupPhase === 'form') return this.$i18n.setup_join_desc;
                return this.$i18n.setup_desc;
            },
            characterReadyConfirmText() {
                if (this.partyLateJoinInviteePlaying) return this.$i18n.character_ready_join_party_playing;
                if (this.usePartyRoomConfirmUi && this.lobbyInline) {
                    return this.$i18n.character_ready_party_room_roster_only;
                }
                if (this.usePartyRoomConfirmUi) return this.$i18n.character_ready_party_room;
                return this.$i18n.character_ready_confirm;
            },
            confirmCharacterButtonLabel() {
                return this.$i18n.confirm_character_continue;
            },
        },
        watch: {
            setupPhase() {
                this.emitLobbyInlineConfirmSheet();
            },
            confirmSheetCharacter: {
                deep: true,
                handler() {
                    this.emitLobbyInlineConfirmSheet();
                },
            },
            lobbyInline(isLobby) {
                if (!isLobby) {
                    this.$emit('lobby-inline-confirm-sheet', null);
                } else {
                    this.emitLobbyInlineConfirmSheet();
                }
            },
            '$store.state.setupWizardResetTick'() {
                this.resetWizardFromHeaderNew();
            },
            '$store.state.gameId'(to, from) {
                if (String(to ?? '') !== String(from ?? '')) {
                    this.setupViewerIsGameOwner = null;
                }
            },
            joinGameIdFromRoute: {
                immediate: true,
                handler(val) {
                    if (val) this.initJoinFlow(val);
                },
            },
            /** Lobby: when Vuex gains your sheet after load, fill the wizard if picks are still default (race/class random). */
            '$store.state.gameSetup.playerCharacters': {
                deep: true,
                handler() {
                    this.onLobbyPlayerCharactersMaybePrefill();
                },
            },
        },
        mounted() {
            // Vue 3: string watch '$store.state.gameId' is unreliable; use $watch on the getter.
            this._unwatchGameId = this.$watch(
                () => this.$store.state.gameId,
                (val) => {
                    try {
                        const j = sessionStorage.getItem(SESSION_JOIN_UI_GAME_ID);
                        if (!j) return;
                        // Do not clear join session when gameId is null — transient null would drop the only
                        // copy of the party gameId and the next submit could lose the join target.
                        if (!val) return;
                        if (String(j) !== String(val)) {
                            this.joinFlowPersistedGameId = null;
                            sessionStorage.removeItem(SESSION_JOIN_UI_GAME_ID);
                        }
                    } catch (e) {
                        /* ignore */
                    }
                }
            );
            try {
                const j = sessionStorage.getItem(SESSION_JOIN_UI_GAME_ID);
                const gid = this.$store.state.gameId;
                if (j && gid && String(j) === String(gid)) {
                    this.joinFlowPersistedGameId = j;
                }
            } catch (e) {
                /* ignore */
            }
            let consumeToken = '';
            try {
                consumeToken = String(sessionStorage.getItem(SESSION_CONSUME_INVITE) || '').trim();
                if (consumeToken) {
                    sessionStorage.removeItem(SESSION_CONSUME_INVITE);
                }
            } catch (e) {
                /* ignore */
            }
            if (consumeToken && !this.joinGameIdFromRoute) {
                void this.consumeJoinInviteToken(consumeToken);
            } else if (!this.joinGameIdFromRoute) {
                this.tryRestoreSetupFromServer();
            }
            if (this.lobbyInline) {
                this.prefillInlineLobbyFormIfPossible();
                this.$nextTick(() => this.prefillInlineLobbyFormIfPossible());
            }
            this.logCharacterDebugBootstrap();
            this.emitLobbyInlineConfirmSheet();
        },
        beforeUnmount() {
            if (this.lobbyInline) {
                this.$emit('lobby-inline-wizard-hold', false);
                this.$emit('lobby-inline-confirm-sheet', null);
            }
            if (typeof this._unwatchGameId === 'function') {
                this._unwatchGameId();
                this._unwatchGameId = null;
            }
        },
        methods: {
            /** Keeps ChatRoom roster in sync when `confirmSheetCharacter` is set (including mid-submit before `setupPhase` flips). */
            emitLobbyInlineConfirmSheet() {
                if (!this.lobbyInline) return;
                const sheet =
                    this.confirmSheetCharacter && typeof this.confirmSheetCharacter === 'object'
                        ? this.confirmSheetCharacter
                        : null;
                this.$emit('lobby-inline-confirm-sheet', sheet);
            },
            persistJoinUiGameId(gid) {
                if (!gid) return;
                const s = String(gid);
                this.joinFlowPersistedGameId = s;
                try {
                    sessionStorage.setItem(SESSION_JOIN_UI_GAME_ID, s);
                } catch (e) {
                    /* ignore */
                }
            },
            clearJoinUiGameId() {
                this.joinFlowPersistedGameId = null;
                try {
                    sessionStorage.removeItem(SESSION_JOIN_UI_GAME_ID);
                } catch (e) {
                    /* ignore */
                }
            },
            /** Matches ChatRoom: hide lobby wizard after confirm until user taps “edit”. */
            persistLobbyCharacterFlowDoneSession(gid) {
                const g = gid != null ? String(gid).trim() : '';
                if (!g) return;
                try {
                    sessionStorage.setItem(`dm_lobby_char_done_${g}`, '1');
                } catch (e) {
                    /* ignore */
                }
            },
            /**
             * Host and every invitee share one string id: `GameState.gameId`. Never invent `Date.now()` for joiners.
             * @returns {boolean} true if store.gameId is set after recovery
             */
            recoverSetupGameIdFromSession() {
                if (this.$store.state.gameId) return true;
                if (this.joinGameIdFromRoute) {
                    this.$store.commit('setGameId', this.joinGameIdFromRoute);
                    return true;
                }
                if (this.joinFlowPersistedGameId) {
                    this.$store.commit('setGameId', this.joinFlowPersistedGameId);
                    return true;
                }
                let recovered = '';
                try {
                    recovered =
                        String(sessionStorage.getItem(SESSION_JOIN_UI_GAME_ID) || '').trim() ||
                        String(sessionStorage.getItem(SESSION_SETUP_GAME_ID) || '').trim();
                } catch (e) {
                    /* ignore */
                }
                if (recovered) {
                    this.$store.commit('setGameId', recovered);
                    this.joinFlowPersistedGameId = recovered;
                    return true;
                }
                return false;
            },
            /** True if we must not start an ephemeral client-only game id (join / party setup). */
            isPartyJoinSetupContext() {
                if (
                    this.joinUiActive ||
                    this.setupViewerIsGameOwner === false ||
                    this.joinMode ||
                    this.inJoinPartyFlow ||
                    this.joinGameIdFromRoute
                ) {
                    return true;
                }
                try {
                    if (sessionStorage.getItem(SESSION_JOIN_UI_GAME_ID)) return true;
                    if (sessionStorage.getItem('dm_pending_invite')) return true;
                } catch (e) {
                    /* ignore */
                }
                return false;
            },
            /**
             * After /generate-character, URL join flags may be missing; load game once and treat non-owners as join UX.
             */
            /**
             * Sync join/host flags from server for the current store gameId.
             * @returns {boolean} false if the flow should stop (stale party game or auth lost); true otherwise.
             */
            async syncJoinUiFromServerIfNeeded() {
                if (this.joinUiActive) return true;
                const gid = this.$store.state.gameId;
                if (!gid) return true;
                if (!this.$store.getters.isAuthenticated) {
                    return false;
                }
                try {
                    const { data } = await fetchGameStateLoad(gid);
                    if (!data || String(data.gameId) !== String(gid)) return true;
                    this.applyViewerRoleFromLoadPayload(data);
                    let notOwner = false;
                    if (Object.prototype.hasOwnProperty.call(data, 'viewerIsGameOwner')) {
                        notOwner = data.viewerIsGameOwner === false;
                    } else {
                        const uid = this.resolveViewerUserId();
                        if (!uid) return true;
                        notOwner = data.ownerUserId != null && String(data.ownerUserId) !== String(uid);
                    }
                    if (notOwner) {
                        this.joinMode = true;
                        this.persistJoinUiGameId(gid);
                    }
                    return true;
                } catch (e) {
                    const st = e && e.response && e.response.status;
                    if (st === 404 && this.isPartyJoinSetupContext()) {
                        this.joinFlowError = this.$i18n.join_game_gone;
                        await this.handleStaleOrMissingPartyGame(gid);
                        return false;
                    }
                    if (st === 401) {
                        return false;
                    }
                    return true;
                }
            },
            /**
             * After prefill from the persisted sheet, restore wizard selects that stayed empty/random
             * when the sheet did not map cleanly (e.g. label/id edge cases). Used after regenerate.
             */
            restoreWizardFieldsIfIncompleteAfterPrefill(prev) {
                if (!prev || typeof prev !== 'object') return;
                const fd = this.formData;
                const pick = (cur, p) => {
                    const bad = !cur || cur === 'random';
                    const pOk = p && p !== 'random';
                    return bad && pOk ? p : cur;
                };
                fd.characterRace = pick(fd.characterRace, prev.characterRace);
                fd.characterClass = pick(fd.characterClass, prev.characterClass);
                fd.subclass = pick(fd.subclass, prev.subclass);
                fd.subrace = pick(fd.subrace, prev.subrace);
                const lvl = Number(fd.characterLevel);
                const plvl = Number(prev.characterLevel);
                if (Number.isNaN(lvl) || lvl < 1 || lvl > 20) {
                    if (!Number.isNaN(plvl) && plvl >= 1 && plvl <= 20) {
                        fd.characterLevel = plvl;
                    }
                }
                if (prev.gender === 'Male' || prev.gender === 'Female') {
                    if (fd.gender !== 'Male' && fd.gender !== 'Female') {
                        fd.gender = prev.gender;
                    }
                }
            },
            /**
             * After the server persists a generated sheet, rehydrate from GET /game-state/load (same as refresh).
             * Runs optional afterCharacterPersisted first (e.g. full ChatRoom load), then applies load payload here.
             * @param {{ retainWizardFieldsIfPrefillIncomplete?: boolean }} [opts] — after regenerate, keep prior form picks when prefill leaves race/class as random.
             */
            async syncPersistedCharacterFromServer(opts = {}) {
                const retainWizard = opts.retainWizardFieldsIfPrefillIncomplete === true;
                const wizardSnap = retainWizard
                    ? {
                          characterRace: this.formData.characterRace,
                          characterClass: this.formData.characterClass,
                          subclass: this.formData.subclass,
                          subrace: this.formData.subrace,
                          characterLevel: this.formData.characterLevel,
                          gender: this.formData.gender,
                      }
                    : null;
                const gid = this.$store.state.gameId;
                if (!gid || !this.$store.getters.isAuthenticated) {
                    return false;
                }
                if (typeof this.afterCharacterPersisted === 'function') {
                    try {
                        await this.afterCharacterPersisted(gid);
                    } catch (e) {
                        // eslint-disable-next-line no-console
                        console.warn('afterCharacterPersisted failed:', e);
                    }
                }
                try {
                    const { data } = await fetchGameStateLoad(gid);
                    if (!data || String(data.gameId) !== String(gid)) {
                        return false;
                    }
                    this.applyViewerRoleFromLoadPayload(data);
                    if (!this.joinUiActive) {
                        let notOwner = false;
                        if (Object.prototype.hasOwnProperty.call(data, 'viewerIsGameOwner')) {
                            notOwner = data.viewerIsGameOwner === false;
                        } else {
                            const uid = this.resolveViewerUserId();
                            if (uid) {
                                notOwner = data.ownerUserId != null && String(data.ownerUserId) !== String(uid);
                            }
                        }
                        if (notOwner) {
                            this.joinMode = true;
                            this.persistJoinUiGameId(gid);
                        }
                    }
                    this.$store.commit('setGameId', data.gameId);
                    this.$store.commit('setGameSetup', data.gameSetup);
                    const uid = this.resolveViewerUserId();
                    const sheet = this.pickPlayerSheet(data.gameSetup && data.gameSetup.playerCharacters, uid);
                    if (sheet && typeof sheet === 'object') {
                        try {
                            this.confirmSheetCharacter = JSON.parse(JSON.stringify(sheet));
                        } catch (e) {
                            this.confirmSheetCharacter = { ...sheet };
                        }
                        this.prefillFormFromPlayerCharacter(sheet, { skipName: true });
                        if (retainWizard && wizardSnap) {
                            this.restoreWizardFieldsIfIncompleteAfterPrefill(wizardSnap);
                        }
                        this.restoreWizardSelectionSession();
                        this.applyCharacterNameFieldFromSheetAndSession(sheet);
                    }
                    return true;
                } catch (e) {
                    const st = e && e.response && e.response.status;
                    if (st === 404 && this.isPartyJoinSetupContext()) {
                        this.joinFlowError = this.$i18n.join_game_gone;
                        await this.handleStaleOrMissingPartyGame(gid);
                        return false;
                    }
                    if (st === 401) {
                        return false;
                    }
                    return true;
                }
            },
            /** Stable id for playerCharacters / party logic (userId was sometimes null while user._id was set). */
            resolveViewerUserId() {
                const st = this.$store.state;
                const a = st.userId != null && st.userId !== '' ? String(st.userId).trim() : '';
                if (a) return a;
                const u = st.user && st.user._id;
                return u != null && u !== '' ? String(u).trim() : '';
            },
            pickPlayerSheet(pcMap, uid) {
                if (!uid || !pcMap || typeof pcMap !== 'object') return null;
                if (pcMap[uid] && typeof pcMap[uid] === 'object') return pcMap[uid];
                for (const k of Object.keys(pcMap)) {
                    if (String(k) === String(uid) && pcMap[k] && typeof pcMap[k] === 'object') return pcMap[k];
                }
                return null;
            },
            /** Display name on stored PC sheet (top-level or identity). */
            displayNameFromPlayerSheet(sheet) {
                if (!sheet || typeof sheet !== 'object') return '';
                if (sheet.name != null && String(sheet.name).trim()) return String(sheet.name).trim();
                const id = sheet.identity;
                if (id && typeof id === 'object' && id.name != null && String(id.name).trim()) {
                    return String(id.name).trim();
                }
                return '';
            },
            characterNameManualSessionKey() {
                const gid = this.$store.state.gameId;
                const uid = this.resolveViewerUserId();
                if (!gid || !uid) return '';
                return `dm_char_name_manual_${String(gid)}_${String(uid)}`;
            },
            persistCharacterNameManualSession() {
                const k = this.characterNameManualSessionKey();
                if (!k || typeof sessionStorage === 'undefined') return;
                try {
                    if (this.characterNameIsManual) {
                        sessionStorage.setItem(k, '1');
                    } else {
                        sessionStorage.removeItem(k);
                    }
                } catch (e) {
                    /* ignore */
                }
            },
            readCharacterNameManualFromSession() {
                const k = this.characterNameManualSessionKey();
                if (!k || typeof sessionStorage === 'undefined') return false;
                try {
                    return sessionStorage.getItem(k) === '1';
                } catch (e) {
                    return false;
                }
            },
            clearCharacterNameManualSession() {
                const k = this.characterNameManualSessionKey();
                if (!k || typeof sessionStorage === 'undefined') return;
                try {
                    sessionStorage.removeItem(k);
                } catch (e) {
                    /* ignore */
                }
            },
            wizardSelectionSessionKey() {
                const gid = this.$store.state.gameId;
                const uid = this.resolveViewerUserId();
                if (!gid || !uid) return '';
                return `dm_wizard_picks_${String(gid)}_${String(uid)}`;
            },
            persistWizardSelectionSession() {
                const k = this.wizardSelectionSessionKey();
                if (!k || typeof sessionStorage === 'undefined') return;
                try {
                    const fd = this.formData || {};
                    this.syncWizardRandomIntentFromForm();
                    sessionStorage.setItem(
                        k,
                        JSON.stringify({
                            characterRace: fd.characterRace,
                            characterClass: fd.characterClass,
                            subrace: fd.subrace,
                            subclass: fd.subclass,
                            characterLevel: fd.characterLevel,
                            gender: fd.gender,
                            randomIntent: this.wizardRandomIntent,
                        })
                    );
                } catch (e) {
                    /* ignore */
                }
            },
            syncWizardRandomIntentFromForm() {
                const isRandom = (v) => String(v == null ? '' : v).trim().toLowerCase() === 'random';
                this.wizardRandomIntent.characterRace = isRandom(this.formData.characterRace);
                this.wizardRandomIntent.characterClass = isRandom(this.formData.characterClass);
                this.wizardRandomIntent.subrace = isRandom(this.formData.subrace);
                this.wizardRandomIntent.subclass = isRandom(this.formData.subclass);
            },
            restoreWizardSelectionSession() {
                const k = this.wizardSelectionSessionKey();
                if (!k || typeof sessionStorage === 'undefined') return;
                try {
                    const raw = sessionStorage.getItem(k);
                    if (!raw) return;
                    const saved = JSON.parse(raw);
                    if (!saved || typeof saved !== 'object') return;
                    const fd = this.formData;
                    if (saved.characterRace != null && String(saved.characterRace).trim()) {
                        fd.characterRace = String(saved.characterRace).trim();
                    }
                    if (saved.characterClass != null && String(saved.characterClass).trim()) {
                        fd.characterClass = String(saved.characterClass).trim();
                    }
                    if (saved.subrace != null && String(saved.subrace).trim()) {
                        fd.subrace = String(saved.subrace).trim();
                    }
                    if (saved.subclass != null && String(saved.subclass).trim()) {
                        fd.subclass = String(saved.subclass).trim();
                    }
                    if (saved.characterLevel != null && !Number.isNaN(Number(saved.characterLevel))) {
                        fd.characterLevel = Math.min(20, Math.max(1, Number(saved.characterLevel)));
                    }
                    if (saved.gender === 'Male' || saved.gender === 'Female') {
                        fd.gender = saved.gender;
                    }
                    if (saved.randomIntent && typeof saved.randomIntent === 'object') {
                        this.wizardRandomIntent = {
                            characterRace: saved.randomIntent.characterRace === true,
                            characterClass: saved.randomIntent.characterClass === true,
                            subrace: saved.randomIntent.subrace === true,
                            subclass: saved.randomIntent.subclass === true,
                        };
                    } else {
                        this.syncWizardRandomIntentFromForm();
                    }
                    if (this.wizardRandomIntent.characterRace) fd.characterRace = 'random';
                    if (this.wizardRandomIntent.characterClass) fd.characterClass = 'random';
                    if (this.wizardRandomIntent.subrace || this.wizardRandomIntent.characterRace) fd.subrace = 'random';
                    if (this.wizardRandomIntent.subclass || this.wizardRandomIntent.characterClass) fd.subclass = 'random';
                    this.characterFlowDebug('wizard.restore.session', this.characterFlowDebugFormSnapshot());
                } catch (e) {
                    /* ignore */
                }
            },
            /**
             * Autogenerated name → empty field (next generate runs preview again). Manually typed name → keep text from sheet.
             * Uses sessionStorage so it survives SetupForm remount (e.g. lobby editor).
             */
            applyCharacterNameFieldFromSheetAndSession(sheet) {
                if (!sheet || typeof sheet !== 'object') return;
                if (this.characterNameIsManual || this.readCharacterNameManualFromSession()) {
                    this.characterNameIsManual = true;
                    const nm = this.displayNameFromPlayerSheet(sheet);
                    if (nm) this.formData.characterName = nm;
                } else {
                    this.characterNameIsManual = false;
                    this.formData.characterName = '';
                }
                this.persistCharacterNameManualSession();
            },
            /**
             * Party lobby (inline SetupForm): with gameId set, `joinGameIdFromRoute` is truthy so we skip tryRestoreSetupFromServer
             * on mount — the wizard would stay on random. Copy the saved PC from Vuex into the form before the user clicks Generate.
             */
            prefillInlineLobbyFormIfPossible() {
                if (!this.lobbyInline || !this.$store.state.gameId) return;
                const uid = this.resolveViewerUserId();
                if (!uid) return;
                const sheet = this.pickPlayerSheet(
                    this.$store.state.gameSetup && this.$store.state.gameSetup.playerCharacters,
                    uid
                );
                if (!sheet || typeof sheet !== 'object') return;
                try {
                    this.confirmSheetCharacter = JSON.parse(JSON.stringify(sheet));
                } catch (e) {
                    this.confirmSheetCharacter = { ...sheet };
                }
                this.prefillFormFromPlayerCharacter(sheet, { skipName: true });
                this.applyRegenerateFallbackFromGameSetup();
                this.restoreWizardSelectionSession();
                this.applyCharacterNameFieldFromSheetAndSession(sheet);
                this.emitLobbyInlineConfirmSheet();
            },
            onLobbyPlayerCharactersMaybePrefill() {
                if (!this.lobbyInline || !this.$store.state.gameId) return;
                const rr = this.formData.characterRace;
                const cc = this.formData.characterClass;
                const stillDefault = !rr || rr === 'random' || !cc || cc === 'random';
                if (!stillDefault) return;
                this.prefillInlineLobbyFormIfPossible();
            },
            characterFlowDebugEnabled() {
                try {
                    if (typeof window !== 'undefined' && window.__GMAI_DEBUG_CHARACTER__ === true) {
                        return true;
                    }
                } catch (e) {
                    /* ignore */
                }
                if (characterFlowDebugUrlOn()) {
                    return true;
                }
                try {
                    if (typeof process !== 'undefined' && process.env && process.env.NODE_ENV === 'development') {
                        return true;
                    }
                } catch (e) {
                    /* ignore */
                }
                try {
                    return typeof localStorage !== 'undefined' && localStorage.getItem(GM_DEBUG_CHARACTER_FLOW_LS) === '1';
                } catch (e) {
                    return false;
                }
            },
            /** One visible line when SetupForm mounts so you know client debug is (not) active — browser console, not server. */
            logCharacterDebugBootstrap() {
                const on = this.characterFlowDebugEnabled();
                if (on) {
                    try {
                        if (sessionStorage.getItem(SESSION_DEBUG_ON_ACK_LS) === '1') return;
                        sessionStorage.setItem(SESSION_DEBUG_ON_ACK_LS, '1');
                    } catch (e) {
                        /* ignore */
                    }
                    // Defer so DevTools stack points here less often when SetupForm remounts (e.g. lobby editor).
                    setTimeout(() => {
                        console.warn(
                            '[GMAI] Debug personaje/regenerar ACTIVO. Filtra la consola por: GMAI character — esas líneas aparecen al pulsar Crear personaje o Regenerar (no al abrir el editor).'
                        );
                    }, 0);
                    return;
                }
                try {
                    if (sessionStorage.getItem(SESSION_DEBUG_HINT_LS) === '1') return;
                    sessionStorage.setItem(SESSION_DEBUG_HINT_LS, '1');
                } catch (e) {
                    /* ignore */
                }
                console.warn(
                    '[GMAI] Debug de personaje en consola del navegador está DESACTIVADO. Para activarlo: añade ?gmaiDebugCharacter=1 a la URL, o en consola ejecuta localStorage.setItem("gmDebugCharacterFlow","1") y recarga. (Los logs del servidor Go usan DM_DEBUG_CHARACTER_FLOW en la terminal del API.)'
                );
            },
            /** @param {string} tag */
            characterFlowDebug(tag, detail) {
                if (!this.characterFlowDebugEnabled()) return;
                const prefix = `[GMAI character:${tag}]`;
                if (detail === undefined) {
                    console.log(prefix);
                } else {
                    console.log(prefix, detail);
                }
            },
            characterFlowDebugFormSnapshot() {
                const fd = this.formData;
                return {
                    characterName: fd.characterName,
                    characterNameIsManual: this.characterNameIsManual,
                    characterRace: fd.characterRace,
                    characterClass: fd.characterClass,
                    characterLevel: fd.characterLevel,
                    subclass: fd.subclass,
                    subrace: fd.subrace,
                    gender: fd.gender,
                };
            },
            /** Minimal PC fields for logs (avoid dumping full sheet). */
            characterFlowDebugPcShape(pc) {
                if (!pc || typeof pc !== 'object') return null;
                return {
                    name: pc.name,
                    race: pc.race,
                    class: pc.class,
                    characterClass: pc.characterClass,
                    level: pc.level,
                    subclass: pc.subclass,
                    subclassId: pc.subclassId,
                    subrace: pc.subrace,
                    subraceId: pc.subraceId,
                    gender: pc.gender,
                };
            },
            /** Map a saved sheet to setup form ids (invite flow always starts on the form, not confirm). */
            onCharacterNameInput() {
                const t = String(this.formData.characterName || '').trim();
                const randomWord = String(this.$i18n.random || '').trim().toLowerCase();
                if (!t || t.toLowerCase() === randomWord) {
                    this.characterNameIsManual = false;
                    this.persistCharacterNameManualSession();
                    return;
                }
                this.characterNameIsManual = true;
                this.persistCharacterNameManualSession();
            },
            /** Collapse whitespace for label compare (sheet may use EN/ES display strings). */
            collapseWs(s) {
                return String(s || '')
                    .trim()
                    .toLowerCase()
                    .replace(/\s+/g, ' ');
            },
            /** Map sheet race string (id or localized label) to catalog race id. */
            resolveSetupRaceId(value) {
                const raw = String(value == null ? '' : value).trim();
                if (!raw) return '';
                const low = raw.toLowerCase();
                const races = this.$i18n.races || [];
                for (const r of races) {
                    if (!r || !r.id || r.id === 'random') continue;
                    if (String(r.id).toLowerCase() === low) return r.id;
                }
                for (const r of races) {
                    if (!r || !r.id || r.id === 'random') continue;
                    const lab = r.label != null ? String(r.label).trim().toLowerCase() : '';
                    if (lab && lab === low) return r.id;
                }
                const cl = this.collapseWs(raw);
                for (const r of races) {
                    if (!r || !r.label) continue;
                    if (this.collapseWs(r.label) === cl) return r.id;
                }
                return '';
            },
            /** Map sheet class string to catalog class id. */
            resolveSetupClassId(value) {
                const raw = String(value == null ? '' : value).trim();
                if (!raw) return '';
                const low = raw.toLowerCase();
                const classes = this.$i18n.classes || [];
                for (const c of classes) {
                    if (!c || !c.id || c.id === 'random') continue;
                    if (String(c.id).toLowerCase() === low) return c.id;
                }
                for (const c of classes) {
                    if (!c || !c.id || c.id === 'random') continue;
                    const lab = c.label != null ? String(c.label).trim().toLowerCase() : '';
                    if (lab && lab === low) return c.id;
                }
                const cl = this.collapseWs(raw);
                for (const c of classes) {
                    if (!c || !c.label) continue;
                    if (this.collapseWs(c.label) === cl) return c.id;
                }
                return '';
            },
            /**
             * @param {object} pc
             * @param {{ skipName?: boolean }} [opts] — skipName: do not overwrite characterName (e.g. regenerate: keep manual name or clear for new roll separately).
             */
            prefillFormFromPlayerCharacter(pc, opts) {
                if (!pc || typeof pc !== 'object') return;
                const skipName = opts && opts.skipName === true;
                const nm = pc.name != null && String(pc.name).trim();
                if (!skipName && nm) {
                    const next = String(pc.name).trim();
                    const prev = String(this.formData.characterName || '').trim();
                    this.formData.characterName = next;
                    if (!(this.characterNameIsManual && prev === next)) {
                        this.characterNameIsManual = false;
                    }
                }
                if (pc.level != null && !Number.isNaN(Number(pc.level))) {
                    const lv = Math.min(20, Math.max(1, Number(pc.level)));
                    this.formData.characterLevel = lv;
                }
                const raceRaw = pc.race;
                const raceResolved = this.resolveSetupRaceId(raceRaw);
                if (raceResolved) this.formData.characterRace = raceResolved;
                const classSrc = pc.class != null ? pc.class : pc.characterClass;
                const classResolved = this.resolveSetupClassId(classSrc);
                if (classResolved) this.formData.characterClass = classResolved;
                const g = pc.gender;
                if (g === 'Male' || g === 'Female') {
                    this.formData.gender = g;
                } else if (typeof g === 'string') {
                    const gl = g.toLowerCase();
                    if (gl.startsWith('f')) this.formData.gender = 'Female';
                    else if (gl.startsWith('m')) this.formData.gender = 'Male';
                }
                const subs = this.getAvailableSubclassesForClass(this.formData.characterClass, this.formData.characterLevel);
                let subPick = '';
                if (typeof pc.subclassId === 'string' && pc.subclassId.trim()) {
                    const cand = pc.subclassId.trim().toLowerCase().replace(/-/g, '_');
                    if (subs.some((s) => s.id === cand)) subPick = cand;
                }
                if (!subPick && typeof pc.subclass === 'string' && pc.subclass.trim()) {
                    const low = pc.subclass.trim().toLowerCase();
                    const byId = subs.find((s) => s.id === low || s.id === low.replace(/\s+/g, '_'));
                    if (byId) subPick = byId.id;
                    else {
                        const byLab = subs.find((s) => s.label && String(s.label).trim().toLowerCase() === low);
                        if (byLab) subPick = byLab.id;
                        else {
                            const cl = this.collapseWs(pc.subclass);
                            const byLab2 = subs.find((s) => s.label && this.collapseWs(s.label) === cl);
                            if (byLab2) subPick = byLab2.id;
                        }
                    }
                }
                if (subPick) this.formData.subclass = subPick;
                const srs = this.getAvailableSubracesForRace(this.formData.characterRace);
                let srPick = '';
                if (typeof pc.subraceId === 'string' && pc.subraceId.trim()) {
                    const cand = pc.subraceId.trim().toLowerCase().replace(/-/g, '_').replace(/\s+/g, '_');
                    const match = srs.find((s) => s.id === cand || s.id === pc.subraceId.trim().toLowerCase());
                    if (match) srPick = match.id;
                }
                if (!srPick && typeof pc.subrace === 'string' && pc.subrace.trim()) {
                    const low = pc.subrace.trim().toLowerCase();
                    const norm = low.replace(/-/g, '_').replace(/\s+/g, '_');
                    const byId = srs.find((s) => s.id === norm || s.id === low);
                    if (byId) srPick = byId.id;
                    else {
                        const byLab = srs.find((s) => s.label && String(s.label).trim().toLowerCase() === low);
                        if (byLab) srPick = byLab.id;
                        else {
                            const cl = this.collapseWs(pc.subrace);
                            const byLab2 = srs.find((s) => s.label && this.collapseWs(s.label) === cl);
                            if (byLab2) srPick = byLab2.id;
                        }
                    }
                }
                if (srPick) this.formData.subrace = srPick;
                this.characterFlowDebug('prefill.done', {
                    skipName,
                    pcRaceRaw: raceRaw,
                    pcClassSrc: classSrc,
                    resolvedRaceId: raceResolved || null,
                    resolvedClassId: classResolved || null,
                    subPick: subPick || null,
                    srPick: srPick || null,
                    form: this.characterFlowDebugFormSnapshot(),
                });
            },
            /** If sheet prefill missed ids, reuse last merged wizard fields from Vuex (regenerate only). */
            applyRegenerateFallbackFromGameSetup() {
                const gs = this.$store.state.gameSetup;
                if (!gs || typeof gs !== 'object') {
                    this.characterFlowDebug('regenerate.fallback', { skipped: true, reason: 'no gameSetup in store' });
                    return;
                }
                const before = this.characterFlowDebugFormSnapshot();
                const fd = this.formData;
                const validRace = (id) => id && id !== 'random' && (this.$i18n.races || []).some((r) => r.id === id);
                const validClass = (id) => id && id !== 'random' && (this.$i18n.classes || []).some((c) => c.id === id);
                const explicitRandom = (v) => String(v == null ? '' : v).trim().toLowerCase() === 'random';
                const raceMissing = fd.characterRace == null || String(fd.characterRace).trim() === '';
                if (raceMissing || (!explicitRandom(fd.characterRace) && !validRace(fd.characterRace))) {
                    const g = gs.characterRace != null ? gs.characterRace : gs.race;
                    const rid = this.resolveSetupRaceId(g);
                    if (rid) fd.characterRace = rid;
                }
                const classMissing = fd.characterClass == null || String(fd.characterClass).trim() === '';
                if (classMissing || (!explicitRandom(fd.characterClass) && !validClass(fd.characterClass))) {
                    const g = gs.characterClass != null ? gs.characterClass : gs.class;
                    const cid = this.resolveSetupClassId(g);
                    if (cid) fd.characterClass = cid;
                }
                const subclassMissing = fd.subclass == null || String(fd.subclass).trim() === '';
                if (subclassMissing) {
                    const g = gs.subclass;
                    if (g && String(g).trim() && String(g).trim() !== 'random') fd.subclass = String(g).trim();
                }
                const subraceMissing = fd.subrace == null || String(fd.subrace).trim() === '';
                if (subraceMissing) {
                    const g = gs.subrace;
                    if (g && String(g).trim() && String(g).trim() !== 'random') fd.subrace = String(g).trim();
                }
                if (gs.gender === 'Male' || gs.gender === 'Female') fd.gender = gs.gender;
                this.characterFlowDebug('regenerate.fallback', {
                    before,
                    after: this.characterFlowDebugFormSnapshot(),
                    gameSetupFields: {
                        characterRace: gs.characterRace,
                        race: gs.race,
                        characterClass: gs.characterClass,
                        class: gs.class,
                        subclass: gs.subclass,
                        subrace: gs.subrace,
                        characterLevel: gs.characterLevel,
                        gender: gs.gender,
                    },
                });
            },
            /** Header "New game" while already on /setup: store resets but local phase/sheet must clear too. */
            resetWizardFromHeaderNew() {
                this.clearCharacterNameManualSession();
                this.setupPhase = 'form';
                this.joinMode = false;
                this.setupViewerIsGameOwner = null;
                this.clearJoinUiGameId();
                this.joinFlowError = '';
                this.joinInitBusy = false;
                this.joinTokenConsumeBusy = false;
                this.confirmSheetCharacter = null;
                this.characterPreviewKey = 0;
                this.progressMessage = '';
                this.isStarting = false;
                this.setupBusyAction = null;
                this.characterNameIsManual = false;
                this.formData = {
                    characterName: '',
                    characterClass: 'random',
                    characterRace: 'random',
                    characterLevel: 1,
                    gender: 'Male',
                    subclass: 'random',
                    subrace: 'random',
                };
                this.clearSetupSessionGameId();
            },
            resetJoinWizardFormDefaults() {
                this.characterNameIsManual = false;
                this.formData = {
                    characterName: '',
                    characterClass: 'random',
                    characterRace: 'random',
                    characterLevel: 1,
                    gender: 'Male',
                    subclass: 'random',
                    subrace: 'random',
                };
            },
            applyViewerRoleFromLoadPayload(data) {
                if (!data || typeof data !== 'object') {
                    this.setupViewerIsGameOwner = null;
                    return;
                }
                if (Object.prototype.hasOwnProperty.call(data, 'viewerIsGameOwner')) {
                    const v = data.viewerIsGameOwner;
                    this.setupViewerIsGameOwner = v === true ? true : v === false ? false : null;
                } else {
                    this.setupViewerIsGameOwner = null;
                }
            },
            async consumeJoinInviteToken(token) {
                if (!token || this.joinTokenConsumeBusy || this.joinInitBusy) return;
                this.joinTokenConsumeBusy = true;
                this.joinFlowError = '';
                this.clearSetupSessionGameId();
                this.confirmSheetCharacter = null;
                this.setupPhase = 'form';
                this.setupViewerIsGameOwner = null;
                this.resetJoinWizardFormDefaults();
                try {
                    const { data } = await axios.post('/api/auth/join', { inviteToken: token });
                    const gameId = data && data.gameId ? String(data.gameId) : '';
                    if (!gameId) {
                        this.joinFlowError = this.$i18n.join_game_failed;
                        await this.$router.replace({ path: '/setup' });
                        return;
                    }
                    try {
                        sessionStorage.removeItem('dm_pending_invite');
                    } catch (e) {
                        /* ignore */
                    }
                    this.$store.commit('setGameId', gameId);
                    if (data && data.alreadyMember === true) {
                        try {
                            sessionStorage.setItem(
                                SESSION_CHAT_TOAST,
                                JSON.stringify({
                                    message: this.$i18n.join_already_in_party,
                                    variant: 'success',
                                })
                            );
                        } catch (e) {
                            /* ignore */
                        }
                        this.joinTokenConsumeBusy = false;
                        await this.$router.replace({ name: 'ChatRoomWithId', params: { id: gameId } });
                        return;
                    }
                    this.joinTokenConsumeBusy = false;
                    await this.initJoinFlow(gameId);
                    await this.$router.replace({ name: 'ChatRoomWithId', params: { id: gameId } });
                } catch (e) {
                    const code = e.response && e.response.data && e.response.data.code;
                    this.joinFlowError =
                        code === 'INVITE_INVALID'
                            ? this.$i18n.join_invite_invalid
                            : e.response?.data?.error || this.$i18n.join_game_failed;
                    try {
                        sessionStorage.removeItem('dm_pending_invite');
                    } catch (err) {
                        /* ignore */
                    }
                    await this.$router.replace({ path: '/setup' });
                } finally {
                    this.joinTokenConsumeBusy = false;
                }
            },
            async initJoinFlow(gid) {
                if (!gid || this.joinInitBusy || this.joinTokenConsumeBusy) return;
                /* Avoid duplicate load when invite flow already hydrated join state (e.g. ChatRoom remount). */
                if (
                    String(this.$store.state.gameId) === String(gid) &&
                    this.joinMode === true &&
                    this.setupViewerIsGameOwner === false &&
                    this.$store.state.gameSetup &&
                    typeof this.$store.state.gameSetup === 'object'
                ) {
                    if (this.lobbyInline) {
                        this.prefillInlineLobbyFormIfPossible();
                    }
                    return;
                }
                this.joinInitBusy = true;
                this.joinFlowError = '';
                this.joinMode = false;
                try {
                    const { data } = await fetchGameStateLoad(gid);
                    if (!data || String(data.gameId) !== String(gid)) {
                        this.joinFlowError = this.$i18n.join_load_failed;
                        this.joinMode = false;
                        this.clearJoinUiGameId();
                        this.applyViewerRoleFromLoadPayload(null);
                        return;
                    }
                    this.applyViewerRoleFromLoadPayload(data);
                    this.$store.commit('setGameId', data.gameId);
                    const gs = data.gameSetup && typeof data.gameSetup === 'object' ? { ...data.gameSetup } : {};
                    gs.language = gs.language || this.$store.state.language;
                    delete gs.generatedCharacter;
                    this.confirmSheetCharacter = null;
                    this.setupPhase = 'form';
                    this.characterPreviewKey = 0;
                    this.resetJoinWizardFormDefaults();
                    this.$store.commit('setGameSetup', gs);
                    this.tryPersistSetupSessionGameId();
                    if (this.lobbyInline) {
                        this.prefillInlineLobbyFormIfPossible();
                    }
                    if (Object.prototype.hasOwnProperty.call(data, 'viewerIsGameOwner') && data.viewerIsGameOwner === true) {
                        this.joinMode = false;
                        this.clearJoinUiGameId();
                    } else {
                        this.joinMode = true;
                        this.persistJoinUiGameId(gid);
                    }
                } catch (e) {
                    const st = e && e.response && e.response.status;
                    if (st === 404) {
                        this.joinFlowError = this.$i18n.join_game_gone;
                        this.joinMode = false;
                        this.applyViewerRoleFromLoadPayload(null);
                        await this.handleStaleOrMissingPartyGame(gid);
                    } else {
                        this.joinFlowError = this.$i18n.join_load_failed;
                        this.joinMode = false;
                        this.clearJoinUiGameId();
                        this.applyViewerRoleFromLoadPayload(null);
                    }
                } finally {
                    this.joinInitBusy = false;
                }
            },
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
            /** Game was deleted, user removed, or stale session (e.g. after DB cleanup). Drop ids and URL. */
            async handleStaleOrMissingPartyGame(gid) {
                const g = gid != null ? String(gid).trim() : '';
                try {
                    const s = sessionStorage.getItem(SESSION_SETUP_GAME_ID);
                    if (g && s === g) sessionStorage.removeItem(SESSION_SETUP_GAME_ID);
                    const j = sessionStorage.getItem(SESSION_JOIN_UI_GAME_ID);
                    if (g && j === g) sessionStorage.removeItem(SESSION_JOIN_UI_GAME_ID);
                } catch (e) {
                    /* ignore */
                }
                this.joinFlowPersistedGameId = null;
                if (g && String(this.$store.state.gameId || '') === g) {
                    this.$store.commit('setGameId', null);
                }
                this.joinMode = false;
                this.setupViewerIsGameOwner = null;
                try {
                    await this.$router.replace({ path: '/setup' });
                } catch (e) {
                    /* ignore */
                }
            },
            /** From a game-state load payload: party shape and whether the current user is a non-owner invitee. */
            partyLoadContext(data) {
                const uid = this.resolveViewerUserId();
                const ownerStr = data && data.ownerUserId != null ? String(data.ownerUserId) : '';
                const listed = data && Array.isArray(data.memberUserIds) ? data.memberUserIds.map((x) => String(x)) : [];
                const gs = data && data.gameSetup && typeof data.gameSetup === 'object' ? data.gameSetup : {};
                const pcMap = gs.playerCharacters && typeof gs.playerCharacters === 'object' ? gs.playerCharacters : {};
                const myPc = this.pickPlayerSheet(pcMap, uid);
                const partyLooksMulti = listed.length >= 2 || Object.keys(pcMap).length >= 2;
                const treatAsMulti = partyLooksMulti || (ownerStr && uid && ownerStr !== uid);
                let viewerIsOwner = false;
                if (data && Object.prototype.hasOwnProperty.call(data, 'viewerIsGameOwner')) {
                    viewerIsOwner = data.viewerIsGameOwner === true;
                } else {
                    viewerIsOwner = !!(ownerStr && uid && ownerStr === uid);
                }
                const isInvitedNonOwner = !!(uid && !viewerIsOwner);
                return { uid, ownerStr, treatAsMulti, partyLooksMulti, myPc, isInvitedNonOwner, viewerIsOwner };
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
                    const { data } = await fetchGameStateLoad(sid);
                    let sessionStill = null;
                    try {
                        sessionStill = sessionStorage.getItem(SESSION_SETUP_GAME_ID);
                    } catch (e) {
                        return;
                    }
                    if (sessionStill !== sid) return;
                    const gid = this.$store.state.gameId;
                    if (gid && gid !== sid) return;

                    const ctx = this.partyLoadContext(data);
                    this.applyViewerRoleFromLoadPayload(data);
                    if (ctx.isInvitedNonOwner) {
                        if (ctx.myPc) {
                            this.$store.commit('setGameId', sid);
                            const gs =
                                data.gameSetup && typeof data.gameSetup === 'object' ? { ...data.gameSetup } : {};
                            gs.language = this.$store.state.language;
                            this.joinMode = true;
                            this.persistJoinUiGameId(sid);
                            delete gs.generatedCharacter;
                            this.confirmSheetCharacter = null;
                            this.setupPhase = 'form';
                            this.characterPreviewKey = 0;
                            this.resetJoinWizardFormDefaults();
                            this.$store.commit('setGameSetup', gs);
                            return;
                        }
                        this.$store.commit('setGameId', sid);
                        const gsStrip =
                            data.gameSetup && typeof data.gameSetup === 'object' ? { ...data.gameSetup } : {};
                        delete gsStrip.generatedCharacter;
                        this.joinMode = true;
                        this.persistJoinUiGameId(sid);
                        this.$store.commit('setGameSetup', {
                            ...gsStrip,
                            language: this.$store.state.language,
                        });
                        this.confirmSheetCharacter = null;
                        this.setupPhase = 'form';
                        this.characterPreviewKey = 0;
                        this.resetJoinWizardFormDefaults();
                        return;
                    }

                    if (ctx.partyLooksMulti) {
                        if (!ctx.uid) {
                            await this.$router.replace({ path: '/setup', query: { joinGame: sid } });
                            return;
                        }
                        if (ctx.myPc) {
                            this.$store.commit('setGameId', sid);
                            const gs =
                                data.gameSetup && typeof data.gameSetup === 'object' ? { ...data.gameSetup } : {};
                            gs.language = this.$store.state.language;
                            this.joinMode = true;
                            this.persistJoinUiGameId(sid);
                            delete gs.generatedCharacter;
                            this.confirmSheetCharacter = null;
                            this.setupPhase = 'form';
                            this.characterPreviewKey = 0;
                            this.prefillFormFromPlayerCharacter(ctx.myPc, { skipName: true });
                            this.applyCharacterNameFieldFromSheetAndSession(ctx.myPc);
                            this.$store.commit('setGameSetup', gs);
                            return;
                        }
                        await this.$router.replace({ path: '/setup', query: { joinGame: sid } });
                        return;
                    }

                    if (
                        data &&
                        Object.prototype.hasOwnProperty.call(data, 'viewerIsGameOwner') &&
                        data.viewerIsGameOwner === false
                    ) {
                        this.$store.commit('setGameId', sid);
                        const gsStrip =
                            data.gameSetup && typeof data.gameSetup === 'object' ? { ...data.gameSetup } : {};
                        delete gsStrip.generatedCharacter;
                        this.joinMode = true;
                        this.persistJoinUiGameId(sid);
                        this.$store.commit('setGameSetup', {
                            ...gsStrip,
                            language: this.$store.state.language,
                        });
                        this.confirmSheetCharacter = null;
                        this.setupPhase = 'form';
                        this.characterPreviewKey = 0;
                        this.resetJoinWizardFormDefaults();
                        return;
                    }

                    const uid = ctx.uid;
                    if (!uid) return;
                    const sheet = data?.gameSetup?.playerCharacters?.[uid];
                    if (!sheet || typeof sheet !== 'object') return;

                    let clone;
                    try {
                        clone = JSON.parse(JSON.stringify(sheet));
                    } catch (e) {
                        clone = sheet;
                    }
                    this.confirmSheetCharacter = clone;
                    this.$store.commit('setGameId', sid);
                    const gs = data.gameSetup && typeof data.gameSetup === 'object' ? { ...data.gameSetup } : {};
                    delete gs.generatedCharacter;
                    this.joinMode = false;
                    this.clearJoinUiGameId();
                    this.$store.commit('setGameSetup', { ...gs, language: this.$store.state.language });
                    this.prefillFormFromPlayerCharacter(sheet, { skipName: true });
                    this.applyCharacterNameFieldFromSheetAndSession(sheet);
                    this.setupPhase = 'confirm_character';
                    await this.$nextTick();
                    this.characterPreviewKey += 1;
                } catch (e) {
                    const st = e && e.response && e.response.status;
                    if (st === 404 && sid) {
                        this.joinFlowError = this.$i18n.join_game_gone;
                        await this.handleStaleOrMissingPartyGame(sid);
                    } else {
                        this.clearSetupSessionGameId();
                    }
                }
            },
            /**
             * Optimistic persist of generated/regenerated PC to Vuex; caller should run
             * syncPersistedCharacterFromServer() next so GET /game-state/load matches a full refresh.
             * @param {object} playerCharacter
             * @param {{ skipLobbyHoldRelease?: boolean }} [options] — when true, do not emit lobby-inline-wizard-hold false (e.g. party lobby + late join to running game: keep confirm UI with “Join”).
             */
            commitGameSetupWithCharacter(playerCharacter, options) {
                const skipLobbyHoldRelease = options && options.skipLobbyHoldRelease === true;
                let pc = playerCharacter;
                try {
                    pc = JSON.parse(JSON.stringify(playerCharacter));
                } catch (e) {
                    /* use as-is */
                }
                this.confirmSheetCharacter = pc;
                const uid = this.resolveViewerUserId();
                const base = {
                    ...this.$store.state.gameSetup,
                    ...this.formData,
                    language: this.$store.state.language,
                };
                delete base.generatedCharacter;
                if (uid) {
                    base.playerCharacters = {
                        ...(this.$store.state.gameSetup.playerCharacters || {}),
                        [uid]: pc,
                    };
                }
                this.$store.commit('setGameSetup', base);
                this.tryPersistSetupSessionGameId();
                if (this.lobbyInline && !skipLobbyHoldRelease) {
                    this.$emit('lobby-inline-wizard-hold', false);
                }
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
                this.formData.subclass = 'random';
                this.formData.subrace = 'random';
                this.syncWizardRandomIntentFromForm();
                this.persistWizardSelectionSession();
            },
            onClassChange() {
                this.formData.subclass = 'random';
                this.syncWizardRandomIntentFromForm();
                this.persistWizardSelectionSession();
            },
            onSubraceChange() {
                this.syncWizardRandomIntentFromForm();
                this.persistWizardSelectionSession();
            },
            onSubclassChange() {
                this.syncWizardRandomIntentFromForm();
                this.persistWizardSelectionSession();
            },
            onLevelChange() {
                const subs = this.getAvailableSubclassesForClass(this.formData.characterClass, this.formData.characterLevel);
                const ids = new Set(subs.map((s) => s.id));
                if (this.formData.subclass && this.formData.subclass !== 'random' && !ids.has(this.formData.subclass)) {
                    this.formData.subclass = 'random';
                }
                this.persistWizardSelectionSession();
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
            getAvailableSubracesForRace(raceId) {
                if (!raceId || raceId === 'random') return [];
                const list = this.subracesByRace[raceId] || [];
                return list;
            },
            /** After race/class/level are fixed, pick concrete subclass and subrace when random or invalid. */
            finalizeSubclassAndSubraceSelections() {
                const subs = this.getAvailableSubclassesForClass(this.formData.characterClass, this.formData.characterLevel);
                const subIds = new Set(subs.map((s) => s.id));
                if (subs.length) {
                    if (!this.formData.subclass || this.formData.subclass === 'random' || !subIds.has(this.formData.subclass)) {
                        this.formData.subclass = subs[Math.floor(Math.random() * subs.length)].id;
                    }
                } else {
                    this.formData.subclass = 'random';
                }
                const srList = this.getAvailableSubracesForRace(this.formData.characterRace);
                const srIds = new Set(srList.map((s) => s.id));
                if (srList.length) {
                    if (!this.formData.subrace || this.formData.subrace === 'random' || !srIds.has(this.formData.subrace)) {
                        this.formData.subrace = srList[Math.floor(Math.random() * srList.length)].id;
                    }
                } else {
                    this.formData.subrace = 'random';
                }
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
                    this.finalizeSubclassAndSubraceSelections();
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
                    this.finalizeSubclassAndSubraceSelections();
                    return;
                }
                // If class is random but race is concrete: pick a class allowed for that race
                if (this.formData.characterClass === 'random' && this.formData.characterRace && this.formData.characterRace !== 'random') {
                    const cls = this.chooseRandomClassForRace(this.formData.characterRace);
                    this.formData.characterClass = cls;
                    this.finalizeSubclassAndSubraceSelections();
                    return;
                }
                this.finalizeSubclassAndSubraceSelections();
            },

        /** World-only campaign core + stages; does not receive character data (server ignores it anyway). */
        async generateCampaignConcept(gameId = null) {
            const response = await axios.post(
                '/api/game-session/generate-campaign-core',
                {
                    gameId,
                    language: this.$store.state.language,
                    waitForStages: true,
                },
                { timeout: 600000 }
            );
            return response.data;
        },

        characterGenerationErrorMessage(err) {
            const st = err && err.response && err.response.status;
            const aborted = err && err.code === 'ECONNABORTED';
            if (st === 504 || aborted) {
                return this.$i18n.error_character_gateway_timeout;
            }
            const d = err && err.response && err.response.data;
            if (d && d.code === 'CHARACTER_PERSIST_FAILED') {
                return this.$i18n.character_persist_failed;
            }
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

        normalizedNameForCharacterApi() {
            const raw = String(this.formData.characterName || '').trim();
            const randomWord = String(this.$i18n.random || '').trim().toLowerCase();
            if (!raw || raw.toLowerCase() === randomWord) return '';
            return raw;
        },

        subclassForCharacterApi() {
            const s = this.formData.subclass;
            if (!s || s === 'random') return undefined;
            return s;
        },

        subraceForCharacterApi() {
            const s = this.formData.subrace;
            if (!s || s === 'random') return undefined;
            return s;
        },

        async generatePlayerCharacter(gameId = null) {
            const gid = gameId != null && String(gameId).trim() !== '' ? String(gameId).trim() : '';
            this.syncWizardRandomIntentFromForm();
            const buildGameSetupPayload = () => ({
                name: this.normalizedNameForCharacterApi(),
                gender: this.formData.gender,
                class: this.wizardRandomIntent.characterClass ? 'random' : this.formData.characterClass,
                race: this.wizardRandomIntent.characterRace ? 'random' : this.formData.characterRace,
                level: this.formData.characterLevel,
                subclass:
                    this.wizardRandomIntent.subclass || this.wizardRandomIntent.characterClass
                        ? undefined
                        : this.subclassForCharacterApi(),
                subrace:
                    this.wizardRandomIntent.subrace || this.wizardRandomIntent.characterRace
                        ? undefined
                        : this.subraceForCharacterApi(),
                background: this.formData.characterBackground,
                language: this.$store.state.language,
                randomIntent: {
                    characterRace: this.wizardRandomIntent.characterRace === true,
                    characterClass: this.wizardRandomIntent.characterClass === true,
                    subrace: this.wizardRandomIntent.subrace === true,
                    subclass: this.wizardRandomIntent.subclass === true,
                },
            });
            let gameSetupPayload = buildGameSetupPayload();
            this.characterFlowDebug('generate.start', {
                gameId: gid || null,
                form: this.characterFlowDebugFormSnapshot(),
                gameSetupPayload,
            });
            const existingNameRaw = String(this.formData.characterName || '').trim();
            const randomWord = String(this.$i18n.random || '').trim().toLowerCase();
            const nameFieldEmptyOrRandom =
                !existingNameRaw || existingNameRaw.toLowerCase() === randomWord;
            if (!nameFieldEmptyOrRandom) {
                this.characterNameIsManual = true;
                this.persistCharacterNameManualSession();
            }
            {
                try {
                    const previewBody = {
                        gameSetup: gameSetupPayload,
                        language: this.$store.state.language,
                    };
                    if (gid) {
                        previewBody.gameId = gid;
                    } else if (!this.isPartyJoinSetupContext()) {
                        previewBody.newParty = true;
                    }
                    this.characterFlowDebug('generate.preview.request', previewBody);
                    const prev = await axios.post('/api/game-session/preview-character-name', previewBody, {
                        timeout: 20000,
                    });
                    const n = prev.data && prev.data.name != null && String(prev.data.name).trim();
                    this.characterFlowDebug('generate.preview.response', {
                        status: prev.status,
                        name: n ? String(prev.data.name).trim() : null,
                        rawKeys: prev.data && typeof prev.data === 'object' ? Object.keys(prev.data) : null,
                    });
                    const resolved =
                        prev.data && prev.data.resolvedGameSetup && typeof prev.data.resolvedGameSetup === 'object'
                            ? prev.data.resolvedGameSetup
                            : null;
                    if (resolved) {
                        const rr = this.resolveSetupRaceId(resolved.race);
                        const cc = this.resolveSetupClassId(resolved.class);
                        if (rr) this.formData.characterRace = rr;
                        if (cc) this.formData.characterClass = cc;
                        const sr = resolved.subrace != null ? String(resolved.subrace).trim() : '';
                        const sc = resolved.subclass != null ? String(resolved.subclass).trim() : '';
                        if (sr) this.formData.subrace = sr;
                        if (sc) this.formData.subclass = sc;
                        this.characterFlowDebug('generate.preview.resolvedSetup', {
                            resolvedGameSetup: resolved,
                            form: this.characterFlowDebugFormSnapshot(),
                        });
                    }
                    if (n && nameFieldEmptyOrRandom) {
                        this.formData.characterName = String(prev.data.name).trim();
                        this.characterNameIsManual = false;
                        this.persistCharacterNameManualSession();
                        await this.$nextTick();
                    }
                } catch (prevErr) {
                    const ax = prevErr && prevErr.response;
                    this.characterFlowDebug('generate.preview.error', {
                        message: prevErr && prevErr.message,
                        status: ax && ax.status,
                        data:
                            ax && ax.data != null
                                ? typeof ax.data === 'string'
                                    ? String(ax.data).slice(0, 800)
                                    : JSON.stringify(ax.data).slice(0, 800)
                                : null,
                    });
                }
            }
            gameSetupPayload = buildGameSetupPayload();
            const body = {
                gameSetup: gameSetupPayload,
                language: this.$store.state.language,
            };
            if (gid) {
                body.gameId = gid;
            } else if (!this.isPartyJoinSetupContext()) {
                body.newParty = true;
            }
            const preName = this.formData.characterName != null && String(this.formData.characterName).trim();
            if (preName) {
                body.preassignedDisplayName = preName;
            }
            this.characterFlowDebug('generate.post', {
                bodyKeys: Object.keys(body),
                gameSetup: gameSetupPayload,
                preassignedDisplayName: body.preassignedDisplayName || null,
            });
            const response = await axios.post('/api/game-session/generate-character', body, { timeout: 600000 });
            this.characterFlowDebug('generate.response.ok', {
                hasPlayerCharacter: !!(response.data && response.data.playerCharacter),
                returnedName:
                    response.data &&
                    response.data.playerCharacter &&
                    response.data.playerCharacter.name != null
                        ? String(response.data.playerCharacter.name)
                        : null,
            });
            return response.data;
        },

        async submitForm() {
            if (this.setupPhase === 'confirm_character') return;
            this.syncWizardRandomIntentFromForm();
            this.persistWizardSelectionSession();
            this.isStarting = true;
            this.progressMessage = '';
            this.recoverSetupGameIdFromSession();
            if (!this.$store.state.gameId) {
                if (this.isPartyJoinSetupContext()) {
                    this.progressMessage = this.$i18n.join_missing_game_id;
                    this.isStarting = false;
                    setTimeout(() => {
                        this.progressMessage = '';
                    }, 6500);
                    return;
                }
                this.clearSetupSessionGameId();
                // New host party: no client gameId until /generate-character succeeds (server assigns gameId).
            }
            const syncJoinOk = await this.syncJoinUiFromServerIfNeeded();
            if (!syncJoinOk) {
                this.isStarting = false;
                return;
            }
            const mergedSetup = {
                ...this.$store.state.gameSetup,
                ...this.formData,
                language: this.$store.state.language,
            };
            if (this.joinUiActive) {
                delete mergedSetup.generatedCharacter;
            }
            this.$store.commit('setGameSetup', mergedSetup);
            this.persistWizardSelectionSession();
            this.persistWizardSelectionSession();

            try {
                const charData = await this.generatePlayerCharacter(this.$store.state.gameId);
                if (charData && charData.playerCharacter) {
                    if (charData.gameId) {
                        this.$store.commit('setGameId', String(charData.gameId));
                    }
                    const keepLobbyWizardForLateJoinPlaying =
                        this.lobbyInline && this.partyLateJoinInviteePlaying;
                    this.commitGameSetupWithCharacter(charData.playerCharacter, {
                        skipLobbyHoldRelease: keepLobbyWizardForLateJoinPlaying,
                    });
                    if (!(await this.syncPersistedCharacterFromServer())) {
                        this.isStarting = false;
                        return;
                    }
                    await this.$nextTick();
                    this.characterPreviewKey += 1;

                    // Party lobby (normal): same view as refresh — embedded sheet + roster, no “Review your character” step.
                    if (this.lobbyInline && !this.partyLateJoinInviteePlaying) {
                        this.setupPhase = 'form';
                        this.progressMessage = '';
                        this.$emit('lobby-inline-after-character-saved');
                    } else {
                        this.setupPhase = 'confirm_character';
                        if (this.lobbyInline) {
                            this.progressMessage = '';
                            if (this.partyLateJoinInviteePlaying) {
                                this.$emit('lobby-inline-wizard-hold', true);
                            }
                        } else if (this.partyLateJoinInviteePlaying) {
                            this.progressMessage = this.$i18n.character_ready_join_party_playing;
                        } else if (this.usePartyRoomConfirmUi) {
                            this.progressMessage = this.$i18n.character_ready_party_room;
                        } else if (this.joinUiActive) {
                            this.progressMessage = this.$i18n.character_ready_confirm_join;
                        } else {
                            this.progressMessage = this.$i18n.character_ready_confirm;
                        }
                    }
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
                if (this.lobbyInline) {
                    this.$emit('lobby-inline-wizard-hold', false);
                }
                setTimeout(() => {
                    this.progressMessage = '';
                }, 5200);
                return;
            }

            this.isStarting = false;
        },

        backToForm() {
            this.setupPhase = 'form';
            this.confirmSheetCharacter = null;
            this.setupBusyAction = null;
            const gs = { ...this.$store.state.gameSetup };
            delete gs.generatedCharacter;
            this.$store.commit('setGameSetup', gs);
            this.progressMessage = '';
            if (this.lobbyInline) {
                this.$emit('lobby-inline-wizard-hold', true);
            }
        },

        async regenerateCharacter() {
            this.characterFlowDebug('regenerate.enter', {
                gameId: this.$store.state.gameId,
                setupPhase: this.setupPhase,
                hasConfirmSheetCharacter: !!(
                    this.confirmSheetCharacter && typeof this.confirmSheetCharacter === 'object'
                ),
            });
            // Preserve explicit "random" choices even if prefill/fallback temporarily replaces form ids.
            const gs = (this.$store.state && this.$store.state.gameSetup) || {};
            const isRandomId = (v) => String(v == null ? '' : v).trim().toLowerCase() === 'random';
            const preserveRandom = {
                characterRace:
                    isRandomId(this.formData.characterRace) ||
                    isRandomId(gs.characterRace) ||
                    isRandomId(gs.race),
                characterClass:
                    isRandomId(this.formData.characterClass) ||
                    isRandomId(gs.characterClass) ||
                    isRandomId(gs.class),
                subclass: isRandomId(this.formData.subclass) || isRandomId(gs.subclass),
                subrace: isRandomId(this.formData.subrace) || isRandomId(gs.subrace),
            };
            this.characterFlowDebug('regenerate.preserveRandom', preserveRandom);
            this.syncWizardRandomIntentFromForm();
            this.persistWizardSelectionSession();
            this.recoverSetupGameIdFromSession();
            if (!this.$store.state.gameId) {
                this.characterFlowDebug('regenerate.abort', { reason: 'missing gameId' });
                this.progressMessage = this.isPartyJoinSetupContext()
                    ? this.$i18n.join_missing_game_id
                    : this.$i18n.character_regenerate_needs_game;
                setTimeout(() => {
                    this.progressMessage = '';
                }, 6500);
                return;
            }
            // Regenerate must use current wizard/store picks as source of truth.
            // Do not prefill from previous sheet here, otherwise random choices get concretized.
            this.characterFlowDebug('regenerate.sheet', { skipped: true, reason: 'use current wizard values' });
            if (preserveRandom.characterRace) this.formData.characterRace = 'random';
            if (preserveRandom.characterClass) this.formData.characterClass = 'random';
            if (preserveRandom.subclass || preserveRandom.characterClass) this.formData.subclass = 'random';
            if (preserveRandom.subrace || preserveRandom.characterRace) this.formData.subrace = 'random';
            // New name from preview/server unless the player typed this one.
            if (!this.characterNameIsManual) {
                this.formData.characterName = '';
            }
            this.characterFlowDebug('regenerate.afterNameClear', this.characterFlowDebugFormSnapshot());
            const syncJoinOk = await this.syncJoinUiFromServerIfNeeded();
            if (!syncJoinOk) {
                return;
            }
            const mergedSetup = {
                ...this.$store.state.gameSetup,
                ...this.formData,
                language: this.$store.state.language,
            };
            if (this.joinUiActive) {
                delete mergedSetup.generatedCharacter;
            }
            this.$store.commit('setGameSetup', mergedSetup);

            // Show “Crea tu personaje” with the same build (race/class/level/…) while the new sheet generates.
            if (this.setupPhase === 'confirm_character' && !this.partyLateJoinInviteePlaying) {
                this.setupPhase = 'form';
                await this.$nextTick();
            }

            this.isStarting = true;
            this.setupBusyAction = 'regenerate';
            this.progressMessage = '';

            try {
                const charData = await this.generatePlayerCharacter(this.$store.state.gameId);
                if (charData && charData.playerCharacter) {
                    if (charData.gameId) {
                        this.$store.commit('setGameId', String(charData.gameId));
                    }
                    const keepLobbyWizardForLateJoinPlaying =
                        this.lobbyInline && this.partyLateJoinInviteePlaying;
                    this.commitGameSetupWithCharacter(charData.playerCharacter, {
                        skipLobbyHoldRelease: keepLobbyWizardForLateJoinPlaying,
                    });
                    if (
                        !(await this.syncPersistedCharacterFromServer({
                            retainWizardFieldsIfPrefillIncomplete: true,
                        }))
                    ) {
                        return;
                    }
                    await this.$nextTick();
                    this.characterPreviewKey += 1;

                    if (this.lobbyInline && !this.partyLateJoinInviteePlaying) {
                        this.setupPhase = 'form';
                        this.progressMessage = '';
                        this.$emit('lobby-inline-after-character-saved');
                    } else {
                        this.setupPhase = 'confirm_character';
                        if (this.lobbyInline) {
                            this.progressMessage = '';
                            if (this.partyLateJoinInviteePlaying) {
                                this.$emit('lobby-inline-wizard-hold', true);
                            }
                        } else if (this.partyLateJoinInviteePlaying) {
                            this.progressMessage = this.$i18n.character_ready_join_party_playing;
                        } else if (this.usePartyRoomConfirmUi) {
                            this.progressMessage = this.$i18n.character_ready_party_room;
                        } else if (this.joinUiActive) {
                            this.progressMessage = this.$i18n.character_ready_confirm_join;
                        } else {
                            this.progressMessage = this.$i18n.character_ready_confirm;
                        }
                    }
                    this.emitLobbyInlineConfirmSheet();
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
                if (this.lobbyInline) {
                    this.$emit('lobby-inline-wizard-hold', false);
                }
                setTimeout(() => {
                    this.progressMessage = '';
                }, 5200);
            } finally {
                this.isStarting = false;
                this.setupBusyAction = null;
            }
        },

        async hydrateConfirmStepFromServer() {
            const gid0 = this.$store.state.gameId;
            if (!gid0 || !this.$store.getters.isAuthenticated) return true;
            try {
                const { data } = await fetchGameStateLoad(gid0);
                if (data) {
                    this.applyViewerRoleFromLoadPayload(data);
                    if (
                        Object.prototype.hasOwnProperty.call(data, 'viewerIsGameOwner') &&
                        data.viewerIsGameOwner === false
                    ) {
                        this.joinMode = true;
                        this.persistJoinUiGameId(String(data.gameId));
                    }
                    if (data.gameSetup && data.gameSetup.party) {
                        const cur = { ...(this.$store.state.gameSetup || {}) };
                        cur.party = { ...(cur.party || {}), ...data.gameSetup.party };
                        this.$store.commit('setGameSetup', cur);
                    }
                }
            } catch (e) {
                const st = e && e.response && e.response.status;
                if (st === 404 && this.isPartyJoinSetupContext()) {
                    this.joinFlowError = this.$i18n.join_game_gone;
                    await this.handleStaleOrMissingPartyGame(gid0);
                    return false;
                }
            }
            return true;
        },

        /**
         * Non-owner after the table is in play: open shared chat only (no party-ready).
         * Shown only when `party.phase === 'playing'` (see partyLateJoinInviteePlaying).
         */
        async joinPartyRoomInProgress() {
            if (this.setupPhase !== 'confirm_character') return;
            if (!(await this.hydrateConfirmStepFromServer())) return;
            await this.$nextTick();

            if (!this.setupPartyIsExplicitlyPlaying) {
                this.progressMessage = this.$i18n.join_party_only_when_playing;
                setTimeout(() => {
                    this.progressMessage = '';
                }, 7000);
                return;
            }
            if (this.setupViewerIsGameOwner !== false || !this.joinMode) return;

            const gid0 = this.$store.state.gameId;
            const playerCharacter = this.confirmSheetCharacter;
            if (!playerCharacter) {
                this.progressMessage = this.$i18n.error_generating_character;
                return;
            }
            if (!gid0) {
                this.progressMessage = this.$i18n.join_missing_game_id;
                return;
            }

            this.isStarting = true;
            try {
                this.clearSetupSessionGameId();
                this.clearJoinUiGameId();
                this.setupPhase = 'form';
                this.persistLobbyCharacterFlowDoneSession(gid0);
                if (this.lobbyInline) {
                    this.$emit('lobby-character-done', { gameId: gid0 });
                } else {
                    await this.$router.push({ name: 'ChatRoomWithId', params: { id: gid0 } });
                }
            } catch (e) {
                const msg = e.response && e.response.data && e.response.data.error;
                this.progressMessage = msg || e.message || this.$i18n.error_saving_game;
                setTimeout(() => {
                    this.progressMessage = '';
                }, 6500);
            } finally {
                this.isStarting = false;
            }
        },

        /**
         * Called from ChatRoom roster “Ready to start” for the viewer row. Runs the same flow as the old
         * inline primary button: confirm step + party-ready (+ optional start). If the wizard is on the form
         * phase but the sheet is already in Vuex, jumps to confirm first.
         * @returns {Promise<boolean>} true if this component handled the flow (caller should not duplicate POST).
         */
        async runLobbyRosterConfirmAndReady() {
            if (!this.lobbyInline || this.lobbyInteractionsLocked) return false;
            if (this.isStarting) return false;
            const uid = this.resolveViewerUserId();
            if (!uid) return false;

            if (this.setupPhase === 'confirm_character' && this.confirmSheetCharacter) {
                await this.confirmPartyLobbyCharacter();
                return true;
            }

            const fromMap = this.$store.state.gameSetup?.playerCharacters?.[uid];
            if (!fromMap || typeof fromMap !== 'object') return false;
            let clone;
            try {
                clone = JSON.parse(JSON.stringify(fromMap));
            } catch (e) {
                clone = { ...fromMap };
            }
            this.$emit('lobby-inline-wizard-hold', true);
            this.confirmSheetCharacter = clone;
            this.setupPhase = 'confirm_character';
            await this.$nextTick();
            this.characterPreviewKey += 1;
            await this.confirmPartyLobbyCharacter();
            return true;
        },

        /** Party lobby: one user action — POST party-ready (server requires a saved sheet), then optional start; return to chat. */
        async confirmPartyLobbyCharacter() {
            if (this.setupPhase !== 'confirm_character') return;
            if (!(await this.hydrateConfirmStepFromServer())) return;
            await this.$nextTick();

            const gid0 = this.$store.state.gameId;
            const playerCharacter = this.confirmSheetCharacter;
            if (!playerCharacter) {
                this.progressMessage = this.$i18n.error_generating_character;
                return;
            }
            if (!gid0) {
                this.progressMessage = this.$i18n.join_missing_game_id;
                return;
            }

            this.lobbyConfirmBusyStep = 'party';
            this.isStarting = true;
            let partyReadyTop = null;
            try {
                const postPartyReady = () =>
                    axios.post('/api/game-state/party-ready', {
                        gameId: gid0,
                        ready: true,
                    });
                let readyRes;
                try {
                    readyRes = await postPartyReady();
                } catch (eFirst) {
                    const c0 =
                        eFirst.response && eFirst.response.data && eFirst.response.data.code;
                    if (c0 === 'PARTY_READY_NEEDS_CHARACTER') {
                        await new Promise((r) => setTimeout(r, 500));
                        if (await this.hydrateConfirmStepFromServer()) {
                            await this.$nextTick();
                        }
                        readyRes = await postPartyReady();
                    } else {
                        throw eFirst;
                    }
                }
                const { data } = readyRes;
                partyReadyTop = data;
                if (data && data.gameSetup) {
                    this.$store.commit('setGameSetup', data.gameSetup);
                }
                const meta = data && data.partyReadyMeta;
                let partyStartHardFail = false;
                if (meta && meta.allMembersHaveSheets && meta.allMembersReady) {
                    this.lobbyConfirmBusyStep = 'adventure';
                    try {
                        await axios.post('/api/game-session/start-party-adventure', { gameId: gid0 });
                    } catch (startErr) {
                        const code =
                            startErr.response && startErr.response.data && startErr.response.data.code;
                        if (
                            code === 'PARTY_START_IN_PROGRESS' ||
                            code === 'PARTY_NOT_READY' ||
                            code === 'PARTY_SHEETS_INCOMPLETE'
                        ) {
                            /* benign race or lobby state; chat will reflect true state */
                        } else {
                            partyStartHardFail = true;
                            const detail =
                                startErr.response && startErr.response.data && startErr.response.data.error;
                            const errCode =
                                startErr.response && startErr.response.data && startErr.response.data.code;
                            const suffix =
                                errCode && String(errCode).trim()
                                    ? ` (${String(errCode).trim()})`
                                    : '';
                            this.progressMessage =
                                (detail || startErr.message || this.$i18n.error_saving_game) + suffix;
                            setTimeout(() => {
                                this.progressMessage = '';
                            }, 12000);
                        }
                    }
                }
                if (partyStartHardFail) {
                    return;
                }
                this.clearSetupSessionGameId();
                this.clearJoinUiGameId();
                this.setupPhase = 'form';
                this.persistLobbyCharacterFlowDoneSession(gid0);
                if (this.lobbyInline) {
                    const viewerUid = this.resolveViewerUserId();
                    const ownStr =
                        partyReadyTop && partyReadyTop.ownerUserId != null
                            ? String(partyReadyTop.ownerUserId).trim()
                            : '';
                    const lobbyDone = { gameId: gid0 };
                    if (ownStr) {
                        lobbyDone.ownerUserId = ownStr;
                        if (viewerUid) {
                            lobbyDone.viewerIsGameOwner = ownStr === String(viewerUid).trim();
                        }
                    }
                    this.$emit('lobby-character-done', lobbyDone);
                } else {
                    await this.$router.push({ name: 'ChatRoomWithId', params: { id: gid0 } });
                }
            } catch (e) {
                const code = e.response && e.response.data && e.response.data.code;
                if (code === 'PARTY_READY_NEEDS_CHARACTER') {
                    this.progressMessage = this.$i18n.party_ready_needs_character;
                } else {
                    const msg = e.response && e.response.data && e.response.data.error;
                    this.progressMessage = msg || e.message || this.$i18n.error_saving_game;
                }
                setTimeout(() => {
                    this.progressMessage = '';
                }, 9000);
            } finally {
                this.lobbyConfirmBusyStep = null;
                this.isStarting = false;
            }
        },

        async confirmCharacterAndWorld() {
            if (this.setupPhase !== 'confirm_character') return;
            if (!(await this.hydrateConfirmStepFromServer())) return;
            await this.$nextTick();

            let joinParty = this.joinUiActive;
            if (!joinParty && this.setupViewerIsGameOwner === false) {
                joinParty = true;
            }
            const lobbyPh =
                this.$store.state.gameSetup &&
                this.$store.state.gameSetup.party &&
                this.$store.state.gameSetup.party.phase != null
                    ? String(this.$store.state.gameSetup.party.phase)
                    : '';
            if (!joinParty && (lobbyPh === 'lobby' || lobbyPh === 'starting')) {
                joinParty = true;
            }

            const uidCf = this.resolveViewerUserId();
            const fromMap =
                uidCf && this.$store.state.gameSetup?.playerCharacters?.[uidCf]
                    ? this.$store.state.gameSetup.playerCharacters[uidCf]
                    : null;
            const playerCharacter = joinParty
                ? this.confirmSheetCharacter
                : this.confirmSheetCharacter || fromMap;
            if (!playerCharacter) {
                this.progressMessage = this.$i18n.error_generating_character;
                return;
            }

            if (joinParty) {
                this.isStarting = true;
                try {
                    this.clearSetupSessionGameId();
                    this.clearJoinUiGameId();
                    this.setupPhase = 'form';
                    const gid = this.$store.state.gameId;
                    const p = this.partyPhaseFromStore;
                    if (gid && (p === 'lobby' || p === 'starting' || p === 'playing')) {
                        this.persistLobbyCharacterFlowDoneSession(gid);
                    }
                    await this.$router.push({ name: 'ChatRoomWithId', params: { id: gid } });
                } catch (e) {
                    console.error('Join flow: navigate to chat failed:', e);
                    this.progressMessage = this.$i18n.error_saving_game;
                    setTimeout(() => {
                        this.progressMessage = '';
                    }, 4500);
                } finally {
                    this.isStarting = false;
                }
                return;
            }

            this.isStarting = true;
            this.setupBusyAction = 'campaign';
            this.progressMessage = '';

            try {
                let gen;
                try {
                    gen = await this.generateCampaignConcept(this.$store.state.gameId);
                } catch (campaignErr) {
                    console.error('Error generating campaign concept:', campaignErr);
                    const st = campaignErr && campaignErr.response && campaignErr.response.status;
                    const aborted = campaignErr && campaignErr.code === 'ECONNABORTED';
                    if (st === 504 || aborted) {
                        this.progressMessage = this.$i18n.error_campaign_gateway_timeout;
                    } else {
                        this.progressMessage = this.characterGenerationErrorMessage(campaignErr) || this.$i18n.error_generating_campaign;
                    }
                    setTimeout(() => {
                        this.progressMessage = '';
                    }, 8000);
                    return;
                }
                let campaignConcept = '';
                let campaignSpec = null;
                if (gen && typeof gen === 'object' && gen.campaignConcept) {
                    campaignConcept = gen.campaignConcept;
                    campaignSpec = gen;
                } else {
                    console.error('Campaign generation failed or returned no campaignConcept', gen);
                    this.progressMessage = this.$i18n.error_generating_campaign;
                    setTimeout(() => {
                        this.progressMessage = '';
                    }, 4500);
                    return;
                }

                const entry = (campaignSpec && campaignSpec.campaignConcept) ? campaignSpec.campaignConcept : campaignConcept;
                // World-only: campaign premise for local/persisted context. PC identity is not embedded here — /generate loads the sheet from GameState.
                let systemMessageContentDM =
                    entry +
                    ' Assume the player knows nothing. Allow for an organic introduction of information. (Player character is supplied by the server on play turns, not in this message.)';

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
                const uid = this.resolveViewerUserId();
                const gameSetupForSave = {
                    ...this.$store.state.gameSetup,
                    language: this.$store.state.language,
                    ...(uid
                        ? {
                              playerCharacters: {
                                  ...(this.$store.state.gameSetup.playerCharacters || {}),
                                  [uid]: pcSave,
                              },
                          }
                        : {}),
                };
                delete gameSetupForSave.generatedCharacter;
                const initialState = {
                    gameId: gameId,
                    userId: uid || this.$store.state.userId || null,
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

                this.setupBusyAction = 'save';
                await axios.post('/api/game-session/bootstrap-session', initialState);
                console.log('Initial game saved', initialState);
                this.clearSetupSessionGameId();
                this.setupPhase = 'form';
                this.$router.push({ name: 'ChatRoomWithId', params: { id: gameId } });
            } catch (err) {
                console.error('Error confirming character / saving game:', err);
                if (!this.progressMessage) {
                    this.progressMessage = this.$i18n.error_saving_game;
                }
                setTimeout(() => {
                    this.progressMessage = '';
                }, 4500);
            } finally {
                this.isStarting = false;
                this.setupBusyAction = null;
            }
        }
    }
    };</script>


<style scoped>
.setup-page {
  width: 100%;
  max-width: 520px;
  margin: 0 auto;
  padding: 0 12px;
  box-sizing: border-box;
}
.join-flow-status {
  margin: 0 0 12px 0;
  padding: 10px 14px;
  border-radius: 8px;
  background: rgba(60, 90, 120, 0.25);
  border: 1px solid rgba(160, 190, 220, 0.35);
  color: var(--gm-text, #e8e4dc);
  font-size: 0.95rem;
  line-height: 1.45;
}

.join-flow-error {
  margin: 0 0 12px 0;
  padding: 12px 14px;
  border-radius: 8px;
  background: rgba(180, 60, 50, 0.2);
  border: 1px solid rgba(255, 140, 120, 0.35);
  color: #ffc9c0;
  font-size: 0.95rem;
  line-height: 1.45;
}

.setup-page--confirm {
  max-width: min(96vw, 640px);
}
/* Compound selectors beat `.setup-page { max-width: 520px }`; reset horizontal margin from `margin: 0 auto`. */
.setup-page.setup-page--lobby-inline {
  max-width: 100%;
  width: 100%;
  margin: 2px 0 0;
  padding-left: 0;
  padding-right: 0;
}
.setup-page.setup-page--lobby-inline.setup-page--confirm {
  max-width: 100%;
}
.setup-page.setup-page--lobby-inline :deep(.ui-panel) {
  max-width: 100%;
  width: 100%;
  box-sizing: border-box;
}
.setup-page.setup-page--lobby-inline .setup-form {
  padding: 14px 12px;
  gap: 14px;
}
/* Party lobby: stacked row groups (name+gender / race+subrace / class+subclass+level). */
.setup-page.setup-page--lobby-inline .setup-form-fields--lobby-compact {
  display: flex;
  flex-direction: column;
  gap: 14px;
  align-items: stretch;
}
:deep(.ui-panel--setup-confirm) {
  overflow-x: hidden;
  overflow-y: visible;
}
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
.setup-form-fields {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.setup-form-row-group {
  display: grid;
  gap: 16px 20px;
  align-items: start;
  width: 100%;
  box-sizing: border-box;
  min-width: 0;
}
.setup-form-row-group--2 {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.setup-form-row-group--3 {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}
.setup-form-row-group .form-row {
  min-width: 0;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
}
.setup-form-row-group .form-label {
  width: auto;
  padding-left: 0;
}
.setup-form-row-group .form-row .control {
  min-width: 0;
  width: 100%;
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
  margin-bottom: 0;
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}
.confirm-hint {
  margin: 0;
  color: rgba(230, 225, 216, 0.88);
  line-height: 1.45;
  font-size: 0.92rem;
}
.confirm-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  margin-top: 0;
}
.confirm-actions__secondary-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  width: 100%;
}
.confirm-actions__secondary-row--single {
  grid-template-columns: 1fr;
}
.confirm-actions__secondary-row .confirm-btn {
  width: 100%;
  min-width: 0;
}
.confirm-btn {
  width: 100%;
  justify-content: center;
  text-align: center;
  white-space: normal;
  min-height: 44px;
  padding: 0.5rem 0.75rem;
  line-height: 1.3;
  box-sizing: border-box;
}
.confirm-btn--primary {
  font-weight: 700;
}
.confirm-actions--join-party .confirm-btn--join-party {
  font-size: 1rem;
  min-height: 46px;
}
/* Party lobby card: slightly denser confirm chrome so the parchment keeps more vertical room. */
.setup-page.setup-page--lobby-inline.setup-page--confirm .confirm-hint {
  font-size: 0.9rem;
  line-height: 1.42;
}
.setup-page.setup-page--lobby-inline.setup-page--confirm .confirm-actions {
  gap: 7px;
}
.setup-page.setup-page--lobby-inline.setup-page--confirm .confirm-btn {
  min-height: 40px;
  padding: 0.42rem 0.65rem;
  font-size: 0.9rem;
}
.setup-page.setup-page--lobby-inline.setup-page--confirm .confirm-actions--join-party .confirm-btn--join-party {
  min-height: 44px;
  font-size: 0.96rem;
}
@media (max-width: 380px) {
  .confirm-actions__secondary-row {
    grid-template-columns: 1fr;
  }
}
.ui-button.secondary {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.12);
  box-shadow: none;
}
.form-actions .ui-button:disabled {
  cursor: wait;
  opacity: 0.92;
}
.setup-submit-busy {
  display: inline-flex;
  align-items: center;
  gap: 0.35em;
}
.setup-submit-busy::before {
  content: '';
  width: 0.85em;
  height: 0.85em;
  border: 2px solid rgba(255, 255, 255, 0.35);
  border-top-color: rgba(255, 255, 255, 0.95);
  border-radius: 50%;
  animation: setup-spin 0.7s linear infinite;
}
@keyframes setup-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
