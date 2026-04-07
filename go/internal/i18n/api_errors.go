package i18n

// APIError returns a user-facing message for a stable API error code.
// If code is unknown or a locale string is empty, fallback is returned (usually English from the handler).
func APIError(code, fallback, locale string) string {
	row, ok := apiErrorText[code]
	if !ok {
		return fallback
	}
	if locale == "es" && row.es != "" {
		return row.es
	}
	if row.en != "" {
		return row.en
	}
	return fallback
}

type apiRow struct {
	en, es string
}

// Keep keys aligned with JSON "code" fields from handlers and gamesession.
var apiErrorText = map[string]apiRow{
	"AUTH_REQUIRED": {en: "Authentication required", es: "Se requiere autenticación"},
	"AUTH_INVALID":  {en: "Invalid or expired session", es: "Sesión no válida o expirada"},
	"AUTH_CONFIG":   {en: "Server missing DM_GOOGLE_CLIENT_ID", es: "Falta la configuración de inicio de sesión en el servidor (DM_GOOGLE_CLIENT_ID)."},
	"GOOGLE_AUTH_FAILED": {en: "Google sign-in failed", es: "No se pudo iniciar sesión con Google."},
	"ID_TOKEN_REQUIRED":  {en: "idToken required", es: "Se requiere idToken."},
	"GOOGLE_USER_SAVE_FAILED": {
		en: "save failed",
		es: "No se pudo guardar la cuenta. Inténtalo de nuevo.",
	},
	"GOOGLE_INSERT_ID_TYPE": {
		en: "unexpected insert id type",
		es: "Error interno al crear la cuenta (identificador inesperado).",
	},
	"GOOGLE_USER_DB_ERROR": {
		en: "db error",
		es: "Error de base de datos al acceder a la cuenta.",
	},
	"SESSION_TOKEN_FAILED": {
		en: "token",
		es: "No se pudo crear la sesión. Inténtalo de nuevo.",
	},

	"GAME_NOT_FOUND":   {en: "Game not found", es: "Partida no encontrada."},
	"GAME_ID_REQUIRED": {en: "gameId required", es: "Se requiere el identificador de partida (gameId)."},
	"GAME_LOAD_EMPTY": {
		en: "No game state found for this game",
		es: "No hay estado guardado para esta partida.",
	},
	"GAME_STATE_MISSING": {en: "No game state found", es: "No se encontró el estado de la partida."},
	"GAME_LOAD_FAILED":   {en: "Failed to load game state", es: "No se pudo cargar el estado de la partida."},
	"GAME_DEBUG_FAILED":  {en: "Failed to load debug data", es: "No se pudieron cargar los datos de depuración."},
	"DB_ERROR":           {en: "db error", es: "Error de base de datos."},

	"INVITE_TOKEN_REQUIRED": {en: "inviteToken required", es: "Se requiere inviteToken."},
	"INVITE_INVALID":        {en: "Invalid or expired invite", es: "Invitación no válida o caducada."},
	"JOIN_FAILED":           {en: "Join failed", es: "No se pudo unir a la partida."},
	"USER_INVALID":          {en: "Invalid user", es: "Usuario no válido."},
	"USER_NOT_FOUND":        {en: "User not found", es: "Usuario no encontrado."},
	"USER_LOAD_FAILED":      {en: "Failed to load user", es: "No se pudo cargar el usuario."},
	"INVALID_SESSION_USER_ID": {
		en: "Invalid session user id",
		es: "Identificador de usuario de sesión no válido.",
	},

	"NICKNAME_INVALID":     {en: "Nickname must be between 1 and 40 characters.", es: "El apodo debe tener entre 1 y 40 caracteres."},
	"NICKNAME_SAVE_FAILED": {en: "Could not save nickname", es: "No se pudo guardar el apodo."},

	"NOT_OWNER_PREMISE": {en: "Only the table owner can set the premise", es: "Solo el anfitrión de la mesa puede establecer la premisa."},
	"NOT_OWNER_DELETE":  {en: "Only the host can delete this game", es: "Solo el anfitrión puede eliminar esta partida."},

	"PARTY_CREATE_FAILED": {en: "Could not create party", es: "No se pudo crear la mesa."},
	"PARTY_READY_NEEDS_CHARACTER": {
		en: "Your character must be saved on the server before you can mark ready. Finish character creation (generate) and try again.",
		es: "Tu personaje debe estar guardado en el servidor antes de marcar listo. Termina la creación (genera el personaje) e inténtalo de nuevo.",
	},
	"GAMES_LIST_FAILED":   {en: "Failed to load your games", es: "No se pudieron cargar tus partidas."},
	"GAME_DELETE_FAILED":  {en: "Failed to delete game", es: "No se pudo eliminar la partida."},

	"INVALID_JSON": {en: "Invalid JSON", es: "JSON no válido."},

	"GAME_ID_OR_NEW_PARTY_REQUIRED": {
		en: "This request needs a gameId (join an existing party) or newParty: true (start a brand-new party on the server).",
		es: "Esta petición necesita un gameId (unirte a una mesa) o newParty: true (crear una mesa nueva en el servidor).",
	},

	"PLAYER_MESSAGE_CONTENT_REQUIRED": {en: "content is required", es: "Se requiere el contenido del mensaje."},
	"APPEND_MESSAGE_FAILED":           {en: "Failed to append player message", es: "No se pudo guardar el mensaje del jugador."},

	"INVITE_CREATE_FAILED": {en: "Could not create invite", es: "No se pudo crear la invitación."},

	// Character generation (gamesession/character_gen.go)
	"CHARACTER_SCOPE_PROMPT_MISSING": {
		en: "generate-character: dm_character_generation_scope.txt is missing or empty.",
		es: "Falta la plantilla de alcance de generación de personaje en el servidor.",
	},
	"CHARACTER_PROMPT_RENDER_FAILED": {
		en: "generate-character: failed to render skill_character.txt (check Mustache placeholders).",
		es: "No se pudo preparar la plantilla del personaje en el servidor.",
	},
	"AI_RESPONSE_EMPTY": {
		en: "Character generation failed: the model returned no usable text.",
		es: "La generación de personaje falló: el modelo no devolvió texto utilizable.",
	},
	"INVALID_MODEL_JSON": {
		en: "",
		es: "El modelo no devolvió JSON válido con un objeto playerCharacter en la raíz. Revisa los registros del servidor para un extracto.",
	},
	"INVALID_PLAYER_CHARACTER": {
		en: "",
		es: "Los datos del personaje generado no son válidos.",
	},
	"CHARACTER_PERSIST_FAILED": {
		en: "Character was generated but could not be saved to the game. Try again.",
		es: "El personaje se generó pero no se pudo guardar en la partida. Inténtalo de nuevo.",
	},
	"INVALID_PLAYER_CHARACTER_PERSIST": {
		en: "",
		es: "No se pudo validar el personaje para guardarlo.",
	},

	// Party adventure start (gamesession/start_party.go)
	"PARTY_SHEETS_INCOMPLETE": {
		en: "Every member must have a valid character sheet before the adventure starts.",
		es: "Todos los jugadores deben tener una ficha válida antes de empezar la aventura.",
	},
	"PARTY_NOT_READY": {
		en: "Every member must mark ready before the adventure starts.",
		es: "Todos los jugadores deben marcar listo antes de empezar la aventura.",
	},
	"PARTY_START_IN_PROGRESS": {en: "Party start already in progress.", es: "La partida ya se está iniciando."},
	"PARTY_START_CONFLICT":    {en: "Party cannot start from this state.", es: "No se puede iniciar la partida desde este estado."},
	"PARTY_BOOTSTRAP_FAILED": {
		en: "Saved campaign but failed to bootstrap session shell.",
		es: "La campaña se guardó pero no se pudo preparar la sesión. Inténtalo de nuevo.",
	},
	"PARTY_OPENING_FAILED": {
		en: "",
		es: "No se pudo generar la narración inicial de la partida.",
	},
}
