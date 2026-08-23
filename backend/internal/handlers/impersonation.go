package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// StartImpersonation maneja POST /admin/impersonations (§9.5): 201 +
// Set-Cookie de una sesión NUEVA marcada impersonated_by=<director_id>;
// a partir de acá el director actúa COMO el usuario en todos los endpoints.
// reason obligatorio: la auditoría se persiste ANTES de entregar la cookie
// (si el registro falla, no hay sesión).
func (h *Handler) StartImpersonation(w http.ResponseWriter, r *http.Request) {
	director := middleware.UserFromContext(r.Context())
	var req struct {
		UserID string `json:"user_id"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.UserID) == "" ||
		strings.TrimSpace(req.Reason) == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	target, err := h.store.GetUser(r.Context(), req.UserID)
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}

	now := time.Now()
	kind, ttl := model.SessionKindStaff, model.StaffSlidingDuration
	if target.Role == model.RoleAlumno {
		kind, ttl = model.SessionKindPWA, model.PWASlidingDuration
	}
	sess := &model.Session{
		ID:             newID(),
		UserID:         target.ID,
		Kind:           kind,
		CreatedAt:      now,
		LastSeenAt:     now,
		ExpiresAt:      now.Add(ttl),
		ImpersonatedBy: director.ID,
	}
	if err := h.store.CreateSession(r.Context(), sess); err != nil {
		writeInternal(w)
		return
	}
	if err := h.audit(r.Context(), director.ID, "impersonation.start", target.ID, strings.TrimSpace(req.Reason), middleware.ExtractIP(r)); err != nil {
		writeInternal(w)
		return
	}
	middleware.SetSessionCookie(w, sess, h.cfg.CookieDomain, h.cfg.IsProduction())
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":            target,
		"impersonated_by": director.ID,
	})
}

// EndImpersonation maneja DELETE /admin/impersonations/current (§9.5): mata
// la sesión impersonada y devuelve la cookie a una sesión viva del director;
// logout de impersonación ≠ logout del director. Lo llama la propia sesión
// impersonada (que actúa con el rol del usuario), por eso va tras RequireAuth
// y NO tras requireDirector; sesiones sin marca → 404 (nada que terminar).
func (h *Handler) EndImpersonation(w http.ResponseWriter, r *http.Request) {
	sess := middleware.SessionFromContext(r.Context())
	if sess == nil || sess.ImpersonatedBy == "" {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	if err := h.store.DeleteSession(r.Context(), sess.ID); err != nil {
		writeInternal(w)
		return
	}
	if err := h.audit(r.Context(), sess.ImpersonatedBy, "impersonation.end", sess.UserID, "", middleware.ExtractIP(r)); err != nil {
		writeInternal(w)
		return
	}

	sessions, err := h.store.ListSessionsByUser(r.Context(), sess.ImpersonatedBy)
	var back *model.Session
	if err == nil {
		now := time.Now()
		for i := range sessions {
			s := &sessions[i]
			if s.ImpersonatedBy == "" && !s.IsExpired(now) && (back == nil || s.CreatedAt.After(back.CreatedAt)) {
				back = s
			}
		}
	}
	if back == nil {
		middleware.ClearSessionCookie(w, h.cfg.CookieDomain, h.cfg.IsProduction())
		w.WriteHeader(http.StatusNoContent)
		return
	}
	middleware.SetSessionCookie(w, back, h.cfg.CookieDomain, h.cfg.IsProduction())
	w.WriteHeader(http.StatusNoContent)
}

// AuditLog maneja GET /admin/audit-log?actor=&action=&from=&to= (§9.5):
// registro inmutable de acciones sensibles, más nuevo primero.
func (h *Handler) AuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var from, to time.Time
	var err error
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse(time.RFC3339, v); err != nil {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339, v); err != nil {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
	}
	entries, err := h.store.ListAuditEntries(r.Context(), q.Get("actor"), q.Get("action"), from, to)
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
