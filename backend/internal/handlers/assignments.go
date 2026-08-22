package handlers

import (
	"context"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// AttachmentOrphanTTL: adjuntos sin assignment asociado se purgan a las 24 h (§4).
const AttachmentOrphanTTL = 24 * time.Hour

// loadAssignment resuelve {id}; las borradas (soft-delete) ya no existen para la API.
func (h *Handler) loadAssignment(w http.ResponseWriter, r *http.Request) *model.Assignment {
	a, err := h.store.GetAssignment(r.Context(), r.PathValue("id"))
	if err != nil || a.Deleted {
		errors.WriteCode(w, errors.CodeNotFound)
		return nil
	}
	return a
}

// canSeeAssignment: docente dueño, director o miembro del aula (lectura, §4).
func (h *Handler) isAssignmentMember(r *http.Request, a *model.Assignment) bool {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		return false
	}
	if user.Role == model.RoleDirector {
		return true
	}
	c, err := h.store.GetClassroom(r.Context(), a.ClassroomID)
	if err != nil {
		return false
	}
	if c.TeacherID == user.ID {
		return true
	}
	_, err = h.store.GetMember(r.Context(), a.ClassroomID, user.ID)
	return err == nil
}

// attemptsView serializa {"mode":..., "max"?} según §4.
func attemptsView(ac model.AttemptsConfig) map[string]any {
	v := map[string]any{"mode": string(ac.Mode)}
	if ac.Mode == model.AttemptsLimited {
		v["max"] = ac.Max
	}
	return v
}

func assignmentView(a *model.Assignment, extra map[string]any) map[string]any {
	var dueAt any
	if !a.DueAt.IsZero() {
		dueAt = a.DueAt
	}
	v := map[string]any{
		"id":             a.ID,
		"classroom_id":   a.ClassroomID,
		"title":          a.Title,
		"instructions":   a.Instructions,
		"attachment_ids": a.AttachmentIDs,
		"runtime":        string(a.Runtime),
		"due_at":         dueAt,
		"attempts":       attemptsView(a.Attempts),
		"late_policy":    string(a.LatePolicy),
		"created_at":     a.CreatedAt,
		"updated_at":     a.UpdatedAt,
	}
	for k, val := range extra {
		v[k] = val
	}
	return v
}

type assignmentPayload struct {
	Title         *string            `json:"title"`
	Instructions  *string            `json:"instructions"`
	AttachmentIDs []string           `json:"attachment_ids"`
	Runtime       *string            `json:"runtime"`
	DueAt         *string            `json:"due_at"`
	Attempts      *attemptsPayloadJS `json:"attempts"`
	LatePolicy    *string            `json:"late_policy"`
}

type attemptsPayloadJS struct {
	Mode *string `json:"mode"`
	Max  *int    `json:"max"`
}

// parseAttempts normaliza unlimited|limited{max}|one y valida coherencia.
func parseAttempts(p *attemptsPayloadJS) (model.AttemptsConfig, bool) {
	if p == nil {
		return model.AttemptsConfig{}, false
	}
	switch model.AttemptsMode(deref(p.Mode)) {
	case model.AttemptsUnlimited:
		return model.AttemptsConfig{Mode: model.AttemptsUnlimited}, true
	case model.AttemptsOne:
		return model.AttemptsConfig{Mode: model.AttemptsOne}, true
	case model.AttemptsLimited:
		max := derefInt(p.Max)
		if max < 1 {
			return model.AttemptsConfig{}, false
		}
		return model.AttemptsConfig{Mode: model.AttemptsLimited, Max: max}, true
	default:
		return model.AttemptsConfig{}, false
	}
}

// parseDueAt acepta RFC3339 o "" (sin vencimiento).
func parseDueAt(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// applyAssignmentPatch valida y aplica campos parciales sobre a; devuelve
// false ante cualquier campo inválido (validation_error).
func (h *Handler) applyAssignmentPatch(r *http.Request, a *model.Assignment, req *assignmentPayload) bool {
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		return false
	}
	if req.Runtime != nil && !model.ValidRuntime(model.Runtime(*req.Runtime)) {
		return false
	}
	if req.LatePolicy != nil && model.LatePolicy(*req.LatePolicy) != model.LateAllowed && model.LatePolicy(*req.LatePolicy) != model.LateClosed {
		return false
	}
	if req.DueAt != nil {
		t, ok := parseDueAt(*req.DueAt)
		if !ok {
			return false
		}
		a.DueAt = t
	}
	if req.Attempts != nil {
		ac, ok := parseAttempts(req.Attempts)
		if !ok {
			return false
		}
		a.Attempts = ac
	}
	for _, id := range req.AttachmentIDs {
		if _, err := h.store.GetAttachment(r.Context(), id); err != nil {
			return false
		}
	}
	if req.Title != nil {
		a.Title = strings.TrimSpace(*req.Title)
	}
	if req.Instructions != nil {
		a.Instructions = *req.Instructions
	}
	if req.Runtime != nil {
		a.Runtime = model.Runtime(*req.Runtime)
	}
	if req.LatePolicy != nil {
		a.LatePolicy = model.LatePolicy(*req.LatePolicy)
	}
	if req.AttachmentIDs != nil {
		a.AttachmentIDs = req.AttachmentIDs
	}
	a.UpdatedAt = time.Now()
	return true
}

