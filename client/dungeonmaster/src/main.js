import { createApp } from 'vue';
import App from './App.vue';
import router from './router';
import store from './store';
import axios from 'axios';

const app = createApp(App);

// Use environment-provided API base URL when available, otherwise default to backend on port 5001
axios.defaults.baseURL = process.env.DM_API_BASE
    ? process.env.DM_API_BASE.replace(/\/$/, '')
    : typeof window !== 'undefined'
      ? `${window.location.protocol}//${window.location.hostname}:5001`
      : '';

// Add request interceptor
axios.interceptors.request.use(
    function (config) {
        console.log('Request:', config);
        return config;
    },
    function (error) {
        console.log('Request Error:', error);
        return Promise.reject(error);
    }
);

// Add response interceptor
axios.interceptors.response.use(
    function (response) {
        console.log('Response:', response);
        return response;
    },
    function (error) {
        console.log('Response Error:', error);
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
    }
  },
  beforeUnmount() {
    if (this.__unwatchLang) {
      this.__unwatchLang();
      this.__unwatchLang = null;
    }
  }
});

// Simple translation helper that reads the reactive store language
const translations = {
  English: {
    chat_placeholder: 'What do you do?',
    send: 'Send',
    sending: 'Sending...',
    start_game: 'Start Game',
    new_game: 'New Game',
    load_game: 'Load Game',
    load_game_empty: 'No saved games yet. Start a new game from the home screen.',
    load_game_error: 'Could not load saved games. Check that the server is running.',
    setup_title: 'The Start of Your Adventure',
    setup_desc: 'Select the building blocks of your character and story.',
    starting: 'Starting...',
    generating_campaign: 'Generating campaign...',
    campaign_generated: 'Campaign generated.',
    generating_character: 'Generating character...',
    character_ready_confirm: 'Review your character in the floating sheet (bottom-right), then continue to generate the campaign.',
    confirm_character_continue: 'Confirm and generate campaign',
    regenerate_character: 'Regenerate character',
    back_to_edit: 'Back to edit',
    error_generating_character: 'Error generating character',
    error_generating_campaign: 'Error generating campaign',
    saving_game: 'Saving game...',
    error_saving_game: 'Error saving game',
    loading: 'Loading...',
    welcome: 'Welcome to Dungeon Master',
    attributes: 'Attributes',
    dm_label: 'Dungeon Master',
    subtitle: 'Dark Fantasy — DnD inspired',
    background: 'Background',
    equipment: 'Equipment',
    sheet_armor: 'Armor',
    sheet_languages: 'Languages',
    equipment_legacy: 'Equipment (legacy)',
    armor_class_abbr: 'AC',
    weapons_sheet: 'Weapons',
    weapons_sheet_missing: 'Weapon stats are missing from this sheet (no dice). Regenerate from setup so the server saves armor, equipment, and weapons fields.',
    encounter_tracker: 'Encounter',
    hit_points_abbr: 'HP',
    level_prefix: 'Level ',
    character_gender: 'Gender',
    gender_male: 'Male',
    gender_female: 'Female',
    character_name: 'Name',
    character_class: 'Class',
    character_race: 'Race',
    character_level: 'Level',
    subclass: 'Subclass',
    /* short labels for form placeholders */
    character_name_short: 'Name',
    character_class_short: 'Class',
    character_race_short: 'Race',
    character_level_short: 'Level',
    gender_male_short: 'Male',
    gender_female_short: 'Female',
    random: 'Random',
    classes: [
      { id: 'random', label: 'Random' },
      { id: 'barbarian', label: 'Barbarian' },
      { id: 'bard', label: 'Bard' },
      { id: 'cleric', label: 'Cleric' },
      { id: 'druid', label: 'Druid' },
      { id: 'fighter', label: 'Fighter' },
      { id: 'monk', label: 'Monk' },
      { id: 'paladin', label: 'Paladin' },
      { id: 'ranger', label: 'Ranger' },
      { id: 'rogue', label: 'Rogue' },
      { id: 'sorcerer', label: 'Sorcerer' },
      { id: 'warlock', label: 'Warlock' },
      { id: 'wizard', label: 'Wizard' },
      { id: 'artificer', label: 'Artificer' }
    ],
    races: [
      { id: 'random', label: 'Random' },
      { id: 'human', label: 'Human' },
      { id: 'elf', label: 'Elf' },
      { id: 'dwarf', label: 'Dwarf' },
      { id: 'halfling', label: 'Halfling' },
      { id: 'half-elf', label: 'Half-Elf' },
      { id: 'half-orc', label: 'Half-Orc' },
      { id: 'gnome', label: 'Gnome' },
      { id: 'tiefling', label: 'Tiefling' }
    ],
    /* Character sheet: ability abbreviations (same as English PHB) */
    statAbbr: {
      STR: 'STR',
      DEX: 'DEX',
      CON: 'CON',
      INT: 'INT',
      WIS: 'WIS',
      CHA: 'CHA'
    }
  },
  Spanish: {
    chat_placeholder: '¿Qué haces?',
    send: 'Enviar',
    sending: 'Enviando...',
    start_game: 'Iniciar Juego',
    new_game: 'Nuevo Juego',
    load_game: 'Cargar Partida',
    load_game_empty: 'No hay partidas guardadas. Crea una nueva desde el inicio.',
    load_game_error: 'No se pudieron cargar las partidas. Comprueba que el servidor está en marcha.',
    setup_title: 'El comienzo de tu aventura',
    setup_desc: 'Selecciona los elementos básicos de tu personaje y la historia.',
    starting: 'Iniciando...',
    generating_campaign: 'Generando campaña...',
    campaign_generated: 'Campaña generada.',
    generating_character: 'Generando personaje...',
    character_ready_confirm: 'Revisa tu personaje en la hoja flotante (esquina inferior derecha) y continúa para generar la campaña.',
    confirm_character_continue: 'Confirmar y generar campaña',
    regenerate_character: 'Regenerar personaje',
    back_to_edit: 'Volver a editar',
    error_generating_character: 'Error generando personaje',
    error_generating_campaign: 'Error generando campaña',
    saving_game: 'Guardando partida...',
    error_saving_game: 'Error guardando partida',
    loading: 'Cargando...',
    welcome: 'Bienvenido a Dungeon Master',
    attributes: 'Atributos',
    dm_label: 'Dungeon Master',
    subtitle: 'Dark Fantasy — DnD inspirado',
    background: 'Historia',
    equipment: 'Equipo',
    sheet_armor: 'Armadura',
    sheet_languages: 'Idiomas',
    equipment_legacy: 'Equipo (heredado)',
    armor_class_abbr: 'CA',
    weapons_sheet: 'Armas',
    weapons_sheet_missing: 'Faltan las estadísticas de armas en esta hoja (sin dados). Regenera desde la configuración para que el servidor guarde armadura, equipo y armas.',
    encounter_tracker: 'Encuentro',
    hit_points_abbr: 'PG',
    level_prefix: 'Nivel ',
    character_gender: 'Género',
    gender_male: 'Masculino',
    gender_female: 'Femenino',
    character_name: 'Nombre',
    character_class: 'Clase',
    character_race: 'Raza',
    character_level: 'Nivel',
    subclass: 'Subclase',
    /* short labels for form placeholders */
    character_name_short: 'Nombre',
    character_class_short: 'Clase',
    character_race_short: 'Raza',
    character_level_short: 'Nivel',
    gender_male_short: 'Masculino',
    gender_female_short: 'Femenino',
    random: 'Aleatorio',
    classes: [
      { id: 'random', label: 'Aleatorio' },
      { id: 'barbarian', label: 'Bárbaro' },
      { id: 'bard', label: 'Bardo' },
      { id: 'cleric', label: 'Clérigo' },
      { id: 'druid', label: 'Druida' },
      { id: 'fighter', label: 'Guerrero' },
      { id: 'monk', label: 'Monje' },
      { id: 'paladin', label: 'Paladín' },
      { id: 'ranger', label: 'Explorador' },
      { id: 'rogue', label: 'Pícaro' },
      { id: 'sorcerer', label: 'Hechicero' },
      { id: 'warlock', label: 'Brujo' },
      { id: 'wizard', label: 'Mago' },
      { id: 'artificer', label: 'Artífice' }
    ],
    races: [
      { id: 'random', label: 'Aleatorio' },
      { id: 'human', label: 'Humano' },
      { id: 'elf', label: 'Elfo' },
      { id: 'dwarf', label: 'Enano' },
      { id: 'halfling', label: 'Mediano' },
      { id: 'half-elf', label: 'Medio elfo' },
      { id: 'half-orc', label: 'Medio orco' },
      { id: 'gnome', label: 'Gnomo' },
      { id: 'tiefling', label: 'Tiflin' }
    ],
    /* Character sheet: abreviaturas de características (convención D&D en español) */
    statAbbr: {
      STR: 'FUE',
      DEX: 'DES',
      CON: 'CON',
      INT: 'INT',
      WIS: 'SAB',
      CHA: 'CAR'
    }
  }
};

app.config.globalProperties.$t = (key) => {
  try {
    const lang = store.state.language || 'English';
    return (translations[lang] && translations[lang][key]) || key;
  } catch (e) {
    return key;
  }
};

// Provide a reactive computed translations object to all components via a global mixin.
// Components can use `$i18n` in templates (e.g. `{{ $i18n.send }}`) and it will update reactively.
app.mixin({
  computed: {
    $i18n() {
      const lang = (this.$store && this.$store.state && this.$store.state.language) || 'English';
      return translations[lang] || translations.English;
    }
  }
});

app.mount('#app');