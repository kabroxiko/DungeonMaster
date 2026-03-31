<template>
  <GMTheme :key="$store.state.language + '-' + $store.state.languageVersion">
    <header class="ui-panel" style="width:100%;max-width:980px;display:flex;justify-content:space-between;align-items:center;margin-bottom:16px;">
      <div>
        <h1 class="ui-heading">{{ $i18n.dm_label }}</h1>
        <div style="color:var(--gm-muted);font-size:0.9rem">{{ $i18n.subtitle }}</div>
      </div>
      <div style="display:flex;align-items:center;gap:8px;">
        <UIButton @click="navigateTo('/setup')">{{ $i18n.new_game }}</UIButton>
        <UIButton @click="navigateTo('/load-game')" style="margin-left:8px">{{ $i18n.load_game }}</UIButton>
        <!-- Language selector (global) -->
        <div class="language-selector" style="margin-left:12px;display:flex;gap:6px;align-items:center;">
          <button type="button" class="lang-btn flag-btn" :class="{active: $store.state.language === 'English'}" @click="setLanguage('English')" :aria-pressed="$store.state.language === 'English'" title="English">🇺🇸</button>
          <button type="button" class="lang-btn flag-btn" :class="{active: $store.state.language === 'Spanish'}" @click="setLanguage('Spanish')" :aria-pressed="$store.state.language === 'Spanish'" title="Español">🇪🇸</button>
        </div>
      </div>
    </header>

    <main style="width:100%;max-width:980px;">
      <router-view></router-view>
    </main>
  </GMTheme>
</template>

<script>
import { mapGetters } from "vuex";
import GMTheme from "@/ui/Theme.vue";
import UIButton from "@/ui/Button.vue";
import { SESSION_SETUP_GAME_ID } from "@/setupSession.js";
import store from "./store";

export default {
  name: "App",
  components: { GMTheme, UIButton },
  computed: {
    ...mapGetters(["isAuthenticated"]),
    language() { return this.$store.state.language || 'English'; }
  },
  methods: {
    setLanguage(lang) {
      console.log('Setting language to', lang);
      this.$store.commit('setLanguage', lang);
      // notify store so reactive watchers / keys update immediately
      try {
        this.$store.commit('notifyLanguageChanged');
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('notifyLanguageChanged commit failed', e);
      }
      // Force a re-render of the app to ensure all components pick up the change immediately.
      // Also emit a DOM event for any listeners.
      try {
        this.$forceUpdate && this.$forceUpdate();
      } catch (e) {
        // log but don't throw - best-effort update
        // eslint-disable-next-line no-console
        console.warn('forceUpdate failed', e);
      }
      try {
        window.dispatchEvent(new Event('language-changed'));
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('language-changed event dispatch failed', e);
      }
      // As a last-resort in dev, reload the page so all HMR-compiled components pick up changes.
      try {
        if (process.env.NODE_ENV === 'development') {
          window.location.reload();
        }
      } catch (e) {
        // ignore
      }
    },
    onLogout() {
      this.$store.commit("setUser", null);
      this.$router.push("/"); // Navigate to home instead of login
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
  },
};
</script>

<style>
/* component-local styles left intentionally minimal; theme provides tokens */
</style>
