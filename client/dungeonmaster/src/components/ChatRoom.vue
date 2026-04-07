// GMAI/client/dungeonmaster/src/components/ChatRoom.vue

<template>
    <div v-if="errorMessage" class="error-message">
        <p>Error: {{ errorMessage }}</p>
        <button @click="tryAgain">Try again</button>
    </div>
    <!-- Do not add `chat-room` here: that class caps width at 640px and shrinks the whole lobby vs the banner. -->
    <div v-else-if="isPartyLobby" class="party-lobby">
        <header class="party-lobby__hero">
            <div class="party-lobby__hero-top">
                <div class="party-lobby__hero-text">
                    <p class="party-lobby__eyebrow">{{ $i18n.lobby_eyebrow }}</p>
                    <h2 class="party-lobby__title">{{ $i18n.lobby_title }}</h2>
                    <p class="party-lobby__desc">{{ $i18n.lobby_desc }}</p>
                </div>
                <button
                    v-if="!partyLobbyActionsFrozen && canShareInviteLink"
                    type="button"
                    class="party-lobby__invite app-banner__tool chat-room__invite-tool"
                    :disabled="inviteLoading"
                    :aria-busy="inviteLoading"
                    :title="$i18n.invite_players"
                    :aria-label="$i18n.invite_players"
                    @click="copyInviteLink"
                >
                    <span class="visually-hidden">{{ $i18n.invite_players }}</span>
                    <FontAwesomeIcon :icon="faUserPlus" class="ui-icon ui-icon--toolbar" aria-hidden="true" />
                </button>
            </div>
        </header>
        <p v-if="lobbyError" class="party-lobby__err" role="alert">{{ lobbyError }}</p>
        <details v-if="partyLobbyDebugEnabled" class="party-lobby__debug" open>
            <summary class="party-lobby__debug-summary">Party lobby debug</summary>
            <p class="party-lobby__debug-hint">
                Toggle: URL <code>?partyLobbyDebug=1</code> or
                <code>localStorage.setItem('dm_party_lobby_debug','1')</code> (reload). Console:
                <code>[GameMasterAI party-lobby]</code>
            </p>
            <pre class="party-lobby__debug-pre" aria-live="polite">{{ partyLobbyDebugText }}</pre>
        </details>
        <section v-if="lobbyMemberRows.length" class="party-lobby__roster" aria-labelledby="party-lobby-roster-title">
            <h3 id="party-lobby-roster-title" class="party-lobby__roster-title">{{ $i18n.lobby_roster_heading }}</h3>
            <ul class="party-lobby__list">
                <li v-for="row in lobbyMemberRows" :key="row.id" class="party-lobby__item">
                    <span
                        class="party-lobby__avatar"
                        :class="{ 'party-lobby__avatar--two': row.initial.length > 1 }"
                        aria-hidden="true"
                    >{{ row.initial }}</span>
                    <div class="party-lobby__identity">
                        <span class="party-lobby__name">{{ row.display }}</span>
                        <span v-if="row.detailsLine" class="party-lobby__meta">{{ row.detailsLine }}</span>
                    </div>
                    <div class="party-lobby__ready-cell">
                        <template v-if="row.isYou && !row.isReady && !partyLobbyRosterOwnRowFrozen">
                            <div class="party-lobby__ready-actions">
                                <UIButton
                                    v-if="showLobbyCharacterEditCta"
                                    type="button"
                                    class="party-lobby__roster-regenerate-btn"
                                    :aria-label="$i18n.regenerate_character"
                                    @click="openLobbyCharacterEditor"
                                >
                                    {{ $i18n.lobby_roster_regenerate_short }}
                                </UIButton>
                                <UIButton
                                    v-if="row.hasSheet"
                                    class="party-lobby__roster-ready-btn"
                                    :disabled="lobbyRosterConfirmPosting"
                                    :aria-busy="lobbyRosterConfirmPosting ? 'true' : 'false'"
                                    :aria-label="$i18n.lobby_roster_ready_aria"
                                    @click="confirmReadyFromRosterCell"
                                >
                                    <span v-if="lobbyRosterConfirmPosting" class="party-lobby__roster-ready-btn__busy">{{
                                        $i18n.lobby_saving_ready_then_start
                                    }}</span>
                                    <span v-else class="party-lobby__roster-ready-btn__inner">
                                        <FontAwesomeIcon
                                            :icon="faCircleCheck"
                                            class="party-lobby__badge-icon"
                                            aria-hidden="true"
                                        />
                                        {{ $i18n.lobby_roster_ready_short }}
                                    </span>
                                </UIButton>
                                <UIButton
                                    v-else
                                    class="party-lobby__roster-ready-btn party-lobby__roster-ready-btn--blocked"
                                    :disabled="true"
                                    :aria-label="$i18n.lobby_roster_need_character_aria"
                                >
                                    <span class="party-lobby__roster-ready-btn__inner">
                                        <FontAwesomeIcon
                                            :icon="faHourglassHalf"
                                            class="party-lobby__badge-icon"
                                            aria-hidden="true"
                                        />
                                        {{ $i18n.lobby_ready_waiting }}
                                    </span>
                                </UIButton>
                            </div>
                        </template>
                        <span
                            v-else
                            class="party-lobby__badge party-lobby__badge--ready"
                            :class="row.isReady ? 'party-lobby__badge--ok' : 'party-lobby__badge--wait'"
                        >
                            <FontAwesomeIcon
                                :icon="row.isReady ? faCircleCheck : faHourglassHalf"
                                class="party-lobby__badge-icon"
                                aria-hidden="true"
                            />
                            <span>{{ row.isReady ? $i18n.lobby_ready_confirmed : $i18n.lobby_ready_waiting }}</span>
                        </span>
                    </div>
                </li>
            </ul>
        </section>

        <section
            v-if="showLobbyCharacterSetupPanel"
            class="party-lobby__setup"
            :aria-label="$i18n.lobby_setup_section_aria"
        >
            <SetupForm
                ref="lobbyInlineSetup"
                lobby-inline
                :lobby-interactions-locked="partyLobbyActionsFrozen"
                :after-character-persisted="syncLobbyAfterCharacterPersisted"
                @lobby-character-done="onLobbyInlineCharacterDone"
                @lobby-inline-wizard-hold="onLobbyInlineWizardHold"
                @lobby-inline-confirm-sheet="onLobbyInlineConfirmSheet"
                @lobby-inline-after-character-saved="onLobbyInlineAfterCharacterSaved"
            />
        </section>
        <p v-if="lastStartError" class="party-lobby__warn">{{ $i18n.lobby_last_error }}: {{ lastStartError }}</p>
        <!-- In-flow sheet (embedded): uses theme .floating-card--embedded width:100% — avoids fixed FAB 492px cap. -->
        <section
            v-if="showPartyLobbyEmbeddedCharacterSheet"
            class="party-lobby__character-sheet"
            :aria-label="$i18n.lobby_character_sheet_section_aria"
        >
            <FloatingCard
                embedded
                :character="playerCharacter"
                :hp-snapshot="playerHitPoints"
            />
        </section>
    </div>
    <div v-else class="chat-room two-column">
        <header class="chat-room__header">
            <!-- Intentionally no "sync / Sincronizar" control — reload the browser to pull latest state. -->
            <div
                v-if="campaignTitle || canShareInviteLink"
                class="campaign-heading-row"
                :class="{ 'campaign-heading-row--no-title': !campaignTitle }"
            >
                <h2 v-if="campaignTitle" class="campaign-heading">{{ campaignTitle }}</h2>
                <button
                    v-if="canShareInviteLink"
                    type="button"
                    class="app-banner__tool chat-room__invite-tool"
                    :disabled="inviteLoading"
                    :aria-busy="inviteLoading"
                    :title="$i18n.invite_players"
                    :aria-label="$i18n.invite_players"
                    @click="copyInviteLink"
                >
                    <span class="visually-hidden">{{ $i18n.invite_players }}</span>
                    <FontAwesomeIcon :icon="faUserPlus" class="ui-icon ui-icon--toolbar" aria-hidden="true" />
                </button>
            </div>
            <div v-if="partyRosterRows.length > 1" class="party-roster" role="status" :aria-label="$i18n.party_heading">
                <div class="party-roster__head">
                    <span class="party-roster__title">{{ $i18n.party_heading }}</span>
                    <span class="party-roster__count">{{ partyRosterRows.length }}</span>
                </div>
                <ul class="party-roster__list">
                    <li v-for="row in partyRosterRows" :key="row.id" class="party-roster__item" :class="{ 'party-roster__item--you': row.isYou }">
                        <span class="party-roster__name">{{ row.display }}</span>
                        <span v-if="row.detailsLine" class="party-roster__meta">{{ row.detailsLine }}</span>
                    </li>
                </ul>
                <p class="party-roster__tip">{{ $i18n.party_sync_tip }}</p>
            </div>
        </header>
        <!-- floating card handled globally -->
        <div ref="transcriptRoot" class="chat-messages-container chat-room__transcript">
            <ChatMessage
              v-for="(message, index) in messages"
              :key="index"
              :message="renderMarkdown(message.text)"
              :sender="message.user"
              :role="message.role === 'player' ? 'player' : 'dm'"
            />
        </div>
        <p v-if="partyRoundWaitingOthersForViewer && !errorMessage" class="chat-room__dm-wait" role="status">{{ $i18n.party_waiting_others }}</p>
        <p v-else-if="chatInputLocked && !errorMessage" class="chat-room__dm-wait" role="status">{{ chatWaitStatusText }}</p>
        <button
            v-if="showUnlockChatInput"
            type="button"
            class="ui-button ui-button--ghost chat-room__unlock"
            @click="unlockChatInput"
        >{{ $i18n.chat_unlock_input }}</button>
        <form @submit.prevent="submitMessage" class="chat-input-form">
            <input class="chat-input" type="text" v-model="newMessage"
              :placeholder="$i18n.chat_placeholder"
              :disabled="chatInputLocked" />
            <button class="ui-button chat-send-button" type="submit" :disabled="chatInputLocked" :aria-busy="isSending">
              {{ isSending ? $i18n.sending : (dmGenerationLikelyInFlight ? $i18n.chat_dm_working : $i18n.send) }}
            </button>
        </form>
        <!-- Notetaker UI removed -->

    <!-- Floating character sheet: PC HP shown here; enemy HP is not shown to the player -->
    <FloatingCard v-if="playerCharacter" :character="playerCharacter" :hp-snapshot="playerHitPoints" :defaultOpen="false" />
    </div>
    <Teleport to="body">
        <Transition name="invite-toast">
            <div
                v-if="inviteToastVisible"
                class="invite-toast"
                :class="inviteToastVariant === 'error' ? 'invite-toast--error' : 'invite-toast--success'"
                role="status"
                aria-live="polite"
            >
                {{ inviteToastMessage }}
            </div>
        </Transition>
    </Teleport>
</template>

<script>
import axios from 'axios';
import MarkdownIt from 'markdown-it';
import ChatMessage from '@/ui/ChatMessage.vue';
import FloatingCard from '@/ui/FloatingCard.vue';
import UIButton from '@/ui/Button.vue';
import SetupForm from '@/components/SetupForm.vue';
import { absoluteInviteUrl } from '@/utils/inviteLink.js';
import { resolveApiBaseURL, resolveGameStateEventsUrl } from '@/utils/apiBase.js';
import { fetchGameStateLoad } from '@/utils/fetchGameStateLoad.js';
import { SESSION_CHAT_TOAST } from '@/setupSession.js';
import { faUserPlus } from '@fortawesome/free-solid-svg-icons/faUserPlus';
import { faCircleCheck } from '@fortawesome/free-solid-svg-icons/faCircleCheck';
import { faHourglassHalf } from '@fortawesome/free-solid-svg-icons/faHourglassHalf';

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

