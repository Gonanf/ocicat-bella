package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// SchoolInfo maneja GET /school (§9.1): director.
func (h *Handler) SchoolInfo(w http.ResponseWriter, r *http.Request) {
	school, err := h.store.GetSchool(r.Context())
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	stats, err := h.store.SchoolStats(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":        school.Name,
		"global_code": map[string]any{"code": school.GlobalCode, "active": school.GlobalCodeActive},
		"stats":       stats,
	})
}

// RegenerateGlobalCode maneja POST /school/global-code/regenerate (§9.1):
// el código anterior muere al instante y los invitados viejos quedan afuera.
func (h *Handler) RegenerateGlobalCode(w http.ResponseWriter, r *http.Request) {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = h.store.SetSchoolGlobalCode(r.Context(), newGlobalCode(), true)
		if err == nil {
			break
		}
	}
	if err != nil {
		writeInternal(w)
		return
	}
	// sesiones de invitado existentes también mueren ("los invitados viejos quedan afuera")
	if err := h.store.RevokeGuestAccess(r.Context()); err != nil {
		writeInternal(w)
		return
	}
	school, gerr := h.store.GetSchool(r.Context())
	if gerr != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": school.GlobalCode})
}

// DisableGlobalCode maneja DELETE /school/global-code (§9.1): desactivar → 204.
func (h *Handler) DisableGlobalCode(w http.ResponseWriter, r *http.Request) {
	school, err := h.store.GetSchool(r.Context())
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	if err := h.store.SetSchoolGlobalCode(r.Context(), school.GlobalCode, false); err != nil {
		writeInternal(w)
		return
	}
	if err := h.store.RevokeGuestAccess(r.Context()); err != nil {
		writeInternal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PublicSchoolInfo maneja GET /school/public (§10): público.
func (h *Handler) PublicSchoolInfo(w http.ResponseWriter, r *http.Request) {
	school, err := h.store.GetSchool(r.Context())
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": school.Name})
}

// GuestSession maneja POST /guest/sessions (§10/CU-14): público, rate 20/h IP.
func (h *Handler) GuestSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	if retryAfter, ok := h.guestRL.Allow(middleware.ExtractIP(r)); !ok {
		middleware.WriteTooManyRequests(w, retryAfter)
		return
	}

	school, err := h.store.GetSchool(r.Context())
	valid := err == nil && school.GlobalCodeActive && school.GlobalCode != "" &&
		strings.EqualFold(school.GlobalCode, strings.TrimSpace(req.Code))
	if !valid {
		errors.WriteCode(w, errors.CodeInvalidCode)
		return
	}

	now := time.Now()
	guest := &model.User{
		ID:    newID(),
		Name:  "Invitado",
		Email: "invitado-" + now.Format("20060102150405") + "-" + newID()[:8] + "@instancia.local",
		Role:  model.RoleInvitado,
	}
	if err := h.store.CreateUser(r.Context(), guest); err != nil {
		writeInternal(w)
		return
	}

	sess := &model.Session{
		ID:         newID(),
		UserID:     guest.ID,
		Kind:       model.SessionKindGuest,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(model.StaffSlidingDuration),
	}
	if err := h.store.CreateSession(r.Context(), sess); err != nil {
		writeInternal(w)
		return
	}
	middleware.SetSessionCookie(w, sess, h.cfg.CookieDomain, h.cfg.IsProduction())
	writeJSON(w, http.StatusCreated, map[string]string{"school_name": school.Name})
}
