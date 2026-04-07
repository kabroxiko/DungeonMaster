package httpserver

import (
	"net/http"

	"github.com/deckofdmthings/gmai/internal/i18n"
)

func writeJSONCoded(w http.ResponseWriter, r *http.Request, status int, code, fallbackEN string) {
	msg := fallbackEN
	if r != nil {
		msg = i18n.APIError(code, fallbackEN, i18n.LocaleFromRequest(r))
	}
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

func localizeStringMap(r *http.Request, m map[string]string) map[string]string {
	if r == nil || m == nil {
		return m
	}
	code := m["code"]
	errStr := m["error"]
	if code == "" || errStr == "" {
		return m
	}
	msg := i18n.APIError(code, errStr, i18n.LocaleFromRequest(r))
	if msg == errStr {
		return m
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	out["error"] = msg
	return out
}

func localizeIfCoded(r *http.Request, v interface{}) interface{} {
	if r == nil {
		return v
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	code, _ := m["code"].(string)
	errStr, _ := m["error"].(string)
	if code == "" || errStr == "" {
		return v
	}
	msg := i18n.APIError(code, errStr, i18n.LocaleFromRequest(r))
	if msg == errStr {
		return v
	}
	out := make(map[string]interface{}, len(m))
	for k, val := range m {
		out[k] = val
	}
	out["error"] = msg
	return out
}
