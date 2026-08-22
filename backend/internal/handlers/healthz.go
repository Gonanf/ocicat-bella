package handlers

import (
	"net/http"
)

// Healthz handles GET /healthz and responds with 200 {"ok":true}
func Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}
