package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// --- Catálogo default de templates (§10.2): hardcoded por instancia. ---
// PackagesAllowlist es el allowlist del template (§7): packages_extra debe ser
// subconjunto; custom_dockerfile OFF en MVP, imágenes fijas por template.

type Template struct {
	ID                string
	Label             string
	ModeDefault       model.SandboxMode
	Runtimes          []model.Runtime
	PackagesAllowlist []string
	StartCommand      string
}

var catalog = []Template{
	{
		ID: "python/numpy", Label: "Python + NumPy", ModeDefault: model.ModeJob,
		Runtimes: []model.Runtime{model.RuntimePython},
		PackagesAllowlist: []string{"numpy", "pandas", "matplotlib", "scipy"},
		StartCommand:      "python main.py",
	},
	{
		ID: "bun/react", Label: "Bun + React", ModeDefault: model.ModeService,
		Runtimes: []model.Runtime{model.RuntimeWeb},
		PackagesAllowlist: []string{"zod", "axios", "dayjs"},
		StartCommand:      "bun run dev",
	},
	{
		ID: "cpp/sqlite", Label: "C++ + SQLite", ModeDefault: model.ModeJob,
		Runtimes: []model.Runtime{model.RuntimeCpp},
		StartCommand: "./main",
	},
	{
		ID: "arduino", Label: "Arduino", ModeDefault: model.ModeJob,
		Runtimes: []model.Runtime{model.RuntimeArduino},
		StartCommand: "arduino-cli compile --input main.ino",
	},
}

func findTemplate(id string) *Template {
	for i := range catalog {
		if catalog[i].ID == id {
			return &catalog[i]
		}
	}
	return nil
}

// allowedTemplates resuelve settings de sala: lista vacía → catálogo completo.
func allowedTemplates(c *model.Classroom) map[string]bool {
	out := make(map[string]bool, len(catalog))
	for _, t := range catalog {
		if len(c.AllowedTemplates) == 0 || containsStr(c.AllowedTemplates, t.ID) {
			out[t.ID] = true
		}
	}
	return out
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// --- Runner seam (§7 Plan B): inyectable; FakeRunner simula transiciones ---

type Runner interface {
	Available(ctx context.Context) bool
}

type FakeRunner struct{}

func (FakeRunner) Available(context.Context) bool { return true }

type deadRunner struct{}

func (deadRunner) Available(context.Context) bool { return false }

// SetRunner reemplaza el runner (tests: deadRunner simula runner caído).
func (h *Handler) SetRunner(r Runner) { h.runner = r }

// Step avanza todos los runs un paso de la máquina de estados §7 y promueve
// de la cola mientras haya presupuesto. El driver real lo llama un ticker
// (main.go); los tests lo llaman manualmente para polls determinísticos.
func (h *Handler) Step(ctx context.Context) {
	now := time.Now()
	runs, err := h.store.ListRuns(ctx)
	if err != nil {
		return
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.Before(runs[j].CreatedAt) })

	active := 0
	var queued []*model.Run
	sandboxes := make(map[string]*model.Sandbox)
	for i := range runs {
		sbID := runs[i].SandboxID
		if _, ok := sandboxes[sbID]; !ok {
			if sb, gerr := h.store.GetSandbox(ctx, sbID); gerr == nil {
				sandboxes[sbID] = sb
			}
		}
		switch {
		case runs[i].Status.ActiveContainer():
			active++
		case runs[i].Status == model.RunQueued:
			queued = append(queued, &runs[i])
		}
	}

	promote := func() {
		for active < h.cfg.SandboxMaxContainers && len(queued) > 0 {
			r := queued[0]
			queued = queued[1:]
			r.Status = model.RunDownloadingImage
			r.StartedAt = now
			h.appendLog(r, "[runner] descargando imagen")
			_ = h.store.UpdateRun(ctx, r)
			active++
		}
	}

	for i := range runs {
		r := &runs[i]
		switch r.Status {
		case model.RunDownloadingImage:
			r.Status = model.RunStarting
			h.appendLog(r, "[runner] iniciando contenedor")
			_ = h.store.UpdateRun(ctx, r)
		case model.RunStarting:
			r.Status = model.RunReady
			h.appendLog(r, "[runner] contenedor listo")
			_ = h.store.UpdateRun(ctx, r)
		case model.RunReady:
			r.Status = model.RunRunning
			if sandboxes[r.SandboxID] != nil && sandboxes[r.SandboxID].Mode == model.ModeService {
				r.ServiceURL = "/s/" + r.SandboxID
			}
			h.appendLog(r, "[runner] ejecutando: hola mundo")
			_ = h.store.UpdateRun(ctx, r)
		case model.RunRunning:
			if sandboxes[r.SandboxID] != nil && sandboxes[r.SandboxID].Mode == model.ModeJob {
				ec := 0
				r.ExitCode = &ec
				r.Status = model.RunSucceeded
				h.appendLog(r, "[runner] exit 0")
				_ = h.store.UpdateRun(ctx, r)
			}
		case model.RunSucceeded:
			r.Status = model.RunCleaned
			h.appendLog(r, "[runner] contenedor liberado")
			_ = h.store.UpdateRun(ctx, r)
			if sb := sandboxes[r.SandboxID]; sb != nil {
				h.cleanEphemeral(ctx, sb)
			}
		}
	}
	promote()
}