export default {
        components: {
            ChatMessage,
            FloatingCard,
            UIButton,
            SetupForm,
        },
        data() {
            return {
                faUserPlus,
                faCircleCheck,
                faHourglassHalf,
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
                gameOwnerUserId: null,
                /** From GET /game-state/load — host vs member for invite UI. */
                viewerIsGameOwner: null,
                inviteToastVisible: false,
                inviteToastMessage: '',
                inviteToastVariant: 'success',
                inviteToastTimer: null,
                inviteLoading: false,
                /** Poll load while server may still be finishing /generate (backup if SSE misses). */
                pendingReplyPollTimer: null,
                pendingReplyPollTicks: 0,
                /** Fingerprint of poll mode; avoids restart+immediate sync on every loadGameState (that caused a GET /load storm). */
                pendingReplyPollSig: null,
                serverLlmStartedAt: null,
                serverLlmCompletedAt: null,
                pendingAssistantUiHold: false,
                keepaliveAppendPayload: null,
                isInitialOpenBusy: false,
                onPageHideKeepalive: null,
                syncBusy: false,
                memberUserIds: [],
                /** From GET /game-state/load: Mongo user id string → trimmed nickname (lobby roster). */
                memberNicknamesByUserId: {},
                lobbyRosterConfirmPosting: false,
                /** Synced from SetupForm on the inline confirm step — roster cannot rely on Vuex alone. */
                lobbyInlineConfirmSheet: null,
                lobbyCharacterEditorOpen: false,
                /**
                 * Inline SetupForm owns the UI (create / confirm / back-to-form). When false and the viewer has a
                 * saved sheet, show sheet-only (refresh) without requiring sessionStorage or party.readyUserIds.
                 */
                lobbyInlineWizardHold: false,
                lobbyError: '',
                lobbyStartingPollTimer: null,
                /** Multiplayer: this client submitted; DM runs only after every member acts. */
                partyRoundWaitingOthers: false,
                /** SSE (EventSource) for game-state-updated; same gameId + OPEN/CONNECTING avoids reconnect churn. */
                gameStateEventSource: null,
                gameStateEventSourceGameId: null,
                gameStateEsReconnectTimer: null,
                gameStateEsBackoffMs: 1000,
                pushSyncTimer: null,
            };
        },
        // removed duplicate data() 
        async created() {
            // language is now global in the store; no local initialization needed
            if (!this.$store.getters.isAuthenticated) {
                return;
            }

            // check if a gameId is provided in the route
            if (this.$route.params.id) {
                // Same as syncFromServer: trust server sheet for this viewer after load (parity with refresh vs button).
                const loaded = await this.loadGameState(this.$route.params.id, { replaceLocalCharacter: true });
                if (!loaded) return;
                this.systemMessageContentDM = this.$store.state.systemMessageContentDM || '';
                const inLobby =
                    this.$store.state.gameSetup &&
                    this.$store.state.gameSetup.party &&
                    (this.$store.state.gameSetup.party.phase === 'lobby' ||
                        this.$store.state.gameSetup.party.phase === 'starting');
                if (!inLobby && String(this.systemMessageContentDM || '').trim()) {
                    const systemMessageDM = {
                        role: 'system',
                        content: this.systemMessageContentDM,
                    };
                    if (!this.conversation.find(m => m.role === 'system' && m.content === this.systemMessageContentDM)) {
                        this.conversation.unshift(systemMessageDM);
                    }
                }
            }
        },

        computed: {
            myUserIdStr() {
                const id = this.$store.state.userId;
                if (id == null) return '';
                const s = String(id).trim();
                return s;
            },
            /** Mirrors GET /game-state/load `viewerIsGameOwner` so UI (e.g. invite) works before `ownerUserId` is hydrated. */
            isGameOwner() {
                if (this.viewerIsGameOwner === false) return false;
                if (this.viewerIsGameOwner === true) return true;
                const oid = this.gameOwnerUserId;
                return Boolean(oid && this.myUserIdStr && String(oid) === String(this.myUserIdStr));
            },
            /** Host and invited members may copy a share link; matches server assertGameMember on create-invite. */
            canShareInviteLink() {
                if (!this.myUserIdStr) return false;
                if (this.isGameOwner) return true;
                const uid = String(this.myUserIdStr);
                const listed = Array.isArray(this.memberUserIds) ? this.memberUserIds.map(String) : [];
                return listed.includes(uid);
            },
            /** In-chat label for the current user: PC sheet name first; never OAuth real name or email. */
            playerChatLabel() {
                const c = this.playerCharacter;
                if (c && c.name != null && String(c.name).trim()) {
                    return String(c.name).trim();
                }
                const nick = this.$store.state.user && this.$store.state.user.nickname && String(this.$store.state.user.nickname).trim();
                if (nick) return nick;
                return 'Player';
            },
            playerCharacter() {
                const uid = this.myUserIdStr;
                const sheet = uid ? this.pickPlayerCharacterFromStore(uid) : null;
                if (sheet && typeof sheet === 'object') {
                    return this.localPlayerCharacter || sheet;
                }
                if (this.localPlayerCharacter) return this.localPlayerCharacter;
                return null;
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
            },
            /** Roster for UI + aria: union of server members and any persisted playerCharacters keys. */
            partyRosterRows() {
                const gs = this.$store.state.gameSetup;
                const pcMap =
                    gs && gs.playerCharacters && typeof gs.playerCharacters === 'object' ? gs.playerCharacters : {};
                const listed = Array.isArray(this.memberUserIds) ? this.memberUserIds.map(String) : [];
                const idSet = new Set([...listed, ...Object.keys(pcMap).map(String)]);
                const ids = [...idSet];
                if (ids.length < 2) return [];
                const owner = this.gameOwnerUserId ? String(this.gameOwnerUserId) : '';
                ids.sort((a, b) => {
                    if (owner) {
                        if (a === owner) return -1;
                        if (b === owner) return 1;
                    }
                    return a.localeCompare(b);
                });
                const uid = this.myUserIdStr;
                return ids.map((id) => {
                    const sheet = this.pickPlayerCharacterFromStore(id);
                    const nameRaw = sheet && sheet.name != null && String(sheet.name).trim();
                    const label = nameRaw ? String(sheet.name).trim() : this.memberPrimaryLabel(id);
                    const isYou = Boolean(uid && this.normalizeLobbyUserIdKey(uid) === this.normalizeLobbyUserIdKey(id));
                    const isHost = Boolean(
                        owner && this.normalizeLobbyUserIdKey(owner) === this.normalizeLobbyUserIdKey(id)
                    );
                    let display = label;
                    if (isYou) display += ` (${this.$i18n.party_you})`;
                    if (isHost) display += ` — ${this.$i18n.party_host}`;
                    const detailsLine = this.formatPartyMemberSheetSummary(sheet);
                    return { id, display, detailsLine, isYou, isHost };
                });
            },
            /**
             * True when another party member may be in the same game (realtime push + early message append).
             * Do not rely on partyRosterRows alone: roster can be empty before sheets load, while
             * memberUserIds already lists the host + guest.
             */
            gameSessionBroadcastActive() {
                const listed = Array.isArray(this.memberUserIds) ? this.memberUserIds : [];
                if (listed.length >= 2) return true;
                const oid = this.gameOwnerUserId;
                const uid = this.myUserIdStr;
                return Boolean(oid && uid && String(oid) !== String(uid));
            },
            /** Same membership as server `requiredPartyMemberIdStrings`: owner ∪ memberUserIds. */
            canonicalPartyMemberIdsSorted() {
                const listed = Array.isArray(this.memberUserIds) ? this.memberUserIds.map(String) : [];
                const owner = this.gameOwnerUserId ? String(this.gameOwnerUserId) : '';
                const idSet = new Set(listed.filter(Boolean));
                if (owner) idSet.add(owner);
                return [...idSet].filter(Boolean).sort();
            },
            /**
             * Multiplayer round: not every member has appeared in partySubmittedUserIds yet (server gameSetup).
             * While true, players who have not submitted keep the input enabled; those who submitted wait.
             */
            partyRoundCollectingActions() {
                if (!this.gameSessionBroadcastActive) return false;
                if (!this.conversationEndsAwaitingAssistant) return false;
                const req = this.canonicalPartyMemberIdsSorted;
                if (req.length < 2) return false;
                const gs = this.$store.state.gameSetup;
                const submitted =
                    gs && Array.isArray(gs.partySubmittedUserIds) ? gs.partySubmittedUserIds.map(String) : [];
                return !req.every((id) => submitted.includes(id));
            },
            /** This viewer may still send (or replace) their line for the current party round. */
            partyRoundOpenForMyAction() {
                if (!this.partyRoundCollectingActions) return false;
                const uid = this.myUserIdStr;
                if (!uid) return false;
                if (this.partyRoundWaitingOthers) return false;
                const gs = this.$store.state.gameSetup;
                const submitted =
                    gs && Array.isArray(gs.partySubmittedUserIds) ? gs.partySubmittedUserIds.map(String) : [];
                return !submitted.includes(String(uid));
            },
            /** “Waiting for others” only for players who already submitted this round (not the whole table). */
            partyRoundWaitingOthersForViewer() {
                if (this.partyRoundWaitingOthers) return true;
                if (!this.partyRoundCollectingActions || !this.myUserIdStr) return false;
                const gs = this.$store.state.gameSetup;
                const submitted =
                    gs && Array.isArray(gs.partySubmittedUserIds) ? gs.partySubmittedUserIds.map(String) : [];
                return submitted.includes(String(this.myUserIdStr));
            },
            /** Last stored turn is from the player — assistant reply might still be generating on the server. */
            conversationEndsAwaitingAssistant() {
                const c = this.conversation;
                if (!Array.isArray(c) || c.length === 0) return false;
                const last = c[c.length - 1];
                return Boolean(last && last.role === 'user');
            },
            /** True when GameState says /generate started after the last completed stamp (see server markLlmGenerateFinished). */
            dmGenerationLikelyInFlight() {
                const started = this.serverLlmStartedAt;
                if (!started || typeof started !== 'string') return false;
                const ts = Date.parse(started);
                if (Number.isNaN(ts) || Date.now() - ts > 15 * 60 * 1000) return false;
                const completed = this.serverLlmCompletedAt;
                if (!completed || typeof completed !== 'string') return true;
                const tc = Date.parse(completed);
                if (Number.isNaN(tc)) return true;
                return ts > tc;
            },
            chatInputLocked() {
                if (this.isSending || this.isInitialOpenBusy) return true;
                if (this.dmGenerationLikelyInFlight) return true;
                if (this.conversationEndsAwaitingAssistant && !this.errorMessage && this.pendingAssistantUiHold) {
                    if (this.partyRoundOpenForMyAction) return false;
                    return true;
                }
                return false;
            },
            showUnlockChatInput() {
                const dmBlocks = this.dmGenerationLikelyInFlight && !this.partyRoundCollectingActions;
                return (
                    this.conversationEndsAwaitingAssistant &&
                    !this.errorMessage &&
                    this.pendingAssistantUiHold &&
                    !dmBlocks &&
                    !this.isSending &&
                    !this.isInitialOpenBusy &&
                    !this.partyRoundOpenForMyAction &&
                    this.chatInputLocked
                );
            },
            chatWaitStatusText() {
                if (this.dmGenerationLikelyInFlight && !this.partyRoundCollectingActions) {
                    return this.$i18n.chat_waiting_dm_server;
                }
                if (
                    this.conversationEndsAwaitingAssistant &&
                    this.pendingAssistantUiHold &&
                    !this.partyRoundOpenForMyAction
                ) {
                    return this.$i18n.chat_waiting_dm_sync;
                }
                return '';
            },
            partyPhase() {
                const p = this.$store.state.gameSetup && this.$store.state.gameSetup.party;
                return p && p.phase ? String(p.phase) : '';
            },
            isPartyLobby() {
                const ph = this.partyPhase;
                return ph === 'lobby' || ph === 'starting';
            },
            lastStartError() {
                const p = this.$store.state.gameSetup && this.$store.state.gameSetup.party;
                return p && p.lastStartError ? String(p.lastStartError) : '';
            },
            /**
             * Must NOT depend on `lobbyMemberRows` (that computed also drives per-row `hasSheet` / viewer heuristics).
             * Mirror server `allMembersReady`: every canonical member id appears in `party.readyUserIds`.
             */
            partyLobbyAllMembersReady() {
                const owner = this.gameOwnerUserId != null ? String(this.gameOwnerUserId) : '';
                const listed = Array.isArray(this.memberUserIds) ? this.memberUserIds.map(String) : [];
                const idNorm = new Set();
                if (owner) idNorm.add(this.normalizeLobbyUserIdKey(owner));
                for (const x of listed) {
                    const n = this.normalizeLobbyUserIdKey(x);
                    if (n) idNorm.add(n);
                }
                const readyRaw = this.$store.state.gameSetup && this.$store.state.gameSetup.party;
                const readyArr = readyRaw && Array.isArray(readyRaw.readyUserIds) ? readyRaw.readyUserIds : [];
                const readyNorm = new Set(
                    readyArr.map((x) => this.normalizeLobbyUserIdKey(x)).filter(Boolean)
                );
                if (idNorm.size === 0) return false;
                for (const id of idNorm) {
                    if (!readyNorm.has(id)) return false;
                }
                return true;
            },
            /** Lobby toolbar + inline setup: no further ready toggles while starting or everyone is ready. */
            partyLobbyActionsFrozen() {
                return this.partyLobbyAllMembersReady || this.partyPhase === 'starting';
            },
            /**
             * Roster “your row” primary control: do NOT tie to `partyPhase === 'starting'`. If a start attempt
             * stalls, `starting` + empty `readyUserIds` would grey out Pendiente forever while the UI still
             * looks like the lobby. Freeze only when every canonical member is already in `readyUserIds`.
             */
            partyLobbyRosterOwnRowFrozen() {
                return this.partyLobbyAllMembersReady;
            },
            /** Invited player has already pressed mark ready — hide “edit character” so they cannot reopen the wizard. */
            viewerLobbyMarkedReady() {
                if (this.isGameOwner) return false;
                const uid = this.myUserIdStr;
                if (!uid) return false;
                const gs = this.$store.state.gameSetup;
                const ids = gs && gs.party && Array.isArray(gs.party.readyUserIds) ? gs.party.readyUserIds : [];
                return ids.map(String).includes(String(uid));
            },
            /**
             * Same object the lobby embedded sheet uses (`playerCharacter`), plus inline confirm from SetupForm.
             * Avoids false “no sheet” when `playerCharacters` keys disagree but the visible sheet is fine.
             */
            viewerHasLobbyReadySheet() {
                const uid = this.myUserIdStr;
                if (!uid) return false;
                if (this.lobbyInlineConfirmSheet && typeof this.lobbyInlineConfirmSheet === 'object') {
                    return true;
                }
                return this.lobbySheetIsCommitted(this.playerCharacter);
            },
            lobbyMemberRows() {
                const gs = this.$store.state.gameSetup;
                const listed = Array.isArray(this.memberUserIds) ? this.memberUserIds.map(String) : [];
                const owner = this.gameOwnerUserId ? String(this.gameOwnerUserId) : '';
                const pcMap =
                    gs && gs.playerCharacters && typeof gs.playerCharacters === 'object' && !Array.isArray(gs.playerCharacters)
                        ? gs.playerCharacters
                        : {};
                const idSet = new Set([...listed, ...Object.keys(pcMap).map(String)]);
                if (owner) idSet.add(owner);
                const ready = new Set(
                    (gs && gs.party && Array.isArray(gs.party.readyUserIds) ? gs.party.readyUserIds : [])
                        .map((x) => this.normalizeLobbyUserIdKey(x))
                        .filter(Boolean)
                );
                const uid = this.myUserIdStr;
                const ownerNorm = owner ? this.normalizeLobbyUserIdKey(owner) : '';
                const ids = [...idSet];
                ids.sort((a, b) => {
                    if (ownerNorm) {
                        if (this.normalizeLobbyUserIdKey(a) === ownerNorm) return -1;
                        if (this.normalizeLobbyUserIdKey(b) === ownerNorm) return 1;
                    }
                    return a.localeCompare(b);
                });
                return ids.map((id) => {
                    const isYou = Boolean(uid && this.normalizeLobbyUserIdKey(id) === this.normalizeLobbyUserIdKey(uid));
                    const sheet = isYou
                        ? (this.lobbyInlineConfirmSheet && typeof this.lobbyInlineConfirmSheet === 'object'
                              ? this.lobbyInlineConfirmSheet
                              : this.effectivePlayerCharacterForMember(id))
                        : this.effectivePlayerCharacterForMember(id);
                    const hasSheet = isYou ? this.viewerHasLobbyReadySheet : this.lobbySheetIsCommitted(sheet);
                    const primary = this.memberPrimaryLabel(id);
                    const isHost = Boolean(ownerNorm && this.normalizeLobbyUserIdKey(id) === ownerNorm);
                    let display = primary;
                    if (isYou) display += ` (${this.$i18n.party_you})`;
                    if (isHost) display += ` — ${this.$i18n.party_host}`;
                    const detailsLine = hasSheet ? this.formatPartyMemberSheetSummary(sheet) : '';
                    let initial = '?';
                    if (hasSheet && sheet && typeof sheet === 'object') {
                        const nm =
                            (typeof sheet.name === 'string' && sheet.name.trim()) ||
                            (sheet.identity &&
                                typeof sheet.identity === 'object' &&
                                typeof sheet.identity.name === 'string' &&
                                sheet.identity.name.trim()) ||
                            '';
                        if (nm) initial = this.lobbyAvatarInitialsFromDisplayName(nm);
                    }
                    if (initial === '?') {
                        const p = String(primary || '').trim();
                        if (p) initial = this.lobbyAvatarInitialsFromDisplayName(p);
                    }
                    return {
                        id,
                        display,
                        detailsLine,
                        hasSheet,
                        isReady: ready.has(this.normalizeLobbyUserIdKey(id)),
                        initial,
                        isYou,
                        isHost,
                    };
                });
            },
            /**
             * After confirm step (or on refresh with a saved sheet), hide inline wizard until “edit character”.
             * - Session flag: set on success paths.
             * - Server: committed sheet + wizard not holding the UI (see lobbyInlineWizardHold / SetupForm emits).
             * Flow “done” uses committed sheet + wizard hold; ready state is party.readyUserIds (roster column).
             */
            lobbyCharacterFlowDoneForGame() {
                const gid =
                    this.$store.state.gameId || (this.$route.params.id ? String(this.$route.params.id) : '');
                if (!gid) return false;
                try {
                    if (sessionStorage.getItem(`dm_lobby_char_done_${gid}`) === '1') return true;
                } catch (e) {
                    /* ignore */
                }
                if (this.lobbyInlineWizardHold || this.lobbyCharacterEditorOpen) return false;
                const uid = this.myUserIdStr;
                if (!uid) return false;
                const fromStore = this.pickPlayerCharacterFromStore(uid);
                const local =
                    this.localPlayerCharacter && typeof this.localPlayerCharacter === 'object'
                        ? this.localPlayerCharacter
                        : null;
                const sheet =
                    fromStore && typeof fromStore === 'object' ? fromStore : local;
                if (!sheet || !this.lobbySheetIsCommitted(sheet)) return false;
                return true;
            },
            showLobbyCharacterSetupPanel() {
                if (!this.isPartyLobby) return false;
                if (this.lobbyCharacterEditorOpen) return true;
                return !this.lobbyCharacterFlowDoneForGame;
            },
            showLobbyCharacterEditCta() {
                if (
                    !this.isPartyLobby ||
                    !this.lobbyCharacterFlowDoneForGame ||
                    this.lobbyCharacterEditorOpen
                ) {
                    return false;
                }
                if (this.partyLobbyActionsFrozen) return false;
                if (this.viewerLobbyMarkedReady) return false;
                return true;
            },
            /**
             * In lobby, only show the in-flow sheet when this game has a saved PC for the viewer.
             * Avoids stale localPlayerCharacter / Vuex bleed from another game while the inline wizard is open.
             */
            showPartyLobbyEmbeddedCharacterSheet() {
                if (!this.isPartyLobby) return false;
                if (this.showLobbyCharacterSetupPanel) return false;
                const uid = this.myUserIdStr;
                return Boolean(uid && this.memberHasCommittedSheet(uid));
            },
            /** Opt-in: query partyLobbyDebug=1 or localStorage key dm_party_lobby_debug === 1. */
            partyLobbyDebugEnabled() {
                try {
                    const q = this.$route && this.$route.query ? this.$route.query.partyLobbyDebug : null;
                    const qs = q != null ? String(q).toLowerCase() : '';
                    if (qs === '1' || qs === 'true' || qs === 'yes') return true;
                } catch (e) {
                    /* ignore */
                }
                try {
                    return typeof localStorage !== 'undefined' && localStorage.getItem('dm_party_lobby_debug') === '1';
                } catch (e) {
                    return false;
                }
            },
            partyLobbyDebugPayload() {
                if (!this.partyLobbyDebugEnabled) return null;
                const gs = this.$store.state.gameSetup || {};
                const party = gs.party || {};
                const pcMap = gs.playerCharacters && typeof gs.playerCharacters === 'object' ? gs.playerCharacters : {};
                const pcKeys = Object.keys(pcMap);
                const yourRow = (this.lobbyMemberRows || []).find((r) => r.isYou) || null;
                const rosterBranch = yourRow
                    ? yourRow.isYou && !yourRow.isReady && !this.partyLobbyRosterOwnRowFrozen
                        ? yourRow.hasSheet
                            ? 'primary_cta_confirm_ready'
                            : 'inner_disabled_waiting_sheet'
                        : 'outer_badge_readonly'
                    : 'no_your_row';
                return {
                    at: new Date().toISOString(),
                    reason: 'snapshot',
                    gameId: this.$store.state.gameId,
                    routeGameId: this.$route.params.id,
                    partyPhase: this.partyPhase,
                    isPartyLobby: this.isPartyLobby,
                    myUserIdStr: this.myUserIdStr,
                    myUserIdNorm: this.normalizeLobbyUserIdKey(this.myUserIdStr),
                    viewerIsGameOwner: this.viewerIsGameOwner,
                    gameOwnerUserId: this.gameOwnerUserId,
                    ownerNorm: this.normalizeLobbyUserIdKey(this.gameOwnerUserId),
                    memberUserIds: [...(this.memberUserIds || [])],
                    readyUserIdsRaw: [...(party.readyUserIds || [])],
                    readyUserIdsNorm: (party.readyUserIds || [])
                        .map((x) => this.normalizeLobbyUserIdKey(x))
                        .filter(Boolean),
                    partyLobbyAllMembersReady: this.partyLobbyAllMembersReady,
                    partyLobbyRosterOwnRowFrozen: this.partyLobbyRosterOwnRowFrozen,
                    partyLobbyActionsFrozen: this.partyLobbyActionsFrozen,
                    viewerHasLobbyReadySheet: this.viewerHasLobbyReadySheet,
                    lobbyInlineConfirmSheet: Boolean(this.lobbyInlineConfirmSheet),
                    lobbyRosterConfirmPosting: this.lobbyRosterConfirmPosting,
                    memberHasCommittedSheet_me: this.myUserIdStr
                        ? this.memberHasCommittedSheet(this.myUserIdStr)
                        : false,
                    playerCharacters_keys: pcKeys,
                    yourRosterRow: yourRow,
                    rosterUiBranch: rosterBranch,
                    showPrimaryReadyButton: Boolean(
                        yourRow &&
                            yourRow.isYou &&
                            !yourRow.isReady &&
                            !this.partyLobbyRosterOwnRowFrozen &&
                            yourRow.hasSheet
                    ),
                };
            },
            partyLobbyDebugText() {
                if (!this.partyLobbyDebugEnabled) return '';
                try {
                    return JSON.stringify(this.partyLobbyDebugPayload, null, 2);
                } catch (e) {
                    return String(e && e.message ? e.message : e);
                }
            },
            partyLobbyDebugFingerprint() {
                if (!(this.partyLobbyDebugEnabled && this.isPartyLobby)) return '';
                const p = this.partyLobbyDebugPayload;
                if (!p) return '';
                return [
                    p.partyPhase,
                    p.myUserIdStr,
                    p.partyLobbyAllMembersReady ? '1' : '0',
                    p.partyLobbyRosterOwnRowFrozen ? '1' : '0',
                    p.viewerHasLobbyReadySheet ? '1' : '0',
                    p.lobbyInlineConfirmSheet ? '1' : '0',
                    p.memberHasCommittedSheet_me ? '1' : '0',
                    p.rosterUiBranch,
                    JSON.stringify(p.readyUserIdsNorm || []),
                    (p.yourRosterRow && p.yourRosterRow.id) || '',
                    p.yourRosterRow && p.yourRosterRow.isReady ? '1' : '0',
                    p.yourRosterRow && p.yourRosterRow.hasSheet ? '1' : '0',
                ].join('|');
            },
        },

        watch: {
            '$store.state.gameId'(to, from) {
                if (String(to ?? '') !== String(from ?? '')) {
                    this.lobbyCharacterEditorOpen = false;
                    this.lobbyInlineWizardHold = false;
                    this.localPlayerCharacter = null;
                    this.lobbyInlineConfirmSheet = null;
                }
            },
            '$route.params.id'(to, from) {
                if (!this.$store.getters.isAuthenticated) return;
                const next = to != null && String(to).trim() !== '' ? String(to).trim() : '';
                const prev = from != null && String(from).trim() !== '' ? String(from).trim() : '';
                if (!next || next === prev) return;
                this.localPlayerCharacter = null;
                this.lobbyCharacterEditorOpen = false;
                this.lobbyInlineWizardHold = false;
                this.lobbyInlineConfirmSheet = null;
                void this.loadGameState(next, { replaceLocalCharacter: true });
            },
            partyLobbyDebugFingerprint() {
                if (!this.partyLobbyDebugEnabled || !this.isPartyLobby) return;
                this.logPartyLobbyDebug('watch');
            },
            partyPhase: {
                immediate: true,
                handler(ph) {
                    if (ph === 'starting') {
                        this.showInviteToast(this.$i18n.lobby_starting, 'success', 5600);
                        if (this.lobbyStartingPollTimer) clearInterval(this.lobbyStartingPollTimer);
                        this.lobbyStartingPollTimer = null;
                        /* SSE delivers game-state-updated; poll only if push is down (rare safety). */
                        const pushOpen =
                            this.gameStateEventSource && this.gameStateEventSource.readyState === EventSource.OPEN;
                        if (pushOpen) return;
                        const lobbyPollMs = 45000;
                        this.lobbyStartingPollTimer = setInterval(() => {
                            if (this.partyPhase !== 'starting') {
                                clearInterval(this.lobbyStartingPollTimer);
                                this.lobbyStartingPollTimer = null;
                                return;
                            }
                            if (!this.syncBusy && !this.isSending) {
                                void this.syncFromServer();
                            }
                        }, lobbyPollMs);
                    } else if (this.lobbyStartingPollTimer) {
                        clearInterval(this.lobbyStartingPollTimer);
                        this.lobbyStartingPollTimer = null;
                    }
                },
            },
            gameSessionBroadcastActive() {
                this.$nextTick(() => this.maybeStartGameStateEventSource());
            },
            'messages.length'(n, o) {
                if (n === 0) return;
                const prev = typeof o === 'number' ? o : 0;
                this.scrollChatToBottom({ smooth: prev > 0 && n > prev });
            },
        },

        mounted() {
            this.visSyncTimer = null;
            this.onDocumentVisibility = () => {
                if (document.visibilityState !== 'visible') return;
                if (!this.$route.params.id) return;
                if (!this.$store.getters.isAuthenticated) return;
                if (!this.gameSessionBroadcastActive) return;
                if (this.syncBusy || this.isSending) return;
                if (this.visSyncTimer) clearTimeout(this.visSyncTimer);
                this.visSyncTimer = setTimeout(() => {
                    this.visSyncTimer = null;
                    this.syncFromServer();
                }, 450);
            };
            document.addEventListener('visibilitychange', this.onDocumentVisibility);
            this.onPageHideKeepalive = () => this.flushKeepaliveAppend();
            window.addEventListener('pagehide', this.onPageHideKeepalive);
            this.$nextTick(() => {
                this.maybeStartGameStateEventSource();
                this.scrollChatToBottom({ smooth: false });
                this.flushSessionChatToast();
                if (this.partyLobbyDebugEnabled) {
                    this.logPartyLobbyDebug('mounted');
                }
            });
        },

        beforeUnmount() {
            if (this.onPageHideKeepalive) {
                window.removeEventListener('pagehide', this.onPageHideKeepalive);
            }
            document.removeEventListener('visibilitychange', this.onDocumentVisibility);
            if (this.visSyncTimer) {
                clearTimeout(this.visSyncTimer);
                this.visSyncTimer = null;
            }
            if (this.lobbyStartingPollTimer) {
                clearInterval(this.lobbyStartingPollTimer);
                this.lobbyStartingPollTimer = null;
            }
            if (this.inviteToastTimer) {
                clearTimeout(this.inviteToastTimer);
                this.inviteToastTimer = null;
            }
            this.stopPendingReplyPoller();
            this.stopGameStateEventSource();
        },

        methods: {
            /** Same id/label resolution as FloatingCard (race/class lists from $i18n). */
            localizePartySheetField(value, list) {
                if (value == null || value === '') return '';
                const raw = String(value).trim();
                const id = raw.toLowerCase().replace(/_/g, '-').replace(/\s+/g, '-');
                const found = (list || []).find((x) => x.id === id);
                return found ? found.label : raw;
            },
            localizePartySubclassLabel(subRaw) {
                const s = String(subRaw || '').trim();
                if (!s || s.toLowerCase() === 'random') return '';
                const labels = this.$i18n.subclass_labels || {};
                const keyUnderscore = s.toLowerCase().replace(/-/g, '_').replace(/\s+/g, '_');
                if (labels[keyUnderscore]) return labels[keyUnderscore];
                return s.replace(/\*+$/u, '').trim();
            },
            localizePartySubraceLabel(subraceRaw) {
                const s = String(subraceRaw || '').trim();
                if (!s || s.toLowerCase() === 'random') return '';
                const labels = this.$i18n.subrace_labels || {};
                const keyUnderscore = s.toLowerCase().replace(/-/g, '_').replace(/\s+/g, '_');
                if (labels[keyUnderscore]) return labels[keyUnderscore];
                return s.replace(/\*+$/u, '').trim();
            },
            /** Race · subrace · class · subclass — Level n (localized); empty if no sheet fields. */
            formatPartyMemberSheetSummary(sheet) {
                if (!sheet || typeof sheet !== 'object') return '';
                const race = this.localizePartySheetField(sheet.race, this.$i18n.races || []);
                /** Prefer canonical ids so lobby matches FloatingCard (model prose in `subrace` may include footnote markers). */
                const subraceSrc =
                    sheet.subraceId != null && String(sheet.subraceId).trim()
                        ? sheet.subraceId
                        : sheet.subrace;
                const subraceLine = this.localizePartySubraceLabel(subraceSrc);
                const clsRaw =
                    sheet.characterClass != null && String(sheet.characterClass).trim()
                        ? sheet.characterClass
                        : sheet.class;
                const cls = this.localizePartySheetField(clsRaw, this.$i18n.classes || []);
                const subRaw =
                    sheet.subclassId != null && String(sheet.subclassId).trim()
                        ? sheet.subclassId
                        : sheet.subclass;
                const sub = this.localizePartySubclassLabel(subRaw);
                const lv =
                    sheet.level != null && !Number.isNaN(Number(sheet.level))
                        ? Math.min(20, Math.max(1, Math.floor(Number(sheet.level))))
                        : null;
                const parts = [];
                if (race) parts.push(race);
                if (subraceLine) parts.push(subraceLine);
                if (cls) parts.push(cls);
                if (sub) parts.push(sub);
                let line = parts.join(' · ');
                if (lv != null) {
                    const lp = this.$i18n.level_prefix || 'Level ';
                    line = line ? `${line} — ${lp}${lv}` : `${lp}${lv}`;
                }
                return line;
            },
            normalizeLobbyUserIdKey(raw) {
                if (raw == null) return '';
                return String(raw).trim().toLowerCase();
            },
            /** True when the sheet is usable for lobby roster / ready (name, summary line, or core stats). */
            lobbySheetIsCommitted(sheet) {
                if (!sheet || typeof sheet !== 'object') return false;
                if (this.committedCharacterDisplayName(sheet)) return true;
                if (this.formatPartyMemberSheetSummary(sheet)) return true;
                const hp = Number(sheet.max_hp);
                const ac = Number(sheet.ac);
                return Number.isFinite(hp) && Number.isFinite(ac);
            },
            pickPlayerCharacterFromStore(memberId) {
                const want = this.normalizeLobbyUserIdKey(memberId);
                if (!want) return null;
                const gs = this.$store.state.gameSetup;
                if (!gs) return null;
                const pcMap =
                    gs.playerCharacters && typeof gs.playerCharacters === 'object' && !Array.isArray(gs.playerCharacters)
                        ? gs.playerCharacters
                        : {};
                const keys = Object.keys(pcMap);
                for (const k of keys) {
                    if (this.normalizeLobbyUserIdKey(k) === want && pcMap[k] && typeof pcMap[k] === 'object') {
                        return pcMap[k];
                    }
                }
                const nonEmpty = keys.filter((k) => pcMap[k] && typeof pcMap[k] === 'object');
                if (nonEmpty.length === 1) {
                    const only = pcMap[nonEmpty[0]];
                    const ownerN = this.normalizeLobbyUserIdKey(this.gameOwnerUserId);
                    const listed = Array.isArray(this.memberUserIds) ? this.memberUserIds : [];
                    const listedNorm = listed.map((x) => this.normalizeLobbyUserIdKey(x));
                    const soleHostParty =
                        ownerN &&
                        want === ownerN &&
                        (listedNorm.length === 0 ||
                            (listedNorm.length === 1 && listedNorm[0] === ownerN));
                    const soleListedYou = listedNorm.length === 1 && listedNorm[0] === want;
                    if (soleHostParty || soleListedYou) {
                        return only;
                    }
                }
                return null;
            },
            /**
             * Store row plus, for the current user only, lobby `localPlayerCharacter` (sheet visible before
             * Vuex catches up). Shallow-merge so draft edits overlay persisted keys.
             */
            effectivePlayerCharacterForMember(memberId) {
                const id = memberId != null ? String(memberId) : '';
                if (!id) return null;
                const fromStore = this.pickPlayerCharacterFromStore(id);
                const uid = this.myUserIdStr;
                const isYou = Boolean(uid && this.normalizeLobbyUserIdKey(uid) === this.normalizeLobbyUserIdKey(id));
                const local =
                    isYou && this.localPlayerCharacter && typeof this.localPlayerCharacter === 'object'
                        ? this.localPlayerCharacter
                        : null;
                if (local && fromStore && typeof fromStore === 'object') {
                    return { ...fromStore, ...local };
                }
                if (local) return local;
                if (fromStore && typeof fromStore === 'object') return fromStore;
                return null;
            },
            committedCharacterDisplayName(sheet) {
                if (!sheet || typeof sheet !== 'object') return '';
                const top = sheet.name != null && String(sheet.name).trim();
                if (top) return String(sheet.name).trim();
                const idn =
                    sheet.identity &&
                    typeof sheet.identity === 'object' &&
                    sheet.identity.name != null &&
                    String(sheet.identity.name).trim();
                if (idn) return String(sheet.identity.name).trim();
                return '';
            },
            memberHasCommittedSheet(memberId) {
                const uid = this.myUserIdStr;
                if (uid && this.normalizeLobbyUserIdKey(memberId) === this.normalizeLobbyUserIdKey(uid)) {
                    return this.viewerHasLobbyReadySheet;
                }
                return this.lobbySheetIsCommitted(this.effectivePlayerCharacterForMember(memberId));
            },
            /** Character name if saved; else account nickname from load payload; else pending i18n. */
            memberPrimaryLabel(memberId) {
                const sheet = this.effectivePlayerCharacterForMember(memberId);
                const charName = this.committedCharacterDisplayName(sheet);
                if (charName) return charName;
                const nid = memberId != null ? String(memberId) : '';
                const map = this.memberNicknamesByUserId || {};
                const nick = map[nid] && String(map[nid]).trim();
                if (nick) return nick;
                return this.$i18n.party_character_pending;
            },
            /**
             * Party roster avatar: two letters from first + last word when the name has 2+ words;
             * otherwise one letter. Ignores empty tokens after split.
             */
            lobbyAvatarInitialsFromDisplayName(raw) {
                const s = String(raw || '').trim();
                if (!s) return '?';
                const pending = this.$i18n.party_character_pending;
                if (pending && s === String(pending).trim()) return '?';
                const parts = s.split(/\s+/).filter((t) => t.length > 0);
                if (parts.length === 0) return '?';
                const up = (ch) => (ch ? String(ch).charAt(0).toUpperCase() : '');
                if (parts.length === 1) {
                    const one = up(parts[0].charAt(0));
                    return one || '?';
                }
                const a = up(parts[0].charAt(0));
                const b = up(parts[parts.length - 1].charAt(0));
                if (a && b) return `${a}${b}`;
                return a || b || '?';
            },
            openLobbyCharacterEditor() {
                const gid =
                    this.$store.state.gameId || (this.$route.params.id ? String(this.$route.params.id) : '');
                if (gid) {
                    try {
                        sessionStorage.removeItem(`dm_lobby_char_done_${gid}`);
                    } catch (e) {
                        /* ignore */
                    }
                }
                this.lobbyInlineWizardHold = true;
                this.lobbyCharacterEditorOpen = true;
            },
            /**
             * Passed to inline SetupForm: after /generate-character persists, run the same GET /load as a full refresh
             * so transcript, roster, and localPlayerCharacter stay in sync with Mongo.
             */
            async syncLobbyAfterCharacterPersisted(gameId) {
                const gid = gameId != null ? String(gameId).trim() : '';
                if (!gid) return false;
                return await this.loadGameState(gid, { replaceLocalCharacter: true });
            },
            /** Keep the transcript pinned to the latest line after load, send, or DM push. */
            scrollChatToBottom({ smooth = false } = {}) {
                const run = () => {
                    const el = this.$refs.transcriptRoot;
                    if (!el) return;
                    const top = el.scrollHeight;
                    try {
                        if (typeof el.scrollTo === 'function') {
                            el.scrollTo({ top, behavior: smooth ? 'smooth' : 'auto' });
                        } else {
                            el.scrollTop = top;
                        }
                    } catch (e) {
                        el.scrollTop = top;
                    }
                };
                this.$nextTick(() => {
                    requestAnimationFrame(() => {
                        requestAnimationFrame(run);
                    });
                });
            },
            stopGameStateEventSource() {
                if (this.gameStateEsReconnectTimer) {
                    clearTimeout(this.gameStateEsReconnectTimer);
                    this.gameStateEsReconnectTimer = null;
                }
                if (this.pushSyncTimer) {
                    clearTimeout(this.pushSyncTimer);
                    this.pushSyncTimer = null;
                }
                if (this.gameStateEventSource) {
                    try {
                        this.gameStateEventSource.onerror = null;
                        this.gameStateEventSource.onopen = null;
                        this.gameStateEventSource.close();
                    } catch (e) {
                        /* ignore */
                    }
                    this.gameStateEventSource = null;
                }
                this.gameStateEventSourceGameId = null;
            },

            scheduleGameStateEventSourceReconnect() {
                if (this.gameStateEsReconnectTimer) clearTimeout(this.gameStateEsReconnectTimer);
                const id = this.$route.params.id ? String(this.$route.params.id).trim() : '';
                if (!id || !this.$store.getters.isAuthenticated) return;
                const delay = Math.min(Math.max(this.gameStateEsBackoffMs, 500), 30000);
                this.gameStateEsReconnectTimer = setTimeout(() => {
                    this.gameStateEsReconnectTimer = null;
                    this.gameStateEsBackoffMs = Math.min(this.gameStateEsBackoffMs * 2, 30000);
                    this.maybeStartGameStateEventSource();
                }, delay);
            },

            /**
             * SSE (GET /api/game-state/events/:gameId) when GameState changes — same notify payload as WS hub.
             * Reuse OPEN/CONNECTING source for the same gameId; GET /load runs after push or explicit refresh.
             */
            maybeStartGameStateEventSource() {
                const id = this.$route.params.id ? String(this.$route.params.id).trim() : '';
                const token = this.$store.state.authToken;
                if (!id || !this.$store.getters.isAuthenticated || !token || !String(token).trim()) {
                    this.stopGameStateEventSource();
                    return;
                }
                const existing = this.gameStateEventSource;
                if (
                    existing &&
                    this.gameStateEventSourceGameId === id &&
                    (existing.readyState === EventSource.CONNECTING || existing.readyState === EventSource.OPEN)
                ) {
                    if (this.gameStateEsReconnectTimer) {
                        clearTimeout(this.gameStateEsReconnectTimer);
                        this.gameStateEsReconnectTimer = null;
                    }
                    return;
                }
                if (this.gameStateEsReconnectTimer) {
                    clearTimeout(this.gameStateEsReconnectTimer);
                    this.gameStateEsReconnectTimer = null;
                }
                this.stopGameStateEventSource();
                this.gameStateEventSourceGameId = id;
                const url = resolveGameStateEventsUrl(id, String(token).trim());
                if (!url || typeof EventSource === 'undefined') {
                    this.gameStateEventSourceGameId = null;
                    return;
                }
                let es;
                try {
                    es = new EventSource(url);
                } catch (e) {
                    console.warn('EventSource failed:', e);
                    this.gameStateEventSourceGameId = null;
                    this.scheduleGameStateEventSourceReconnect();
                    return;
                }
                this.gameStateEventSource = es;
                const routeId = id;
                const onMessage = (ev) => {
                    let o;
                    try {
                        o = JSON.parse(ev.data);
                    } catch (err) {
                        return;
                    }
                    if (!o || o.type !== 'game-state-updated') return;
                    if (String(o.gameId || '') !== routeId) return;
                    if (this.syncBusy || this.isSending) return;
                    if (this.pushSyncTimer) clearTimeout(this.pushSyncTimer);
                    this.pushSyncTimer = setTimeout(() => {
                        this.pushSyncTimer = null;
                        this.syncFromServer();
                    }, 350);
                };
                es.addEventListener('message', onMessage);
                es.onopen = () => {
                    this.gameStateEsBackoffMs = 1000;
                    if (this.lobbyStartingPollTimer) {
                        clearInterval(this.lobbyStartingPollTimer);
                        this.lobbyStartingPollTimer = null;
                    }
                    this.stopPendingReplyPoller();
                };
                es.onerror = () => {
                    if (this.gameStateEventSource !== es) {
                        return;
                    }
                    try {
                        es.removeEventListener('message', onMessage);
                        es.close();
                    } catch (e) {
                        /* ignore */
                    }
                    this.gameStateEventSource = null;
                    this.gameStateEventSourceGameId = null;
                    if (this.pushSyncTimer) {
                        clearTimeout(this.pushSyncTimer);
                        this.pushSyncTimer = null;
                    }
                    if (
                        this.$route.params.id &&
                        String(this.$route.params.id).trim() === routeId &&
                        this.$store.getters.isAuthenticated
                    ) {
                        this.scheduleGameStateEventSourceReconnect();
                    }
                };
            },

            stopPendingReplyPoller() {
                if (this.pendingReplyPollTimer) {
                    clearInterval(this.pendingReplyPollTimer);
                    this.pendingReplyPollTimer = null;
                }
                this.pendingReplyPollTicks = 0;
                this.pendingReplyPollSig = null;
            },

            unlockChatInput() {
                this.pendingAssistantUiHold = false;
            },

            flushKeepaliveAppend() {
                const p = this.keepaliveAppendPayload;
                const token = this.$store.state.authToken;
                if (!p || !p.gameId || !token) return;
                try {
                    const base = resolveApiBaseURL() || (typeof window !== 'undefined' ? window.location.origin : '');
                    if (!base) return;
                    const url = `${base.replace(/\/$/, '')}/api/game-state/append-player-message`;
                    fetch(url, {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                            Authorization: `Bearer ${String(token).trim()}`,
                        },
                        body: JSON.stringify({
                            gameId: p.gameId,
                            content: p.content,
                            displayName: p.displayName,
                        }),
                        keepalive: true,
                    }).catch(() => {});
                } catch (e) {
                    /* ignore */
                }
            },

            /**
             * Rare HTTP fallback when SSE is down; with a live EventSource, push drives GET /load (no interval poll).
             */
            maybeStartPendingReplyPoller() {
                if (!this.conversationEndsAwaitingAssistant) {
                    this.stopPendingReplyPoller();
                    return;
                }
                if (this.partyRoundOpenForMyAction) {
                    this.stopPendingReplyPoller();
                    return;
                }
                const pushOpen = this.gameStateEventSource && this.gameStateEventSource.readyState === EventSource.OPEN;
                if (pushOpen) {
                    this.stopPendingReplyPoller();
                    return;
                }
                const partyWait = this.partyRoundWaitingOthers === true;
                const broadcast = this.gameSessionBroadcastActive;
                const sig = `0:${partyWait ? 1 : 0}:${broadcast ? 1 : 0}`;
                /* loadGameState() calls this every sync; restarting always reset ticks and ran sync immediately → tight HTTP loop. */
                if (this.pendingReplyPollTimer && this.pendingReplyPollSig === sig) {
                    return;
                }
                this.stopPendingReplyPoller();
                this.pendingReplyPollSig = sig;
                const maxTicks = broadcast ? 24 : 36;
                const intervalMs = broadcast ? 120000 : 90000;
                const run = async () => {
                    if (this.gameStateEventSource && this.gameStateEventSource.readyState === EventSource.OPEN) {
                        this.stopPendingReplyPoller();
                        return;
                    }
                    if (!this.conversationEndsAwaitingAssistant) {
                        this.stopPendingReplyPoller();
                        return;
                    }
                    if (this.isSending || this.syncBusy) return;
                    this.pendingReplyPollTicks += 1;
                    if (this.pendingReplyPollTicks > maxTicks) {
                        this.pendingAssistantUiHold = false;
                        this.stopPendingReplyPoller();
                        return;
                    }
                    try {
                        await this.syncFromServer();
                    } catch (e) {
                        /* ignore */
                    }
                };
                void run();
                this.pendingReplyPollTimer = setInterval(run, intervalMs);
            },

            /** Resolve visible label for a stored user line: character name from party map, then legacy displayName. */
            displayLabelForPlayerMessage(storedDisplayName, messageUserId) {
                const gs = this.$store.state.gameSetup;
                const mid = messageUserId != null ? String(messageUserId) : '';
                if (mid && gs && gs.playerCharacters && typeof gs.playerCharacters === 'object') {
                    const pc = gs.playerCharacters[mid];
                    if (pc && typeof pc === 'object' && pc.name != null && String(pc.name).trim()) {
                        return String(pc.name).trim();
                    }
                }
                if (mid && this.myUserIdStr && mid === this.myUserIdStr) {
                    const ch = this.playerCharacter;
                    if (ch && ch.name != null && String(ch.name).trim()) {
                        return String(ch.name).trim();
                    }
                }
                if (storedDisplayName != null && String(storedDisplayName).trim()) {
                    return String(storedDisplayName).trim();
                }
                return 'Player';
            },
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

            /** When the DM envelope includes coinage, sync the sheet in Vuex + local copy. */
            applyCoinageFromGenerateResponse(data) {
                if (!data || !data.coinage || typeof data.coinage !== 'object') return;
                const keys = ['pp', 'gp', 'ep', 'sp', 'cp'];
                const cur = {};
                for (const k of keys) {
                    const n = Math.floor(Number(data.coinage[k]));
                    cur[k] = Number.isFinite(n) && n >= 0 ? n : 0;
                }
                const gs = { ...this.$store.state.gameSetup };
                const uid = this.myUserIdStr;
                if (uid) {
                    const pc = { ...(gs.playerCharacters && typeof gs.playerCharacters === 'object' ? gs.playerCharacters : {}) };
                    const sheet = { ...(pc[uid] || {}) };
                    sheet.coinage = cur;
                    pc[uid] = sheet;
                    gs.playerCharacters = pc;
                }
                delete gs.generatedCharacter;
                this.$store.commit('setGameSetup', gs);
                if (this.localPlayerCharacter) {
                    this.localPlayerCharacter = { ...this.localPlayerCharacter, coinage: cur };
                }
            },
            showInviteToast(message, variant = 'success', durationMs = 3200) {
                if (this.inviteToastTimer) {
                    clearTimeout(this.inviteToastTimer);
                    this.inviteToastTimer = null;
                }
                this.inviteToastMessage = message;
                this.inviteToastVariant = variant;
                this.inviteToastVisible = true;
                const ms = typeof durationMs === 'number' && durationMs > 0 ? durationMs : 3200;
                this.inviteToastTimer = setTimeout(() => {
                    this.inviteToastVisible = false;
                    this.inviteToastTimer = null;
                }, ms);
            },
            flushSessionChatToast() {
                try {
                    const raw = sessionStorage.getItem(SESSION_CHAT_TOAST);
                    if (!raw) return;
                    sessionStorage.removeItem(SESSION_CHAT_TOAST);
                    const o = JSON.parse(raw);
                    const msg = o && typeof o.message === 'string' ? o.message.trim() : '';
                    if (!msg) return;
                    this.showInviteToast(msg, o.variant === 'error' ? 'error' : 'success');
                } catch (e) {
                    /* ignore */
                }
            },
            async copyInviteLink() {
                const gid = this.$route.params.id ? String(this.$route.params.id).trim() : '';
                if (this.inviteLoading) return;
                if (!gid) {
                    this.showInviteToast(this.$i18n.invite_no_game_id, 'error');
                    return;
                }
                this.inviteLoading = true;
                try {
                    const { data } = await axios.post('/api/game-session/create-invite', { gameId: gid });
                    const token = String((data && data.inviteToken) || '').trim();
                    if (!token) throw new Error('no token');
                    const url = absoluteInviteUrl(this.$router, token);
                    try {
                        await navigator.clipboard.writeText(url);
                    } catch (e) {
                        window.prompt(this.$i18n.invite_copy_prompt_title || 'Copy invite link:', url);
                    }
                    this.showInviteToast(this.$i18n.invite_copied, 'success');
                } catch (e) {
                    console.warn('create-invite failed:', e?.response?.data || e?.message || e);
                    this.showInviteToast(this.$i18n.invite_failed, 'error');
                } finally {
                    this.inviteLoading = false;
                }
            },

            /**
             * Pull latest conversation from DB (other players / other tabs).
             * SSE (`/api/game-state/events/:gameId`) signals updates; this GET applies the snapshot.
             */
            async syncFromServer() {
                const id = this.$route.params.id ? String(this.$route.params.id).trim() : '';
                if (!id || this.syncBusy) return;
                this.syncBusy = true;
                this.errorMessage = null;
                try {
                    const ok = await this.loadGameState(id, { replaceLocalCharacter: true });
                    if (!ok && this.$store.getters.isAuthenticated) {
                        this.errorMessage = this.$i18n.state_refresh_failed;
                    }
                } finally {
                    this.syncBusy = false;
                }
            },

            onLobbyInlineWizardHold(val) {
                this.lobbyInlineWizardHold = Boolean(val);
            },

            onLobbyInlineConfirmSheet(sheet) {
                this.lobbyInlineConfirmSheet = sheet && typeof sheet === 'object' ? sheet : null;
            },

            /** Normal party lobby: after generate/regenerate, leave “edit character” so embedded sheet matches refresh. */
            onLobbyInlineAfterCharacterSaved() {
                this.lobbyCharacterEditorOpen = false;
            },

            logPartyLobbyDebug(reason) {
                if (!this.partyLobbyDebugEnabled) return;
                const payload = this.partyLobbyDebugPayload;
                if (!payload) return;
                const line = { ...payload, reason };
                try {
                    console.info('[GameMasterAI party-lobby]', line);
                } catch (e) {
                    /* ignore */
                }
            },

            /** Roster “Ready to start” for your row: same commit+ready flow as SetupForm (or POST party-ready if wizard unmounted). */
            async confirmReadyFromRosterCell() {
                if (this.partyLobbyDebugEnabled) {
                    console.info('[GameMasterAI party-lobby] confirmReadyFromRosterCell click', {
                        isPartyLobby: this.isPartyLobby,
                        partyPhase: this.partyPhase,
                        rosterOwnRowFrozen: this.partyLobbyRosterOwnRowFrozen,
                        posting: this.lobbyRosterConfirmPosting,
                        uid: this.myUserIdStr,
                        memberHasCommittedSheet: this.myUserIdStr
                            ? this.memberHasCommittedSheet(this.myUserIdStr)
                            : false,
                    });
                }
                if (!this.isPartyLobby) {
                    if (this.partyLobbyDebugEnabled) {
                        console.warn('[GameMasterAI party-lobby] confirm blocked: not party lobby UI', {
                            partyPhase: this.partyPhase,
                        });
                    }
                    return;
                }
                if (this.partyLobbyRosterOwnRowFrozen) {
                    if (this.partyLobbyDebugEnabled) {
                        console.warn('[GameMasterAI party-lobby] confirm blocked: roster row frozen', {
                            partyLobbyAllMembersReady: this.partyLobbyAllMembersReady,
                            readyUserIds: (this.$store.state.gameSetup &&
                                this.$store.state.gameSetup.party &&
                                this.$store.state.gameSetup.party.readyUserIds) || [],
                        });
                    }
                    return;
                }
                if (this.lobbyRosterConfirmPosting) {
                    if (this.partyLobbyDebugEnabled) {
                        console.warn('[GameMasterAI party-lobby] confirm blocked: already posting');
                    }
                    return;
                }
                const uid = this.myUserIdStr;
                if (!uid || !this.memberHasCommittedSheet(uid)) {
                    if (this.partyLobbyDebugEnabled) {
                        console.warn('[GameMasterAI party-lobby] confirm blocked: no uid or sheet not committed', {
                            uid: uid || '',
                            memberHasCommittedSheet: uid ? this.memberHasCommittedSheet(uid) : false,
                            viewerHasLobbyReadySheet: this.viewerHasLobbyReadySheet,
                        });
                    }
                    return;
                }
                this.lobbyRosterConfirmPosting = true;
                try {
                    const setup = this.$refs.lobbyInlineSetup;
                    if (setup && typeof setup.runLobbyRosterConfirmAndReady === 'function') {
                        const handled = await setup.runLobbyRosterConfirmAndReady();
                        if (handled) return;
                    }
                    await this.postPartyReadyFromLobbyRoster();
                } finally {
                    this.lobbyRosterConfirmPosting = false;
                }
            },

            async postPartyReadyFromLobbyRoster() {
                const uid = this.myUserIdStr;
                if (!uid || !this.memberHasCommittedSheet(uid)) return;
                const gid0 =
                    (this.$store.state.gameId && String(this.$store.state.gameId).trim()) ||
                    (this.$route.params.id ? String(this.$route.params.id).trim() : '');
                if (!gid0) {
                    this.showInviteToast(this.$i18n.join_missing_game_id, 'error');
                    return;
                }
                try {
                    const postPartyReady = () =>
                        axios.post('/api/game-state/party-ready', { gameId: gid0, ready: true });
                    let readyRes;
                    try {
                        readyRes = await postPartyReady();
                    } catch (eFirst) {
                        const c0 = eFirst.response && eFirst.response.data && eFirst.response.data.code;
                        if (c0 === 'PARTY_READY_NEEDS_CHARACTER') {
                            await new Promise((r) => setTimeout(r, 450));
                            await this.loadGameState(gid0, { replaceLocalCharacter: true });
                            readyRes = await postPartyReady();
                        } else {
                            throw eFirst;
                        }
                    }
                    const { data } = readyRes;
                    if (data && data.gameSetup) {
                        this.$store.commit('setGameSetup', data.gameSetup);
                    }
                    const meta = data && data.partyReadyMeta;
                    let partyStartHardFail = false;
                    if (meta && meta.allMembersHaveSheets && meta.allMembersReady) {
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
                                /* benign race */
                            } else {
                                partyStartHardFail = true;
                                const detail =
                                    startErr.response && startErr.response.data && startErr.response.data.error;
                                const errCode =
                                    startErr.response && startErr.response.data && startErr.response.data.code;
                                const suffix =
                                    errCode && String(errCode).trim() ? ` (${String(errCode).trim()})` : '';
                                this.showInviteToast(
                                    (detail || startErr.message || this.$i18n.error_saving_game) + suffix,
                                    'error'
                                );
                            }
                        }
                    }
                    if (partyStartHardFail) {
                        return;
                    }
                    const donePayload = { gameId: gid0 };
                    if (data && data.ownerUserId != null && String(data.ownerUserId).trim() !== '') {
                        donePayload.ownerUserId = String(data.ownerUserId).trim();
                        donePayload.viewerIsGameOwner = donePayload.ownerUserId === String(uid).trim();
                    }
                    this.onLobbyInlineCharacterDone(donePayload);
                } catch (e) {
                    const code = e.response && e.response.data && e.response.data.code;
                    const msg =
                        (e.response && e.response.data && e.response.data.error) ||
                        e.message ||
                        this.$i18n.error_saving_game;
                    if (code === 'PARTY_READY_NEEDS_CHARACTER') {
                        this.showInviteToast(this.$i18n.party_ready_needs_character, 'error');
                    } else {
                        this.showInviteToast(msg, 'error');
                    }
                }
            },

            onLobbyInlineCharacterDone(payload) {
                const p = payload && typeof payload === 'object' ? payload : {};
                const gid =
                    this.$store.state.gameId ||
                    (this.$route.params.id ? String(this.$route.params.id) : '') ||
                    (p.gameId != null ? String(p.gameId) : '');
                this.lobbyInlineWizardHold = false;
                this.lobbyInlineConfirmSheet = null;
                if (gid) {
                    try {
                        sessionStorage.setItem(`dm_lobby_char_done_${gid}`, '1');
                    } catch (e) {
                        /* ignore */
                    }
                }
                if (p.ownerUserId != null && String(p.ownerUserId).trim() !== '') {
                    this.gameOwnerUserId = String(p.ownerUserId).trim();
                }
                if (Object.prototype.hasOwnProperty.call(p, 'viewerIsGameOwner')) {
                    this.viewerIsGameOwner = p.viewerIsGameOwner === true;
                }
                this.lobbyCharacterEditorOpen = false;
                void this.syncFromServer();
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
                    requestingUserId: this.myUserIdStr || undefined,
                };
            },

            /** Apply a /generate-shaped payload (or partyDm from append flush) to local chat state. */
            applyDmPayloadToConversation(responseData, userMessage) {
                if (responseData && Object.prototype.hasOwnProperty.call(responseData, 'encounterState')) {
                    this.lastEncounterState = responseData.encounterState || null;
                }
                this.applyCoinageFromGenerateResponse(responseData);
                const aiMessageContent = this.narrationFromGenerateResponse(responseData);
                if (!aiMessageContent) {
                    throw new Error(responseData?.error || 'Empty narration from server');
                }
                this.incrementTokenCount(aiMessageContent);
                const aiMessage = {
                    role: 'assistant',
                    content: aiMessageContent,
                };
                this.conversation.push(aiMessage);
                this.summaryConversation.push(aiMessage);
                this.messages.push({ user: this.$i18n.dm_label, text: aiMessageContent, role: 'dm' });
                if (userMessage.role === 'user' || aiMessage.role === 'assistant') {
                    this.userAndAssistantMessageCount++;
                }
                this.pendingAssistantUiHold = false;
                this.keepaliveAppendPayload = null;
                this.stopPendingReplyPoller();
                this.partyRoundWaitingOthers = false;
            },

            async submitMessage() {
                if (this.newMessage.trim() === '') return;
                this.isSending = true;
                this.partyRoundWaitingOthers = false;
                const text = this.newMessage.trim();
                this.messages.push({ user: this.playerChatLabel, text, role: 'player' });
                const userMessage = {
                    role: 'user',
                    content: text,
                    userId: this.myUserIdStr || undefined,
                    displayName: this.playerChatLabel,
                };
                this.conversation.push(userMessage);
                this.summaryConversation.push(userMessage);

                try {
                    this.errorMessage = null;
                    const gidSend = this.$store.state.gameId;
                    this.keepaliveAppendPayload = gidSend
                        ? {
                              gameId: gidSend,
                              content: userMessage.content,
                              displayName: userMessage.displayName,
                          }
                        : null;

                    if (gidSend) {
                        try {
                            const { data: ad } = await axios.post('/api/game-state/append-player-message', {
                                gameId: gidSend,
                                content: userMessage.content,
                                displayName: userMessage.displayName,
                            });
                            this.keepaliveAppendPayload = null;

                            if (ad && ad.partyWait === true) {
                                this.partyRoundWaitingOthers = true;
                                if (this.gameSessionBroadcastActive) this.$nextTick(() => this.syncFromServer());
                                return;
                            }

                            if (ad && ad.partyDm && String(ad.partyDm.narration || '').trim()) {
                                this.applyDmPayloadToConversation(ad.partyDm, userMessage);
                                return;
                            }

                            if (ad && ad.duplicate === true) {
                                if (this.gameSessionBroadcastActive) this.$nextTick(() => this.syncFromServer());
                                return;
                            }
                        } catch (appendErr) {
                            console.warn(
                                'append-player-message failed (message may disappear on refresh until DM reply):',
                                appendErr
                            );
                        }
                    }

                    if (!this.gameSessionBroadcastActive) {
                        const lastMessages = this.conversation.slice(-this.ContextLength * 2);
                        lastMessages.forEach((message) => this.incrementTokenCount(message.content));
                        const response = await axios.post('/api/game-session/generate', {
                            messages: lastMessages.slice(),
                            language: this.language,
                            gameId: this.$store.state.gameId,
                            requestingUserId: this.myUserIdStr || undefined,
                            persist: this.buildPersistPayload(),
                        });
                        this.applyDmPayloadToConversation(response.data, userMessage);
                    } else {
                        this.$nextTick(() => this.syncFromServer());
                    }
                } catch (error) {
                    console.error('Error generating AI message:', error);
                    const apiErr = error.response?.data?.error;
                    this.errorMessage = apiErr || error.message || 'Failed to send message. Please try again.';
                    if (this.gameSessionBroadcastActive) {
                        this.$nextTick(() => this.syncFromServer());
                    } else {
                        this.maybeStartPendingReplyPoller();
                    }
                } finally {
                    this.isSending = false;
                    this.newMessage = '';
                }
            },
            
            // Generate an initial AI message using the current conversation (used when entering a new game)
            async generateInitialMessage() {
                this.isInitialOpenBusy = true;
                try {
                    const messagesToSend = this.conversation.slice(-this.ContextLength * 2);
                    // Initial opening: server uses persisted campaignSpec + GameState character; do not send PC in sessionSummary (world/PC separation).
                    const sessionSummary = '';

                        const response = await axios.post('/api/game-session/generate', {
                        gameId: this.$store.state.gameId,
                        messages: messagesToSend,
                        mode: 'initial',
                        includeFullSkill: true,
                        language: this.language,
                        sessionSummary,
                        requestingUserId: this.myUserIdStr || undefined,
                        persist: this.buildPersistPayload(),
                    });
                    if (response.data && Object.prototype.hasOwnProperty.call(response.data, 'encounterState')) {
                        this.lastEncounterState = response.data.encounterState || null;
                    }
                    this.applyCoinageFromGenerateResponse(response.data);
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
                    this.messages.push({ user: this.$i18n.dm_label, text: aiMessageContent, role: 'dm' });
                    this.pendingAssistantUiHold = false;
                } catch (err) {
                    console.error('Error generating initial AI message:', err);
                    const apiErr = err.response?.data?.error;
                    const preview = err.response?.data?.rawPreview;
                    this.errorMessage =
                        apiErr ||
                        (preview ? `${err.message || 'Request failed'}. ${String(preview).slice(0, 280)}` : null) ||
                        err.message ||
                        'Could not start the adventure.';
                } finally {
                    this.isInitialOpenBusy = false;
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

            async loadGameState(gameId, opts = {}) {
                const gidRaw = gameId != null ? String(gameId).trim() : '';
                if (!gidRaw) return false;
                const replaceLocalCharacter = opts.replaceLocalCharacter === true;
                let response;
                try {
                    response = await fetchGameStateLoad(gidRaw);
                } catch (error) {
                    const st = error && error.response && error.response.status;
                    if (st === 401 && !this.$store.getters.isAuthenticated) {
                        return false;
                    }
                    if (
                        st === 404 &&
                        this.$route.params.id &&
                        String(this.$route.params.id).trim() === gidRaw
                    ) {
                        this.stopGameStateEventSource();
                        try {
                            sessionStorage.setItem(
                                SESSION_CHAT_TOAST,
                                JSON.stringify({
                                    message: this.$i18n.game_load_not_found,
                                    variant: 'error',
                                })
                            );
                        } catch (e) {
                            /* ignore */
                        }
                        this.$store.commit('setGameId', null);
                        try {
                            await this.$router.replace({ name: 'LoadGame' });
                        } catch (e) {
                            /* ignore */
                        }
                        return false;
                    }
                    console.error('Error loading game state:', error);
                    return false;
                }
                const gameState = response.data;

                try {

                    // Restore the game state
                    this.$store.commit('setGameId', gameState.gameId);
                    this.$store.commit('setGameSetup', gameState.gameSetup);
                    this.memberNicknamesByUserId =
                        gameState.memberNicknamesByUserId &&
                        typeof gameState.memberNicknamesByUserId === 'object'
                            ? { ...gameState.memberNicknamesByUserId }
                            : {};
                    // set local language from loaded game setup
                    this.language = (gameState.gameSetup && gameState.gameSetup.language) || this.language;
                    this.gameOwnerUserId = gameState.ownerUserId != null ? String(gameState.ownerUserId) : null;
                    this.viewerIsGameOwner = Object.prototype.hasOwnProperty.call(gameState, 'viewerIsGameOwner')
                        ? gameState.viewerIsGameOwner === true
                        : null;
                    this.memberUserIds = Array.isArray(gameState.memberUserIds)
                        ? gameState.memberUserIds.map((x) => String(x))
                        : [];
                    this.serverLlmStartedAt =
                        gameState.llmCallStartedAt != null ? String(gameState.llmCallStartedAt) : null;
                    this.serverLlmCompletedAt =
                        gameState.llmCallCompletedAt != null ? String(gameState.llmCallCompletedAt) : null;
                    this.conversation = Array.isArray(gameState.conversation) ? gameState.conversation : [];
                    this.summaryConversation = gameState.summaryConversation;
                    this.summary = gameState.summary;
                    this.totalTokenCount = gameState.totalTokenCount;
                    this.userAndAssistantMessageCount = gameState.userAndAssistantMessageCount;
                    this.systemMessageContentDM = gameState.systemMessageContentDM;
                    this.lastEncounterState = gameState.encounterState || null;
                    const spec = gameState.campaignSpec;
                    this.campaignTitle =
                        spec && typeof spec.title === 'string' && spec.title.trim() ? spec.title.trim() : '';

                    const uid = this.myUserIdStr;
                    const sheetForMe = uid ? this.pickPlayerCharacterFromStore(uid) : null;
                    if (replaceLocalCharacter) {
                        if (sheetForMe && typeof sheetForMe === 'object') {
                            try {
                                this.localPlayerCharacter = JSON.parse(JSON.stringify(sheetForMe));
                            } catch (e) {
                                this.localPlayerCharacter = { ...sheetForMe };
                            }
                        } else {
                            this.localPlayerCharacter = null;
                        }
                    } else if (sheetForMe && typeof sheetForMe === 'object' && !this.localPlayerCharacter) {
                        this.localPlayerCharacter = sheetForMe;
                    }

                    // Map the conversation array to match the structure needed by this.messages
                    // And only include the messages that are not of role 'system'
                    this.messages = this.conversation
                        .filter(({ role }) => role !== 'system')  // Filter out the 'system' role messages
                        .map(({ role, content, displayName, userId }) => {
                            const isAsst = role === 'assistant';
                            const user = isAsst
                                ? this.$i18n.dm_label
                                : this.displayLabelForPlayerMessage(displayName, userId);
                            return {
                                user,
                                text: content,
                                role: isAsst ? 'dm' : 'player',
                            };
                        });

                    const lastLoaded = this.conversation.length ? this.conversation[this.conversation.length - 1] : null;
                    if (lastLoaded && lastLoaded.role === 'assistant') {
                        this.partyRoundWaitingOthers = false;
                    }

                    if (!this.conversationEndsAwaitingAssistant) {
                        this.pendingAssistantUiHold = false;
                    } else {
                        this.pendingAssistantUiHold = !this.errorMessage;
                    }

                    const ownerStr = this.gameOwnerUserId || '';
                    const listed = this.memberUserIds || [];
                    const multi =
                        listed.length >= 2 ||
                        (ownerStr && uid && String(ownerStr) !== String(uid));
                    const gs = this.$store.state.gameSetup;
                    const pcMap =
                        gs && gs.playerCharacters && typeof gs.playerCharacters === 'object'
                            ? gs.playerCharacters
                            : {};
                    const partyPh = gs && gs.party && gs.party.phase ? String(gs.party.phase) : '';
                    const inPartyLobby = partyPh === 'lobby' || partyPh === 'starting';
                    if (multi && uid && !pcMap[uid] && !inPartyLobby) {
                        await this.$router.replace({ path: '/setup', query: { joinGame: gidRaw } });
                        return true;
                    }

                    // Opening narration: solo bootstrap only (party lobby uses server start-party-adventure).
                    const hasOnlySystem = this.conversation.length === 1 && this.conversation[0]?.role === 'system';
                    if (this.messages.length === 0 && hasOnlySystem && !inPartyLobby) {
                        this.generateInitialMessage();
                    }

                    this.$nextTick(() => {
                        this.maybeStartGameStateEventSource();
                        this.maybeStartPendingReplyPoller();
                        this.scrollChatToBottom({ smooth: false });
                    });
                    return true;
                } catch (err) {
                    console.error('Error applying game state:', err);
                    return false;
                }
            }

        }
    };</script>

