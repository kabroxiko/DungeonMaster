package i18n

import (
	"net/http"
	"strings"
)

// LocaleFromRequest picks UI language for API error strings.
// Prefer X-DM-Language (set by the SPA from user settings), then Accept-Language.
// Returns "en" or "es".
func LocaleFromRequest(r *http.Request) string {
	if r == nil {
		return "en"
	}
	if v := strings.TrimSpace(r.Header.Get("X-DM-Language")); v != "" {
		if isSpanishLanguageTag(v) {
			return "es"
		}
		return "en"
	}
	al := r.Header.Get("Accept-Language")
	for _, part := range strings.Split(al, ",") {
		tag := strings.TrimSpace(strings.Split(part, ";")[0])
		low := strings.ToLower(tag)
		if strings.HasPrefix(low, "es") {
			return "es"
		}
		if strings.HasPrefix(low, "en") {
			return "en"
		}
	}
	return "en"
}

func isSpanishLanguageTag(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(s, "span") || s == "es" || strings.HasPrefix(s, "es-")
}

// GameLanguageToLocale maps persisted gameSetup.language values (e.g. English, Spanish) to "en" or "es".
func GameLanguageToLocale(language string) string {
	if isSpanishLanguageTag(language) {
		return "es"
	}
	return "en"
}
