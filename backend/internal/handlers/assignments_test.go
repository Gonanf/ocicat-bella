package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

type fixtureConsignas struct {
	h         *Handler
	doc       *http.Cookie
	doc2      *http.Cookie
	dir       *http.Cookie
	alumA     *http.Cookie // miembro del aula
	alumB     *http.Cookie // miembro del aula
	alumFuera *http.Cookie // sin aulas
	aulaID    string
}

func seedFixtureConsignas(t *testing.T) *fixtureConsignas {
	t.Helper()
	f := &fixtureConsignas{}
	f.h, _ = newTestHandler(t)

	docID := seedUser(t, f.h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	f.doc = sesionDeRol(t, f.h, docID, model.SessionKindStaff)
	doc2ID := seedUser(t, f.h, "doc2@escuela.edu.ar", "p", model.RoleDocente, false)
	f.doc2 = sesionDeRol(t, f.h, doc2ID, model.SessionKindStaff)
	dirID := seedUser(t, f.h, "dir@escuela.edu.ar", "p", model.RoleDirector, false)
	f.dir = sesionDeRol(t, f.h, dirID, model.SessionKindStaff)
	alumAID := seedUser(t, f.h, "aluma@escuela.edu.ar", "", model.RoleAlumno, false)
	f.alumA = sesionDeRol(t, f.h, alumAID, model.SessionKindPWA)
	alumBID := seedUser(t, f.h, "alumb@escuela.edu.ar", "", model.RoleAlumno, false)
	f.alumB = sesionDeRol(t, f.h, alumBID, model.SessionKindPWA)
	fueraID := seedUser(t, f.h, "fuera@escuela.edu.ar", "", model.RoleAlumno, false)
	f.alumFuera = sesionDeRol(t, f.h, fueraID, model.SessionKindPWA)

	aula := crearAula(t, f.h, f.doc, "Aula Consignas")
	f.aulaID = aula["id"].(string)
	if got := unirse(t, f.h, aula["join_code"].(string), f.alumA); *got != 200 {
		t.Fatalf("join alumA: %d", *got)
	}
	if got := unirse(t, f.h, aula["join_code"].(string), f.alumB); *got != 200 {
		t.Fatalf("join alumB: %d", *got)
	}
	return f
}

// crearConsigna POSTea el body tal cual y devuelve la respuesta cruda.
func crearConsigna(t *testing.T, h *Handler, cookie *http.Cookie, aulaID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, h, "POST", "/classrooms/"+aulaID+"/assignments", body, cookie)
}

func consignaDeRespuesta(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	return out
}

