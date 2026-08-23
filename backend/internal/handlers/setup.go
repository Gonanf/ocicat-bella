package handlers

import (
	"crypto/rand"
	"net/http"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"

	"golang.org/x/crypto/bcrypt"
)

// globalCodeAlphabet sin caracteres ambiguos (0/O, 1/I/L).
const globalCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newGlobalCode() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = globalCodeAlphabet[int(b[i])%len(globalCodeAlphabet)]
	}
	return "ESCUELA-" + string(b)
}

// SetupStatus maneja GET /setup/status (§1): 200 siempre.
func (h *Handler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	configured, err := h.store.IsConfigured(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"configured": configured})
}

// SetupSchool maneja POST /setup/school (§1). Retomable: si ya hay escuela
// creada sin admin, devuelve la existente en vez de fallar.
func (h *Handler) SetupSchool(w http.ResponseWriter, r *http.Request) {
	configured, err := h.store.IsConfigured(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}
	if configured {
		errors.WriteCode(w, errors.CodeSetupAlreadyDone)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	existing, gerr := h.store.GetSchool(r.Context())
	if gerr == nil {
		writeJSON(w, http.StatusOK, map[string]string{"school_id": existing.ID, "name": existing.Name})
		return
	}
	if gerr != store.ErrNotFound {
		writeInternal(w)
		return
	}

	school := &model.School{
		ID:         newID(),
		Name:       strings.TrimSpace(req.Name),
		GlobalCode: newGlobalCode(),
		CreatedAt:  time.Now(),
	}
	if err := h.store.CreateSchool(r.Context(), school); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"school_id": school.ID, "name": school.Name})
}

// SetupAdmin maneja POST /setup/admin (§1): crea el director, genera el código
// global y abre sesión staff. Último paso del wizard → instancia configurada.
func (h *Handler) SetupAdmin(w http.ResponseWriter, r *http.Request) {
	configured, err := h.store.IsConfigured(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}
	if configured {
		errors.WriteCode(w, errors.CodeSetupAlreadyDone)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil ||
		strings.TrimSpace(req.Name) == "" || !strings.Contains(req.Email, "@") || len(req.Password) < 8 {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	school, gerr := h.store.GetSchool(r.Context())
	if gerr != nil {
		// requiere school creada (§1); wizard fuera de orden
		errors.WriteCode(w, errors.CodeValidationError, "primero creá la escuela")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeInternal(w)
		return
	}
	user := &model.User{
		ID:           newID(),
		Name:         strings.TrimSpace(req.Name),
		Email:        strings.TrimSpace(req.Email),
		Role:         model.RoleDirector,
		PasswordHash: string(hash),
	}
	if err := h.store.CreateUser(r.Context(), user); err != nil {
		writeInternal(w)
		return
	}

	now := time.Now()
	sess := &model.Session{
		ID:         newID(),
		UserID:     user.ID,
		Kind:       model.SessionKindStaff,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(model.StaffSlidingDuration),
	}
	if err := h.store.CreateSession(r.Context(), sess); err != nil {
		writeInternal(w)
		return
	}
	middleware.SetSessionCookie(w, sess, h.cfg.CookieDomain, h.cfg.IsProduction())
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":   user,
		"school": map[string]string{"global_code": school.GlobalCode},
	})
}
