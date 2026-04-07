//GMAI/client/dungeonmaster/src/store.js

import { createStore } from "vuex";

const storedUser = localStorage.getItem("user");
const parsedUser = storedUser && storedUser !== "undefined" ? JSON.parse(storedUser) : null;
const storedLanguage = localStorage.getItem("language");
import axios from 'axios';

/** Keep only fields the app uses; drop legacy Google name/email from persisted state. */
function clientUserProfile(u) {
    if (!u || typeof u !== 'object') return null;
    return {
        _id: u._id,
        picture: u.picture != null ? u.picture : '',
        nickname: u.nickname != null ? String(u.nickname).trim() : '',
    };
}

export default createStore({
    state: {
        user: clientUserProfile(parsedUser),
        authToken: localStorage.getItem("authToken") || null,
        userId: parsedUser && parsedUser._id != null ? parsedUser._id : null, // Initialize userId from localStorage
        gameId: null, // Initialize gameId as null 
        credits: 0,
        gameSetup: { // Initialize gameSetup as an object
            characterName: '',
            characterClass: '',
            characterRace: '',
            characterLevel: 1,
            characterGender: 'Male'
        },
        systemMessageContentDM: '',
        summary: '',
        // global language setting for the UI (persisted)
        language: storedLanguage || 'English',
        // incrementing counter to notify components of language change events
        languageVersion: 0,
        /** Bumped on login/logout so the shell (e.g. App header) re-renders reliably. */
        authTick: 0,
        /** Bumped when header "New game" resets the setup wizard (same-route /setup must reset local UI). */
        setupWizardResetTick: 0,
        /** DnD 5e race/class/sub lists from GET /api/meta/character-options (authoritative). */
        characterCatalog: null,
        /** Bumped when characterCatalog updates so $i18n recomputes. */
        characterCatalogVersion: 0,

    },
    mutations: {
        setUser(state, user) {
            const next = clientUserProfile(user);
            state.user = next;
            if (next == null) {
                state.userId = null;
                try {
                    localStorage.removeItem('user');
                } catch (e) {
                    /* ignore */
                }
            } else {
                if (next._id != null && next._id !== '') {
                    state.userId = String(next._id);
                }
                localStorage.setItem('user', JSON.stringify(next));
            }
        },
        setAuthToken(state, authToken) {
            state.authToken = authToken;
            localStorage.setItem("authToken", authToken);
        },
        setUserId(state, userId) {
            state.userId = userId; // Add a new mutation to set the userId in state
        },
        setCredits(state, credits) {
            state.credits = credits;
        },
        setGameSetup(state, gameSetup) {
            if (!gameSetup || typeof gameSetup !== 'object') {
                state.gameSetup = gameSetup;
                return;
            }
            const next = { ...gameSetup };
            delete next.generatedCharacter;
            delete next.gameSystem;
            if (next.playerCharacters != null && typeof next.playerCharacters === 'object') {
                try {
                    next.playerCharacters = JSON.parse(JSON.stringify(next.playerCharacters));
                } catch (e) {
                    // eslint-disable-next-line no-console
                    console.warn('setGameSetup: could not clone playerCharacters', e);
                }
            }
            state.gameSetup = next;
        },
        clearSession(state) {
            state.user = null;
            state.authToken = null;
            state.userId = null;
            state.credits = 0;
            try {
                localStorage.removeItem('user');
                localStorage.removeItem('authToken');
            } catch (e) {
                /* ignore */
            }
        },
        createNewGame(state) {
            state.gameId = Date.now().toString();
        },
        setGameId(state, gameId) {
            state.gameId = gameId; // Add a new mutation to set the gameId in state
        },
        setSystemMessageContentDM(state, systemMessageContentDM) {
            state.systemMessageContentDM = systemMessageContentDM;
        },
        setSummary(state, summary) {
            state.summary = summary;
        },
        setLanguage(state, language) {
            state.language = language;
            try {
                localStorage.setItem('language', language);
            } catch (e) {
                // ignore storage errors
            }
        },
        notifyLanguageChanged(state) {
            state.languageVersion = (state.languageVersion || 0) + 1;
        },
        bumpAuthTick(state) {
            state.authTick = (state.authTick || 0) + 1;
        },
        /** Clears in-memory wizard id/setup when starting a new game (header → Setup). */
        resetSetupWizard(state) {
            state.gameId = null;
            state.gameSetup = {
                characterName: "",
                characterClass: "",
                characterRace: "",
                characterLevel: 1,
                characterGender: "Male",
            };
            state.setupWizardResetTick = (state.setupWizardResetTick || 0) + 1;
        },
        setCharacterCatalog(state, payload) {
            state.characterCatalog = payload && typeof payload === 'object' ? payload : null;
            state.characterCatalogVersion = (state.characterCatalogVersion || 0) + 1;
        },

    },
    actions: {
        loginUser({ commit }, { user, authToken, userId }) {
            commit('setUser', user);
            commit('setAuthToken', authToken);
            commit('setUserId', userId);
            commit('bumpAuthTick');
        },
        async loginWithGoogleCredential({ commit }, idToken) {
            const { data } = await axios.post('/api/auth/google', { idToken });
            const u = data.user;
            commit('setAuthToken', data.token);
            commit('setUser', u);
            commit('setUserId', u && u._id != null ? String(u._id) : null);
            commit('bumpAuthTick');
        },
        logout({ commit }) {
            commit('clearSession');
            commit('bumpAuthTick');
        },
        async saveNickname({ commit, state }, nickname) {
            const { data } = await axios.patch('/api/auth/nickname', { nickname });
            const u = data && data.user ? data.user : null;
            if (!u) {
                throw new Error('Invalid nickname response');
            }
            commit('setUser', { ...state.user, ...u });
            commit('bumpAuthTick');
        },
        /** Merge server profile (nickname, etc.) after reload or when JWT may be stale. */
        async loadCharacterCatalog({ commit, state }) {
            try {
                const { data } = await axios.get('/api/meta/character-options', {
                    headers: state.language ? { 'X-DM-Language': String(state.language).trim() } : {},
                });
                if (data && typeof data === 'object') {
                    commit('setCharacterCatalog', data);
                }
            } catch (e) {
                // eslint-disable-next-line no-console
                console.warn('loadCharacterCatalog failed', e && e.message);
                commit('setCharacterCatalog', null);
            }
        },
        async refreshSessionUser({ commit, state, dispatch }) {
            if (!state.authToken) return;
            try {
                const { data } = await axios.get('/api/auth/me');
                const u = data && data.user ? data.user : null;
                if (u) {
                    const prev = state.user;
                    const merged = { ...prev, ...u };
                    const unchanged =
                        prev &&
                        String(prev._id || '') === String(merged._id || '') &&
                        String(prev.nickname || '').trim() === String(merged.nickname || '').trim() &&
                        String(prev.picture || '') === String(merged.picture || '');
                    if (!unchanged) {
                        commit('setUser', merged);
                        commit('bumpAuthTick');
                    }
                }
            } catch (e) {
                if (e.response && e.response.status === 401) {
                    await dispatch('logout');
                }
            }
        },
    },
    getters: {
        isAuthenticated(state) {
            return Boolean(state.user && state.authToken);
        },
        needsNickname(state) {
            const u = state.user;
            if (!u) return false;
            return !String(u.nickname || '').trim();
        },
    }
});
