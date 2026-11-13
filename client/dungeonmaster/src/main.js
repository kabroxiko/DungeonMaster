import { createApp } from 'vue';
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome';
import App from './App.vue';
import router from './router';
import store from './store';
import axios from 'axios';
import { resolveApiBaseURL } from '@/utils/apiBase.js';
import english from '@/i18n/en.js';
import spanish from '@/i18n/es.js';

const app = createApp(App);
app.component('FontAwesomeIcon', FontAwesomeIcon);

axios.defaults.baseURL = resolveApiBaseURL();

const __dmDevLog = process.env.NODE_ENV === 'development';

axios.interceptors.request.use(
    function (config) {
        const t = store.state.authToken;
        if (t) {
            config.headers = config.headers || {};
            if (!config.headers.Authorization) config.headers.Authorization = `Bearer ${t}`;
        }
        try {
            const lang = store.state.language;
            if (lang && String(lang).trim() !== '') {
                config.headers = config.headers || {};
                if (!config.headers['X-DM-Language']) {
                    config.headers['X-DM-Language'] = String(lang).trim();
                }
            }
        } catch (e) {
            /* ignore */
        }
        if (__dmDevLog) console.log('Request:', config.url);
        return config;
    },
    function (error) {
        return Promise.reject(error);
    }
);

axios.interceptors.response.use(
    function (response) {
        if (__dmDevLog) console.log('Response:', response.status, response.config && response.config.url);
        return response;
    },
    function (error) {
        if (__dmDevLog) console.log('Response Error:', error.response && error.response.status, error.config && error.config.url);
        return Promise.reject(error);
    }
);

app.use(router);
app.use(store);
app.config.globalProperties.$http = axios;
// Listen for language-changed events dispatched from the header and notify the store.
window.addEventListener('language-changed', () => {
  try {
    store.commit('notifyLanguageChanged');
  } catch (e) {
    // eslint-disable-next-line no-console
    console.warn('language-changed handler failed', e);
  }
});

// Global mixin: force components to re-render when languageVersion changes.
app.mixin({
  created() {
    if (this.$store && typeof this.$watch === 'function') {
      // watch store.languageVersion and force update when it increments
      this.__unwatchLang = this.$watch(
        () => this.$store.state.languageVersion,
        () => {
          try {
            this.$forceUpdate && this.$forceUpdate();
          } catch (e) {
            // eslint-disable-next-line no-console
            console.warn('forceUpdate in mixin failed', e);
          }
        }
      );
      // Same for auth: login/logout must refresh chrome without a full page reload
      this.__unwatchAuth = this.$watch(
        () => this.$store.state.authTick,
        () => {
          try {
            this.$forceUpdate && this.$forceUpdate();
          } catch (e) {
            // eslint-disable-next-line no-console
            console.warn('forceUpdate (auth) in mixin failed', e);
          }
        }
      );
    }
  },
  beforeUnmount() {
    if (this.__unwatchLang) {
      this.__unwatchLang();
      this.__unwatchLang = null;
    }
    if (this.__unwatchAuth) {
      this.__unwatchAuth();
      this.__unwatchAuth = null;
    }
  }
});

// Language name → bundle. The copy lives in src/i18n so a wording change touches one small file.
const translations = { English: english, Spanish: spanish };
/** Map store language strings to a translations bundle (handles `Spanish`, `spanish`, etc.). */
function resolveTranslationBundle(lang) {
  const raw = String(lang || 'English').trim();
  let base;
  if (translations[raw]) base = translations[raw];
  else {
    const low = raw.toLowerCase();
    base = low.startsWith('span') ? translations.Spanish : translations.English;
  }
  const cat = store.state.characterCatalog;
  const out = { ...base };
  if (cat && Array.isArray(cat.races) && cat.races.length) {
    out.races = cat.races;
    out.classes = Array.isArray(cat.classes) ? cat.classes : [];
    out.subclass_labels = cat.subclass_labels && typeof cat.subclass_labels === 'object' ? cat.subclass_labels : {};
    out.subrace_labels = cat.subrace_labels && typeof cat.subrace_labels === 'object' ? cat.subrace_labels : {};
  } else {
    out.races = [];
    out.classes = [];
    out.subclass_labels = {};
    out.subrace_labels = {};
  }
  return out;
}

app.config.globalProperties.$t = (key) => {
  try {
    const lang = store.state.language || 'English';
    const bundle = resolveTranslationBundle(lang);
    return (bundle && bundle[key]) || key;
  } catch (e) {
    return key;
  }
};

// Provide a reactive computed translations object to all components via a global mixin.
// Components can use `$i18n` in templates (e.g. `{{ $i18n.send }}`) and it will update reactively.
app.mixin({
  computed: {
    $i18n() {
      void this.$store.state.characterCatalogVersion;
      const lang = (this.$store && this.$store.state && this.$store.state.language) || 'English';
      return resolveTranslationBundle(lang);
    }
  }
});

const AUTH_UI_MUTATIONS = new Set([
  'bumpAuthTick',
  'setUser',
  'setAuthToken',
  'clearSession',
]);

let appVm;
// Runtime config (OAuth client ID) and the character catalog both come from the API, so mount after they resolve.
Promise.all([
  store.dispatch('loadCharacterCatalog').catch(() => {}),
  store.dispatch('loadRuntimeConfig').catch(() => {}),
]).finally(() => {
  appVm = app.mount('#app');
  store.subscribe((mutation) => {
    if (!AUTH_UI_MUTATIONS.has(mutation.type)) return;
    queueMicrotask(() => {
      try {
        appVm.$forceUpdate();
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('store.subscribe auth UI refresh failed', e);
      }
    });
  });
});
