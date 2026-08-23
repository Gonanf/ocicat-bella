package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"

	"golang.org/x/crypto/bcrypt"
)

// audit persiste una acción sensible §9.5; el caller decide si un fallo
// bloquea la operación (impersonación) o es tolerable.
func (h *Handler) audit(ctx context.Context, actorID, action, targetID, reason, ip string) error {
	return h.store.CreateAuditEntry(ctx, &model.AuditEntry{
		ID:           newID(),
		ActorID:      actorID,
		Action:       action,
		TargetUserID: targetID,
		Reason:       reason,
		IP:           ip,
		CreatedAt:    time.Now(),
	})
}

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
	user := middleware.UserFromContext(r.Context())
	_ = h.audit(r.Context(), user.ID, "code.global_rotate", "", "", middleware.ExtractIP(r))
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
	user := middleware.UserFromContext(r.Context())
	_ = h.audit(r.Context(), user.ID, "code.global_disable", "", "", middleware.ExtractIP(r))
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

// --- Docentes §9.2 (CU-15) — todo director ---

type credentialReq struct {
	Type     string `json:"type"` // password | magic_link
	Password string `json:"password"`
}

func teacherView(u model.User, classroomsCount int) map[string]any {
	status := "active"
	if u.Disabled {
		status = "disabled"
	}
	return map[string]any{
		"id":               u.ID,
		"name":             u.Name,
		"email":            u.Email,
		"status":           status,
		"classrooms_count": classroomsCount,
		"created_at":       u.CreatedAt,
	}
}

// applyCredential valida y aplica credential al usuario docente; con
// magic_link genera y envía el enlace de primer acceso/recovery.
func (h *Handler) applyCredential(r *http.Request, u *model.User, cred credentialReq) error {
	switch cred.Type {
	case "password":
		if cred.Password == "" {
			return errValidation
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(cred.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.PasswordHash = string(hash)
	case "magic_link":
		now := time.Now()
		tok := &model.MagicToken{
			Token:     newToken(),
			UserID:    u.ID,
			Context:   model.MagicContextPairingPWA,
			CreatedAt: now,
			ExpiresAt: now.Add(model.MagicLinkTTL),
		}
		if err := h.store.CreateMagicToken(r.Context(), tok); err != nil {
			return err
		}
		link := schemeFromRequest(r) + "://" + r.Host + "/auth/consume?token=" + tok.Token
		h.sendMagicLink(u.Email, link)
	default:
		return errValidation
	}
	return nil
}

// ListTeachers maneja GET /school/teachers (§9.2): docentes + aulas a cargo.
func (h *Handler) ListTeachers(w http.ResponseWriter, r *http.Request) {
	teachers, err := h.store.ListUsersByRole(r.Context(), model.RoleDocente)
	if err != nil {
		writeInternal(w)
		return
	}
	classrooms, err := h.store.ListClassrooms(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}
	count := make(map[string]int)
	for _, c := range classrooms {
		count[c.TeacherID]++
	}
	items := []map[string]any{}
	for _, t := range teachers {
		items = append(items, teacherView(t, count[t.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"teachers": items})
}

// CreateTeacher maneja POST /school/teachers (§9.2/CU-15): alta de cuenta
// docente; 409 email_already_exists ([C5]).
func (h *Handler) CreateTeacher(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string        `json:"name"`
		Email      string        `json:"email"`
		Credential credentialReq `json:"credential"`
	}
	if err := decodeJSON(r, &req); err != nil ||
		strings.TrimSpace(req.Name) == "" || !validEmail(req.Email) {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	if _, err := h.store.GetUserByEmail(r.Context(), req.Email); err == nil {
		errors.WriteCode(w, errors.CodeEmailAlreadyExists)
		return
	}

	now := time.Now()
	u := &model.User{
		ID:        newID(),
		Name:      strings.TrimSpace(req.Name),
		Email:     strings.TrimSpace(req.Email),
		Role:      model.RoleDocente,
		CreatedAt: now,
	}
	if err := h.applyCredential(r, u, req.Credential); err != nil {
		if err == errValidation {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
		writeInternal(w)
		return
	}
	if err := h.store.CreateUser(r.Context(), u); err != nil {
		writeInternal(w)
		return
	}
	dir := middleware.UserFromContext(r.Context())
	_ = h.audit(r.Context(), dir.ID, "teacher.create", u.ID, "", middleware.ExtractIP(r))
	writeJSON(w, http.StatusCreated, teacherView(*u, 0))
}

// getTeacher resuelve {id} como docente existente; otra cosa → 404.
func (h *Handler) getTeacher(w http.ResponseWriter, r *http.Request) *model.User {
	u, err := h.store.GetUser(r.Context(), r.PathValue("id"))
	if err != nil || u.Role != model.RoleDocente {
		errors.WriteCode(w, errors.CodeNotFound)
		return nil
	}
	return u
}

// ResetTeacherCredential maneja POST /school/teachers/{id}/reset-credential
// (§9.2 recovery CU-15): mismo shape que el alta.
func (h *Handler) ResetTeacherCredential(w http.ResponseWriter, r *http.Request) {
	u := h.getTeacher(w, r)
	if u == nil {
		return
	}
	var req struct {
		Credential credentialReq `json:"credential"`
	}
	if err := decodeJSON(r, &req); err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	if err := h.applyCredential(r, u, req.Credential); err != nil {
		if err == errValidation {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
		writeInternal(w)
		return
	}
	if err := h.store.UpdateUser(r.Context(), u); err != nil {
		writeInternal(w)
		return
	}
	dir := middleware.UserFromContext(r.Context())
	_ = h.audit(r.Context(), dir.ID, "teacher.credential_reset", u.ID, "", middleware.ExtractIP(r))
	writeJSON(w, http.StatusOK, teacherView(*u, 0))
}

// setTeacherDisabled aplica disable/enable (§9.2): disable mata sus sesiones
// vivas además de bloquear futuros logins (403 account_disabled).
func (h *Handler) setTeacherDisabled(w http.ResponseWriter, r *http.Request, disabled bool, action string) {
	u := h.getTeacher(w, r)
	if u == nil {
		return
	}
	u.Disabled = disabled
	if err := h.store.UpdateUser(r.Context(), u); err != nil {
		writeInternal(w)
		return
	}
	if disabled {
		sessions, err := h.store.ListSessionsByUser(r.Context(), u.ID)
		if err == nil {
			for i := range sessions {
				_ = h.store.DeleteSession(r.Context(), sessions[i].ID)
			}
		}
	}
	dir := middleware.UserFromContext(r.Context())
	_ = h.audit(r.Context(), dir.ID, action, u.ID, "", middleware.ExtractIP(r))
	writeJSON(w, http.StatusOK, teacherView(*u, 0))
}

func (h *Handler) DisableTeacher(w http.ResponseWriter, r *http.Request) {
	h.setTeacherDisabled(w, r, true, "teacher.disable")
}

func (h *Handler) EnableTeacher(w http.ResponseWriter, r *http.Request) {
	h.setTeacherDisabled(w, r, false, "teacher.enable")
}
