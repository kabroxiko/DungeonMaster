<template>
  <!-- Avoid :key on language — it remounts children and clears open modals (e.g. nickname field). -->
  <!-- Do not put authTick on this root: App mounted → refreshSessionUser → bumpAuthTick would remount
       the whole tree (including ChatRoom) and duplicate GET /api/game-state/load. Loads are also
       deduped in fetchGameStateLoad.js. Header uses v-if for login/logout; authTick still drives
       $forceUpdate via the global mixin in main.js. -->
  <GMTheme>
    <div class="app-layout-root">
      <header v-if="isAuthenticated" class="app-banner app-banner--authed" aria-label="Main navigation">
        <div class="app-banner__brand">
          <h1 class="ui-heading app-banner__title">{{ $i18n.dm_label }}</h1>
          <p class="app-banner__tagline">{{ $i18n.subtitle }}</p>
        </div>
        <div class="app-banner__end">
          <div v-if="displayName" class="app-banner__user">
            <span class="app-banner__nickname">{{ displayName }}</span>
          </div>
          <div class="app-banner__controls">
            <div class="app-banner__toolbar" role="toolbar" :aria-label="$i18n.toolbar_aria">
              <button
                type="button"
                class="app-banner__tool"
                :title="$i18n.sign_out"
                :aria-label="$i18n.sign_out"
                @click="signOut"
              >
                <span class="visually-hidden">{{ $i18n.sign_out }}</span>
                <FontAwesomeIcon :icon="faRightFromBracket" class="ui-icon ui-icon--toolbar" aria-hidden="true" />
              </button>
              <button
                type="button"
                class="app-banner__tool"
                :title="$i18n.new_game"
                :aria-label="$i18n.new_game"
                @click="onPlusToolbarClick"
              >
                <span class="visually-hidden">{{ $i18n.new_game }}</span>
                <FontAwesomeIcon :icon="faCirclePlus" class="ui-icon ui-icon--toolbar" aria-hidden="true" />
              </button>
              <button
                type="button"
                class="app-banner__tool"
                :title="$i18n.load_game"
                :aria-label="$i18n.load_game"
                @click="navigateTo('/load-game')"
              >
                <span class="visually-hidden">{{ $i18n.load_game }}</span>
                <FontAwesomeIcon :icon="faFolderOpen" class="ui-icon ui-icon--toolbar" aria-hidden="true" />
              </button>
              <button
                type="button"
                class="app-banner__tool"
                :title="$i18n.settings_open"
                :aria-label="$i18n.settings_open"
                @click="settingsOpen = true"
              >
                <span class="visually-hidden">{{ $i18n.settings_open }}</span>
                <FontAwesomeIcon :icon="faGear" class="ui-icon ui-icon--toolbar" aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>
      </header>
      <header v-else class="app-banner app-banner--public" aria-label="Sign in">
        <div class="app-banner__brand">
          <h1 class="ui-heading app-banner__title">{{ $i18n.dm_label }}</h1>
          <p class="app-banner__tagline">{{ $i18n.subtitle }}</p>
        </div>
        <div class="app-banner__end app-banner__end--public">
          <div v-if="googleClientId" id="google-signin-btn" class="google-signin-host" aria-label="Google sign-in"></div>
          <button
            type="button"
            class="app-banner__tool"
            :title="$i18n.settings_open"
            :aria-label="$i18n.settings_open"
            @click="settingsOpen = true"
          >
            <span class="visually-hidden">{{ $i18n.settings_open }}</span>
            <FontAwesomeIcon :icon="faGear" class="ui-icon ui-icon--toolbar" aria-hidden="true" />
          </button>
        </div>
      </header>
      <main :class="isAuthenticated ? '' : 'public-app-main'" class="app-main-column">
        <router-view></router-view>
      </main>
    </div>
  </GMTheme>
  <SettingsModal
    v-model="settingsOpen"
    :authenticated="isAuthenticated"
    @language="setLanguage"
    @nickname-saved="onNicknameSavedFromSettings"
  />
</template>

