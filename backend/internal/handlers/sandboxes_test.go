package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

// --- fixture FASE 6 ---

type fxSandboxes struct {
	h      *Handler
	doc    *http.Cookie // dueño del aula
	dir    *http.Cookie
	alum   *http.Cookie // miembro del aula
	fuera  *http.Cookie // alumno sin aulas
	guest  *http.Cookie
	aulaID string
}

func seedSandboxes(t *testing.T, maxContainers, queueLimit int) *fxSandboxes {
	t.Helper()
	cfg := &config.Config{
		RateLimitRPH:         10000,
		Env:                  "development",
		SandboxMaxContainers: maxContainers,
		SandboxQueueLimit:    queueLimit,
	}
	f := &fxSandboxes{h: NewRouter(cfg, store.NewMemStore())}

	docID := seedUser(t, f.h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	f.doc = sesionDeRol(t, f.h, docID, model.SessionKindStaff)
	dirID := seedUser(t, f.h, "dir@escuela.edu.ar", "p", model.RoleDirector, false)
	f.dir = sesionDeRol(t, f.h, dirID, model.SessionKindStaff)
	alumID := seedUser(t, f.h, "alum@escuela.edu.ar", "", model.RoleAlumno, false)
	f.alum = sesionDeRol(t, f.h, alumID, model.SessionKindPWA)
	fueraID := seedUser(t, f.h, "fuera@escuela.edu.ar", "", model.RoleAlumno, false)
	f.fuera = sesionDeRol(t, f.h, fueraID, model.SessionKindPWA)
	invID := seedUser(t, f.h, "inv@escuela.edu.ar", "", model.RoleInvitado, false)
	f.guest = sesionDeRol(t, f.h, invID, model.SessionKindGuest)

	aula := crearAula(t, f.h, f.doc, "Aula Sandboxes")
	f.aulaID = aula["id"].(string)
	if got := unirse(t, f.h, aula["join_code"].(string), f.alum); *got != 200 {
		t.Fatalf("join alum: %d", *got)
	}
	return f
}

func crearSandboxOK(t *testing.T, h *Handler, cookie *http.Cookie, body string) map[string]any {
	t.Helper()
	rec := doJSON(t, h, "POST", "/sandboxes", body, cookie)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("crear sandbox: esperaba 202, got %d (%s)", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func stepHasta(t *testing.T, h *Handler, cookie *http.Cookie, runID, want string, maxSteps int) map[string]any {
	t.Helper()
	for i := 0; i < maxSteps; i++ {
		h.Step(t.Context())
		rec := doJSON(t, h, "GET", "/runs/"+runID, "", cookie)
		if rec.Code == http.StatusNotFound {
			continue
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["status"] == want {
			return out
		}
	}
	t.Fatalf("run %s nunca llegó a %s tras %d steps", runID, want, maxSteps)
	return nil
}

func runStatusDe(t *testing.T, h *Handler, cookie *http.Cookie, runID string) map[string]any {
	t.Helper()
	rec := doJSON(t, h, "GET", "/runs/"+runID, "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET run: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func crearConsignaRapida(t *testing.T, f *fxSandboxes) (assignmentID string) {
	t.Helper()
	body := `{"title":"TP","instructions":"x","runtime":"python","attempts":{"mode":"unlimited"}}`
	rec := doJSON(t, f.h, "POST", "/classrooms/"+f.aulaID+"/assignments", body, f.doc)
	if rec.Code != 201 {
		t.Fatalf("crear consigna: %d (%s)", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out["id"].(string)
}

func subirDraft(t *testing.T, f *fxSandboxes, assignmentID string) string {
	t.Helper()
	up := subirArchivos(t, f.h, fmt.Sprintf("/assignments/%s/submissions/files", assignmentID),
		map[string]string{"main.py": "print('hola')"}, f.alum)
	if up.Code != 201 {
		t.Fatalf("upload draft: esperaba 201, got %d (%s)", up.Code, up.Body.String())
	}
	var out struct {
		SubmissionID string `json:"submission_id"`
	}
	if err := json.Unmarshal(up.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.SubmissionID
}

// --- Templates y settings de sala (§10.2/G4) ---

func TestTemplates_YSettingsPorSala(t *testing.T) {
	f := seedSandboxes(t, 4, 8)

	cases := []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"alumno miembro", f.alum, 200},
		{"docente dueño", f.doc, 200},
		{"director", f.dir, 200},
		{"alumno fuera", f.fuera, 403},
		{"invitado", f.guest, 403},
		{"anónimo", nil, 401},
	}
	for _, tc := range cases {
		t.Run("templates "+tc.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if tc.cookie == nil { // anónimo: doJSON no tolera cookies nil
				rec = doJSON(t, f.h, "GET", "/templates?classroom_id="+f.aulaID, "")
			} else {
				rec = doJSON(t, f.h, "GET", "/templates?classroom_id="+f.aulaID, "", tc.cookie)
			}
			if rec.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
			if tc.want != 200 || !strings.Contains(rec.Body.String(), "arduino") ||
				!strings.Contains(rec.Body.String(), `"mode_default"`) {
				return
			}
			var out struct {
				Templates []map[string]any `json:"templates"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if len(out.Templates) != 4 {
				t.Fatalf("default: catálogo completo (4), got %d", len(out.Templates))
			}
		})
	}

	rec := doJSON(t, f.h, "GET", "/templates", "", f.alum)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "arduino") {
		t.Fatalf("sin classroom_id: catálogo completo; got %d %s", rec.Code, rec.Body.String())
	}

	patch := doJSON(t, f.h, "PATCH", "/classrooms/"+f.aulaID+"/settings",
		`{"allowed_templates":["python/numpy"],"custom_dockerfile_enabled":true}`, f.doc)
	if patch.Code != 200 {
		t.Fatalf("patch settings: esperaba 200, got %d (%s)", patch.Code, patch.Body.String())
	}
	rec = doJSON(t, f.h, "GET", "/templates?classroom_id="+f.aulaID, "", f.alum)
	if !strings.Contains(rec.Body.String(), "python/numpy") || strings.Contains(rec.Body.String(), "arduino") {
		t.Fatalf("settings no filtraron el catálogo: %s", rec.Body.String())
	}

	casesErr := []struct {
		name   string
		cookie *http.Cookie
		body   string
		want   int
	}{
		{"invitado", f.guest, `{"allowed_templates":["python/numpy"]}`, 403},
		{"alumno", f.alum, `{}`, 403},
		{"template desconocido", f.doc, `{"allowed_templates":["rust/tokio"]}`, 400},
		{"lista vacía", f.doc, `{"allowed_templates":[]}`, 400},
	}
	for _, tc := range casesErr {
		t.Run("patch "+tc.name, func(t *testing.T) {
			rec := doJSON(t, f.h, "PATCH", "/classrooms/"+f.aulaID+"/settings", tc.body, tc.cookie)
			if rec.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- POST /sandboxes: validaciones §7 ---

func TestCrearSandbox_Validaciones(t *testing.T) {
	f := seedSandboxes(t, 4, 8)

	base := func(extra string) string {
		b := `{"template_id":"python/numpy"`
		if extra != "" {
			b += "," + extra
		}
		return b + "}"
	}

	cases := []struct {
		name   string
		cookie *http.Cookie
		body   string
		want   int
		code   string
	}{
		{"feliz", f.alum, base(`"packages_extra":"pandas"`), 202, ""},
		{"allowlist violada", f.alum, base(`"packages_extra":"pandas torch"`), 400, "validation_error"},
		{"template desconocido", f.alum, `{"template_id":"rust/tokio"}`, 400, "validation_error"},
		{"mode inválido", f.alum, base(`"mode":"daemon"`), 400, "validation_error"},
		{"retention inválido", f.alum, base(`"retention":"forever"`), 400, "validation_error"},
		{"purpose inválido", f.alum, base(`"purpose":"grader"`), 400, "validation_error"},
		{"submission_test sin submission_id", f.alum, base(`"purpose":"submission_test"`), 400, "validation_error"},
		{"submission inexistente", f.alum, base(`"purpose":"submission_test","submission_id":"nope"`), 404, "not_found"},
		{"alumno sin aulas", f.fuera, base(""), 403, ""},
		{"invitado no crea", f.guest, base(""), 403, "guest_read_only"},
		{"director crea", f.dir, base(""), 202, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, f.h, "POST", "/sandboxes", tc.body, tc.cookie)
			if rec.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
			if tc.code != "" && errCode(t, rec) != tc.code {
				t.Fatalf("esperaba code %s, got %s", tc.code, errCode(t, rec))
			}
		})
	}
}

// --- Presupuesto §0.3: 202+cola cuando lleno; 503 solo con la cola dura llena ---

func TestPresupuesto_ColaYColaLlena(t *testing.T) {
	f := seedSandboxes(t, 2, 1)

	a := crearSandboxOK(t, f.h, f.alum, `{"template_id":"python/numpy"}`)
	f.h.Step(t.Context()) // A promovido
	crearSandboxOK(t, f.h, f.alum, `{"template_id":"bun/react"}`)
	f.h.Step(t.Context()) // A y B activos (2/2)

	if got := runStatusDe(t, f.h, f.alum, a["run_id"].(string))["status"]; got != "starting" {
		t.Fatalf("A esperaba starting (promovido en el step 1), got %v", got)
	}

	// presupuesto lleno → 202 Accepted + queue_position, el run ENTRA EN COLA
	rec := doJSON(t, f.h, "POST", "/sandboxes", `{"template_id":"python/numpy"}`, f.alum)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("presupuesto lleno: esperaba 202, got %d (%s)", rec.Code, rec.Body.String())
	}
	var c map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c["status"] != "queued" || c["queue_position"].(float64) != 1 {
		t.Fatalf("esperaba queued/pos 1, got %v/%v", c["status"], c["queue_position"])
	}
	if !strings.Contains(c["budget_note"].(string), "2 de 2") {
		t.Fatalf("budget_note: %v", c["budget_note"])
	}

	// cola dura (1) llena → 503 capacity_unavailable SIN crear nada
	listaAntes := doJSON(t, f.h, "GET", "/sandboxes", "", f.doc)
	rec = doJSON(t, f.h, "POST", "/sandboxes", `{"template_id":"python/numpy"}`, f.alum)
	if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "capacity_unavailable" {
		t.Fatalf("cola llena: esperaba 503 capacity_unavailable, got %d %s", rec.Code, rec.Body.String())
	}
	listaDespues := doJSON(t, f.h, "GET", "/sandboxes", "", f.doc)
	if strings.Count(listaDespues.Body.String(), `"template_id"`) != strings.Count(listaAntes.Body.String(), `"template_id"`) {
		t.Fatal("el 503 no debió crear registros")
	}

	// al liberar presupuesto, el encolado se promueve y corre hasta cleaned
	stepHasta(t, f.h, f.alum, c["run_id"].(string), "cleaned", 14)
}

// --- Plan B: runner caído → 503 en POST /sandboxes; deliver funciona igual (§7) ---

func TestRunnerCaido_DeliverFunciona(t *testing.T) {
	f := seedSandboxes(t, 4, 8)
	f.h.SetRunner(deadRunner{})

	rec := doJSON(t, f.h, "POST", "/sandboxes", `{"template_id":"python/numpy"}`, f.alum)
	if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "capacity_unavailable" {
		t.Fatalf("runner caído: esperaba 503 capacity_unavailable, got %d %s", rec.Code, rec.Body.String())
	}

	asgID := crearConsignaRapida(t, f)
	subID := subirDraft(t, f, asgID)

	del := doJSON(t, f.h, "POST", "/submissions/"+subID+"/deliver", `{"confirm_attempt":1}`, f.alum)
	if del.Code != 201 {
		t.Fatalf("deliver con runner caído: esperaba 201, got %d (%s)", del.Code, del.Body.String())
	}
}
