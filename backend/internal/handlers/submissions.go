package handlers

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

func fileView(f model.SubmissionFile) map[string]any {
	return map[string]any{"id": f.ID, "name": f.Name, "size": f.Size}
}

func filesView(fs []model.SubmissionFile) []map[string]any {
	out := make([]map[string]any, 0, len(fs))
	for _, f := range fs {
		out = append(out, fileView(f))
	}
	return out
}

// visibleFiles: el snapshot congelado una vez entregada, los vivos mientras draft.
func visibleFiles(s *model.Submission) []model.SubmissionFile {
	if s.State == model.SubDelivered {
		return s.SnapshotFiles
	}
	return s.Files
}

// latestDeliveredByStudent reduce a la última entrega de cada alumno (stats §4).
func latestDeliveredByStudent(subs []model.Submission) map[string]*model.Submission {
	out := map[string]*model.Submission{}
	for i := range subs {
		s := &subs[i]
		if s.State != model.SubDelivered {
			continue
		}
		if cur, ok := out[s.StudentID]; !ok || s.AttemptNumber > cur.AttemptNumber {
			out[s.StudentID] = s
		}
	}
	return out
}

func submissionView(s *model.Submission, extra map[string]any) map[string]any {
	var deliveredAt any
	if !s.DeliveredAt.IsZero() {
		deliveredAt = s.DeliveredAt
	}
	v := map[string]any{
		"id":               s.ID,
		"assignment_id":    s.AssignmentID,
		"attempt_number":   s.AttemptNumber,
		"state":            string(s.State),
		"files":            filesView(visibleFiles(s)),
		"late":             s.Late,
		"delivered_at":     deliveredAt,
		"last_test_result": s.LastTestResult, // null hasta FASE 6 (§5.2)
		"created_at":       s.CreatedAt,
	}
	for k, val := range extra {
		v[k] = val
	}
	return v
}

// loadDraft busca el intento en curso (draft) del alumno para la consigna;
// devuelve nil si todavía no existe.
func loadDraft(subs []model.Submission, studentID string) *model.Submission {
	var draft *model.Submission
	for i := range subs {
		s := &subs[i]
		if s.StudentID == studentID && s.State == model.SubDraft {
			if draft == nil || s.CreatedAt.After(draft.CreatedAt) {
				draft = s
			}
		}
	}
	return draft
}

func countDelivered(subs []model.Submission, studentID string) int {
	n := 0
	for _, s := range subs {
		if s.StudentID == studentID && s.State == model.SubDelivered {
			n++
		}
	}
	return n
}

// UploadSubmissionFiles maneja POST /assignments/{id}/submissions/files (§5.1):
// multipart uno o más archivos ≤10 MB; crea o reusa el draft del intento en curso.
func (h *Handler) UploadSubmissionFiles(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	a := h.loadAssignment(w, r)
	if a == nil {
		return
	}
	if _, err := h.store.GetMember(r.Context(), a.ClassroomID, user.ID); err != nil {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize*16+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			errors.WriteCode(w, errors.CodeFileTooLarge)
			return
		}
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	type incoming struct {
		name string
		data []byte
	}
	var files []incoming
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
		data, tooBig, rerr := readFormFile(file)
		file.Close()
		if rerr != nil {
			writeInternal(w)
			return
		}
		if tooBig {
			errors.WriteCode(w, errors.CodeFileTooLarge)
			return
		}
		files = append(files, incoming{name: filepath.Base(header.Filename), data: data})
	}

	subs, err := h.store.ListSubmissionsByAssignment(r.Context(), a.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	now := time.Now()
	s := loadDraft(subs, user.ID)
	if s == nil {
		s = &model.Submission{
			ID:            newID(),
			AssignmentID:  a.ID,
			StudentID:     user.ID,
			AttemptNumber: countDelivered(subs, user.ID) + 1,
			State:         model.SubDraft,
			CreatedAt:     now,
		}
	}
	for _, f := range files {
		s.Files = append(s.Files, model.SubmissionFile{
			ID:   newID(),
			Name: f.name,
			Size: int64(len(f.data)),
			Data: f.data,
		})
	}
	var serr error
	if existsIn(subs, s.ID) {
		serr = h.store.UpdateSubmission(r.Context(), s)
	} else {
		serr = h.store.CreateSubmission(r.Context(), s)
	}
	if serr != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"submission_id": s.ID,
		"files":         filesView(s.Files),
		"state":         string(s.State),
	})
}

func existsIn(subs []model.Submission, id string) bool {
	for i := range subs {
		if subs[i].ID == id {
			return true
		}
	}
	return false
}

