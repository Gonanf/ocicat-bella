package handlers

import (
	"net/http"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
)

// ListSessions maneja GET /sessions (§2.4): sesiones activas de la propia cuenta.
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	me := middleware.UserFromContext(r.Context())
	sessions, err := h.store.ListSessionsByUser(r.Context(), me.ID)
	if err != nil {
		writeInternal(w)
		return
	}
	out := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, map[string]any{
			"id":           s.ID,
			"kind":         string(s.Kind),
			"device_label": s.DeviceLabel,
			"created_at":   s.CreatedAt.UTC().Format(time.RFC3339),
			"last_seen_at": s.LastSeenAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// DeleteSession maneja DELETE /sessions/{id} (§2.4): revoca una sesión puntual
// (desvincular celular libera cupo). Sesión ajena o inexistente → 404 (indistinguible).
func (h *Handler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	me := middleware.UserFromContext(r.Context())
	target, _, err := h.store.GetSession(r.Context(), r.PathValue("id"))
	if err != nil || target.UserID != me.ID {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	if err := h.store.DeleteSession(r.Context(), target.ID); err != nil {
		writeInternal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CloseOthers maneja POST /sessions/close-others (§2.4): revoca todas menos la actual.
func (h *Handler) CloseOthers(w http.ResponseWriter, r *http.Request) {
	me := middleware.UserFromContext(r.Context())
	current := middleware.SessionFromContext(r.Context())
	if current == nil {
		errors.WriteCode(w, errors.CodeUnauthenticated)
		return
	}

	sessions, err := h.store.ListSessionsByUser(r.Context(), me.ID)
	if err != nil {
		writeInternal(w)
		return
	}
	now := time.Now()
	for _, s := range sessions {
		if s.ID != current.ID && !s.IsExpired(now) {
			if err := h.store.DeleteSession(r.Context(), s.ID); err != nil {
				writeInternal(w)
				return
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