// subirArchivos arma multipart con una o más partes "files".
func subirArchivos(t *testing.T, h *Handler, target string, files map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for name, content := range files {
		fw, err := mw.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", target, body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// flujoFeliz crea consigna limited{max} y deja un intento entregado del alumno;
// devuelve la consigna y el submission_id entregado.
func entregarIntento(t *testing.T, h *Handler, cookie *http.Cookie, assignmentID string, files map[string]string, confirmAttempt int) *httptest.ResponseRecorder {
	t.Helper()
	rec := subirArchivos(t, h, "/assignments/"+assignmentID+"/submissions/files", files, cookie)
	if rec.Code != 201 {
		t.Fatalf("upload: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var up struct {
		SubmissionID string `json:"submission_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	return doJSON(t, h, "POST", "/submissions/"+up.SubmissionID+"/deliver",
		`{"confirm_attempt":`+strconv.Itoa(confirmAttempt)+`}`, cookie)
}

// --- §4 creación por rol y validaciones ---

func TestConsignas_CreacionPorRol(t *testing.T) {
	f := seedFixtureConsignas(t)

	okBody := `{"title":"TP2 — Listas","instructions":"markdown","runtime":"python",
		"attempts":{"mode":"limited","max":3},"late_policy":"allowed"}`
	cases := []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"docente dueño → 201", f.doc, 201},
		{"director → 201", f.dir, 201},
		{"otro docente → 403", f.doc2, 403},
		{"alumno miembro → 403", f.alumA, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := crearConsigna(t, f.h, tc.cookie, f.aulaID, okBody)
			if rec.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}

	rec := crearConsigna(t, f.h, f.doc, f.aulaID, okBody)
	v := consignaDeRespuesta(t, rec)
	if v["id"] == "" || v["runtime"] != "python" || v["late_policy"] != "allowed" || v["classroom_id"] != f.aulaID {
		t.Fatalf("vista incompleta: %+v", v)
	}
	attempts, _ := v["attempts"].(map[string]any)
	if attempts == nil || attempts["mode"] != "limited" || attempts["max"].(float64) != 3 {
		t.Fatalf("attempts eco esperado {limited,3}: %+v", attempts)
	}

	validations := []struct {
		name string
		body string
	}{
		{"sin título", `{"runtime":"python"}`},
		{"runtime inválido", `{"title":"x","runtime":"ruby"}`},
		{"limited sin max", `{"title":"x","runtime":"web","attempts":{"mode":"limited"}}`},
		{"limited max 0", `{"title":"x","runtime":"web","attempts":{"mode":"limited","max":0}}`},
		{"late_policy inválida", `{"title":"x","runtime":"web","late_policy":"quizas"}`},
		{"due_at malformado", `{"title":"x","runtime":"web","due_at":"ayer"}`},
		{"attachment inexistente", `{"title":"x","runtime":"web","attachment_ids":["nope"]}`},
		{"body roto", `{`},
	}
	for _, tc := range validations {
		t.Run("validación: "+tc.name, func(t *testing.T) {
			rec := crearConsigna(t, f.h, f.doc, f.aulaID, tc.body)
			if rec.Code != 400 || errCode(t, rec) != "validation_error" {
				t.Fatalf("esperaba 400 validation_error, got %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}

	// aula inexistente → 404
	rec = crearConsigna(t, f.h, f.doc, "noexiste", okBody)
	if rec.Code != 404 {
		t.Fatalf("aula inexistente: esperaba 404, got %d", rec.Code)
	}
}

func TestConsignas_ListadoPorRol(t *testing.T) {
	f := seedFixtureConsignas(t)
	for _, titulo := range []string{"TP1", "TP2"} {
		rec := crearConsigna(t, f.h, f.doc, f.aulaID,
			`{"title":"`+titulo+`","runtime":"python","attempts":{"mode":"unlimited"},"late_policy":"allowed"}`)
		if rec.Code != 201 {
			t.Fatalf("crear %s: %d (%s)", titulo, rec.Code, rec.Body.String())
		}
	}

	cases := []struct {
		name      string
		cookie    *http.Cookie
		wantCode  int
		wantCount int
		wantStats bool
	}{
		{"docente dueño: todas + métricas", f.doc, 200, 2, true},
		{"director: todas + métricas", f.dir, 200, 2, true},
		{"alumno miembro: activas sin métricas", f.alumA, 200, 2, false},
		{"alumno de otra aula: prohibido", f.alumFuera, 403, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, f.h, "GET", "/classrooms/"+f.aulaID+"/assignments", "", tc.cookie)
			if rec.Code != tc.wantCode {
				t.Fatalf("esperaba %d, got %d (%s)", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.wantCode != 200 {
				return
			}
			var resp struct {
				Assignments []map[string]any `json:"assignments"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if len(resp.Assignments) != tc.wantCount {
				t.Fatalf("esperaba %d consignas, got %d", tc.wantCount, len(resp.Assignments))
			}
			_, hasStats := resp.Assignments[0]["stats"]
			if hasStats != tc.wantStats {
				t.Fatalf("stats presente=%v, esperaba %v", hasStats, tc.wantStats)
			}
			if !tc.wantStats {
				return
			}
			st := resp.Assignments[0]["stats"].(map[string]any)
			if st["total_students"].(float64) != 2 || st["delivered"].(float64) != 0 || st["missing"].(float64) != 2 {
				t.Fatalf("stats iniciales incorrectas: %+v", st)
			}
		})
	}
}

func TestConsignas_GetPatchDeleteAccesos(t *testing.T) {
	f := seedFixtureConsignas(t)
	rec := crearConsigna(t, f.h, f.doc, f.aulaID,
		`{"title":"TP1","instructions":"hacer X","runtime":"python","attempts":{"mode":"one"},"late_policy":"closed"}`)
	id := consignaDeRespuesta(t, rec)["id"].(string)

	// lectura: miembros también (§4)
	cases := []struct {
		name      string
		cookie    *http.Cookie
		want      int
		wantStats bool
	}{
		{"docente dueño", f.doc, 200, true},
		{"director", f.dir, 200, true},
		{"alumno miembro", f.alumA, 200, false},
		{"alumno externo", f.alumFuera, 403, false},
	}
	for _, tc := range cases {
		t.Run("GET "+tc.name, func(t *testing.T) {
			got := doJSON(t, f.h, "GET", "/assignments/"+id, "", tc.cookie)
			if got.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, got.Code, got.Body.String())
			}
			if tc.wantStats {
				v := consignaDeRespuesta(t, got)
				if _, ok := v["stats"]; !ok {
					t.Fatal("staff debía ver stats")
				}
			}
		})
	}

	// PATCH modificable en cualquier momento (§9.1): baja intentos con entrega hecha más abajo
	patch := doJSON(t, f.h, "PATCH", "/assignments/"+id, `{"title":"TP1 v2","attempts":{"mode":"limited","max":5}}`, f.doc)
	if patch.Code != 200 {
		t.Fatalf("patch docente: esperaba 200, got %d (%s)", patch.Code, patch.Body.String())
	}
	v := consignaDeRespuesta(t, patch)
	attempts := v["attempts"].(map[string]any)
	if v["title"] != "TP1 v2" || attempts["mode"] != "limited" || attempts["max"].(float64) != 5 {
		t.Fatalf("patch no aplicado: %+v", v)
	}

	// PATCH inválido
	if got := doJSON(t, f.h, "PATCH", "/assignments/"+id, `{"runtime":"fortran"}`, f.doc); got.Code != 400 {
		t.Fatalf("patch runtime inválido: esperaba 400, got %d", got.Code)
	}
	// PATCH por otro docente → 403
	if got := doJSON(t, f.h, "PATCH", "/assignments/"+id, `{"title":"robado"}`, f.doc2); got.Code != 403 {
		t.Fatalf("patch ajeno: esperaba 403, got %d", got.Code)
	}

	// DELETE soft-delete → 204 y luego 404
	if got := doJSON(t, f.h, "DELETE", "/assignments/"+id, "", f.doc); got.Code != 204 {
		t.Fatalf("delete: esperaba 204, got %d (%s)", got.Code, got.Body.String())
	}
	if got := doJSON(t, f.h, "GET", "/assignments/"+id, "", f.doc); got.Code != 404 {
		t.Fatalf("borrada: esperaba 404, got %d", got.Code)
	}
	if got := doJSON(t, f.h, "DELETE", "/assignments/"+id, "", f.doc); got.Code != 404 {
		t.Fatalf("delete doble: esperaba 404, got %d", got.Code)
	}
	// desaparece del listado del aula
	lista := doJSON(t, f.h, "GET", "/classrooms/"+f.aulaID+"/assignments", "", f.doc)
	if strings.Contains(lista.Body.String(), "TP1 v2") {
		t.Fatal("consigna borrada sigue en el listado")
	}
}