<style scoped>
  /* Match app column width (--gm-app-column-max); avoid 640/760 caps that made party vs transcript differ. */
  .chat-room {
    width: 100%;
    max-width: 100%;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    min-height: 0;
    box-sizing: border-box;
  }

  .chat-room__transcript {
    margin-bottom: 0.35rem;
  }

  .two-column {
    display: flex;
    flex-direction: column;
    gap: 14px;
    align-items: stretch;
    width: 100%;
    max-width: 100%;
    margin: 0 auto;
    box-sizing: border-box;
  }

  .chat-room__header {
    display: flex;
    flex-direction: column;
    gap: 10px;
    align-items: stretch;
    width: 100%;
    box-sizing: border-box;
  }

  .chat-room__dm-wait {
    font-size: 0.9rem;
    opacity: 0.88;
    margin: 0.25rem 0 0.15rem;
    line-height: 1.35;
  }

  .chat-room__unlock {
    align-self: flex-start;
    margin-bottom: 0.35rem;
    font-size: 0.85rem;
  }

  .party-roster {
    width: 100%;
    box-sizing: border-box;
    font-size: 0.84rem;
    line-height: 1.4;
    color: var(--gm-muted, #9a8f85);
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(212, 180, 106, 0.15);
    background: rgba(0, 0, 0, 0.22);
  }

  .party-roster__head {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 6px;
  }

  .party-roster__title {
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    font-size: 0.72rem;
    color: rgba(230, 225, 216, 0.75);
  }

  .party-roster__count {
    font-variant-numeric: tabular-nums;
    opacity: 0.85;
  }

  .party-roster__list {
    margin: 0;
    padding-left: 1.1rem;
    color: var(--gm-text, #e8e4dc);
  }

  .party-roster__item {
    margin: 4px 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    align-items: flex-start;
  }

  .party-roster__name {
    display: inline;
  }

  .party-roster__meta {
    display: block;
    font-size: 0.78rem;
    line-height: 1.3;
    opacity: 0.82;
    font-weight: 400;
    color: var(--gm-muted, #9a8f85);
  }

  .party-roster__item--you .party-roster__meta {
    color: rgba(212, 180, 106, 0.75);
  }

  .party-roster__item--you {
    color: rgba(212, 180, 106, 0.95);
  }

  .party-roster__tip {
    margin: 8px 0 0;
    font-size: 0.78rem;
    opacity: 0.88;
    line-height: 1.35;
  }

  .campaign-heading-row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    flex-wrap: nowrap;
    min-width: 0;
    width: 100%;
    max-width: 98%;
    margin-inline: auto;
    box-sizing: border-box;
    padding-bottom: 2px;
    border-bottom: 1px solid rgba(212, 180, 106, 0.2);
  }

  .campaign-heading-row .campaign-heading {
    flex: 1 1 auto;
    min-width: 0;
  }

  .campaign-heading-row .chat-room__invite-tool {
    margin-left: auto;
    flex-shrink: 0;
  }

  .campaign-heading-row--no-title .chat-room__invite-tool {
    margin-left: 0;
  }

  .chat-room__invite-tool {
    width: 36px;
    height: 36px;
    border-radius: 9px;
  }

  .chat-room__invite-tool:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .campaign-heading {
    margin: 0;
    font-family: var(--gm-font-serif, Georgia, serif);
    font-size: clamp(1.2rem, 2.8vw, 1.45rem);
    font-weight: 600;
    letter-spacing: 0.02em;
    color: var(--gm-text, #e8e4dc);
    text-shadow: 0 2px 14px rgba(0, 0, 0, 0.45);
    line-height: 1.25;
  }

  .chat-toolbar__secondary {
    flex: 0 0 auto;
  }

  /* Chat input form: wider text box and aligned button */
  .chat-input-form {
    display: flex;
    gap: 12px;
    align-items: stretch;
    width: 100%;
    margin-top: 6px;
    padding-top: 4px;
  }

  .chat-input {
    flex: 1 1 auto;
    min-height: 48px;
    padding: 0.75rem 1rem;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(0, 0, 0, 0.35);
    color: var(--gm-text);
    box-sizing: border-box;
    font-family: var(--gm-font-sans);
    font-size: 1rem;
    outline: none;
    transition: border-color 0.15s ease, box-shadow 0.15s ease;
    box-shadow: inset 0 2px 8px rgba(0, 0, 0, 0.35);
    position: relative;
    z-index: 6;
  }

  .chat-input:hover {
    border-color: rgba(212, 180, 106, 0.22);
  }

  .chat-input:focus {
    border-color: rgba(212, 180, 106, 0.42);
    box-shadow:
      inset 0 2px 8px rgba(0, 0, 0, 0.35),
      0 0 0 3px rgba(212, 180, 106, 0.12);
  }

  .chat-input::placeholder {
    color: rgba(230, 225, 216, 0.55);
    opacity: 1;
  }

  .chat-send-button {
    flex: 0 0 auto;
    min-height: 48px;
    padding: 0 1.15rem;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 12px;
  }

  /* ensure form sits above any background overlays */
  .chat-input-form {
    position: relative;
    z-index: 6;
  }

  .error-message {
    color: red;
    margin: 1rem 0;
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

  .party-lobby {
    --party-lobby-gutter: 12px;
    /* Roster CTAs + status chips: one width (matches primary “Ready to play” / ES “Listo para jugar”). */
    --party-lobby-roster-cta-width: 12rem;
    /* Match .app-layout-root / .app-banner column (theme.css). */
    --party-lobby-max: 100%;
    width: 100%;
    max-width: var(--party-lobby-max);
    margin: 0 auto;
    padding: 0.25rem var(--party-lobby-gutter) 1.75rem;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
    align-items: stretch;
    min-height: 0;
  }

  .party-lobby__hero {
    width: 100%;
    box-sizing: border-box;
    text-align: left;
    padding-bottom: 0.35rem;
    border-bottom: 1px solid rgba(212, 180, 106, 0.18);
    box-shadow: 0 1px 0 rgba(0, 0, 0, 0.35);
  }

  .party-lobby__hero-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }

  .party-lobby__hero-text {
    min-width: 0;
    flex: 1;
  }

  .party-lobby__eyebrow {
    margin: 0 0 0.2rem;
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--gm-gold, #d4b46a);
    opacity: 0.92;
  }

  .party-lobby__title {
    margin: 0 0 0.45rem;
    font-family: var(--gm-font-serif, Georgia, serif);
    font-size: clamp(1.35rem, 3.5vw, 1.75rem);
    font-weight: 600;
    color: var(--gm-gold-bright, #f0dfa8);
    line-height: 1.2;
    text-shadow: 0 1px 2px rgba(0, 0, 0, 0.45);
  }

  .party-lobby__desc {
    margin: 0;
    font-size: 0.94rem;
    line-height: 1.5;
    color: var(--gm-text, #e6e1d8);
    opacity: 0.94;
    max-width: 42rem;
  }

  .party-lobby__invite {
    flex-shrink: 0;
    margin-top: 0.15rem;
  }

  .party-lobby__err {
    width: 100%;
    box-sizing: border-box;
    margin: 0;
    padding: 10px 12px;
    border-radius: 8px;
    background: rgba(180, 60, 50, 0.2);
    border: 1px solid rgba(255, 140, 120, 0.35);
    color: #ffc9c0;
    font-size: 0.92rem;
  }

  .party-lobby__debug {
    width: 100%;
    box-sizing: border-box;
    margin: 0.5rem 0 0;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px dashed rgba(120, 200, 255, 0.45);
    background: rgba(0, 40, 60, 0.35);
    color: #c8e8ff;
    font-size: 0.78rem;
    line-height: 1.35;
  }

  .party-lobby__debug-summary {
    cursor: pointer;
    font-weight: 600;
    letter-spacing: 0.04em;
  }

  .party-lobby__debug-hint {
    margin: 0.35rem 0 0.5rem;
    opacity: 0.9;
  }

  .party-lobby__debug-hint code {
    font-size: 0.72rem;
    word-break: break-all;
  }

  .party-lobby__debug-pre {
    margin: 0;
    max-height: 42vh;
    overflow: auto;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.7rem;
    white-space: pre-wrap;
    word-break: break-word;
    color: #e4f4ff;
  }

  .party-lobby__roster {
    width: 100%;
    box-sizing: border-box;
    margin: 0;
    padding: 1rem var(--party-lobby-gutter, 12px) 1.1rem;
    border-radius: 16px;
    border: 1px solid rgba(212, 180, 106, 0.28);
    background: linear-gradient(180deg, rgba(255, 248, 235, 0.08) 0%, rgba(24, 20, 18, 0.58) 100%);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.05),
      0 8px 28px rgba(0, 0, 0, 0.35);
  }

  .party-lobby__roster-title {
    margin: 0 0 0.65rem;
    font-family: var(--gm-font-serif, Georgia, serif);
    font-size: 1rem;
    font-weight: 600;
    color: var(--gm-text, #e6e1d8);
    letter-spacing: 0.02em;
  }

  .party-lobby__list {
    list-style: none;
    margin: 0;
    width: 100%;
    box-sizing: border-box;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    background: transparent;
    border: none;
    box-shadow: none;
  }

  /* Avatar | player | ready (single status column) */
  .party-lobby__item {
    display: grid;
    grid-template-columns: 44px minmax(0, 1fr) minmax(10.5rem, 1.25fr);
    column-gap: 10px;
    row-gap: 6px;
    align-items: center;
    padding: 10px 6px;
    border-radius: 10px;
    background: rgba(0, 0, 0, 0.2);
    border: 1px solid rgba(255, 255, 255, 0.05);
  }

  .party-lobby__ready-cell {
    min-width: 0;
    display: flex;
    justify-content: flex-end;
    align-items: center;
  }

  .party-lobby__ready-actions {
    display: flex;
    flex-direction: row;
    flex-wrap: wrap;
    align-items: stretch;
    justify-content: flex-end;
    gap: 0.35rem 0.45rem;
    width: auto;
    max-width: 100%;
    margin-left: auto;
    box-sizing: border-box;
  }

  .party-lobby__roster-ready-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--party-lobby-roster-cta-width);
    min-width: var(--party-lobby-roster-cta-width);
    max-width: var(--party-lobby-roster-cta-width);
    flex: 0 0 var(--party-lobby-roster-cta-width);
    min-height: 2.2rem;
    box-sizing: border-box;
    font-size: 0.68rem;
    font-weight: 600;
    line-height: 1.2;
    padding: 0.32rem 0.42rem;
    gap: 0.3rem;
  }

  .party-lobby__roster-regenerate-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--party-lobby-roster-cta-width);
    min-width: var(--party-lobby-roster-cta-width);
    max-width: var(--party-lobby-roster-cta-width);
    flex: 0 0 var(--party-lobby-roster-cta-width);
    min-height: 2.2rem;
    box-sizing: border-box;
    font-size: 0.66rem;
    font-weight: 600;
    line-height: 1.2;
    padding: 0.32rem 0.4rem;
    background: rgba(255, 255, 255, 0.06) !important;
    color: var(--gm-muted, #c4bbb0) !important;
    border: 1px solid rgba(212, 180, 106, 0.28) !important;
    box-shadow: none !important;
  }

  .party-lobby__roster-regenerate-btn:hover {
    color: var(--gm-text, #e6e1d8) !important;
    border-color: rgba(212, 180, 106, 0.45) !important;
    background: rgba(255, 255, 255, 0.09) !important;
  }

  .party-lobby__roster-ready-btn__inner,
  .party-lobby__roster-ready-btn__busy {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    text-align: center;
    flex-wrap: wrap;
    max-width: 100%;
    box-sizing: border-box;
  }

  .party-lobby__roster-ready-btn--blocked {
    cursor: not-allowed;
    background: rgba(255, 255, 255, 0.06) !important;
    color: var(--gm-muted, #9a8f85) !important;
    border: 1px solid rgba(255, 255, 255, 0.1) !important;
    box-shadow: none !important;
    transform: none !important;
  }

  .party-lobby__roster-ready-btn--blocked:hover,
  .party-lobby__roster-ready-btn--blocked:active {
    transform: none !important;
    box-shadow: none !important;
  }

  .party-lobby__avatar {
    width: 44px;
    height: 44px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: var(--gm-font-serif, Georgia, serif);
    font-weight: 700;
    font-size: 1rem;
    color: var(--gm-gold-bright, #f0dfa8);
    background: radial-gradient(circle at 30% 25%, rgba(240, 223, 168, 0.2), rgba(40, 28, 22, 0.95));
    border: 2px solid rgba(212, 180, 106, 0.35);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.4);
  }

  .party-lobby__avatar--two {
    font-size: 0.78rem;
    letter-spacing: 0.04em;
  }

  .party-lobby__identity {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .party-lobby__name {
    font-weight: 600;
    font-size: 0.95rem;
    line-height: 1.25;
    color: var(--gm-text, #e6e1d8);
  }

  .party-lobby__meta {
    font-size: 0.78rem;
    line-height: 1.35;
    opacity: 0.85;
    color: var(--gm-muted, #9a8f85);
  }

  .party-lobby__badge {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    padding: 0.4rem 0.35rem;
    border-radius: 8px;
    font-size: 0.72rem;
    font-weight: 600;
    line-height: 1.2;
    text-align: center;
    border: 1px solid transparent;
    min-height: 2.2rem;
    width: var(--party-lobby-roster-cta-width);
    min-width: var(--party-lobby-roster-cta-width);
    max-width: var(--party-lobby-roster-cta-width);
    flex: 0 0 var(--party-lobby-roster-cta-width);
    box-sizing: border-box;
  }

  .party-lobby__badge-icon {
    width: 0.85rem;
    height: 0.85rem;
    flex-shrink: 0;
    opacity: 0.95;
  }

  .party-lobby__badge--ok {
    background: rgba(60, 110, 70, 0.35);
    border-color: rgba(143, 217, 155, 0.35);
    color: #c8f0ce;
  }

  .party-lobby__badge--wait {
    background: rgba(255, 255, 255, 0.04);
    border-color: rgba(255, 255, 255, 0.08);
    color: var(--gm-muted, #9a8f85);
  }

  .party-lobby__warn {
    width: 100%;
    box-sizing: border-box;
    margin: 0;
    font-size: 0.85rem;
    opacity: 0.9;
    color: #f0c674;
  }

  /* Match .app-banner content inset: lobby root uses --party-lobby-gutter; banner uses --gm-banner-padding-x. */
  .party-lobby__character-sheet {
    width: 100%;
    max-width: 100%;
    min-width: 0;
    box-sizing: border-box;
    margin-top: 0.5rem;
    padding-inline: max(0px, calc(var(--gm-banner-padding-x, 1.35rem) - var(--party-lobby-gutter, 12px)));
    overflow-x: hidden;
  }
  .party-lobby__character-sheet :deep(.floating-card--embedded),
  .party-lobby__character-sheet :deep(.floating-card--embedded .floating-panel.char-sheet-panel) {
    max-width: 100%;
    box-sizing: border-box;
  }
  /* Cancel parchment/tab negative margins that bleed past the aligned column */
  .party-lobby__character-sheet :deep(.char-sheet-hero) {
    margin-left: 0;
    margin-right: 0;
  }
  .party-lobby__character-sheet :deep(.char-sheet-tablist-wrap) {
    margin-left: 0;
    margin-right: 0;
  }

  .party-lobby__setup {
    width: 100%;
    box-sizing: border-box;
    margin-top: 0;
    padding: 1rem var(--party-lobby-gutter, 12px) 1.15rem;
    border-radius: 16px;
    border: 1px solid rgba(212, 180, 106, 0.28);
    background: linear-gradient(165deg, rgba(18, 14, 12, 0.92), rgba(28, 22, 18, 0.88));
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.45), inset 0 1px 0 rgba(255, 255, 255, 0.04);
    max-height: min(78vh, 720px);
    overflow: auto;
  }

  /* Embedded confirm sheet only in this section: slightly shorter than default min(70vh,640px) so
     header + parchment + actions fit the scroll card. Do NOT use container-type:size on .party-lobby__setup —
     size containment makes auto height ignore children and collapses the whole block. */
  .party-lobby__setup :deep(.floating-card--embedded .floating-panel.char-sheet-panel) {
    height: min(58vh, 560px);
    max-height: min(58vh, 560px);
    max-width: 100%;
    box-sizing: border-box;
  }
  .party-lobby__setup :deep(.char-sheet-hero),
  .party-lobby__setup :deep(.char-sheet-tablist-wrap) {
    margin-left: 0;
    margin-right: 0;
  }

  /* Inline character form: full lobby width (all setup phases; beats SetupForm .setup-page 520px). */
  .party-lobby__setup :deep(.setup-page.setup-page--lobby-inline) {
    max-width: 100%;
  }

  @media (max-width: 560px) {
    .party-lobby__item {
      grid-template-columns: 44px 1fr;
      grid-template-rows: auto auto;
    }
    .party-lobby__identity {
      grid-column: 2;
      grid-row: 1;
    }
    .party-lobby__ready-cell {
      grid-column: 1 / -1;
      grid-row: 2;
    }
  }

  .invite-toast {
    position: fixed;
    z-index: 10050;
    bottom: max(1.25rem, env(safe-area-inset-bottom, 0px));
    left: 50%;
    transform: translateX(-50%);
    max-width: min(420px, calc(100vw - 2rem));
    padding: 0.65rem 1.15rem;
    border-radius: 12px;
    font-size: 0.92rem;
    line-height: 1.35;
    text-align: center;
    box-shadow: 0 8px 28px rgba(0, 0, 0, 0.5);
    border: 1px solid rgba(255, 255, 255, 0.12);
    font-family: var(--gm-font-sans, system-ui, sans-serif);
    pointer-events: none;
  }

  .invite-toast--success {
    background: rgba(32, 38, 30, 0.96);
    color: rgba(230, 225, 216, 0.96);
    border-color: rgba(143, 217, 155, 0.38);
  }

  .invite-toast--error {
    background: rgba(44, 26, 24, 0.96);
    color: #ffd4ce;
    border-color: rgba(255, 140, 120, 0.42);
  }

  .invite-toast-enter-active,
  .invite-toast-leave-active {
    transition: opacity 0.28s ease;
  }

  .invite-toast-enter-from,
  .invite-toast-leave-to {
    opacity: 0;
  }
</style>
