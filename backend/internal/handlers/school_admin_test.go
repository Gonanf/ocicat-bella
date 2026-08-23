package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

func postCSV(t *testing.T, h *Handler, target, csvContent string, fields map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, err := mw.CreateFormFile("csv", "alumnos.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(csvContent)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", target, body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// setupEscuelaFull: director + docente dueño de un aula + alumno ya existente.
func setupEscuelaFull(t *testing.T, h *Handler) (dir, doc *http.Cookie, aulaID, dirID string) {
	t.Helper()
	dirID = seedUser(t, h, "dir@escuela.edu.ar", "pass12345", model.RoleDirector, false)
	dir = sesionDeRol(t, h, dirID, model.SessionKindStaff)
	docID := seedUser(t, h, "doc@escuela.edu.ar", "pass12345", model.RoleDocente, false)
	doc = sesionDeRol(t, h, docID, model.SessionKindStaff)
	seedUser(t, h, "existente@escuela.edu.ar", "", model.RoleAlumno, false)
	aula := crearAula(t, h, doc, "5A")
	return dir, doc, aula["id"].(string), dirID
}

const csvMixto = "Juan López,juan@escuela.edu.ar\n" +
	"María Díaz,juan@escuela.edu.ar\n" + // duplicate
	"Pedro Sosa,pedro@\n" + // invalid_email
	"Ana Ruiz,existente@escuela.edu.ar\n" + // exists
	"Lucía Fernández,lucia@escuela.edu.ar\n" // ok

func TestDocentes_CRUDPorDirector(t *testing.T) {
	h, links := newTestHandler(t)
	dir, _, _, _ := setupEscuelaFull(t, h)

	// alta con password → 201 y login funciona
	rec := doJSON(t, h, "POST", "/school/teachers",
		`{"name":"Carlos Pérez","email":"carlos@escuela.edu.ar","credential":{"type":"password","password":"temp1234!"}}`, dir)
	if rec.Code != 201 {
		t.Fatalf("alta docente: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var creado struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &creado)
	if creado.ID == "" || creado.Status != "active" {
		t.Fatalf("alta docente sin id/status: %s", rec.Body.String())
	}

	login := doJSON(t, h, "POST", "/auth/login/password", `{"email":"carlos@escuela.edu.ar","password":"temp1234!"}`)
	if login.Code != 200 {
		t.Fatalf("login docente nuevo: esperaba 200, got %d (%s)", login.Code, login.Body.String())
	}

	// email duplicado ([C5]) → 409 aunque sea alumno
	recDup := doJSON(t, h, "POST", "/school/teachers",
		`{"name":"Otro","email":"existente@escuela.edu.ar","credential":{"type":"magic_link"}}`, dir)
	if recDup.Code != 409 || errCode(t, recDup) != "email_already_exists" {
		t.Fatalf("duplicado: esperaba 409 email_already_exists, got %d (%s)", recDup.Code, recDup.Body.String())
	}

	// credencial magic_link → envía enlace
	recML := doJSON(t, h, "POST", "/school/teachers",
		`{"name":"Marta Real","email":"marta@escuela.edu.ar","credential":{"type":"magic_link"}}`, dir)
	if recML.Code != 201 || len(*links) != 1 || !strings.Contains((*links)[0], "/auth/consume?token=") {
		t.Fatalf("alta con magic_link: esperaba 201+link, got %d links=%v (%s)", recML.Code, *links, recML.Body.String())
	}

	// listado con classrooms_count
	lista := doJSON(t, h, "GET", "/school/teachers", "", dir)
	if lista.Code != 200 || !strings.Contains(lista.Body.String(), `"classrooms_count":1`) {
		t.Fatalf("listado docentes: esperaba count 1 para doc@, got %d (%s)", lista.Code, lista.Body.String())
	}

	// reset-credential: la vieja deja de valer
	recReset := doJSON(t, h, "POST", "/school/teachers/"+creado.ID+"/reset-credential",
		`{"credential":{"type":"password","password":"nueva12345"}}`, dir)
	if recReset.Code != 200 {
		t.Fatalf("reset: esperaba 200, got %d (%s)", recReset.Code, recReset.Body.String())
	}
	vieja := doJSON(t, h, "POST", "/auth/login/password", `{"email":"carlos@escuela.edu.ar","password":"temp1234!"}`)
	nueva := doJSON(t, h, "POST", "/auth/login/password", `{"email":"carlos@escuela.edu.ar","password":"nueva12345"}`)
	if vieja.Code != 401 || nueva.Code != 200 {
		t.Fatalf("tras reset: vieja=%d (esperaba 401) nueva=%d (esperaba 200)", vieja.Code, nueva.Code)
	}

	// disable → 403 account_disabled en login y sesión viva muerta
	sessDoc := sesionDeRol(t, h, docIDFromList(t, h, "doc@escuela.edu.ar"), model.SessionKindStaff)
	recDis := doJSON(t, h, "POST", "/school/teachers/"+docIDFromList(t, h, "doc@escuela.edu.ar")+"/disable", "", dir)
	if recDis.Code != 200 || !strings.Contains(recDis.Body.String(), `"status":"disabled"`) {
		t.Fatalf("disable: esperaba 200 disabled, got %d (%s)", recDis.Code, recDis.Body.String())
	}
	loginDis := doJSON(t, h, "POST", "/auth/login/password", `{"email":"doc@escuela.edu.ar","password":"pass12345"}`)
	if loginDis.Code != 403 || errCode(t, loginDis) != "account_disabled" {
		t.Fatalf("login disabled: esperaba 403 account_disabled, got %d (%s)", loginDis.Code, loginDis.Body.String())
	}
	congelado := doJSON(t, h, "GET", "/classrooms", "", sessDoc)
	if congelado.Code != 401 && congelado.Code != 403 {
		t.Fatalf("sesión tras disable debe morir: esperaba 401/403, got %d", congelado.Code)
	}

	// enable → vuelve a entrar
	recEn := doJSON(t, h, "POST", "/school/teachers/"+docIDFromList(t, h, "doc@escuela.edu.ar")+"/enable", "", dir)
	if recEn.Code != 200 || !strings.Contains(recEn.Body.String(), `"status":"active"`) {
		t.Fatalf("enable: esperaba 200 active, got %d (%s)", recEn.Code, recEn.Body.String())
	}
	loginEn := doJSON(t, h, "POST", "/auth/login/password", `{"email":"doc@escuela.edu.ar","password":"pass12345"}`)
	if loginEn.Code != 200 {
		t.Fatalf("login tras enable: esperaba 200, got %d (%s)", loginEn.Code, loginEn.Body.String())
	}

	// roles: otro docente ACTIVO y anónimo jamás gestionan docentes (CU-13);
	// la cookie de doc quedó muerta por el disable de arriba
	docActivoID := seedUser(t, h, "doc-activo@escuela.edu.ar", "p", model.RoleDocente, false)
	docActivo := sesionDeRol(t, h, docActivoID, model.SessionKindStaff)
	for _, tc := range []struct {
		method, target, body string
	}{
		{"GET", "/school/teachers", ""},
		{"POST", "/school/teachers", `{"name":"x","email":"x@x.edu.ar","credential":{"type":"magic_link"}}`},
		{"POST", "/school/teachers/x/reset-credential", `{}`},
		{"POST", "/school/teachers/x/disable", ""},
		{"POST", "/school/teachers/x/enable", ""},
	} {
		if rec := doJSON(t, h, tc.method, tc.target, tc.body, docActivo); rec.Code != 403 {
			t.Fatalf("%s %s docente: esperaba 403, got %d", tc.method, tc.target, rec.Code)
		}
		if rec := doJSON(t, h, tc.method, tc.target, tc.body); rec.Code != 401 && rec.Code != 403 {
			t.Fatalf("%s %s anónimo: esperaba 401/403, got %d", tc.method, tc.target, rec.Code)
		}
	}
}

// docIDFromList busca el id real del usuario por email vía store.
func docIDFromList(t *testing.T, h *Handler, email string) string {
	t.Helper()
	u, err := h.store.GetUserByEmail(t.Context(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestImportCSV_PreviewYCommitParcial(t *testing.T) {
	h, links := newTestHandler(t)
	dir, doc, aulaID, _ := setupEscuelaFull(t, h)
	target := "/classrooms/" + aulaID + "/students/import"

	// paso 1 preview: clasifica fila por fila sin crear nada
	prev := postCSV(t, h, target, csvMixto, map[string]string{"dry_run": "true"}, doc)
	if prev.Code != 200 {
		t.Fatalf("preview: esperaba 200, got %d (%s)", prev.Code, prev.Body.String())
	}
	var pv struct {
		ImportToken string `json:"import_token"`
		Rows        []struct {
			Line   int    `json:"line"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"rows"`
		Summary struct {
			Ok       int `json:"ok"`
			Rejected int `json:"rejected"`
		} `json:"summary"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(prev.Body.Bytes(), &pv); err != nil {
		t.Fatal(err)
	}
	wantStatus := []string{"ok", "duplicate", "invalid_email", "exists", "ok"}
	for i, row := range pv.Rows {
		if row.Line != i+1 || row.Status != wantStatus[i] {
			t.Fatalf("fila %d: esperaba status %q, got %+v", i+1, wantStatus[i], row)
		}
	}
	if len(pv.Rows) == 5 && pv.Rows[1].Reason == "" {
		t.Fatalf("duplicate debe explicar la línea original: %+v", pv.Rows[1])
	}
	if pv.Summary.Ok != 2 || pv.Summary.Rejected != 3 || pv.ImportToken == "" || len(pv.Warnings) != 1 {
		t.Fatalf("preview summary/warnings incorrectos: %+v", pv)
	}

	// preview NO crea cuentas ni manda links
	if n := len(*links); n != 0 {
		t.Fatalf("preview no debe enviar links, envió %d", n)
	}

	// commit sin token → 400; token ajeno al aula → 400
	badToken := postCSV(t, h, target, csvMixto, map[string]string{"dry_run": "false"}, doc)
	if badToken.Code != 400 {
		t.Fatalf("commit sin token: esperaba 400, got %d", badToken.Code)
	}
	otraAula := crearAula(t, h, doc, "5B")
	prev2 := postCSV(t, h, "/classrooms/"+otraAula["id"].(string)+"/students/import", csvMixto, map[string]string{"dry_run": "true"}, doc)
	var tok2 struct {
		ImportToken string `json:"import_token"`
	}
	_ = json.Unmarshal(prev2.Body.Bytes(), &tok2)
	crossAula := postCSV(t, h, target, csvMixto, map[string]string{"dry_run": "false", "import_token": tok2.ImportToken}, doc)
	if crossAula.Code != 400 {
		t.Fatalf("commit con token de otra aula: esperaba 400, got %d", crossAula.Code)
	}

	// paso 2 commit: solo filas ok; las demás omitidas
	commit := postCSV(t, h, target, csvMixto, map[string]string{"dry_run": "false", "import_token": pv.ImportToken}, doc)
	if commit.Code != 201 {
		t.Fatalf("commit: esperaba 201, got %d (%s)", commit.Code, commit.Body.String())
	}
	var cm struct {
		Created int `json:"created"`
		Omitted int `json:"omitted"`
	}
	if err := json.Unmarshal(commit.Body.Bytes(), &cm); err != nil {
		t.Fatal(err)
	}
	if cm.Created != 2 || cm.Omitted != 3 {
		t.Fatalf("commit created/omitted: esperaba 2/3, got %d/%d (%s)", cm.Created, cm.Omitted, commit.Body.String())
	}
	// magic link de primer acceso para CADA alumno creado
	if len(*links) != 2 {
		t.Fatalf("esperaba 2 magic links, got %d", len(*links))
	}
	for _, email := range []string{"juan@escuela.edu.ar", "lucia@escuela.edu.ar"} {
		if u, err := h.store.GetUserByEmail(t.Context(), email); err != nil || u.Role != model.RoleAlumno || u.PasswordHash != "" {
			t.Fatalf("alumno %s mal creado: %v", email, err)
		}
	}
	students := doJSON(t, h, "GET", "/classrooms/"+aulaID+"/students", "", doc)
	if !strings.Contains(students.Body.String(), "Juan López") || !strings.Contains(students.Body.String(), "Lucía Fernández") {
		t.Fatalf("alumnos deben quedar en el aula: %s", students.Body.String())
	}
	if strings.Contains(students.Body.String(), "existente@") {
		t.Fatalf("fila exists no debe duplicarse en el aula: %s", students.Body.String())
	}

	// roles: otro docente 403, alumno 403, invitado guest_read_only, director OK
	doc2ID := seedUser(t, h, "doc2@escuela.edu.ar", "p", model.RoleDocente, false)
	doc2 := sesionDeRol(t, h, doc2ID, model.SessionKindStaff)
	if r := postCSV(t, h, target, csvMixto, map[string]string{"dry_run": "true"}, doc2); r.Code != 403 {
		t.Fatalf("docente ajeno import: esperaba 403, got %d", r.Code)
	}
	alumID := seedUser(t, h, "alum@escuela.edu.ar", "", model.RoleAlumno, false)
	alum := sesionDeRol(t, h, alumID, model.SessionKindPWA)
	if r := postCSV(t, h, target, csvMixto, map[string]string{"dry_run": "true"}, alum); r.Code != 403 {
		t.Fatalf("alumno import: esperaba 403, got %d", r.Code)
	}
	if r := postCSV(t, h, target, csvMixto, map[string]string{"dry_run": "true"}, nil); r.Code != 403 {
		t.Fatalf("anónimo import: esperaba 403, got %d", r.Code)
	}
	if r := postCSV(t, h, target, csvMixto, map[string]string{"dry_run": "true"}, dir); r.Code != 200 {
		t.Fatalf("director import: esperaba 200, got %d (%s)", r.Code, r.Body.String())
	}
}

func TestImportCSV_LimiteYMalformado(t *testing.T) {
	h, _ := newTestHandler(t)
	_, doc, aulaID, _ := setupEscuelaFull(t, h)
	target := "/classrooms/" + aulaID + "/students/import"

	// exactamente 200 pasa
	sb := &strings.Builder{}
	for i := 0; i < model.MaxImportRows; i++ {
		sb.WriteString("Alumno " + strings.Repeat("x", 3) + ",alumno" + strings.Repeat("x", 3) + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "@escuela.edu.ar\n")
	}
	okRec := postCSV(t, h, target, sb.String(), map[string]string{"dry_run": "true"}, doc)
	if okRec.Code != 200 {
		t.Fatalf("200 filas debe pasar: got %d (%s)", okRec.Code, okRec.Body.String())
	}

	// 201 → 400 validation_error
	sb.WriteString("Uno Más,uno@escuela.edu.ar\n")
	overRec := postCSV(t, h, target, sb.String(), map[string]string{"dry_run": "true"}, doc)
	if overRec.Code != 400 || errCode(t, overRec) != "validation_error" {
		t.Fatalf("201 filas: esperaba 400 validation_error, got %d (%s)", overRec.Code, overRec.Body.String())
	}

	// CSV vacío / campo faltante → 400
	if r := postCSV(t, h, target, "", map[string]string{"dry_run": "true"}, doc); r.Code != 400 {
		t.Fatalf("csv vacío: esperaba 400, got %d", r.Code)
	}
}

func TestInviteYResend_AntiEnumeracion(t *testing.T) {
	h, links := newTestHandler(t)
	dir, doc, aulaID, _ := setupEscuelaFull(t, h)
	target := "/classrooms/" + aulaID + "/students/invite"

	inv := doJSON(t, h, "POST", target,
		`{"name":"Lucía Fernández","email":"lucia@escuela.edu.ar","send_invite":true}`, doc)
	if inv.Code != 201 || !strings.Contains(inv.Body.String(), `"first_magic_link_sent":true`) {
		t.Fatalf("invite: esperaba 201+link enviado, got %d (%s)", inv.Code, inv.Body.String())
	}
	if len(*links) != 1 {
		t.Fatalf("invite debe enviar 1 link, envió %d", len(*links))
	}

	dup := doJSON(t, h, "POST", target, `{"name":"X","email":"lucia@escuela.edu.ar","send_invite":false}`, doc)
	if dup.Code != 409 || errCode(t, dup) != "email_already_exists" {
		t.Fatalf("invite duplicado: esperaba 409, got %d (%s)", dup.Code, dup.Body.String())
	}

	// send_invite=false: cuenta creada sin link
	silent := doJSON(t, h, "POST", target, `{"name":"Mudo","email":"mudo@escuela.edu.ar","send_invite":false}`, doc)
	if silent.Code != 201 || !strings.Contains(silent.Body.String(), `"first_magic_link_sent":false`) || len(*links) != 1 {
		t.Fatalf("invite silencioso: %d (%s) links=%d", silent.Code, silent.Body.String(), len(*links))
	}
	mudo := docIDFromList(t, h, "mudo@escuela.edu.ar")

	// resend-magic-link: 202 genérico SIEMPRE ([C2])
	resend := doJSON(t, h, "POST", "/students/"+mudo+"/resend-magic-link", "", doc)
	fantasma := doJSON(t, h, "POST", "/students/no-existe/resend-magic-link", "", doc)
	if resend.Code != 202 || fantasma.Code != 202 || resend.Body.String() != fantasma.Body.String() {
		t.Fatalf("resend anti-enumeración: %d/%d bodies distintos", resend.Code, fantasma.Code)
	}
	if len(*links) != 2 { // solo el usuario real recibió link
		t.Fatalf("resend debe enviar 1 link más, hay %d", len(*links))
	}

	// docente ajeno: mismo 202 pero SIN link (no ve alumnos que no son suyos)
	doc2ID := seedUser(t, h, "doc2@escuela.edu.ar", "p", model.RoleDocente, false)
	doc2 := sesionDeRol(t, h, doc2ID, model.SessionKindStaff)
	antes := len(*links)
	if r := doJSON(t, h, "POST", "/students/"+mudo+"/resend-magic-link", "", doc2); r.Code != 202 {
		t.Fatalf("resend docente ajeno: esperaba 202, got %d", r.Code)
	}
	if len(*links) != antes {
		t.Fatalf("docente ajeno no debe disparar link")
	}

	// director puede reenviar; invitado/alumno chocan con la puerta
	if r := doJSON(t, h, "POST", "/students/"+mudo+"/resend-magic-link", "", dir); r.Code != 202 {
		t.Fatalf("resend director: esperaba 202, got %d", r.Code)
	}
	if r := doJSON(t, h, "POST", "/students/x/resend-magic-link", ""); r.Code != 403 && r.Code != 401 {
		t.Fatalf("resend anónimo: esperaba 401/403, got %d", r.Code)
	}
}

func TestImpersonacion_CicloYAuditoria(t *testing.T) {
	h, _ := newTestHandler(t)
	dir, doc, _, dirID := setupEscuelaFull(t, h)

	// reason obligatorio
	noReason := doJSON(t, h, "POST", "/admin/impersonations",
		`{"user_id":"`+docIDFromList(t, h, "doc@escuela.edu.ar")+`"}`, dir)
	if noReason.Code != 400 {
		t.Fatalf("impersonación sin reason: esperaba 400, got %d", noReason.Code)
	}
	fantasma := doJSON(t, h, "POST", "/admin/impersonations", `{"user_id":"no-existe","reason":"x"}`, dir)
	if fantasma.Code != 404 {
		t.Fatalf("target inexistente: esperaba 404, got %d", fantasma.Code)
	}

	docID := docIDFromList(t, h, "doc@escuela.edu.ar")
	start := doJSON(t, h, "POST", "/admin/impersonations",
		`{"user_id":"`+docID+`","reason":"configurar plantilla del aula 5A a pedido del docente"}`, dir)
	if start.Code != 201 {
		t.Fatalf("start: esperaba 201, got %d (%s)", start.Code, start.Body.String())
	}
	impCookie := cookieSesion(start)
	if impCookie == nil {
		t.Fatal("start sin Set-Cookie")
	}
	if !strings.Contains(start.Body.String(), `"impersonated_by":"`+dirID+`"`) {
		t.Fatalf("respuesta sin impersonated_by: %s", start.Body.String())
	}

	// actúa COMO el docente: ve SOLO sus aulas (vista docente, no la global del director)
	comoDoc := doJSON(t, h, "GET", "/classrooms", "", impCookie)
	if comoDoc.Code != 200 || !strings.Contains(comoDoc.Body.String(), `"join_code"`) ||
		strings.Count(comoDoc.Body.String(), `"id"`) != 1 {
		t.Fatalf("impersonado debe ver vista del docente (1 aula con join_code): %d (%s)", comoDoc.Code, comoDoc.Body.String())
	}
	// banner: GET /auth/me marca impersonated_by
	me := doJSON(t, h, "GET", "/auth/me", "", impCookie)
	if !strings.Contains(me.Body.String(), `"impersonated_by":"`+dirID+`"`) ||
		!strings.Contains(me.Body.String(), `"role":"docente"`) {
		t.Fatalf("me impersonado: %s", me.Body.String())
	}
	// y puede actuar sobre recursos del docente (crear consigna §4 usa requireStaff)
	crearAula(t, h, impCookie, "Creada como docente")

	// auditoría: start registrado con quién/a quién/motivo/timestamp
	logs := doJSON(t, h, "GET", "/admin/audit-log?action=impersonation.start", "", dir)
	if logs.Code != 200 ||
		!strings.Contains(logs.Body.String(), `"actor_id":"`+dirID+`"`) ||
		!strings.Contains(logs.Body.String(), `"target_user_id":"`+docID+`"`) ||
		!strings.Contains(logs.Body.String(), "plantilla del aula 5A") {
		t.Fatalf("audit-log sin start auditado: %d (%s)", logs.Code, logs.Body.String())
	}

	// end: vuelve a la sesión del director (logout de impersonación ≠ logout del director)
	end := doJSON(t, h, "DELETE", "/admin/impersonations/current", "", impCookie)
	if end.Code != 204 {
		t.Fatalf("end: esperaba 204, got %d (%s)", end.Code, end.Body.String())
	}
	backCookie := cookieSesion(end)
	if backCookie == nil {
		t.Fatal("end debe devolver cookie de director")
	}
	meBack := doJSON(t, h, "GET", "/auth/me", "", backCookie)
	if !strings.Contains(meBack.Body.String(), `"role":"director"`) || strings.Contains(meBack.Body.String(), "impersonated_by") {
		t.Fatalf("sesión de director restaurada: %s", meBack.Body.String())
	}
	// la sesión impersonada está muerta
	if r := doJSON(t, h, "GET", "/auth/me", "", impCookie); r.Code != 401 && r.Code != 403 {
		t.Fatalf("sesión impersonada debe morir: got %d", r.Code)
	}
	// y la sesión original del director sigue viva aparte
	if r := doJSON(t, h, "GET", "/school/teachers", "", dir); r.Code != 200 {
		t.Fatalf("director original sigue logueado: got %d", r.Code)
	}

	logsEnd := doJSON(t, h, "GET", "/admin/audit-log?action=impersonation.end", "", dir)
	if !strings.Contains(logsEnd.Body.String(), `"target_user_id":"`+docID+`"`) {
		t.Fatalf("audit-log sin end: %s", logsEnd.Body.String())
	}

	// filtros: action filtra, actor filtra
	allLogs := doJSON(t, h, "GET", "/admin/audit-log", "", dir)
	if !strings.Contains(allLogs.Body.String(), "teacher.create") == false &&
		!strings.Contains(allLogs.Body.String(), "impersonation.start") {
		t.Fatalf("audit-log completo: %s", allLogs.Body.String())
	}
	fromISO := time.Now().Add(time.Hour).Format(time.RFC3339)
	futuro := doJSON(t, h, "GET", "/admin/audit-log?from="+fromISO, "", dir)
	if strings.Contains(futuro.Body.String(), "impersonation.start") {
		t.Fatalf("filtro from futuro no debe traer entradas viejas: %s", futuro.Body.String())
	}
	badDate := doJSON(t, h, "GET", "/admin/audit-log?from=no-es-fecha", "", dir)
	if badDate.Code != 400 {
		t.Fatalf("from malformado: esperaba 400, got %d", badDate.Code)
	}

	// roles: docente y anónimo jamás
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
	}{
		{"docente", doc},
		{"anónimo", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cookies []*http.Cookie
			if tc.cookie != nil {
				cookies = append(cookies, tc.cookie)
			}
			if r := doJSON(t, h, "GET", "/admin/audit-log", "", cookies...); r.Code != 401 && r.Code != 403 {
				t.Fatalf("audit-log: esperaba 401/403, got %d", r.Code)
			}
			if r := doJSON(t, h, "POST", "/admin/impersonations", `{"user_id":"x","reason":"y"}`, cookies...); r.Code != 401 && r.Code != 403 {
				t.Fatalf("impersonar: esperaba 401/403, got %d", r.Code)
			}
			if r := doJSON(t, h, "DELETE", "/admin/impersonations/current", "", cookies...); r.Code != 401 && r.Code != 403 && r.Code != 404 {
				t.Fatalf("end impersonación: esperaba 401/403/404, got %d", r.Code)
			}
		})
	}
}