// --- §4 adjuntos ---

func TestAdjuntos_UploadLimitesYGC(t *testing.T) {
	f := seedFixtureConsignas(t)

	// feliz
	rec := subirMaterial(t, f.h, "/assignments/attachments", "enunciado.pdf", "contenido",
		map[string]string{}, f.doc)
	if rec.Code != 201 {
		t.Fatalf("upload adjunto: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var up struct {
		AttachmentID string `json:"attachment_id"`
		Filename     string `json:"filename"`
		SizeBytes    int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	if up.AttachmentID == "" || up.Filename != "enunciado.pdf" || up.SizeBytes != int64(len("contenido")) {
		t.Fatalf("respuesta adjunto incompleta: %+v", up)
	}

	// >10MB → 400 file_too_large
	rec = subirMaterial(t, f.h, "/assignments/attachments", "grande.pdf", strings.Repeat("x", 11<<20), map[string]string{}, f.doc)
	if rec.Code != 400 || errCode(t, rec) != "file_too_large" {
		t.Fatalf(">10MB: esperaba 400 file_too_large, got %d (%s)", rec.Code, rec.Body.String())
	}

	// alumno no sube
	rec = subirMaterial(t, f.h, "/assignments/attachments", "a.pdf", "x", map[string]string{}, f.alumA)
	if rec.Code != 403 {
		t.Fatalf("alumno subiendo adjunto: esperaba 403, got %d", rec.Code)
	}

	// el id sirve en attachment_ids del create
	rec = crearConsigna(t, f.h, f.doc, f.aulaID,
		`{"title":"con adjunto","runtime":"python","attachment_ids":["`+up.AttachmentID+`"],
		  "attempts":{"mode":"unlimited"},"late_policy":"allowed"}`)
	if rec.Code != 201 {
		t.Fatalf("create con adjunto: %d (%s)", rec.Code, rec.Body.String())
	}
	v := consignaDeRespuesta(t, rec)
	ids := v["attachment_ids"].([]any)
	if len(ids) != 1 || ids[0].(string) != up.AttachmentID {
		t.Fatalf("attachment_ids no conservado: %+v", v)
	}

	// GC: huérfano viejo se purga; reclamado y fresco quedan
	oldOrphan := &model.Attachment{ID: "old-orphan", Filename: "viejo.pdf", Size: 3, Data: []byte("abc"), CreatedAt: time.Now().Add(-25 * time.Hour)}
	freshOrphan := &model.Attachment{ID: "fresh-orphan", Filename: "nuevo.pdf", Size: 3, Data: []byte("abc"), CreatedAt: time.Now()}
	if err := f.h.store.CreateAttachment(t.Context(), oldOrphan); err != nil {
		t.Fatal(err)
	}
	if err := f.h.store.CreateAttachment(t.Context(), freshOrphan); err != nil {
		t.Fatal(err)
	}
	purged, err := f.h.SweepOrphanAttachments(t.Context(), time.Now(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if purged != 1 {
		t.Fatalf("GC: esperaba purgar 1, purgó %d", purged)
	}
	if _, err := f.h.store.GetAttachment(t.Context(), oldOrphan.ID); err != store.ErrNotFound {
		t.Fatalf("huérfano viejo debía purgarse, got %v", err)
	}
	if _, err := f.h.store.GetAttachment(t.Context(), freshOrphan.ID); err != nil {
		t.Fatalf("huérfano fresco debía sobrevivir: %v", err)
	}
	if _, err := f.h.store.GetAttachment(t.Context(), up.AttachmentID); err != nil {
		t.Fatalf("adjunto reclamado debía sobrevivir: %v", err)
	}
}

// --- §5 flujo draft → deliver ---

func TestEntregas_FlujoFeliz(t *testing.T) {
	f := seedFixtureConsignas(t)
	rec := crearConsigna(t, f.h, f.doc, f.aulaID,
		`{"title":"TP2","runtime":"python","attempts":{"mode":"limited","max":3},"late_policy":"allowed"}`)
	aid := consignaDeRespuesta(t, rec)["id"].(string)

	// subida inicial crea draft
	up := subirArchivos(t, f.h, "/assignments/"+aid+"/submissions/files",
		map[string]string{"tp2.py": "print('hola')"}, f.alumA)
	if up.Code != 201 {
		t.Fatalf("upload: %d (%s)", up.Code, up.Body.String())
	}
	var first struct {
		SubmissionID string `json:"submission_id"`
		State        string `json:"state"`
		Files        []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Size int    `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(up.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.State != "draft" || len(first.Files) != 1 || first.Files[0].Name != "tp2.py" || first.Files[0].Size != len("print('hola')") {
		t.Fatalf("draft inicial incorrecto: %+v", first)
	}

	// segunda subida REUSA el mismo draft y acumula archivos
	up2 := subirArchivos(t, f.h, "/assignments/"+aid+"/submissions/files",
		map[string]string{"index.html": "<h1>x</h1>"}, f.alumA)
	var second struct {
		SubmissionID string `json:"submission_id"`
		Files        []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.Unmarshal(up2.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.SubmissionID != first.SubmissionID || len(second.Files) != 2 {
		t.Fatalf("draft debía reusarse y acumular: %+v vs %+v", first, second)
	}

	// checkpoint explícito
	del := doJSON(t, f.h, "POST", "/submissions/"+first.SubmissionID+"/deliver", `{"confirm_attempt":1}`, f.alumA)
	if del.Code != 201 {
		t.Fatalf("deliver: esperaba 201, got %d (%s)", del.Code, del.Body.String())
	}
	v := consignaDeRespuesta(t, del)
	if v["attempt_number"].(float64) != 1 || v["state"] != "delivered" || v["late"] != false ||
		v["delivered_at"] == "" || v["last_test_result"] != nil || v["attempts_remaining"].(float64) != 2 {
		t.Fatalf("respuesta deliver incorrecta: %+v", v)
	}

	// historial propio
	me := doJSON(t, f.h, "GET", "/assignments/"+aid+"/submissions/me", "", f.alumA)
	if me.Code != 200 {
		t.Fatalf("/me: %d (%s)", me.Code, me.Body.String())
	}
	var mine struct {
		Submissions []struct {
			ID            string `json:"id"`
			AttemptNumber int    `json:"attempt_number"`
			State         string `json:"state"`
			Late          bool   `json:"late"`
		} `json:"submissions"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &mine); err != nil {
		t.Fatal(err)
	}
	if len(mine.Submissions) != 1 || mine.Submissions[0].AttemptNumber != 1 || mine.Submissions[0].State != "delivered" {
		t.Fatalf("historial propio incorrecto: %+v", mine.Submissions)
	}

	// reintento = nueva draft con intento 2
	up3 := subirArchivos(t, f.h, "/assignments/"+aid+"/submissions/files",
		map[string]string{"tp2.py": "intento 2"}, f.alumA)
	if up3.Code != 201 {
		t.Fatalf("reintento: %d (%s)", up3.Code, up3.Body.String())
	}
	var third struct {
		SubmissionID string `json:"submission_id"`
	}
	_ = json.Unmarshal(up3.Body.Bytes(), &third)
	if third.SubmissionID == first.SubmissionID {
		t.Fatal("reintento debía crear NUEVA draft")
	}
}

func TestEntregas_Guardas409(t *testing.T) {
	f := seedFixtureConsignas(t)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	type setupFn func(f *fixtureConsignas) string // devuelve id de consigna
	scenarios := []struct {
		name    string
		setup   setupFn
		confirm int // intento que "vio" el cliente
		wantErr string
	}{
		{
			name: "intentos limitados agotados",
			setup: func(f *fixtureConsignas) string {
				rec := crearConsigna(t, f.h, f.doc, f.aulaID,
					`{"title":"una sola","runtime":"python","attempts":{"mode":"one"},"late_policy":"allowed"}`)
				aid := consignaDeRespuesta(t, rec)["id"].(string)
				if got := entregarIntento(t, f.h, f.alumA, aid, map[string]string{"a.py": "1"}, 1); got.Code != 201 {
					t.Fatalf("primera entrega debía pasar: %d (%s)", got.Code, got.Body.String())
				}
				return aid
			},
			confirm: 2,
			wantErr: "attempts_exhausted",
		},
		{
			name: "deadline pasada con policy closed",
			setup: func(f *fixtureConsignas) string {
				rec := crearConsigna(t, f.h, f.doc, f.aulaID,
					`{"title":"cerrada","runtime":"python","due_at":"`+past+`","attempts":{"mode":"unlimited"},"late_policy":"closed"}`)
				return consignaDeRespuesta(t, rec)["id"].(string)
			},
			confirm: 1,
			wantErr: "deadline_passed",
		},
		{
			name: "confirm_attempt desactualizado",
			setup: func(f *fixtureConsignas) string {
				rec := crearConsigna(t, f.h, f.doc, f.aulaID,
					`{"title":"conflicto","runtime":"python","due_at":"`+future+`","attempts":{"mode":"unlimited"},"late_policy":"allowed"}`)
				return consignaDeRespuesta(t, rec)["id"].(string)
			},
			confirm: 7,
			wantErr: "attempt_conflict",
		},
	}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			aid := tc.setup(f)
			rec := entregarIntento(t, f.h, f.alumA, aid, map[string]string{"tp.py": "código"}, tc.confirm)
			if rec.Code != 409 || errCode(t, rec) != tc.wantErr {
				t.Fatalf("esperaba 409 %q, got %d (%s)", tc.wantErr, rec.Code, rec.Body.String())
			}
		})
	}

	// re-entregar una submission ya entregada → conflict (cliente desactualizado)
	rec := crearConsigna(t, f.h, f.doc, f.aulaID,
		`{"title":"redeliver","runtime":"python","attempts":{"mode":"unlimited"},"late_policy":"allowed"}`)
	aid := consignaDeRespuesta(t, rec)["id"].(string)
	first := entregarIntento(t, f.h, f.alumA, aid, map[string]string{"a.py": "1"}, 1)
	if first.Code != 201 {
		t.Fatalf("primera: %d (%s)", first.Code, first.Body.String())
	}
	var d struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &d)
	if got := doJSON(t, f.h, "POST", "/submissions/"+d.ID+"/deliver", `{"confirm_attempt":1}`, f.alumA); got.Code != 409 || errCode(t, got) != "attempt_conflict" {
		t.Fatalf("re-deliver: esperaba 409 attempt_conflict, got %d (%s)", got.Code, got.Body.String())
	}
}