<script>
import { faCirclePlus } from '@fortawesome/free-solid-svg-icons/faCirclePlus';
import { faFolderOpen } from '@fortawesome/free-solid-svg-icons/faFolderOpen';
import { faGear } from '@fortawesome/free-solid-svg-icons/faGear';
import { faRightFromBracket } from '@fortawesome/free-solid-svg-icons/faRightFromBracket';
import GMTheme from "@/ui/Theme.vue";
import SettingsModal from "@/components/SettingsModal.vue";
import axios from "axios";
import { SESSION_SETUP_GAME_ID } from "@/setupSession.js";
import { ensureGsiInitialized, renderGsiButton, whenGsiReady } from "./googleSignIn.js";
import store from "./store";

export default {
  name: "App",
  components: {
    GMTheme,
    SettingsModal,
  },
  data() {
    return {
      settingsOpen: false,
      faRightFromBracket,
      faCirclePlus,
      faFolderOpen,
      faGear,
    };
  },
  computed: {
    isAuthenticated() {
      return Boolean(this.$store.state.user && this.$store.state.authToken);
    },
    language() { return this.$store.state.language || 'English'; },
    /** Table / account label only — never show OAuth real name or email in the shell. */
    displayName() {
      const u = this.$store.state.user;
      const n = u && u.nickname && String(u.nickname).trim();
      return n || '';
    },
    googleClientId() {
      return process.env.VUE_APP_DM_GOOGLE_CLIENT_ID || '';
    },
    authTick() {
      return this.$store.state.authTick;
    },
    /** GSI `locale` for renderButton (not the same as app i18n object). */
    gsiLocale() {
      return this.language && String(this.language).toLowerCase().startsWith('span') ? 'es' : 'en';
    },
  },
  watch: {
    language() {
      this.$store.dispatch('loadCharacterCatalog');
    },
    isAuthenticated(val) {
      if (!val) {
        this.$nextTick(() => this.resetAndInitGoogleSignIn());
      }
    },
    authTick() {
      this.$nextTick(() => {
        if (!this.isAuthenticated) this.resetAndInitGoogleSignIn();
      });
    },
    '$store.state.languageVersion'() {
      if (!this.isAuthenticated) {
        this.$nextTick(() => this.resetAndInitGoogleSignIn());
      }
    },
    '$route.fullPath'() {
      this.applyLayoutDebug();
    },
  },
  mounted() {
    this.applyLayoutDebug();
    this.$nextTick(() => {
      whenGsiReady(() => {
        if (!this.isAuthenticated) this.initGoogleSignIn();
      });
      if (this.isAuthenticated) {
        this.$store.dispatch('refreshSessionUser');
      }
    });
  },
  methods: {
    /** Dashed outlines on layout regions: ?layoutDebug=1 or localStorage dm_layout_debug=1 */
    applyLayoutDebug() {
      if (typeof document === 'undefined') return;
      try {
        const raw = this.$route && this.$route.query && this.$route.query.layoutDebug;
        const qs = raw != null ? String(raw).trim().toLowerCase() : '';
        const fromQuery = qs === '1' || qs === 'true' || qs === 'yes';
        let fromLs = false;
        try {
          fromLs = typeof localStorage !== 'undefined' && localStorage.getItem('dm_layout_debug') === '1';
        } catch (e) {
          /* ignore */
        }
        document.body.classList.toggle('gm-layout-debug', fromQuery || fromLs);
      } catch (e) {
        /* ignore */
      }
    },
    refreshChrome() {
      try {
        this.$forceUpdate();
      } catch (e) {
        /* ignore */
      }
    },
    resetAndInitGoogleSignIn() {
      const el = typeof document !== 'undefined' ? document.getElementById('google-signin-btn') : null;
      if (el) {
        try {
          el.innerHTML = '';
          delete el.dataset.gsiRendered;
          delete el.dataset.gsiLocale;
        } catch (e) {
          /* ignore */
        }
      }
      this.initGoogleSignIn();
    },
    initGoogleSignIn() {
      if (this.isAuthenticated) return;
      const clientId = this.googleClientId;
      if (!clientId || typeof window === 'undefined') return;

      const el = document.getElementById('google-signin-btn');
      const locale = this.gsiLocale;
      if (el && el.dataset.gsiRendered === '1' && el.dataset.gsiLocale === locale) {
        return;
      }

      if (!window.google?.accounts?.id) {
        whenGsiReady(() => {
          if (!this.isAuthenticated) this.initGoogleSignIn();
        });
        return;
      }

      try {
        const ok = ensureGsiInitialized(clientId, (res) => this.onGoogleCredential(res));
        if (!ok || !el) return;
        renderGsiButton(el, {
          theme: 'filled_blue',
          size: 'medium',
          text: 'signin_with',
          shape: 'rectangular',
          locale,
        });
        el.dataset.gsiLocale = locale;
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('Google Sign-In init failed', e);
      }
    },
    async onGoogleCredential(response) {
      const cred = response && response.credential;
      if (!cred) return;
      try {
        await this.$store.dispatch('loginWithGoogleCredential', cred);
        await this.$nextTick();
        this.refreshChrome();
        let pending = '';
        try {
          pending = sessionStorage.getItem('dm_pending_invite') || '';
        } catch (e) {
          /* ignore */
        }
        if (pending) {
          const t = String(pending).trim();
          if (t) {
            this.$router.replace({ name: 'JoinParty', params: { token: t } });
          } else {
            this.$router.replace({ name: 'LoadGame' });
          }
        } else {
          this.$router.replace({ name: 'LoadGame' });
        }
        await this.$nextTick();
        this.refreshChrome();
      } catch (e) {
        // eslint-disable-next-line no-console
        console.error('Google login failed', e);
        alert((e.response && e.response.data && e.response.data.error) || 'Sign-in failed');
      }
    },
    onNicknameSavedFromSettings() {
      if (this.$route.name !== 'ChooseNickname') return;
      const r = this.$route.query.redirect;
      if (r && typeof r === 'string' && r.startsWith('/') && !r.startsWith('//')) {
        this.$router.replace(r);
      } else {
        this.$router.replace({ name: 'LoadGame' });
      }
    },
    setLanguage(lang) {
      this.$store.commit('setLanguage', lang);
      try {
        this.$store.commit('notifyLanguageChanged');
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('notifyLanguageChanged commit failed', e);
      }
      try {
        this.$forceUpdate && this.$forceUpdate();
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('forceUpdate failed', e);
      }
      try {
        window.dispatchEvent(new Event('language-changed'));
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('language-changed event dispatch failed', e);
      }
    },
    async signOut() {
      // Leave /chat-room before clearing auth. Otherwise authTick remounts the tree while the route
      // is still chat-room/* and ChatRoom's created() runs load without a Bearer token (401 noise).
      try {
        await this.$router.replace({ name: 'HomePath' });
      } catch (e) {
        /* ignore duplicate navigation */
      }
      await this.$nextTick();
      await this.$store.dispatch('logout');
      await this.$nextTick();
      this.refreshChrome();
      try {
        if (!this.$store.getters.isAuthenticated) {
          await this.$router.replace({ name: 'HomePath' });
        }
      } catch (e) {
        /* ignore duplicate navigation */
      }
      await this.$nextTick();
      this.resetAndInitGoogleSignIn();
    },
    navigateTo(route) {
      if (route === "/setup" || route === "/setup/") {
        try {
          sessionStorage.removeItem(SESSION_SETUP_GAME_ID);
        } catch (e) {
          /* ignore */
        }
        store.commit("resetSetupWizard");
      }
      this.$router.push(route);
    },
    /** Header (+): create an empty party lobby (characters + ready → server opens the adventure). */
    async onPlusToolbarClick() {
      if (!this.isAuthenticated) {
        this.navigateTo('/setup');
        return;
      }
      try {
        const { data } = await axios.post('/api/game-state/create-party', {
          language: this.language,
        });
        const gid = data && data.gameId != null ? String(data.gameId).trim() : '';
        if (!gid) {
          throw new Error('create-party: missing gameId');
        }
        store.commit('setGameId', gid);
        if (data.gameSetup && typeof data.gameSetup === 'object') {
          store.commit('setGameSetup', data.gameSetup);
        }
        await this.$router.push({ name: 'ChatRoomWithId', params: { id: gid } });
      } catch (e) {
        // eslint-disable-next-line no-console
        console.error('create-party failed', e);
        this.navigateTo('/setup');
      }
    },
  },
};
</script>
