import { createApp } from 'vue';
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome';
import App from './App.vue';
import router from './router';
import store from './store';
import axios from 'axios';
import { resolveApiBaseURL } from '@/utils/apiBase.js';

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

// Simple translation helper that reads the reactive store language
const translations = {
  English: {
    chat_placeholder: 'What do you do?',
    send: 'Send',
    sending: 'Sending...',
    chat_dm_working: 'Working…',
    chat_waiting_dm_server:
        'The Dungeon Master is still generating a reply on the server. Your last message is saved — you can leave this page and return, or refresh.',
    chat_waiting_dm_sync:
        'Waiting for the Dungeon Master’s reply. If it takes a while, refresh the page.',
    chat_unlock_input: 'Unlock typing (if the DM seems stuck)',
    create_character: 'Create character',
    new_game: 'New Game',
    toolbar_aria: 'Account and game actions',
    load_game: 'Load Game',
    load_game_empty: 'No saved games yet. Start a new game from the home screen.',
    load_game_error: 'Could not load saved games. Check that the server is running.',
    load_game_subtitle: 'Open a table you host or have joined.',
    load_game_open: 'Open',
    load_game_phase_lobby: 'Lobby',
    load_game_phase_starting: 'Starting',
    load_game_phase_playing: 'In play',
    load_game_members: '{n} players',
    load_game_messages: '{n} exchanges',
    load_game_language: 'Language',
    load_game_created: 'Created',
    load_game_role_host: 'Host',
    load_game_role_guest: 'Guest',
    load_game_campaign_ready: 'Campaign',
    load_game_no_campaign_yet: 'No campaign yet',
    load_game_delete: 'Delete',
    load_game_delete_aria: 'Delete this game (host only)',
    load_game_delete_confirm: 'Delete this game for everyone? This cannot be undone.',
    load_game_delete_error: 'Could not delete this game.',
    load_game_deleting: 'Deleting…',
    setup_title: 'The Start of Your Adventure',
    setup_confirm_title: 'Review your character',
    setup_desc: 'Select the building blocks of your character and story.',
    setup_join_title: 'Join the party',
    setup_join_desc:
        'Create your character for this campaign. The host has already set the world; your choices add your hero to the table.',
    character_ready_confirm_join:
        'Your character is ready. Use Join party to open the shared chat with the group.',
    confirm_character_join_game: 'Join party',
    confirm_character_mark_ready: 'Confirm character — ready to play',
    setup_party_lobby_form_title: 'Create your character',
    setup_party_lobby_form_desc:
        'Build your hero for this table. After you submit, review the sheet, then use the Ready to start button on your row in the party table to confirm and mark yourself ready.',
    setup_party_late_join_desc:
        'This table is already in play. Create your character below, then use Join party on the confirmation step to open the shared chat.',
    confirm_secondary_actions_aria: 'Edit or regenerate',
    confirm_regenerate_action_aria: 'Regenerate character',
    character_ready_join_party_playing:
        'The adventure has started. Review the sheet above, then join the party to enter the shared chat. Join party is only available once the table is in play.',
    join_party_only_when_playing:
        'The table is not in play yet — finish setup from the lobby, or refresh after the host has started the adventure.',
    character_ready_party_room:
        'Review the sheet above. The main button confirms your character and marks you ready in one step, then returns you to the party room. When everyone with a character is ready, the table starts the adventure. Use Regenerate below if you want a different sheet.',
    character_ready_party_room_roster_only:
        'Use the Ready to play button in your row in the party table above to confirm your character and mark yourself ready. Use Regenerate below if you want a different sheet.',
    join_redirect_character_setup: 'Creating your character…',
    join_loading_campaign: 'Loading this campaign…',
    join_load_failed: 'Could not load this game. Try again or use your invite link again.',
    join_game_gone:
        'This game no longer exists or you no longer have access (it may have been deleted). Ask the host for a new invite, or start a new game from home.',
    join_missing_game_id:
        'This setup lost the party game id (do not use “New game” here). Open your invite link again, or add ?joinGame=PASTE_HOST_GAME_ID to the setup URL.',
    character_regenerate_needs_game:
        'Generate a character first (submit the form), or use Load game if you already have a party.',
    starting: 'Starting...',
    generating_campaign: 'Generating campaign...',
    generating_character: 'Generating character...',
    character_ready_confirm: 'Check stats, gear, and spells in the sheet above. When it looks right, generate the campaign.',
    confirm_character_continue: 'Confirm and generate campaign',
    regenerate_character: 'Regenerate character',
    back_to_edit: 'Back to edit',
    error_generating_character: 'Error generating character',
    error_generating_campaign: 'Error generating campaign',
    error_campaign_gateway_timeout:
        'The server took too long (gateway timeout). Campaign generation runs several AI steps; increase proxy timeouts for /api (e.g. Nginx Proxy Manager: Custom Nginx Configuration with proxy_read_timeout 600s; or Cloudflare: allow 100s+). Then try again.',
    error_character_gateway_timeout:
        'Character generation took too long (gateway timeout). The AI call can exceed default proxy limits; increase proxy_read_timeout / send_timeout for /api (e.g. 600s in Nginx Proxy Manager custom config), then try again.',
    saving_game: 'Saving game...',
    error_saving_game: 'Error saving game',
    party_ready_needs_character:
        'The server has no saved character for you yet. Use Regenerate or submit the form again so the sheet persists, then confirm on the review step.',
    character_persist_failed:
        'Your character was created but could not be saved. Try generating again; if it keeps failing, check server logs.',
    loading: 'Loading...',
    attributes: 'Attributes',
    dm_label: 'Dungeon Master',
    party_waiting_others: "Waiting for other players' actions this round...",
    subtitle: 'Dark Fantasy — DnD inspired',
    background: 'Background',
    sheet_story: 'Story',
    sheet_tab_stats: 'Stats',
    sheet_tab_bio: 'Background, story & languages',
    sheet_tab_gear: 'Combat & gear',
    sheet_tabs_aria: 'Character sheet pages',
    sheet_tab_spells_empty: 'No spells on this sheet.',
    equipment: 'Equipment',
    sheet_tools: 'Tools',
    sheet_armor: 'Armor',
    sheet_languages: 'Languages',
    sheet_spells: 'Spells',
    spell_level_cantrip: 'Cantrip',
    spell_level_n: 'Level {n}',
    spell_level_pager_aria: 'Browse spells by spell level',
    spell_level_page_prev: 'Previous spell level',
    spell_level_page_next: 'Next spell level',
    spell_slot_ordinal: '{n}',
    spell_slots_prefix: 'Slots',
    spell_slots_explain:
      'Level 1+ spells on this list are spells your character has access to on the sheet. Casting a leveled spell spends one spell slot of that spell’s level (or higher when the rules allow). Cantrips (level 0) do not use slots.',
    spell_prepared_sheet_explain:
      'Rows in green are prepared for today (after a long rest you can change them). Muted rows are in your spellbook or repertoire but not prepared — you cannot cast them until you prepare them.',
    spell_overflow_known_explain:
      'Green leveled spells fit your level-1 spells known limit (PHB). Gray rows are extras the generator listed by mistake — they do not count as known for play; regenerate the character for a correct list. Spell slots still limit how many leveled spells you can cast per long rest.',
    spell_spellbook_prep_enforced_explain:
      'Prepared spells were limited to PHB wizard rules: wizard level + Intelligence modifier (minimum 1). That is separate from spell slots (ammo): at level 1 you still have two 1st-level slots per long rest. You may prepare more 1st-level spells than you have slots. Regenerate if you want a different prepared set (the sheet picks a stable subset when the model marks too many prepared).',
    spell_prepared_short: 'Prepared',
    spell_prepared_title: 'Prepared after a long rest: you can cast this spell using an appropriate spell slot.',
    spell_not_prepared_short: 'Not prepared',
    spell_not_prepared_title:
      'Known or in your spellbook, but not among today’s prepared spells — prepare it after a long rest (or use the rules for your class) before casting with a slot.',
    spell_atwill_short: 'At will',
    spell_atwill_title: 'Cantrip: no spell slot — you can cast it freely (subject to casting time and the table).',
    armor_class_abbr: 'AC',
    weapons_sheet: 'Weapons',
    weapons_sheet_missing:
      'Weapon stats are missing from this sheet (no dice). Regenerate from setup so the server saves armor, equipment, tools, and weapons fields.',
    hit_points_abbr: 'HP',
    sheet_coinage: 'Coinage',
    coin_abbr: { pp: 'pp', gp: 'gp', ep: 'ep', sp: 'sp', cp: 'cp' },
    sheet_aria_vitals: 'Hit points, armor class, and coin purse (D&D 5e)',
    level_prefix: 'Level ',
    character_gender: 'Gender',
    character_name: 'Name',
    character_class: 'Class',
    character_race: 'Race',
    character_level: 'Level',
    subclass: 'Subclass',
    subrace: 'Subrace',
    gender_male_short: 'Male',
    gender_female_short: 'Female',
    random: 'Random',
    character_name_placeholder: 'Leave blank for a random name',
    /* races, classes, subclass_labels, subrace_labels: from GET /api/meta/character-options (Vuex) */
    /* Character sheet: ability abbreviations (same as English PHB) */
    statAbbr: {
      STR: 'STR',
      DEX: 'DEX',
      CON: 'CON',
      INT: 'INT',
      WIS: 'WIS',
      CHA: 'CHA'
    },
    sign_in: 'Sign in with Google',
    sign_out: 'Sign out',
    invite_players: 'Invite players',
    invite_copied: 'Invite link copied',
    invite_failed: 'Could not create invite',
    invite_no_game_id: 'No game id in this page. Reload the page or open the game from Load game.',
    invite_copy_prompt_title: 'Copy invite link',
    join_game_title: 'Join game',
    /** Shown on /join-party while POST /api/auth/join runs. */
    invite_joining_party: 'Joining party…',
    join_game_failed: 'Could not join. Check the link or sign in.',
    join_invite_invalid:
        'This invite link was already used or is no longer valid. Ask a player in the game for a new invite, or open the game from Load game if you already joined.',
    join_already_in_party: 'You are already in this party.',
    join_game_missing_token: 'Missing invite token in the URL.',
    join_game_sign_in_first: 'Sign in first, then open your invite link again.',
    state_refresh_failed: 'Could not refresh from server. Reload the page or try again.',
    game_load_not_found:
        'This game was not found. It may have been removed or expired. Choose another save below.',
    party_heading: 'Party',
    party_you: 'you',
    party_host: 'Host',
    party_character_pending: 'No character yet',
    party_sync_tip:
        'Multiplayer: live updates over SSE when possible; we also reload when you return to this tab. Refresh the page if something looks out of date.',
    lobby_title: 'Party lobby',
    lobby_eyebrow: 'Before the tale begins',
    lobby_roster_heading: 'Who is at the table',
    lobby_col_player: 'Player',
    lobby_desc:
        'Create or edit your hero below. When the sheet is saved, confirm and mark yourself ready with the button in your row under Ready to start. When everyone with a character is ready, the table launches the adventure.',
    lobby_roster_need_character_aria: 'Create your character first; the Ready to start control unlocks after the sheet is saved.',
    lobby_col_table: 'Ready to start',
    /** Second column: confirmed on the setup step vs still waiting */
    lobby_ready_confirmed: 'Marked ready',
    lobby_ready_waiting: 'Not yet',
    lobby_starting: 'Starting the adventure…',
    lobby_saving_ready_then_start: 'Saving…',
    lobby_setup_section_aria: 'Character creation and confirmation',
    lobby_character_sheet_section_aria: 'Your character sheet',
    /** Roster column: short primary label (full sentence stays in aria). */
    lobby_roster_ready_short: 'Ready to play',
    lobby_roster_ready_aria: 'Confirm character and mark ready to play',
    /** Roster column: opens character flow before marking ready (left of primary). */
    lobby_roster_regenerate_short: 'Regenerate',
    lobby_last_error: 'Last start error',
    auth_env_hint:
      'Sign-in is not configured: set VUE_APP_DM_GOOGLE_CLIENT_ID for the client and DM_GOOGLE_CLIENT_ID (and DM_JWT_SECRET) on the server.',
    public_hero_title: 'AI-powered party fantasy play (one player or a full group)',
    public_intro_p1:
      'Dungeon Master is a web app where an AI runs the narration and rules assistance for tabletop-style adventures. Campaigns, characters, and play sessions are private to signed-in players.',
    public_intro_p2:
      'Sign in with Google to create a game, load your saves, or accept an invite from another player. Nothing here talks to the AI until you are logged in.',
    public_features_heading: 'What you need to know',
    public_features: [
      'Google sign-in is required to play; there is no guest mode.',
      'Your games are tied to your account; use Invite to add friends to the same campaign.',
      'This page is informational only—no character or campaign data is loaded below.',
    ],
    public_footer_note: 'Open source. Configure Google OAuth and environment variables to run your own instance.',
    join_go_home_sign_in: 'Go to home to sign in',
    nickname_title: 'Choose your player name',
    nickname_hint: 'This is how other players see you in the game (1–40 characters).',
    nickname_label: 'Nickname',
    nickname_placeholder_example: 'e.g. Aria',
    nickname_submit: 'Continue',
    nickname_error_empty: 'Enter a nickname to continue.',
    nickname_error_length: 'Use 1–40 characters.',
    nickname_error_save: 'Could not save. Try again.',
    nickname_title_change: 'Change your player name',
    nickname_hint_change: 'Update how other players see you (1–40 characters).',
    nickname_submit_change: 'Save',
    settings_title: 'Settings',
    settings_open: 'Open settings',
    settings_language_label: 'Language',
    settings_nickname_label: 'Player name',
    settings_nickname_hint: 'How other players see you in-game (1–40 characters).',
    settings_apply: 'Apply',
    settings_save: 'Save',
    settings_saved: 'Applied.',
  },
  Spanish: {
    chat_placeholder: '¿Qué haces?',
    send: 'Enviar',
    sending: 'Enviando...',
    chat_dm_working: 'Trabajando…',
    chat_waiting_dm_server:
        'El Maestro de Calabozos sigue generando una respuesta en el servidor. Tu último mensaje está guardado: puedes salir y volver, o recargar la página.',
    chat_waiting_dm_sync:
        'Esperando la respuesta del Maestro de Calabozos. Si tarda, recarga la página.',
    chat_unlock_input: 'Desbloquear escritura (si el DM parece atascado)',
    create_character: 'Crear personaje',
    new_game: 'Nuevo Juego',
    toolbar_aria: 'Cuenta y acciones de partida',
    load_game: 'Cargar Partida',
    load_game_empty: 'No hay partidas guardadas. Crea una nueva desde el inicio.',
    load_game_error: 'No se pudieron cargar las partidas. Comprueba que el servidor está en marcha.',
    load_game_subtitle: 'Abre una mesa de la que eres anfitrión o invitado.',
    load_game_open: 'Abrir',
    load_game_phase_lobby: 'Sala',
    load_game_phase_starting: 'Iniciando',
    load_game_phase_playing: 'En juego',
    load_game_members: '{n} jugadores',
    load_game_messages: '{n} intercambios',
    load_game_language: 'Idioma',
    load_game_created: 'Creada',
    load_game_role_host: 'Anfitrión',
    load_game_role_guest: 'Invitado',
    load_game_campaign_ready: 'Campaña',
    load_game_no_campaign_yet: 'Sin campaña aún',
    load_game_delete: 'Eliminar',
    load_game_delete_aria: 'Eliminar esta partida (solo anfitrión)',
    load_game_delete_confirm: '¿Eliminar esta partida para todos? No se puede deshacer.',
    load_game_delete_error: 'No se pudo eliminar la partida.',
    load_game_deleting: 'Eliminando…',
    setup_title: 'El comienzo de tu aventura',
    setup_confirm_title: 'Revisa tu personaje',
    setup_desc: 'Selecciona los elementos básicos de tu personaje y la historia.',
    setup_join_title: 'Unirse al grupo',
    setup_join_desc:
        'Crea tu personaje para esta campaña. El anfitrión ya definió el mundo; tus elecciones añaden tu héroe a la mesa.',
    confirm_secondary_actions_aria: 'Editar o regenerar',
    confirm_regenerate_action_aria: 'Regenerar personaje',
    character_ready_confirm_join:
        'Tu personaje está listo. Pulsa Unirse al grupo para abrir el chat compartido.',
    confirm_character_join_game: 'Unirse al grupo',
    confirm_character_mark_ready: 'Confirmar personaje — listo para jugar',
    setup_party_lobby_form_title: 'Crea tu personaje',
    setup_party_lobby_form_desc:
        'Crea tu héroe para esta mesa. Tras enviar el formulario revisa la hoja y usa el botón Listo para empezar en tu fila de la tabla para confirmar y marcarte listo.',
    setup_party_late_join_desc:
        'La mesa ya está en juego. Crea tu personaje abajo y luego usa Unirse al grupo en el paso de confirmación para abrir el chat compartido.',
    character_ready_join_party_playing:
        'La aventura ya ha empezado. Revisa la hoja de arriba y únete al grupo para entrar al chat. Unirse al grupo solo está disponible cuando la partida está en marcha.',
    join_party_only_when_playing:
        'La mesa aún no está en marcha: termina la preparación en el lobby o actualiza cuando el anfitrión haya iniciado la aventura.',
    character_ready_party_room:
        'Revisa la hoja de arriba. El botón principal confirma el personaje y te marca listo en un solo paso, y te devuelve a la sala. Cuando todos los que tienen personaje estén listos, la mesa inicia la aventura. Usa Regenerar personaje abajo si quieres otra hoja.',
    character_ready_party_room_roster_only:
        'Para confirmarte y marcarte listo, usa el botón Listo para jugar en tu fila de la tabla de arriba. Usa Regenerar personaje abajo si quieres otra hoja.',
    join_redirect_character_setup: 'Creando tu personaje…',
    join_loading_campaign: 'Cargando esta campaña…',
    join_load_failed: 'No se pudo cargar la partida. Vuelve a intentarlo o usa otra vez el enlace de invitación.',
    join_game_gone:
        'Esta partida ya no existe o ya no tienes acceso (puede haberse borrado). Pide al anfitrión un enlace nuevo o empieza una partida nueva desde el inicio.',
    join_missing_game_id:
        'Se perdió el id de la partida del grupo (no uses «Nuevo juego» aquí). Vuelve a abrir el enlace de invitación o añade ?joinGame=ID_QUE_TE_DIO_EL_ANFITRIÓN a la URL de configuración.',
    character_regenerate_needs_game:
        'Primero genera un personaje (envía el formulario) o usa Cargar partida si ya tienes una.',
    starting: 'Iniciando...',
    generating_campaign: 'Generando campaña...',
    generating_character: 'Generando personaje...',
    character_ready_confirm: 'Comprueba atributos, equipo y conjuros en la hoja de arriba. Si encaja, genera la campaña.',
    confirm_character_continue: 'Confirmar y generar campaña',
    regenerate_character: 'Regenerar personaje',
    back_to_edit: 'Volver a editar',
    error_generating_character: 'Error generando personaje',
    error_generating_campaign: 'Error generando campaña',
    error_campaign_gateway_timeout:
        'El servidor tardó demasiado (tiempo de espera del proxy). La campaña usa varias llamadas a la IA; aumenta los timeouts del proxy para /api (p. ej. Nginx Proxy Manager: configuración personalizada con proxy_read_timeout 600s). Vuelve a intentarlo.',
    error_character_gateway_timeout:
        'La generación del personaje tardó demasiado (tiempo de espera del proxy). La llamada a la IA puede superar el límite por defecto del proxy; aumenta proxy_read_timeout / send_timeout para /api (p. ej. 600s en NPM). Vuelve a intentarlo.',
    saving_game: 'Guardando partida...',
    error_saving_game: 'Error guardando partida',
    party_ready_needs_character:
        'El servidor aún no tiene guardada tu hoja. Usa Regenerar o vuelve a enviar el formulario para que se guarde, y luego confirma en el paso de revisión.',
    character_persist_failed:
        'El personaje se generó pero no se pudo guardar. Vuelve a generar; si sigue fallando, revisa los registros del servidor.',
    loading: 'Cargando...',
    attributes: 'Atributos',
    dm_label: 'Dungeon Master',
    party_waiting_others: 'Esperando las acciones del resto del grupo en esta ronda…',
    subtitle: 'Dark Fantasy — DnD inspirado',
    background: 'Historia',
    sheet_story: 'Historia',
    sheet_tab_stats: 'Atributos',
    sheet_tab_bio: 'Trasfondo, historia e idiomas',
    sheet_tab_gear: 'Combate y equipo',
    sheet_tabs_aria: 'Páginas de la hoja de personaje',
    sheet_tab_spells_empty: 'Sin conjuros en esta hoja.',
    equipment: 'Equipo',
    sheet_tools: 'Herramientas',
    sheet_armor: 'Armadura',
    sheet_languages: 'Idiomas',
    sheet_spells: 'Conjuros',
    spell_level_cantrip: 'Truco',
    spell_level_n: 'Nivel {n}',
    spell_level_pager_aria: 'Conjuros por nivel',
    spell_level_page_prev: 'Nivel de conjuro anterior',
    spell_level_page_next: 'Nivel de conjuro siguiente',
    spell_slot_ordinal: '{n}º',
    spell_slots_prefix: 'Espacios',
    spell_slots_explain:
      'Los conjuros de nivel 1+ en esta lista son a los que tu personaje tiene acceso en la hoja. Lanzar un conjuro de nivel 1+ gasta un espacio de al menos ese nivel (o superior si las reglas lo permiten). Los trucos (nivel 0) no gastan espacios.',
    spell_prepared_sheet_explain:
      'Las filas en verde están preparados para hoy (tras un descanso largo puedes cambiarlos). Las apagadas están en el grimorio o repertorio pero no preparados: no puedes lanzarlos con espacio hasta prepararlos.',
    spell_overflow_known_explain:
      'Los conjuros de nivel 1 en verde encajan en tu límite de conjuros conocidos de nivel 1 (PHB). Los grises son entradas de más del generador: no cuentan como conocidos para jugar; regenera el personaje para una lista correcta. Los espacios siguen limitando cuántos conjuros de nivel 1+ puedes lanzar por descanso largo.',
    spell_spellbook_prep_enforced_explain:
      'Los conjuros preparados se ajustaron a las reglas PHB del mago: nivel de mago + modificador de Inteligencia (mínimo 1). Eso es aparte de los espacios (munición): en nivel 1 sigues teniendo dos espacios de nivel 1 por descanso largo. Puedes preparar más conjuros de nivel 1 de los que tienes espacios. Regenera si quieres otro conjunto preparado (la hoja elige un subconjunto estable si el modelo preparó de más).',
    spell_prepared_short: 'Preparado',
    spell_prepared_title: 'Preparado tras un descanso largo: puedes lanzarlo gastando un espacio del nivel adecuado.',
    spell_not_prepared_short: 'No preparado',
    spell_not_prepared_title:
      'Conocido o en el grimorio, pero no entre los conjuros preparados de hoy — prepáralo tras un descanso largo (o según las reglas de tu clase) antes de gastar un espacio.',
    spell_atwill_short: 'A voluntad',
    spell_atwill_title: 'Truco: no gasta espacio; puedes lanzarlo con libertad (tiempo de lanzamiento y mesa mediante).',
    armor_class_abbr: 'CA',
    weapons_sheet: 'Armas',
    weapons_sheet_missing:
      'Faltan las estadísticas de armas en esta hoja (sin dados). Regenera desde la configuración para que el servidor guarde armadura, equipo, herramientas y armas.',
    hit_points_abbr: 'PG',
    sheet_coinage: 'Monedas',
    coin_abbr: { pp: 'pp', gp: 'po', ep: 'me', sp: 'mp', cp: 'pc' },
    sheet_aria_vitals: 'Puntos de golpe, clase de armadura y monedas (D&D 5e)',
    level_prefix: 'Nivel ',
    character_gender: 'Género',
    character_name: 'Nombre',
    character_class: 'Clase',
    character_race: 'Raza',
    character_level: 'Nivel',
    subclass: 'Subclase',
    subrace: 'Subraza',
    gender_male_short: 'Masculino',
    gender_female_short: 'Femenino',
    random: 'Aleatorio',
    character_name_placeholder: 'Déjalo vacío para un nombre al azar',
    /* races, classes, subclass_labels, subrace_labels: from GET /api/meta/character-options (Vuex) */
    /* Character sheet: abreviaturas de características (convención D&D en español) */
    statAbbr: {
      STR: 'FUE',
      DEX: 'DES',
      CON: 'CON',
      INT: 'INT',
      WIS: 'SAB',
      CHA: 'CAR'
    },
    sign_in: 'Iniciar sesión con Google',
    sign_out: 'Cerrar sesión',
    invite_players: 'Invitar jugadores',
    invite_copied: 'Enlace de invitación copiado',
    invite_failed: 'No se pudo crear la invitación',
    invite_no_game_id: 'No hay id de partida en esta página. Recarga la página o abre la partida desde Cargar partida.',
    invite_copy_prompt_title: 'Copiar enlace de invitación',
    join_game_title: 'Unirse a la partida',
    invite_joining_party: 'Uniéndote al grupo…',
    join_game_failed: 'No se pudo unir. Revisa el enlace o inicia sesión.',
    join_invite_invalid:
        'Este enlace de invitación ya se usó o ya no es válido. Pide un enlace nuevo a alguien de la partida, o abre la partida desde Cargar partida si ya te uniste.',
    join_already_in_party: 'Ya estás en esta partida.',
    join_game_missing_token: 'Falta el token de invitación en la URL.',
    join_game_sign_in_first: 'Inicia sesión y vuelve a abrir el enlace de invitación.',
    state_refresh_failed: 'No se pudo actualizar desde el servidor. Recarga la página o inténtalo de nuevo.',
    game_load_not_found:
        'No se encontró esta partida. Puede haberse eliminado o expirado. Elige otro guardado en la lista.',
    party_heading: 'Grupo',
    party_you: 'tú',
    party_host: 'Anfitrión',
    party_character_pending: 'Sin personaje aún',
    party_sync_tip:
      'Multijugador: actualizaciones en vivo (SSE) cuando hay varios jugadores; también al volver a la pestaña. Recarga la página si ves algo desactualizado.',
    lobby_title: 'Sala del grupo',
    lobby_eyebrow: 'Antes de empezar la historia',
    lobby_roster_heading: 'Quién está en la mesa',
    lobby_col_player: 'Jugador',
    lobby_desc:
        'Crea o edita tu héroe abajo. Cuando la hoja esté guardada, confirma y márcate listo con el botón de tu fila en Listo para empezar. Cuando todos los que tienen personaje estén listos, la mesa inicia la aventura.',
    lobby_roster_need_character_aria:
        'Primero crea tu personaje; el control Listo para empezar se activa cuando la hoja esté guardada.',
    lobby_col_table: 'Listo para empezar',
    lobby_ready_confirmed: 'Marcó listo',
    lobby_ready_waiting: 'Pendiente',
    lobby_starting: 'Iniciando la aventura…',
    lobby_saving_ready_then_start: 'Guardando…',
    lobby_setup_section_aria: 'Creación y confirmación de personaje',
    lobby_character_sheet_section_aria: 'Tu hoja de personaje',
    lobby_roster_ready_short: 'Listo para jugar',
    lobby_roster_ready_aria: 'Confirmar personaje y marcarte listo para jugar',
    lobby_roster_regenerate_short: 'Regenerar',
    lobby_last_error: 'Último error al empezar',
    auth_env_hint:
      'Inicio de sesión no configurado: define VUE_APP_DM_GOOGLE_CLIENT_ID en el cliente y DM_GOOGLE_CLIENT_ID (y DM_JWT_SECRET) en el servidor.',
    public_hero_title: 'Juego de fantasía en grupo con IA (un jugador o muchos)',
    public_intro_p1:
      'Dungeon Master es una aplicación web donde una IA guía la narración y apoya las reglas al estilo de rol en mesa. Campañas, personajes y partidas son privados para usuarios identificados.',
    public_intro_p2:
      'Inicia sesión con Google para crear una partida, cargar tus guardados o aceptar una invitación. Nada llama a la IA hasta que hay sesión.',
    public_features_heading: 'Qué debes saber',
    public_features: [
      'Se requiere Google para jugar; no hay modo invitado.',
      'Tus partidas están ligadas a tu cuenta; usa Invitar para añadir amigos a la misma campaña.',
      'Esta página es solo informativa: no se cargan datos de personaje ni campaña aquí.',
    ],
    public_footer_note: 'Código abierto. Configura OAuth de Google y variables de entorno para tu propia instancia.',
    join_go_home_sign_in: 'Ir al inicio para iniciar sesión',
    nickname_title: 'Elige tu nombre de jugador',
    nickname_hint: 'Así te verán los demás en la partida. Elige algo que te guste.',
    nickname_label: 'Apodo',
    nickname_placeholder_example: 'p. ej. Aria',
    nickname_submit: 'Continuar',
    nickname_error_empty: 'Escribe un apodo para continuar.',
    nickname_error_length: 'Usa entre 1 y 40 caracteres.',
    nickname_error_save: 'No se pudo guardar. Inténtalo de nuevo.',
    nickname_title_change: 'Cambiar tu nombre de jugador',
    nickname_hint_change: 'Actualiza cómo te ven los demás (1–40 caracteres).',
    nickname_submit_change: 'Guardar',
    settings_title: 'Ajustes',
    settings_open: 'Abrir ajustes',
    settings_language_label: 'Idioma',
    settings_nickname_label: 'Nombre de jugador',
    settings_nickname_hint: 'Así te verán los demás en la partida (1–40 caracteres).',
    settings_apply: 'Aplicar',
    settings_save: 'Guardar',
    settings_saved: 'Aplicado.',
  }
};

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
store
  .dispatch('loadCharacterCatalog')
  .catch(() => {})
  .finally(() => {
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