// CreateAssignment maneja POST /classrooms/{id}/assignments (§4): docente
// dueño/director; publicación inmediata.
func (h *Handler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.GetClassroom(r.Context(), r.PathValue("id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	user := middleware.UserFromContext(r.Context())
	if !canManageClassroom(user, c) {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}

	var req assignmentPayload
	if err := decodeJSON(r, &req); err != nil ||
		req.Title == nil || strings.TrimSpace(*req.Title) == "" ||
		req.Runtime == nil || !model.ValidRuntime(model.Runtime(*req.Runtime)) {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	dueAt, ok := parseDueAt(deref(req.DueAt))
	if !ok {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	attempts, ok := parseAttempts(req.Attempts)
	if req.Attempts != nil && !ok {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	} else if req.Attempts == nil {
		attempts = model.AttemptsConfig{Mode: model.AttemptsUnlimited}
	}
	policy := model.LateAllowed
	if req.LatePolicy != nil {
		if model.LatePolicy(*req.LatePolicy) != model.LateAllowed && model.LatePolicy(*req.LatePolicy) != model.LateClosed {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
		policy = model.LatePolicy(*req.LatePolicy)
	}
	ids := dedupNonEmpty(req.AttachmentIDs)
	for _, id := range ids {
		if _, err := h.store.GetAttachment(r.Context(), id); err != nil {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
	}

	now := time.Now()
	a := &model.Assignment{
		ID:            newID(),
		ClassroomID:   c.ID,
		CreatedBy:     user.ID,
		Title:         strings.TrimSpace(*req.Title),
		Instructions:  deref(req.Instructions),
		AttachmentIDs: ids,
		Runtime:       model.Runtime(*req.Runtime),
		DueAt:         dueAt,
		Attempts:      attempts,
		LatePolicy:    policy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.store.CreateAssignment(r.Context(), a); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusCreated, assignmentView(a, nil))
}

func dedupNonEmpty(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

// ListAssignments maneja GET /classrooms/{id}/assignments (§4): alumnos miembros
// ven activas con vencimientos; docente dueño/director todas + métricas.
func (h *Handler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.GetClassroom(r.Context(), r.PathValue("id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	user := middleware.UserFromContext(r.Context())
	staff := canManageClassroom(user, c)
	if !staff {
		if _, merr := h.store.GetMember(r.Context(), c.ID, user.ID); merr != nil {
			errors.WriteCode(w, errors.CodeForbidden)
			return
		}
	}

	all, err := h.store.ListAssignments(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}
	items := []map[string]any{}
	for i := range all {
		a := &all[i]
		if a.ClassroomID != c.ID || a.Deleted {
			continue
		}
		extra := map[string]any{}
		if staff {
			stats, serr := h.assignmentStats(r, a)
			if serr != nil {
				writeInternal(w)
				return
			}
			extra["stats"] = stats
		}
		items = append(items, assignmentView(a, extra))
	}
	writeJSON(w, http.StatusOK, map[string]any{"assignments": items})
}

// GetAssignment maneja GET /assignments/{id} (§4): lectura también para miembros.
func (h *Handler) GetAssignment(w http.ResponseWriter, r *http.Request) {
	a := h.loadAssignment(w, r)
	if a == nil {
		return
	}
	user := middleware.UserFromContext(r.Context())
	staff := false
	if c, err := h.store.GetClassroom(r.Context(), a.ClassroomID); err == nil {
		staff = canManageClassroom(user, c)
	}
	if !staff && !h.isAssignmentMember(r, a) {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}
	extra := map[string]any{}
	if staff {
		stats, serr := h.assignmentStats(r, a)
		if serr != nil {
			writeInternal(w)
			return
		}
		extra["stats"] = stats
	}
	writeJSON(w, http.StatusOK, assignmentView(a, extra))
}

// PatchAssignment maneja PATCH /assignments/{id}: config modificable en cualquier
// momento incluso con entregas (§9.1); bajar intentos no invalida hechos.
func (h *Handler) PatchAssignment(w http.ResponseWriter, r *http.Request) {
	a := h.loadAssignment(w, r)
	if a == nil {
		return
	}
	if !h.canManageAssignment(w, r, a) {
		return
	}
	var req assignmentPayload
	if err := decodeJSON(r, &req); err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	if !h.applyAssignmentPatch(r, a, &req) {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	if err := h.store.UpdateAssignment(r.Context(), a); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, assignmentView(a, nil))
}

func (h *Handler) canManageAssignment(w http.ResponseWriter, r *http.Request, a *model.Assignment) bool {
	user := middleware.UserFromContext(r.Context())
	ok := user.Role == model.RoleDirector
	if !ok {
		if c, err := h.store.GetClassroom(r.Context(), a.ClassroomID); err == nil {
			ok = c.TeacherID == user.ID
		}
	}
	if !ok {
		errors.WriteCode(w, errors.CodeForbidden)
	}
	return ok
}

// DeleteAssignment maneja DELETE /assignments/{id}: soft-delete → 204 (§4);
// entregas conservadas.
func (h *Handler) DeleteAssignment(w http.ResponseWriter, r *http.Request) {
	a := h.loadAssignment(w, r)
	if a == nil {
		return
	}
	if !h.canManageAssignment(w, r, a) {
		return
	}
	a.Deleted = true
	a.UpdatedAt = time.Now()
	if err := h.store.UpdateAssignment(r.Context(), a); err != nil {
		writeInternal(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AssignmentStats alimenta GET /assignments/{id}/stats (CU-9 paso 4):
// entregados/testeados/tardíos sobre la ÚLTIMA entrega de cada alumno.
func (h *Handler) GetAssignmentStats(w http.ResponseWriter, r *http.Request) {
	a := h.loadAssignment(w, r)
	if a == nil {
		return
	}
	if !h.canManageAssignment(w, r, a) {
		return
	}
	stats, err := h.assignmentStats(r, a)
	if err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) assignmentStats(r *http.Request, a *model.Assignment) (*model.AssignmentStats, error) {
	students, err := h.store.ListClassroomStudents(r.Context(), a.ClassroomID)
	if err != nil {
		return nil, err
	}
	total := 0
	for _, st := range students {
		if st.Status == model.MemberActive {
			total++
		}
	}
	subs, err := h.store.ListSubmissionsByAssignment(r.Context(), a.ID)
	if err != nil {
		return nil, err
	}
	latest := latestDeliveredByStudent(subs)
	stats := &model.AssignmentStats{TotalStudents: total}
	for _, s := range latest {
		stats.Delivered++
		if s.Late {
			stats.Late++
		}
		if s.TestedOK {
			stats.TestedOK++
		}
		if s.TestError {
			stats.TestErrors++
		}
	}
	stats.Missing = total - stats.Delivered
	return stats, nil
}

// UploadAttachment maneja POST /assignments/attachments (§4): multipart file
// ≤10 MB → {attachment_id, filename, size_bytes}. Docente/director.
func (h *Handler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			errors.WriteCode(w, errors.CodeFileTooLarge)
			return
		}
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	defer file.Close()

	data, tooBig, err := readFormFile(file)
	if err != nil {
		writeInternal(w)
		return
	}
	if tooBig {
		errors.WriteCode(w, errors.CodeFileTooLarge)
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		if ct := mime.TypeByExtension(filepath.Ext(header.Filename)); ct != "" {
			contentType = ct
		}
	}
	at := &model.Attachment{
		ID:          newID(),
		Filename:    filepath.Base(header.Filename),
		ContentType: contentType,
		Size:        int64(len(data)),
		Data:        data,
		UploadedBy:  user.ID,
		CreatedAt:   time.Now(),
	}
	if err := h.store.CreateAttachment(r.Context(), at); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"attachment_id": at.ID,
		"filename":      at.Filename,
		"size_bytes":    at.Size,
	})
}

// readFormFile drena un part del multipart; tooBig corta apenas supera MaxFileSize.
func readFormFile(file multipart.File) (data []byte, tooBig bool, err error) {
	buf := make([]byte, 32<<10)
	for {
		n, rerr := file.Read(buf)
		data = append(data, buf[:n]...)
		if int64(len(data)) > MaxFileSize {
			return nil, true, nil
		}
		if rerr != nil {
			if rerr == io.EOF {
				return data, false, nil
			}
			return nil, false, rerr
		}
	}
}

// SweepOrphanAttachments purga adjuntos sin assignment con más de ttl (§4 GC).
// Seam inyectable para tests: recibe now. Devuelve cuántos borró.
func (h *Handler) SweepOrphanAttachments(ctx context.Context, now time.Time, ttl time.Duration) (int, error) {
	ats, err := h.store.ListAttachments(ctx)
	if err != nil {
		return 0, err
	}
	as, err := h.store.ListAssignments(ctx)
	if err != nil {
		return 0, err
	}
	claimed := make(map[string]bool, len(as))
	for _, a := range as { // incluye soft-deleted: sus entregas conservan referencias
		for _, id := range a.AttachmentIDs {
			claimed[id] = true
		}
	}
	purged := 0
	for _, at := range ats {
		if at.AssignmentID == "" && !claimed[at.ID] && now.Sub(at.CreatedAt) > ttl {
			if err := h.store.DeleteAttachment(ctx, at.ID); err != nil {
				return purged, err
			}
			purged++
		}
	}
	return purged, nil
}

// StartAttachmentGC corre el sweep al iniciar y cada intervalo hasta ctx.Done().
func (h *Handler) StartAttachmentGC(ctx context.Context, interval time.Duration) {
	go func() {
		sweep := func() {
			if _, err := h.SweepOrphanAttachments(ctx, time.Now(), AttachmentOrphanTTL); err != nil {
				log.Printf("attachment GC: %v", err)
			}
		}
		sweep()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweep()
			}
		}
	}()
}