func (h *Handler) appendLog(r *model.Run, line string) {
	r.Logs = append(r.Logs, model.LogLine{Stream: "stdout", Line: line})
}

// latestTestResult busca el último run terminado con exit_code de los
// sandboxes purpose=submission_test ligados a la entrega (§5.2/§5.3).
func (h *Handler) latestTestResult(ctx context.Context, submissionID string) *model.TestResult {
	sandboxes, err := h.store.ListSandboxes(ctx)
	if err != nil {
		return nil
	}
	var best *model.TestResult
	for _, sb := range sandboxes {
		if sb.Purpose != model.PurposeSubmissionTest || sb.SubmissionID != submissionID {
			continue
		}
		runs, rerr := h.store.ListRunsBySandbox(ctx, sb.ID)
		if rerr != nil {
			continue
		}
		for i := range runs {
			r := &runs[i]
			if r.ExitCode == nil {
				continue
			}
			best = &model.TestResult{RunID: r.ID, ExitCode: *r.ExitCode}
		}
	}
	return best
}

func (h *Handler) cleanEphemeral(ctx context.Context, sb *model.Sandbox) {
	if sb.Retention != model.RetentionEphemeral || sb.Cleaned {
		return
	}
	sb.Cleaned = true
	_ = h.store.UpdateSandbox(ctx, sb)
}

// --- Vistas ---

func templateView(t Template) map[string]any {
	return map[string]any{
		"id": t.ID, "label": t.Label,
		"mode_default": string(t.ModeDefault),
		"runtimes":     t.Runtimes,
	}
}

func runHistory(ctx context.Context, h *Handler, sandboxID string) ([]model.RunHistoryEntry, error) {
	runs, err := h.store.ListRunsBySandbox(ctx, sandboxID)
	if err != nil {
		return nil, err
	}
	var out []model.RunHistoryEntry
	n := 0
	for _, r := range runs {
		n++
		if r.ExitCode == nil {
			continue
		}
		outcome := "success"
		switch {
		case r.Status == model.RunFailed:
			outcome = "timeout"
		case *r.ExitCode != 0:
			outcome = "error"
		}
		out = append(out, model.RunHistoryEntry{N: n, Outcome: outcome})
	}
	return out, nil
}

func runView(r *model.Run, history []model.RunHistoryEntry) map[string]any {
	var exitCode any
	if r.ExitCode != nil {
		exitCode = *r.ExitCode
	}
	var startedAt any
	if !r.StartedAt.IsZero() {
		startedAt = r.StartedAt
	}
	return map[string]any{
		"run_id":      r.ID,
		"sandbox_id":  r.SandboxID,
		"status":      string(r.Status),
		"exit_code":   exitCode,
		"service_url": r.ServiceURL,
		"started_at":  startedAt,
		"history":     history,
	}
}

