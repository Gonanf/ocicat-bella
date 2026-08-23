package handlers

import (
	"context"
	"encoding/csv"
	stderrors "errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

var errValidation = stderrors.New("validation")

// validEmail: casilla con algo a ambos lados de la @ y punto en el dominio
// (el contrato marca "pedro@" como invalid_email, §9.3).
func validEmail(s string) bool {
	at := strings.Index(s, "@")
	return at > 0 && at < len(s)-1 && strings.Contains(s[at+1:], ".")
}

type importRow struct {
	Line   int    `json:"line"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status string `json:"status"` // ok | duplicate | invalid_email | exists
	Reason string `json:"reason,omitempty"`
}

// parseImportCSV lee el campo multipart "csv" (nombre+email por fila, §9.3).
func parseImportCSV(r *http.Request) ([]importRow, error) {
	file, _, err := r.FormFile("csv")
	if err != nil {
		return nil, errValidation
	}
	defer file.Close()

	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, errValidation
	}
	rows := make([]importRow, 0, len(records))
	for i, rec := range records {
		if len(rec) < 2 {
			return nil, errValidation
		}
		name, email := strings.TrimSpace(rec[0]), strings.TrimSpace(rec[1])
		if name == "" && email == "" {
			continue // línea vacía
		}
		rows = append(rows, importRow{Line: i + 1, Name: name, Email: email})
	}
	if len(rows) == 0 || len(rows) > model.MaxImportRows {
		return nil, errValidation
	}
	return rows, nil
}

// analyzeRows clasifica fila por fila (CU-18): inválidas, repetidas en el
// CSV y cuentas existentes se rechazan sin bloquear al resto.
func analyzeRows(ctx context.Context, s store.Store, rows []importRow) []importRow {
	firstSeen := make(map[string]int)
	out := make([]importRow, len(rows))
	copy(out, rows)
	for i := range out {
		email := strings.ToLower(out[i].Email)
		switch {
		case !validEmail(out[i].Email):
			out[i].Status = "invalid_email"
		case firstSeen[email] != 0:
			out[i].Status = "duplicate"
			out[i].Reason = fmt.Sprintf("email repetido en el CSV (línea %d)", firstSeen[email])
		default:
			if _, err := s.GetUserByEmail(ctx, out[i].Email); err == nil {
				out[i].Status = "exists"
				out[i].Reason = "ya tiene cuenta en la escuela"
			} else {
				out[i].Status = "ok"
				firstSeen[email] = out[i].Line
			}
		}
	}
	return out
}

func countStatus(rows []importRow, status string) int {
	n := 0
	for _, row := range rows {
		if row.Status == status {
			n++
		}
	}
	return n
}

// altaAlumno crea la cuenta del alumno sin password + membresía activa +
// magic link de primer acceso enviado por email (§9.3).
func (h *Handler) altaAlumno(r *http.Request, classroomID, name, email string) error {
	now := time.Now()
	stu := &model.User{ID: newID(), Name: name, Email: email, Role: model.RoleAlumno, CreatedAt: now}
	if err := h.store.CreateUser(r.Context(), stu); err != nil {
		return err
	}
	mem := &model.Membership{ClassroomID: classroomID, UserID: stu.ID, Status: model.MemberActive, CreatedAt: now}
	if err := h.store.AddMember(r.Context(), mem); err != nil {
		return err
	}
	mt := &model.MagicToken{
		Token:     newToken(),
		UserID:    stu.ID,
		Context:   model.MagicContextPairingPWA,
		CreatedAt: now,
		ExpiresAt: now.Add(model.MagicLinkTTL),
	}
	if err := h.store.CreateMagicToken(r.Context(), mt); err != nil {
		return err
	}
	link := schemeFromRequest(r) + "://" + r.Host + "/auth/consume?token=" + mt.Token
	h.sendMagicLink(stu.Email, link)
	return nil
}

// ImportStudents maneja POST /classrooms/{id}/students/import (§9.3/CU-18):
// dos pasos — preview dry_run:true y commit dry_run:false + import_token.
// Solo las filas ok se dan de alta; el resto se omite y se reporta.
func (h *Handler) ImportStudents(w http.ResponseWriter, r *http.Request) {
	classroom := h.getClassroomOwned(w, r)
	if classroom == nil {
		return
	}
	user := middleware.UserFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	rows, err := parseImportCSV(r)
	if err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	dryRun := true
	if v := r.FormValue("dry_run"); v != "" {
		if dryRun, err = strconv.ParseBool(v); err != nil {
			errors.WriteCode(w, errors.CodeValidationError)
			return
		}
	}

	analyzed := analyzeRows(r.Context(), h.store, rows)
	okCount := countStatus(analyzed, "ok")
	warnings := []string{}
	if countStatus(analyzed, "duplicate") > 0 {
		warnings = append(warnings, "detectadas casillas compartidas (mismo email en varias filas): preferí casillas individuales")
	}

	if dryRun {
		tok := &model.ImportToken{
			Token:       newToken(),
			ClassroomID: classroom.ID,
			CreatedBy:   user.ID,
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(model.ImportTTL),
		}
		if err := h.store.CreateImportToken(r.Context(), tok); err != nil {
			writeInternal(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"import_token": tok.Token,
			"rows":         analyzed,
			"summary":      map[string]int{"ok": okCount, "rejected": len(analyzed) - okCount},
			"warnings":     warnings,
		})
		return
	}

	// commit: exige un token vigente del paso 1 para ESTA aula
	tok, terr := h.store.GetImportToken(r.Context(), r.FormValue("import_token"))
	if terr != nil || tok.ClassroomID != classroom.ID {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	created := 0
	for _, row := range analyzed {
		if row.Status != "ok" {
			continue
		}
		if err := h.altaAlumno(r, classroom.ID, row.Name, row.Email); err != nil {
			writeInternal(w)
			return
		}
		created++
	}
	_ = h.audit(r.Context(), user.ID, "students.import", "",
		fmt.Sprintf("%d alumnos dados de alta en %q", created, classroom.Name), middleware.ExtractIP(r))
	writeJSON(w, http.StatusCreated, map[string]any{
		"created": created,
		"omitted": len(analyzed) - created,
		"rows":    analyzed,
	})
}

// InviteStudent maneja POST /classrooms/{id}/students/invite (§9.3): alta
// individual al momento; 409 email_already_exists si ya tiene cuenta ([C5]).
func (h *Handler) InviteStudent(w http.ResponseWriter, r *http.Request) {
	classroom := h.getClassroomOwned(w, r)
	if classroom == nil {
		return
	}
	var req struct {
		Name       string `json:"name"`
		Email      string `json:"email"`
		SendInvite bool   `json:"send_invite"`
	}
	if err := decodeJSON(r, &req); err != nil ||
		strings.TrimSpace(req.Name) == "" || !validEmail(req.Email) {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	if _, err := h.store.GetUserByEmail(r.Context(), req.Email); err == nil {
		errors.WriteCode(w, errors.CodeEmailAlreadyExists)
		return
	}

	var stu *model.User
	if req.SendInvite {
		if err := h.altaAlumno(r, classroom.ID, req.Name, req.Email); err != nil {
			writeInternal(w)
			return
		}
	} else {
		now := time.Now()
		stu = &model.User{ID: newID(), Name: req.Name, Email: req.Email, Role: model.RoleAlumno, CreatedAt: now}
		if err := h.store.CreateUser(r.Context(), stu); err != nil {
			writeInternal(w)
			return
		}
		mem := &model.Membership{ClassroomID: classroom.ID, UserID: stu.ID, Status: model.MemberActive, CreatedAt: now}
		if err := h.store.AddMember(r.Context(), mem); err != nil {
			writeInternal(w)
			return
		}
	}

	user := middleware.UserFromContext(r.Context())
	_ = h.audit(r.Context(), user.ID, "student.invite", "", req.Email+" a "+classroom.Name, middleware.ExtractIP(r))

	resp := map[string]any{
		"student":               map[string]string{"name": req.Name, "email": req.Email},
		"first_magic_link_sent": req.SendInvite,
	}
	if stu != nil {
		resp["student"] = map[string]string{"id": stu.ID, "name": req.Name, "email": req.Email}
	}
	writeJSON(w, http.StatusCreated, resp)
}

// canSeeStudent: docente puede operar sobre alumnos de SUS aulas; director, todos.
func (h *Handler) canSeeStudent(r *http.Request, teacherID, studentID string) bool {
	classrooms, err := h.store.ListClassrooms(r.Context())
	if err != nil {
		return false
	}
	for _, c := range classrooms {
		if c.TeacherID != teacherID {
			continue
		}
		if _, err := h.store.GetMember(r.Context(), c.ID, studentID); err == nil {
			return true
		}
	}
	return false
}

// ResendMagicLink maneja POST /students/{user_id}/resend-magic-link (§9.3):
// 202 SIEMPRE con respuesta genérica ([C2] anti-enumeración).
func (h *Handler) ResendMagicLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	targetID := r.PathValue("user_id")

	allowed := user.Role == model.RoleDirector
	if !allowed {
		allowed = h.canSeeStudent(r, user.ID, targetID)
	}
	if allowed {
		if target, err := h.store.GetUser(r.Context(), targetID); err == nil &&
			target.Role == model.RoleAlumno && !target.Disabled {
			now := time.Now()
			mt := &model.MagicToken{
				Token:     newToken(),
				UserID:    target.ID,
				Context:   model.MagicContextPairingPWA,
				CreatedAt: now,
				ExpiresAt: now.Add(model.MagicLinkTTL),
			}
			if err := h.store.CreateMagicToken(r.Context(), mt); err == nil {
				link := schemeFromRequest(r) + "://" + r.Host + "/auth/consume?token=" + mt.Token
				h.sendMagicLink(target.Email, link)
			}
		}
	}
	writeJSON(w, http.StatusAccepted, magicLinkResponse())
}