// DeliverSubmission maneja POST /submissions/{submission_id}/deliver (§5.3):
// checkpoint explícito del alumno propietario con confirm_attempt.
func (h *Handler) DeliverSubmission(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	s, ok := h.loadOwnSubmission(w, r, user)
	if !ok {
		return
	}
	a, err := h.store.GetAssignment(r.Context(), s.AssignmentID)
	if err != nil || a.Deleted {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}

	var req struct {
		ConfirmAttempt int `json:"confirm_attempt"`
	}
	if err := decodeJSON(r, &req); err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	now := time.Now()
	subs, err := h.store.ListSubmissionsByAssignment(r.Context(), a.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	// Guardas §5.3: identidad del intento → cupo vigente → plazo cerrado.
	if s.State != model.SubDraft || req.ConfirmAttempt != s.AttemptNumber {
		errors.WriteCode(w, errors.CodeAttemptConflict)
		return
	}
	maxAttempts := a.Attempts.MaxAttempts()
	if maxAttempts >= 0 && countDelivered(subs, user.ID) >= maxAttempts {
		errors.WriteCode(w, errors.CodeAttemptsExhausted)
		return
	}
	late := !a.DueAt.IsZero() && now.After(a.DueAt)
	if late && a.LatePolicy == model.LateClosed {
		errors.WriteCode(w, errors.CodeDeadlinePassed)
		return
	}

	// Snapshot inmutable: archivos se copian al entregar; reintento = nueva draft.
	s.State = model.SubDelivered
	s.DeliveredAt = now
	s.Late = late
	s.SnapshotFiles = copyFileSlice(s.Files)
	// FASE 6 (§5.2): si hubo un run purpose=submission_test terminado para esta
	// entrega, su resultado viaja en el snapshot.
	if tr := h.latestTestResult(r.Context(), s.ID); tr != nil {
		s.LastTestResult = tr
		s.TestedOK = tr.ExitCode == 0
		s.TestError = tr.ExitCode != 0
	}
	if err := h.store.UpdateSubmission(r.Context(), s); err != nil {
		writeInternal(w)
		return
	}

	var remaining any
	if maxAttempts >= 0 {
		remaining = maxAttempts - countDelivered(subs, user.ID) - 1
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":                 s.ID,
		"attempt_number":     s.AttemptNumber,
		"state":              string(s.State),
		"late":               s.Late,
		"delivered_at":       s.DeliveredAt,
		"attempts_remaining": remaining,
		"last_test_result":   s.LastTestResult,
	})
}

func copyFileSlice(fs []model.SubmissionFile) []model.SubmissionFile {
	out := make([]model.SubmissionFile, len(fs))
	copy(out, fs)
	return out
}

// loadOwnSubmission resuelve {submission_id} y verifica que pida su dueño.
func (h *Handler) loadOwnSubmission(w http.ResponseWriter, r *http.Request, user *model.User) (*model.Submission, bool) {
	s, err := h.store.GetSubmission(r.Context(), r.PathValue("submission_id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return nil, false
	}
	if s.StudentID != user.ID {
		errors.WriteCode(w, errors.CodeForbidden)
		return nil, false
	}
	return s, true
}

// MySubmissions maneja GET /assignments/{id}/submissions/me (§5.4): historial
// de los intentos del alumno (#N, estado, tardía).
func (h *Handler) MySubmissions(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	a := h.loadAssignment(w, r)
	if a == nil {
		return
	}
	if _, err := h.store.GetMember(r.Context(), a.ClassroomID, user.ID); err != nil {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}
	subs, err := h.store.ListSubmissionsByAssignment(r.Context(), a.ID)
	if err != nil {
		writeInternal(w)
		return
	}
	items := []map[string]any{}
	for i := range subs {
		s := &subs[i]
		if s.StudentID != user.ID {
			continue
		}
		items = append(items, submissionView(s, nil))
	}
	writeJSON(w, http.StatusOK, map[string]any{"submissions": items})
}

// GetSubmission maneja GET /submissions/{id} (§5.4): propietario, docente dueño
// o director ven el snapshot completo.
func (h *Handler) GetSubmission(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	s, err := h.store.GetSubmission(r.Context(), r.PathValue("submission_id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	a, err := h.store.GetAssignment(r.Context(), s.AssignmentID)
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	staff := false
	if c, cerr := h.store.GetClassroom(r.Context(), a.ClassroomID); cerr == nil {
		staff = canManageClassroom(user, c)
	}
	if !staff && s.StudentID != user.ID {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}
	extra := map[string]any{"student_id": s.StudentID}
	if st, serr := h.store.GetUser(r.Context(), s.StudentID); serr == nil {
		extra["student_name"] = st.Name
	}
	writeJSON(w, http.StatusOK, submissionView(s, extra))
}

// ListAssignmentSubmissions maneja GET /assignments/{id}/submissions?filter=
// (§5.4): vista de corrección del docente/director. late|error filtran
// entregadas; missing lista alumnos sin entrega.
func (h *Handler) ListAssignmentSubmissions(w http.ResponseWriter, r *http.Request) {
	a := h.loadAssignment(w, r)
	if a == nil {
		return
	}
	if !h.canManageAssignment(w, r, a) {
		return
	}
	filter := r.URL.Query().Get("filter")
	switch filter {
	case "", "late", "error", "missing":
	default:
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	subs, err := h.store.ListSubmissionsByAssignment(r.Context(), a.ID)
	if err != nil {
		writeInternal(w)
		return
	}

	if filter == "missing" {
		missing, err := h.missingStudents(r, a, subs)
		if err != nil {
			writeInternal(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"missing": missing})
		return
	}

	items := []map[string]any{}
	for i := range subs {
		s := &subs[i]
		if s.State != model.SubDelivered {
			continue
		}
		if filter == "late" && !s.Late {
			continue
		}
		if filter == "error" && !s.TestError {
			continue
		}
		extra := map[string]any{"student_id": s.StudentID}
		if st, serr := h.store.GetUser(r.Context(), s.StudentID); serr == nil {
			extra["student_name"] = st.Name
		}
		items = append(items, submissionView(s, extra))
	}
	writeJSON(w, http.StatusOK, map[string]any{"submissions": items})
}

func (h *Handler) missingStudents(r *http.Request, a *model.Assignment, subs []model.Submission) ([]map[string]any, error) {
	students, err := h.store.ListClassroomStudents(r.Context(), a.ClassroomID)
	if err != nil {
		return nil, err
	}
	delivered := map[string]bool{}
	for _, s := range subs {
		if s.State == model.SubDelivered {
			delivered[s.StudentID] = true
		}
	}
	out := []map[string]any{}
	for _, st := range students {
		if st.Status == model.MemberActive && !delivered[st.UserID] {
			out = append(out, map[string]any{
				"user_id": st.UserID,
				"name":    st.Name,
				"email":   st.Email,
			})
		}
	}
	return out, nil
}