func TestEntregas_SnapshotInmutable(t *testing.T) {
	f := seedFixtureConsignas(t)
	rec := crearConsigna(t, f.h, f.doc, f.aulaID,
		`{"title":"inmutable","runtime":"python","attempts":{"mode":"limited","max":3},"late_policy":"allowed"}`)
	aid := consignaDeRespuesta(t, rec)["id"].(string)

	del := entregarIntento(t, f.h, f.alumA, aid, map[string]string{"tp.py": "versión 1"}, 1)
	if del.Code != 201 {
		t.Fatalf("entrega: %d (%s)", del.Code, del.Body.String())
	}
	var delivered struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(del.Body.Bytes(), &delivered)

	// editar = nueva subida al intento en curso; lo entregado NO cambia
	if up := subirArchivos(t, f.h, "/assignments/"+aid+"/submissions/files",
		map[string]string{"tp.py": "versión 2 EDITADA"}, f.alumA); up.Code != 201 {
		t.Fatalf("nueva draft: %d", up.Code)
	}

	got := doJSON(t, f.h, "GET", "/submissions/"+delivered.ID, "", f.alumA)
	if got.Code != 200 {
		t.Fatalf("get snapshot: %d (%s)", got.Code, got.Body.String())
	}
	var snap struct {
		State string `json:"state"`
		Files []struct {
			Name string `json:"name"`
			Size int    `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.State != "delivered" || len(snap.Files) != 1 || snap.Files[0].Name != "tp.py" || snap.Files[0].Size != len("versión 1") {
		t.Fatalf("snapshot mutado tras deliver: %+v", snap)
	}

	// docente dueño y director leen el snapshot completo; otro alumno no
	for _, tc := range []struct {
		cookie *http.Cookie
		want   int
	}{{f.doc, 200}, {f.dir, 200}, {f.alumB, 403}} {
		if got := doJSON(t, f.h, "GET", "/submissions/"+delivered.ID, "", tc.cookie); got.Code != tc.want {
			t.Fatalf("acceso snapshot: esperaba %d, got %d", tc.want, got.Code)
		}
	}
}

func TestEntregas_PatchConEntregasExistentes(t *testing.T) {
	f := seedFixtureConsignas(t)
	rec := crearConsigna(t, f.h, f.doc, f.aulaID,
		`{"title":"baja intentos","runtime":"python","attempts":{"mode":"limited","max":3},"late_policy":"allowed"}`)
	aid := consignaDeRespuesta(t, rec)["id"].(string)

	if del := entregarIntento(t, f.h, f.alumA, aid, map[string]string{"tp.py": "hecho"}, 1); del.Code != 201 {
		t.Fatalf("entrega previa: %d (%s)", del.Code, del.Body.String())
	}

	// docente baja intentos y cierra tardías CON entrega ya hecha: permitido (§9.1)
	patch := doJSON(t, f.h, "PATCH", "/assignments/"+aid,
		`{"attempts":{"mode":"limited","max":1},"late_policy":"closed"}`, f.doc)
	if patch.Code != 200 {
		t.Fatalf("patch con entregas: esperaba 200, got %d (%s)", patch.Code, patch.Body.String())
	}

	// lo hecho no se invalida: sigue visible en historial y cuenta en stats
	me := doJSON(t, f.h, "GET", "/assignments/"+aid+"/submissions/me", "", f.alumA)
	if !strings.Contains(me.Body.String(), `"attempt_number":1`) || !strings.Contains(me.Body.String(), `"state":"delivered"`) {
		t.Fatalf("entrega previa invalidada por PATCH: %s", me.Body.String())
	}
	stats := doJSON(t, f.h, "GET", "/assignments/"+aid+"/stats", "", f.doc)
	var st struct {
		Delivered int `json:"delivered"`
		Missing   int `json:"missing"`
	}
	_ = json.Unmarshal(stats.Body.Bytes(), &st)
	if st.Delivered != 1 {
		t.Fatalf("stats post-PATCH: delivered debía seguir 1, got %+v", st)
	}

	// config vigente aplica al siguiente intento: max 1 ya consumido
	next := entregarIntento(t, f.h, f.alumA, aid, map[string]string{"tp2.py": "otro"}, 2)
	if next.Code != 409 || errCode(t, next) != "attempts_exhausted" {
		t.Fatalf("según config vigente debía estar agotado: %d (%s)", next.Code, next.Body.String())
	}
}

func TestStats_YFiltrosCorreccion(t *testing.T) {
	f := seedFixtureConsignas(t)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	rec := crearConsigna(t, f.h, f.doc, f.aulaID,
		`{"title":"con vencimiento","runtime":"python","due_at":"`+
			time.Now().Add(time.Hour).UTC().Format(time.RFC3339)+
			`","attempts":{"mode":"unlimited"},"late_policy":"allowed"}`)
	aid := consignaDeRespuesta(t, rec)["id"].(string)

	// A entrega en término
	if del := entregarIntento(t, f.h, f.alumA, aid, map[string]string{"a.py": "ok"}, 1); del.Code != 201 {
		t.Fatalf("entrega A: %d (%s)", del.Code, del.Body.String())
	}
	// el plazo vence (PATCH en cualquier momento) y B entrega tarde
	if p := doJSON(t, f.h, "PATCH", "/assignments/"+aid, `{"due_at":"`+past+`"}`, f.doc); p.Code != 200 {
		t.Fatalf("patch due_at: %d", p.Code)
	}
	delB := entregarIntento(t, f.h, f.alumB, aid, map[string]string{"b.py": "tarde"}, 1)
	if delB.Code != 201 {
		t.Fatalf("entrega B: %d (%s)", delB.Code, delB.Body.String())
	}
	var vb struct {
		Late bool `json:"late"`
	}
	_ = json.Unmarshal(delB.Body.Bytes(), &vb)
	if !vb.Late {
		t.Fatal("entrega B debía marcarse tardía")
	}
	// C (tercer alumno) falta: lo sumamos al aula ahora
	alumCID := seedUser(t, f.h, "alumc@escuela.edu.ar", "", model.RoleAlumno, false)
	alumC := sesionDeRol(t, f.h, alumCID, model.SessionKindPWA)
	if got := unirse(t, f.h, aulaCode(t, f), alumC); *got != 200 {
		t.Fatalf("join alumC: %d", *got)
	}

	stats := doJSON(t, f.h, "GET", "/assignments/"+aid+"/stats", "", f.doc)
	if stats.Code != 200 {
		t.Fatalf("stats: %d (%s)", stats.Code, stats.Body.String())
	}
	var st struct {
		TotalStudents int `json:"total_students"`
		Delivered     int `json:"delivered"`
		TestedOK      int `json:"tested_ok"`
		TestErrors    int `json:"test_errors"`
		Late          int `json:"late"`
		Missing       int `json:"missing"`
	}
	if err := json.Unmarshal(stats.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.TotalStudents != 3 || st.Delivered != 2 || st.Late != 1 || st.Missing != 1 || st.TestedOK != 0 || st.TestErrors != 0 {
		t.Fatalf("stats incorrectas (FASE 6 aún sin tests): %+v", st)
	}

	// vista de corrección
	cases := []struct {
		filter    string
		wantCount int
		checkLate bool
		wantUsers []string
	}{
		{"late", 1, true, nil},
		{"error", 0, false, nil},
		{"", 2, false, nil},
		{"missing", 0, false, []string{"alumc@escuela.edu.ar"}},
	}
	for _, tc := range cases {
		t.Run("filter="+tc.filter, func(t *testing.T) {
			got := doJSON(t, f.h, "GET", "/assignments/"+aid+"/submissions?filter="+tc.filter, "", f.dir)
			if got.Code != 200 {
				t.Fatalf("%d (%s)", got.Code, got.Body.String())
			}
			if tc.filter == "missing" {
				var m struct {
					Missing []struct {
						UserID string `json:"user_id"`
						Email  string `json:"email"`
					} `json:"missing"`
				}
				if err := json.Unmarshal(got.Body.Bytes(), &m); err != nil {
					t.Fatal(err)
				}
				if len(m.Missing) != 1 || m.Missing[0].Email != tc.wantUsers[0] {
					t.Fatalf("missing esperaba [%s], got %+v", tc.wantUsers, m.Missing)
				}
				return
			}
			var s struct {
				Submissions []struct {
					Late        bool   `json:"late"`
					State       string `json:"state"`
					StudentID   string `json:"student_id"`
					StudentName string `json:"student_name"`
				} `json:"submissions"`
			}
			if err := json.Unmarshal(got.Body.Bytes(), &s); err != nil {
				t.Fatal(err)
			}
			if len(s.Submissions) != tc.wantCount {
				t.Fatalf("filter %q: esperaba %d, got %+v", tc.filter, tc.wantCount, s.Submissions)
			}
			for _, sub := range s.Submissions {
				if sub.StudentID == "" || sub.StudentName == "" {
					t.Fatalf("vista corrección sin datos de alumno: %+v", sub)
				}
				if tc.checkLate && !sub.Late {
					t.Fatalf("filter=late devolvió no-tardía: %+v", sub)
				}
			}
		})
	}

	// filter inválido → 400; alumno → 403
	if got := doJSON(t, f.h, "GET", "/assignments/"+aid+"/submissions?filter=raro", "", f.doc); got.Code != 400 {
		t.Fatalf("filter raro: esperaba 400, got %d", got.Code)
	}
	if got := doJSON(t, f.h, "GET", "/assignments/"+aid+"/submissions", "", f.alumA); got.Code != 403 {
		t.Fatalf("vista corrección para alumno: esperaba 403, got %d", got.Code)
	}
}

func aulaCode(t *testing.T, f *fixtureConsignas) string {
	t.Helper()
	rec := doJSON(t, f.h, "GET", "/classrooms/"+f.aulaID+"/join_code", "", f.doc)
	var c struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &c)
	return c.Code
}

// subida de archivo >10MB a la entrega → 400 file_too_large (§5.1)
func TestEntregas_ArchivoGrande(t *testing.T) {
	f := seedFixtureConsignas(t)
	rec := crearConsigna(t, f.h, f.doc, f.aulaID,
		`{"title":"límite","runtime":"python","attempts":{"mode":"unlimited"},"late_policy":"allowed"}`)
	aid := consignaDeRespuesta(t, rec)["id"].(string)

	up := subirArchivos(t, f.h, "/assignments/"+aid+"/submissions/files",
		map[string]string{"pesado.zip": strings.Repeat("x", 11<<20)}, f.alumA)
	if up.Code != 400 || errCode(t, up) != "file_too_large" {
		t.Fatalf(">10MB: esperaba 400 file_too_large, got %d (%s)", up.Code, up.Body.String())
	}

	// sin archivos → 400
	emptyReq := httptest.NewRequest("POST", "/assignments/"+aid+"/submissions/files", bytes.NewBufferString("--x--"))
	emptyReq.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	emptyReq.AddCookie(f.alumA)
	er := httptest.NewRecorder()
	f.h.ServeHTTP(er, emptyReq)
	if er.Code != 400 {
		t.Fatalf("sin archivos: esperaba 400, got %d", er.Code)
	}

	// alumno que no pertenece al aula → 403
	out := subirArchivos(t, f.h, "/assignments/"+aid+"/submissions/files",
		map[string]string{"x.py": "1"}, f.alumFuera)
	if out.Code != 403 {
		t.Fatalf("externo: esperaba 403, got %d", out.Code)
	}
}
