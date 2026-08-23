package handlers

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// canManageClassroom: docente dueño o director (§0.1 jerarquía).
func canManageClassroom(u *model.User, c *model.Classroom) bool {
	if u == nil {
		return false
	}
	return u.Role == model.RoleDirector || (u.Role == model.RoleDocente && c.TeacherID == u.ID)
}

// newJoinCode genera códigos de 6 caracteres sin ambigüedad (§3/CU-8),
// reusando el alfabeto del código global.
func newJoinCode() string {
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(globalCodeAlphabet))))
		if err != nil {
			panic(err)
		}
		b[i] = globalCodeAlphabet[n.Int64()]
	}
	return string(b)
}

// classroomView arma la respuesta JSON del recurso según quien pregunta.
func classroomView(c *model.Classroom, extra map[string]any) map[string]any {
	v := map[string]any{
		"id":         c.ID,
		"name":       c.Name,
		"course":     c.Course,
		"shift":      c.Shift,
		"created_at": c.CreatedAt,
	}
	for k, val := range extra {
		v[k] = val
	}
	return v
}

// CreateClassroom maneja POST /classrooms (§3): docente/director.
func (h *Handler) CreateClassroom(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	var req struct {
		Name   string `json:"name"`
		Course string `json:"course"`
		Shift  string `json:"shift"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	now := time.Now()
	c := &model.Classroom{
		ID:        newID(),
		TeacherID: user.ID,
		Name:      strings.TrimSpace(req.Name),
		Course:    strings.TrimSpace(req.Course),
		Shift:     strings.TrimSpace(req.Shift),
		CreatedAt: now,
	}
	// ponytail: reintento ante colisión de código (UNIQUE); probabilidad despreciable.
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		c.JoinCode = newJoinCode()
		err = h.store.CreateClassroom(r.Context(), c)
		if err == nil {
			break
		}
	}
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusCreated, classroomView(c, map[string]any{"join_code": c.JoinCode}))
}

// ListClassrooms maneja GET /classrooms con scope por rol (§3).
func (h *Handler) ListClassrooms(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	all, err := h.store.ListClassrooms(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}

	items := []map[string]any{}
	switch user.Role {
	case model.RoleDirector:
		for _, c := range all {
			extra := map[string]any{"archived": false}
			if t, terr := h.store.GetUser(r.Context(), c.TeacherID); terr == nil {
				extra["teacher_name"] = t.Name
			}
			items = append(items, classroomView(&c, extra))
		}

	case model.RoleDocente:
		for _, c := range all {
			if c.TeacherID != user.ID {
				continue
			}
			students, serr := h.store.ListClassroomStudents(r.Context(), c.ID)
			if serr != nil {
				writeInternal(w)
				return
			}
			items = append(items, classroomView(&c, map[string]any{
				"join_code":          c.JoinCode,
				"students_count":     len(students),
				"active_assignments": 0, // consignas llegan en otra fase
				"running_sandboxes":  0,
			}))
		}

	default: // alumno (y cualquier otro autenticado sin aulas propias)
		mems, merr := h.store.ListMembershipsByUser(r.Context(), user.ID)
		if merr != nil {
			writeInternal(w)
			return
		}
		byID := make(map[string]model.Classroom, len(all))
		for _, c := range all {
			byID[c.ID] = c
		}
		for _, mem := range mems {
			c, ok := byID[mem.ClassroomID]
			if !ok { // archivada: fuera del listado del alumno
				continue
			}
			items = append(items, classroomView(&c, map[string]any{
				"pending_assignments": 0,
				"up_to_date":          true,
			}))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"classrooms": items})
}

// getClassroomOwned resuelve el aula y verifica dueño/director; si el que
// pregunta es otro usuario autenticado responde forbidden.
func (h *Handler) getClassroomOwned(w http.ResponseWriter, r *http.Request) *model.Classroom {
	c, err := h.store.GetClassroom(r.Context(), r.PathValue("id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return nil
	}
	if !canManageClassroom(middleware.UserFromContext(r.Context()), c) {
		errors.WriteCode(w, errors.CodeForbidden)
		return nil
	}
	return c
}

// GetClassroomDetail maneja GET /classrooms/{id} (§3): docente dueño, director.
func (h *Handler) GetClassroomDetail(w http.ResponseWriter, r *http.Request) {
	c := h.getClassroomOwned(w, r)
	if c == nil {
		return
	}
	writeJSON(w, http.StatusOK, classroomView(c, map[string]any{"join_code": c.JoinCode}))
}

// PatchClassroom maneja PATCH /classrooms/{id}: campos parciales.
func (h *Handler) PatchClassroom(w http.ResponseWriter, r *http.Request) {
	c := h.getClassroomOwned(w, r)
	if c == nil {
		return
	}
	var req struct {
		Name   *string `json:"name"`
		Course *string `json:"course"`
		Shift  *string `json:"shift"`
	}
	if err := decodeJSON(r, &req); err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
		c.Name = strings.TrimSpace(*req.Name)
	}
	if req.Course != nil {
		c.Course = strings.TrimSpace(*req.Course)
	}
	if req.Shift != nil {
		c.Shift = strings.TrimSpace(*req.Shift)
	}
	if err := h.store.UpdateClassroom(r.Context(), c); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, classroomView(c, map[string]any{"join_code": c.JoinCode}))
}

// DeleteClassroom maneja DELETE /classrooms/{id}: ARCHIVA, no borra (CU-6).
func (h *Handler) DeleteClassroom(w http.ResponseWriter, r *http.Request) {
	c := h.getClassroomOwned(w, r)
	if c == nil {
		return
	}
	c.Archived = true
	if err := h.store.UpdateClassroom(r.Context(), c); err != nil {
		writeInternal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetJoinCode maneja GET /classrooms/{id}/join_code (§3).
func (h *Handler) GetJoinCode(w http.ResponseWriter, r *http.Request) {
	c := h.getClassroomOwned(w, r)
	if c == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": c.JoinCode})
}

// RotateJoinCode maneja POST /classrooms/{id}/join_code/rotate (§3/CU-10):
// el código anterior deja de funcionar al instante porque solo se guarda uno.
func (h *Handler) RotateJoinCode(w http.ResponseWriter, r *http.Request) {
	c := h.getClassroomOwned(w, r)
	if c == nil {
		return
	}
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		c.JoinCode = newJoinCode()
		err = h.store.UpdateClassroom(r.Context(), c)
		if err == nil {
			break
		}
	}
	if err != nil {
		writeInternal(w)
		return
	}
	user := middleware.UserFromContext(r.Context())
	_ = h.audit(r.Context(), user.ID, "code.classroom_rotate", c.ID, "", middleware.ExtractIP(r))
	writeJSON(w, http.StatusOK, map[string]string{"code": c.JoinCode})
}

// JoinClassroom maneja POST /classrooms/join (§3/CU-3): alumno, rate 20/h IP.
func (h *Handler) JoinClassroom(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	if retryAfter, ok := h.joinRL.Allow(middleware.ExtractIP(r)); !ok {
		middleware.WriteTooManyRequests(w, retryAfter)
		return
	}

	c, err := h.store.GetClassroomByCode(r.Context(), strings.TrimSpace(req.Code))
	if err != nil { // inválido, rotado o archivado: mismo error
		errors.WriteCode(w, errors.CodeInvalidCode)
		return
	}
	if _, err := h.store.GetMember(r.Context(), c.ID, user.ID); err == nil {
		errors.WriteCode(w, errors.CodeAlreadyMember)
		return
	}

	mem := &model.Membership{
		ClassroomID: c.ID,
		UserID:      user.ID,
		Status:      model.MemberActive,
		CreatedAt:   time.Now(),
	}
	if err := h.store.AddMember(r.Context(), mem); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"classroom": classroomView(c, map[string]any{})})
}

// LeaveClassroom maneja DELETE /classrooms/{id}/membership/me (§3/CU-6).
// ponytail: las entregas aún no existen; cuando existan quedarán archivadas.
func (h *Handler) LeaveClassroom(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	id := r.PathValue("id")
	if _, err := h.store.GetMember(r.Context(), id, user.ID); err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	if err := h.store.RemoveMember(r.Context(), id, user.ID); err != nil {
		writeInternal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListStudents maneja GET /classrooms/{id}/students (§3).
func (h *Handler) ListStudents(w http.ResponseWriter, r *http.Request) {
	c := h.getClassroomOwned(w, r)
	if c == nil {
		return
	}
	students, err := h.store.ListClassroomStudents(r.Context(), c.ID)
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"students": students})
}

// RemoveStudent maneja DELETE /classrooms/{id}/students/{user_id} (§3/CU-18 alt):
// baja del aula; la cuenta NO se borra.
func (h *Handler) RemoveStudent(w http.ResponseWriter, r *http.Request) {
	c := h.getClassroomOwned(w, r)
	if c == nil {
		return
	}
	uid := r.PathValue("user_id")
	if _, err := h.store.GetMember(r.Context(), c.ID, uid); err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	if err := h.store.RemoveMember(r.Context(), c.ID, uid); err != nil {
		writeInternal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