// --- GET /templates?classroom_id= (§7) ---

func requireMiembroOStaff(next http.Handler) http.Handler {
	return middleware.RequireRole(model.RoleAlumno, model.RoleDocente)(next)
}

func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	items := make([]map[string]any, 0, len(catalog))
	classroomID := r.URL.Query().Get("classroom_id")
	if classroomID == "" {
		for _, t := range catalog {
			items = append(items, templateView(t))
		}
		writeJSON(w, http.StatusOK, map[string]any{"templates": items})
		return
	}

	c, err := h.store.GetClassroom(r.Context(), classroomID)
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	if user.Role == model.RoleAlumno {
		if _, merr := h.store.GetMember(r.Context(), c.ID, user.ID); merr != nil {
			errors.WriteCode(w, errors.CodeForbidden)
			return
		}
	}
	allowed := allowedTemplates(c)
	for _, t := range catalog {
		if allowed[t.ID] {
			items = append(items, templateView(t))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": items})
}

// --- PATCH /classrooms/{id}/settings (§10.2/G4) ---

func (h *Handler) PatchClassroomSettings(w http.ResponseWriter, r *http.Request) {
	c := h.getClassroomOwned(w, r)
	if c == nil {
		return
	}
	var req struct {
		AllowedTemplates        *[]string `json:"allowed_templates"`
		CustomDockerfileEnabled *bool     `json:"custom_dockerfile_enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	if req.AllowedTemplates != nil {
		if len(*req.AllowedTemplates) == 0 {
			errors.WriteCode(w, errors.CodeValidationError, "allowed_templates no puede estar vacío")
			return
		}
		for _, id := range *req.AllowedTemplates {
			if findTemplate(id) == nil {
				errors.WriteCode(w, errors.CodeValidationError, "template desconocido: "+id)
				return
			}
		}
		c.AllowedTemplates = *req.AllowedTemplates
	}
	if req.CustomDockerfileEnabled != nil {
		c.CustomDockerfileEnabled = *req.CustomDockerfileEnabled
	}
	if err := h.store.UpdateClassroom(r.Context(), c); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"classroom_id":              c.ID,
		"allowed_templates":         c.AllowedTemplates,
		"custom_dockerfile_enabled": c.CustomDockerfileEnabled,
	})
}

// --- POST /sandboxes (§7): presupuesto, cola dura, plan B runner caído ---

type createSandboxReq struct {
	TemplateID    string `json:"template_id"`
	PackagesExtra string `json:"packages_extra"`
	Mode          string `json:"mode"`
	StartCommand  string `json:"start_command"`
	Purpose       string `json:"purpose"`
	SubmissionID  string `json:"submission_id"`
	Retention     string `json:"retention"`
	Visibility    string `json:"visibility"` // extensión local: habilita lectura invitado (§7 listado)
}

func (h *Handler) CreateSandbox(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())

	if !h.runner.Available(r.Context()) {
		errors.WriteCode(w, errors.CodeCapacityUnavailable)
		return
	}

	var req createSandboxReq
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.TemplateID) == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	tpl := findTemplate(req.TemplateID)
	if tpl == nil {
		errors.WriteCode(w, errors.CodeValidationError, "template desconocido: "+req.TemplateID)
		return
	}
	mode := tpl.ModeDefault
	if req.Mode != "" {
		if model.SandboxMode(req.Mode) != model.ModeJob && model.SandboxMode(req.Mode) != model.ModeService {
			errors.WriteCode(w, errors.CodeValidationError, "mode inválido")
			return
		}
		mode = model.SandboxMode(req.Mode)
	}
	retention := model.RetentionHistorical
	if req.Retention != "" {
		if model.SandboxRetention(req.Retention) != model.RetentionHistorical &&
			model.SandboxRetention(req.Retention) != model.RetentionEphemeral {
			errors.WriteCode(w, errors.CodeValidationError, "retention inválido")
			return
		}
		retention = model.SandboxRetention(req.Retention)
	}
	visibility := model.VisClassroom
	if req.Visibility != "" {
		if model.Visibility(req.Visibility) != model.VisPublic &&
			model.Visibility(req.Visibility) != model.VisSchool &&
			model.Visibility(req.Visibility) != model.VisClassroom {
			errors.WriteCode(w, errors.CodeValidationError, "visibility inválido")
			return
		}
		visibility = model.Visibility(req.Visibility)
	}
	purpose := model.PurposeStandalone
	if req.Purpose != "" {
		if model.SandboxPurpose(req.Purpose) != model.PurposeStandalone &&
			model.SandboxPurpose(req.Purpose) != model.PurposeSubmissionTest {
			errors.WriteCode(w, errors.CodeValidationError, "purpose inválido")
			return
		}
		purpose = model.SandboxPurpose(req.Purpose)
	}

	// allowlist del template (§7)
	var extra []string
	if strings.TrimSpace(req.PackagesExtra) != "" {
		extra = strings.Fields(req.PackagesExtra)
		for _, p := range extra {
			if !containsStr(tpl.PackagesAllowlist, p) {
				errors.WriteCode(w, errors.CodeValidationError,
					fmt.Sprintf("paquete %q fuera del allowlist de %s", p, tpl.ID))
				return
			}
		}
	}

	// contexto de aula y autorización
	var classroomID string
	if purpose == model.PurposeSubmissionTest {
		if req.SubmissionID == "" {
			errors.WriteCode(w, errors.CodeValidationError, "submission_id requerido con purpose=submission_test")
			return
		}
		sub, serr := h.store.GetSubmission(r.Context(), req.SubmissionID)
		if serr != nil {
			errors.WriteCode(w, errors.CodeNotFound)
			return
		}
		a, aerr := h.store.GetAssignment(r.Context(), sub.AssignmentID)
		if aerr != nil {
			errors.WriteCode(w, errors.CodeNotFound)
			return
		}
		classroomID = a.ClassroomID
		if user.Role == model.RoleAlumno && sub.StudentID != user.ID {
			errors.WriteCode(w, errors.CodeForbidden)
			return
		}
	} else if user.Role == model.RoleAlumno {
		mems, merr := h.store.ListMembershipsByUser(r.Context(), user.ID)
		if merr != nil || len(mems) == 0 {
			errors.WriteCode(w, errors.CodeForbidden)
			return
		}
		ok := false
		for _, mem := range mems {
			if c, cerr := h.store.GetClassroom(r.Context(), mem.ClassroomID); cerr == nil && allowedTemplates(c)[tpl.ID] {
				classroomID = c.ID
				ok = true
				break
			}
		}
		if !ok {
			errors.WriteCode(w, errors.CodeValidationError, "template no permitido en tus aulas")
			return
		}
	} else if user.Role == model.RoleDocente {
		for _, c := range mustListClassrooms(h, r) {
			if c.TeacherID == user.ID {
				classroomID = c.ID
				break
			}
		}
	}

	if classroomID != "" {
		if c, cerr := h.store.GetClassroom(r.Context(), classroomID); cerr == nil && !allowedTemplates(c)[tpl.ID] {
			errors.WriteCode(w, errors.CodeValidationError, "template no permitido en esta sala")
			return
		}
	}

	startCmd := tpl.StartCommand
	if strings.TrimSpace(req.StartCommand) != "" {
		startCmd = strings.TrimSpace(req.StartCommand)
	}

	resp, ok := h.launchRun(w, r, &model.Sandbox{
		ID:            newID(),
		TemplateID:    tpl.ID,
		CreatedBy:     user.ID,
		ClassroomID:   classroomID,
		Mode:          mode,
		StartCommand:  startCmd,
		PackagesExtra: strings.Join(extra, " "),
		Purpose:       purpose,
		SubmissionID:  req.SubmissionID,
		Retention:     retention,
		Visibility:    visibility,
		CreatedAt:     time.Now(),
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func mustListClassrooms(h *Handler, r *http.Request) []model.Classroom {
	cs, _ := h.store.ListClassrooms(r.Context())
	return cs
}

// launchRun aplica presupuesto/cola (§0.3/§7), persiste sandbox+run y arma la
// respuesta 202. Presupuesto lleno → entra en cola; cola dura llena → 503.
func (h *Handler) launchRun(w http.ResponseWriter, r *http.Request, sb *model.Sandbox) (map[string]any, bool) {
	ctx := r.Context()
	active, queued, err := h.budget(ctx)
	if err != nil {
		writeInternal(w)
		return nil, false
	}
	if active >= h.cfg.SandboxMaxContainers && queued >= h.cfg.SandboxQueueLimit {
		errors.WriteCode(w, errors.CodeCapacityUnavailable)
		return nil, false
	}
	if err := h.store.CreateSandbox(ctx, sb); err != nil {
		writeInternal(w)
		return nil, false
	}
	run := &model.Run{
		ID:        newID(),
		SandboxID: sb.ID,
		N:         1,
		Status:    model.RunQueued,
		CreatedAt: time.Now(),
	}
	run.Logs = append(run.Logs, model.LogLine{Stream: "stdout", Line: "[runner] encolado"})
	if err := h.store.CreateRun(ctx, run); err != nil {
		writeInternal(w)
		return nil, false
	}
	return queueResponse(active, queued, h, sb.ID, run.ID), true
}

// budget cuenta contenedores prendidos y runs en cola (§0.3).
func (h *Handler) budget(ctx context.Context) (active, queued int, err error) {
	all, err := h.store.ListRuns(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, r := range all {
		switch {
		case r.Status.ActiveContainer():
			active++
		case r.Status == model.RunQueued:
			queued++
		}
	}
	return active, queued, nil
}

func (h *Handler) isClassroomStaff(r *http.Request, user *model.User, classroomID string) bool {
	c, err := h.store.GetClassroom(r.Context(), classroomID)
	return err == nil && canManageClassroom(user, c)
}

func queueResponse(active, queued int, h *Handler, sandboxID, runID string) map[string]any {
	return map[string]any{
		"sandbox_id":     sandboxID,
		"run_id":         runID,
		"status":         "queued",
		"queue_position": queued + 1,
		"budget_note": fmt.Sprintf("%d de %d sandboxes prendidos",
			min(active, h.cfg.SandboxMaxContainers), h.cfg.SandboxMaxContainers),
	}
}

// --- GET /sandboxes — listado según quien pregunta (§7) ---

func (h *Handler) ListSandboxes(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	all, err := h.store.ListSandboxes(r.Context())
	if err != nil {
		writeInternal(w)
		return
	}

	var myAulas map[string]bool
	if user != nil && user.Role == model.RoleDocente {
		myAulas = make(map[string]bool)
		for _, c := range mustListClassrooms(h, r) {
			if c.TeacherID == user.ID {
				myAulas[c.ID] = true
			}
		}
	}

	items := []map[string]any{}
	for i := range all {
		sb := &all[i]
		if sb.Cleaned {
			continue // ephemeral limpiado: ni listado
		}
		publicReadable := sb.Retention == model.RetentionHistorical &&
			(sb.Visibility == model.VisPublic || sb.Visibility == model.VisSchool)

		switch {
		case user == nil || user.Role == model.RoleInvitado:
			if !publicReadable {
				continue
			}
		case user.Role == model.RoleDirector:
			// ve todo
		case user.Role == model.RoleAlumno:
			if sb.CreatedBy != user.ID {
				continue
			}
		default: // docente
			if sb.CreatedBy != user.ID && !(sb.ClassroomID != "" && myAulas[sb.ClassroomID]) {
				continue
			}
		}
		items = append(items, h.sandboxListItem(r, sb))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sandboxes": items})
}

func (h *Handler) sandboxListItem(r *http.Request, sb *model.Sandbox) map[string]any {
	v := map[string]any{
		"id":          sb.ID,
		"template_id": sb.TemplateID,
		"mode":        string(sb.Mode),
		"purpose":     string(sb.Purpose),
		"retention":   string(sb.Retention),
		"created_at":  sb.CreatedAt,
	}
	if latest, err := h.latestRun(r.Context(), sb.ID); err == nil {
		v["status"] = string(latest.Status)
	}
	if u, uerr := h.store.GetUser(r.Context(), sb.CreatedBy); uerr == nil {
		v["author"] = u.Name
	}
	return v
}

func (h *Handler) latestRun(ctx context.Context, sandboxID string) (*model.Run, error) {
	runs, err := h.store.ListRunsBySandbox(ctx, sandboxID)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[len(runs)-1], nil
}

// canReadSandbox: visibilidad §6 aplicada a registros (public/school cualquiera;
// classroom: autor/miembro/docente dueño/director).
func (h *Handler) canReadSandbox(ctx context.Context, user *model.User, sb *model.Sandbox, w http.ResponseWriter) (*model.User, bool) {
	if user == nil || user.Role == model.RoleInvitado {
		if sb.Retention == model.RetentionHistorical &&
			(sb.Visibility == model.VisPublic || sb.Visibility == model.VisSchool) {
			return user, true
		}
		if user == nil {
			errors.WriteCode(w, errors.CodeUnauthenticated)
		} else {
			errors.WriteCode(w, errors.CodeGuestReadOnly)
		}
		return nil, false
	}
	if user.Role == model.RoleDirector || sb.CreatedBy == user.ID {
		return user, true
	}
	if sb.ClassroomID != "" {
		if c, cerr := h.store.GetClassroom(ctx, sb.ClassroomID); cerr == nil {
			if canManageClassroom(user, c) {
				return user, true
			}
			if user.Role == model.RoleAlumno {
				if _, merr := h.store.GetMember(ctx, c.ID, user.ID); merr == nil {
					return user, true
				}
			}
		}
	}
	errors.WriteCode(w, errors.CodeForbidden)
	return nil, false
}

// --- GET /sandboxes/{id}, POST /sandboxes/{id}/instantiate ---

func (h *Handler) loadVisibleSandbox(w http.ResponseWriter, r *http.Request) (*model.Sandbox, *model.User, bool) {
	sb, err := h.store.GetSandbox(r.Context(), r.PathValue("id"))
	if err != nil || sb.Cleaned {
		errors.WriteCode(w, errors.CodeNotFound)
		return nil, nil, false
	}
	user := middleware.UserFromContext(r.Context())
	u, ok := h.canReadSandbox(r.Context(), user, sb, w)
	if !ok {
		return nil, nil, false
	}
	return sb, u, true
}

func (h *Handler) GetSandboxDetail(w http.ResponseWriter, r *http.Request) {
	sb, _, ok := h.loadVisibleSandbox(w, r)
	if !ok {
		return
	}
	item := h.sandboxListItem(r, sb)
	item["start_command"] = sb.StartCommand
	item["packages_extra"] = sb.PackagesExtra
	item["visibility"] = string(sb.Visibility)

	hist, herr := runHistory(r.Context(), h, sb.ID)
	runs, rerr := h.store.ListRunsBySandbox(r.Context(), sb.ID)
	if rerr == nil && herr == nil {
		rv := make([]map[string]any, 0, len(runs))
		for i := range runs {
			rv = append(rv, runView(&runs[i], hist))
		}
		item["runs"] = rv
	}
	writeJSON(w, http.StatusOK, item)
}

// InstantiateSandbox maneja POST /sandboxes/{id}/instantiate (§7): relanza un
// run nuevo desde la config cacheada del registro historical.
func (h *Handler) InstantiateSandbox(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if !h.runner.Available(r.Context()) {
		errors.WriteCode(w, errors.CodeCapacityUnavailable)
		return
	}
	sb, err := h.store.GetSandbox(r.Context(), r.PathValue("id"))
	if err != nil || sb.Cleaned || sb.Retention != model.RetentionHistorical {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	if user == nil || !(user.Role == model.RoleDirector || sb.CreatedBy == user.ID ||
		(sb.ClassroomID != "" && h.isClassroomStaff(r, user, sb.ClassroomID))) {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}

	prev, lerr := h.latestRun(r.Context(), sb.ID)
	if lerr != nil || prev == nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	run := &model.Run{
		ID:        newID(),
		SandboxID: sb.ID,
		N:         prev.N + 1,
		Status:    model.RunQueued,
		CreatedAt: time.Now(),
	}
	run.Logs = append(run.Logs, model.LogLine{Stream: "stdout", Line: "[runner] re-instanciado"})
	active, queued, berr := h.budget(r.Context())
	if berr != nil {
		writeInternal(w)
		return
	}
	if active >= h.cfg.SandboxMaxContainers && queued >= h.cfg.SandboxQueueLimit {
		errors.WriteCode(w, errors.CodeCapacityUnavailable)
		return
	}
	if err := h.store.CreateRun(r.Context(), run); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusAccepted, queueResponse(active, queued, h, sb.ID, run.ID))
}

// --- GET /runs/{run_id} · GET /runs/{run_id}/logs · POST /runs/{run_id}/stop ---

func (h *Handler) loadRunForRead(w http.ResponseWriter, r *http.Request) (*model.Run, *model.Sandbox, *model.User, bool) {
	run, err := h.store.GetRun(r.Context(), r.PathValue("run_id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return nil, nil, nil, false
	}
	sb, err := h.store.GetSandbox(r.Context(), run.SandboxID)
	if err != nil || sb.Cleaned {
		errors.WriteCode(w, errors.CodeNotFound)
		return nil, nil, nil, false
	}
	user := middleware.UserFromContext(r.Context())
	u, ok := h.canReadSandbox(r.Context(), user, sb, w)
	if !ok {
		return nil, nil, nil, false
	}
	// invitados/anónimos: solo runs históricos terminados (§7 logs)
	if u == nil || u.Role == model.RoleInvitado {
		if !run.Status.Terminal() {
			errors.WriteCode(w, errors.CodeForbidden)
			return nil, nil, nil, false
		}
	}
	return run, sb, u, true
}

// canManageRun: autor, docente del aula o director (§7 stop/logs).
func (h *Handler) canManageRun(w http.ResponseWriter, r *http.Request, run *model.Run, sb *model.Sandbox) (*model.User, bool) {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		errors.WriteCode(w, errors.CodeUnauthenticated)
		return nil, false
	}
	if user.Role == model.RoleDirector || sb.CreatedBy == user.ID ||
		(sb.ClassroomID != "" && h.isClassroomStaff(r, user, sb.ClassroomID)) {
		return user, true
	}
	errors.WriteCode(w, errors.CodeForbidden)
	return nil, false
}

func (h *Handler) GetRunStatus(w http.ResponseWriter, r *http.Request) {
	run, _, _, ok := h.loadRunForRead(w, r)
	if !ok {
		return
	}
	hist, herr := runHistory(r.Context(), h, run.SandboxID)
	if herr != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusOK, runView(run, hist))
}

// RunLogs maneja GET /runs/{run_id}/logs (§7): SSE text/event-stream con
// eventos log/status/exit. El fake runner deja las líneas pre-generadas; se
// reproducen y se corta el stream (sin Docker no hay cola viva).
func (h *Handler) RunLogs(w http.ResponseWriter, r *http.Request) {
	run, _, _, ok := h.loadRunForRead(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	send := func(event string, payload any) {
		b, _ := json.Marshal(payload)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	for _, l := range run.Logs {
		send("log", l)
	}
	send("status", map[string]string{"status": string(run.Status)})
	if run.Status.Terminal() {
		exitCode := 0
		if run.ExitCode != nil {
			exitCode = *run.ExitCode
		}
		send("exit", map[string]any{"exit_code": exitCode, "status": string(run.Status)})
	}
}

func (h *Handler) StopRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.store.GetRun(r.Context(), r.PathValue("run_id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	sb, err := h.store.GetSandbox(r.Context(), run.SandboxID)
	if err != nil || sb.Cleaned {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}
	if _, ok := h.canManageRun(w, r, run, sb); !ok {
		return
	}

	if run.Status != model.RunCleaned {
		run.Status = model.RunCleaned
		h.appendLog(run, "[runner] detenido por el usuario")
		if err := h.store.UpdateRun(r.Context(), run); err != nil {
			writeInternal(w)
			return
		}
		h.cleanEphemeral(r.Context(), sb)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cleaned"})
}
