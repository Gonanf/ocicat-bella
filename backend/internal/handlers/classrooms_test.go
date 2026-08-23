package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// --- helpers Fase 4 ---

func sesionDeRol(t *testing.T, h *Handler, userID string, kind model.SessionKind) *http.Cookie {
	t.Helper()
	now := time.Now()
	sess := &model.Session{
		ID: newID(), UserID: userID, Kind: kind,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := h.store.CreateSession(t.Context(), sess); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: middleware.SessionCookieName, Value: sess.ID}
}

func seedEscuela(t *testing.T, h *Handler, nombre, codigo string, activa bool) {
	t.Helper()
	school := &model.School{
		ID: newID(), Name: nombre, GlobalCode: codigo,
		GlobalCodeActive: activa, CreatedAt: time.Now(),
	}
	if err := h.store.CreateSchool(t.Context(), school); err != nil {
		t.Fatal(err)
	}
}

func crearAula(t *testing.T, h *Handler, cookie *http.Cookie, name string) map[string]any {
	t.Helper()
	rec := doJSON(t, h, "POST", "/classrooms",
		`{"name":"`+name+`","course":"5°","shift":"mañana"}`, cookie)
	if rec.Code != 201 {
		t.Fatalf("crear aula %q: esperaba 201, got %d (%s)", name, rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func unirse(t *testing.T, h *Handler, code string, cookie *http.Cookie) *int {
	t.Helper()
	rec := doJSON(t, h, "POST", "/classrooms/join", `{"code":"`+code+`"}`, cookie)
	return &rec.Code
}

// --- CRUD y scopes por rol (§3) ---

func TestAulas_CRUDPorRol(t *testing.T) {
	h, _ := newTestHandler(t)
	docID := seedUser(t, h, "doc@escuela.edu.ar", "pass12345", model.RoleDocente, false)
	doc := sesionDeRol(t, h, docID, model.SessionKindStaff)
	doc2ID := seedUser(t, h, "doc2@escuela.edu.ar", "pass12345", model.RoleDocente, false)
	doc2 := sesionDeRol(t, h, doc2ID, model.SessionKindStaff)
	dirID := seedUser(t, h, "dir@escuela.edu.ar", "pass12345", model.RoleDirector, false)
	dir := sesionDeRol(t, h, dirID, model.SessionKindStaff)
	alumID := seedUser(t, h, "alum@escuela.edu.ar", "", model.RoleAlumno, false)
	alum := sesionDeRol(t, h, alumID, model.SessionKindPWA)

	cases := []struct {
		name     string
		method   string
		target   string
		body     string
		cookie   *http.Cookie
		wantCode int
		wantErr  string
	}{
		{"crear docente → 201", "POST", "/classrooms", `{"name":"Programación 5A"}`, doc, 201, ""},
		{"crear director → 201", "POST", "/classrooms", `{"name":"Química 3B"}`, dir, 201, ""},
		{"crear alumno → 403", "POST", "/classrooms", `{"name":"x"}`, alum, 403, "forbidden"},
		{"crear sin nombre → 400", "POST", "/classrooms", `{"course":"5°"}`, doc, 400, "validation_error"},
		{"crear anónimo → guest_read_only", "POST", "/classrooms", `{"name":"x"}`, nil, 403, "guest_read_only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cookies []*http.Cookie
			if tc.cookie != nil {
				cookies = append(cookies, tc.cookie)
			}
			rec := doJSON(t, h, tc.method, tc.target, tc.body, cookies...)
			if rec.Code != tc.wantCode || (tc.wantErr != "" && errCode(t, rec) != tc.wantErr) {
				t.Fatalf("esperaba %d %q, got %d (%s)", tc.wantCode, tc.wantErr, rec.Code, rec.Body.String())
			}
		})
	}

	aula := crearAula(t, h, doc, "Programación 5A")
	aulaID := aula["id"].(string)
	code := aula["join_code"].(string)
	if l := len(code); l < 6 || l > 8 {
		t.Fatalf("join_code debe tener 6–8 chars, got %q", code)
	}

	// PATCH: dueño sí, otro docente no, director sí
	rec := doJSON(t, h, "PATCH", "/classrooms/"+aulaID, `{"name":"Prog 5A v2"}`, doc)
	if rec.Code != 200 {
		t.Fatalf("patch dueño: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "PATCH", "/classrooms/"+aulaID, `{"name":"hack"}`, doc2)
	if rec.Code != 403 || errCode(t, rec) != "forbidden" {
		t.Fatalf("patch ajeno: esperaba 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "PATCH", "/classrooms/"+aulaID, `{"shift":"tarde"}`, dir)
	if rec.Code != 200 {
		t.Fatalf("patch director: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// GET puntual: dueño y director sí; alumno ni ajeno; inexistente 404
	for _, tc := range []struct {
		name   string
		target string
		cookie *http.Cookie
		want   int
	}{
		{"dueño ve", "/classrooms/" + aulaID, doc, 200},
		{"director ve", "/classrooms/" + aulaID, dir, 200},
		{"alumno 403", "/classrooms/" + aulaID, alum, 403},
		{"ajeno 403", "/classrooms/" + aulaID, doc2, 403},
		{"inexistente 404", "/classrooms/noexiste", doc2, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := doJSON(t, h, "GET", tc.target, "", tc.cookie)
			if r.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, r.Code, r.Body.String())
			}
		})
	}

	// DELETE archiva → 204; el join posterior da invalid_code
	rec = doJSON(t, h, "DELETE", "/classrooms/"+aulaID, "", doc)
	if rec.Code != 204 {
		t.Fatalf("delete dueño: esperaba 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := unirse(t, h, code, alum); *got != 404 {
		t.Fatalf("join tras archivar: esperaba 404, got %d", *got)
	}
}

func TestAulas_ListadoScopePorRol(t *testing.T) {
	h, _ := newTestHandler(t)
	docID := seedUser(t, h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	doc := sesionDeRol(t, h, docID, model.SessionKindStaff)
	doc2ID := seedUser(t, h, "doc2@escuela.edu.ar", "p", model.RoleDocente, false)
	doc2 := sesionDeRol(t, h, doc2ID, model.SessionKindStaff)
	dirID := seedUser(t, h, "dir@escuela.edu.ar", "p", model.RoleDirector, false)
	dir := sesionDeRol(t, h, dirID, model.SessionKindStaff)
	alumID := seedUser(t, h, "alum@escuela.edu.ar", "", model.RoleAlumno, false)
	alum := sesionDeRol(t, h, alumID, model.SessionKindPWA)

	a := crearAula(t, h, doc, "Aula A")
	b := crearAula(t, h, doc2, "Aula B")
	_ = b
	if got := unirse(t, h, a["join_code"].(string), alum); *got != 200 {
		t.Fatalf("join: esperaba 200, got %d", *got)
	}

	type vista struct {
		Classrooms []map[string]any `json:"classrooms"`
	}
	getLista := func(cookie *http.Cookie) vista {
		t.Helper()
		rec := doJSON(t, h, "GET", "/classrooms", "", cookie)
		if rec.Code != 200 {
			t.Fatalf("listado: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
		}
		var v vista
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	// alumno: sus aulas + up_to_date, SIN join_code
	v := getLista(alum)
	if len(v.Classrooms) != 1 || v.Classrooms[0]["name"] != "Aula A" {
		t.Fatalf("alumno debe ver solo Aula A: %+v", v.Classrooms)
	}
	if _, ok := v.Classrooms[0]["pending_assignments"]; !ok {
		t.Fatal("alumno espera pending_assignments (§3)")
	}
	if _, ok := v.Classrooms[0]["join_code"]; ok {
		t.Fatal("el alumno NO debe ver el join_code")
	}

	// docente: las que dicta + stats
	v = getLista(doc)
	if len(v.Classrooms) != 1 {
		t.Fatalf("docente debe ver 1 aula: %+v", v.Classrooms)
	}
	c := v.Classrooms[0]
	if c["students_count"].(float64) != 1 || c["active_assignments"].(float64) != 0 ||
		c["running_sandboxes"].(float64) != 0 || c["join_code"] == "" {
		t.Fatalf("stats del docente incompletas: %+v", c)
	}

	// director: TODAS + teacher_name
	v = getLista(dir)
	if len(v.Classrooms) != 2 {
		t.Fatalf("director debe ver 2 aulas: %+v", v.Classrooms)
	}
	nombres := map[string]bool{}
	for _, cc := range v.Classrooms {
		nombres[cc["name"].(string)] = true
		if tn, ok := cc["teacher_name"]; !ok || tn == "" {
			t.Fatalf("director espera teacher_name: %+v", cc)
		}
	}
	if !nombres["Aula A"] || !nombres["Aula B"] {
		t.Fatalf("faltan aulas en vista director: %+v", nombres)
	}

	// invitado autenticado: lista vacía, no error
	guestID := seedUser(t, h, "invitado-x@instancia.local", "", model.RoleInvitado, false)
	v = getLista(sesionDeRol(t, h, guestID, model.SessionKindGuest))
	if len(v.Classrooms) != 0 {
		t.Fatalf("invitado debe ver lista vacía: %+v", v.Classrooms)
	}
}

// --- Rotación de join_code (CU-10): el anterior muere AL INSTANTE ---

func TestAulas_RotacionInvalidaCodigoViejo(t *testing.T) {
	h, _ := newTestHandler(t)
	docID := seedUser(t, h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	doc := sesionDeRol(t, h, docID, model.SessionKindStaff)
	dirID := seedUser(t, h, "dir@escuela.edu.ar", "p", model.RoleDirector, false)
	dir := sesionDeRol(t, h, dirID, model.SessionKindStaff)
	alum1ID := seedUser(t, h, "a1@escuela.edu.ar", "", model.RoleAlumno, false)
	alum1 := sesionDeRol(t, h, alum1ID, model.SessionKindPWA)
	alum2ID := seedUser(t, h, "a2@escuela.edu.ar", "", model.RoleAlumno, false)
	alum2 := sesionDeRol(t, h, alum2ID, model.SessionKindPWA)

	aula := crearAula(t, h, doc, "Con código")
	aulaID := aula["id"].(string)
	viejo := aula["join_code"].(string)

	if got := unirse(t, h, viejo, alum1); *got != 200 {
		t.Fatalf("join con código inicial: esperaba 200, got %d", *got)
	}

	// rotar (también lo puede el director)
	rec := doJSON(t, h, "POST", "/classrooms/"+aulaID+"/join_code/rotate", "", dir)
	if rec.Code != 200 {
		t.Fatalf("rotar: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Code == "" {
		t.Fatalf("rotate sin code: %v (%s)", err, rec.Body.String())
	}
	if resp.Code == viejo {
		t.Fatal("el código nuevo debe ser distinto del viejo")
	}

	// el viejo muere al instante…
	if got := unirse(t, h, viejo, alum2); *got != 404 {
		t.Fatalf("join con código viejo: esperaba 404 invalid_code, got %d", *got)
	}
	if ec := errCode(t, doJSON(t, h, "POST", "/classrooms/join", `{"code":"`+viejo+`"}`, alum2)); ec != "invalid_code" {
		t.Fatalf("esperaba invalid_code, got %q", ec)
	}
	// …y el nuevo funciona
	if got := unirse(t, h, resp.Code, alum2); *got != 200 {
		t.Fatalf("join con código nuevo: esperaba 200, got %d", *got)
	}

	// GET join_code refleja el actual
	rec = doJSON(t, h, "GET", "/classrooms/"+aulaID+"/join_code", "", doc)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), resp.Code) {
		t.Fatalf("join_code GET: esperaba %q, got %d (%s)", resp.Code, rec.Code, rec.Body.String())
	}

	// alumnos que ya entraron siguen dentro
	students := doJSON(t, h, "GET", "/classrooms/"+aulaID+"/students", "", doc)
	if students.Code != 200 || !strings.Contains(students.Body.String(), `"total_submissions":0`) {
		t.Fatalf("students: esperaba 200 con entregas 0, got %d (%s)", students.Code, students.Body.String())
	}
}

// --- Join: duplicado, inválido, rate limit, leave/rejoin, baja de alumno ---

func TestAulas_JoinDuplicadoEInvalido(t *testing.T) {
	h, _ := newTestHandler(t)
	docID := seedUser(t, h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	doc := sesionDeRol(t, h, docID, model.SessionKindStaff)
	alumID := seedUser(t, h, "a@escuela.edu.ar", "", model.RoleAlumno, false)
	alum := sesionDeRol(t, h, alumID, model.SessionKindPWA)

	aula := crearAula(t, h, doc, "Aula")
	code := aula["join_code"].(string)

	if got := unirse(t, h, code, alum); *got != 200 {
		t.Fatalf("primer join: esperaba 200, got %d", *got)
	}
	rec := doJSON(t, h, "POST", "/classrooms/join", `{"code":"`+code+`"}`, alum)
	if rec.Code != 409 || errCode(t, rec) != "already_member" {
		t.Fatalf("segundo join: esperaba 409 already_member, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "POST", "/classrooms/join", `{"code":"ZZZZZZ"}`, alum)
	if rec.Code != 404 || errCode(t, rec) != "invalid_code" {
		t.Fatalf("join inválido: esperaba 404 invalid_code, got %d (%s)", rec.Code, rec.Body.String())
	}
	// docente NO puede usar join (es de alumnos)
	docRec := doJSON(t, h, "POST", "/classrooms/join", `{"code":"`+code+`"}`, doc)
	if docRec.Code != 403 {
		t.Fatalf("join docente: esperaba 403, got %d", docRec.Code)
	}
}

func TestAulas_JoinRateLimit(t *testing.T) {
	h, _ := newTestHandler(t)
	h.joinRL = middleware.NewIPRateLimiter(2, 2)
	alumID := seedUser(t, h, "a@escuela.edu.ar", "", model.RoleAlumno, false)
	alum := sesionDeRol(t, h, alumID, model.SessionKindPWA)

	for i := 0; i < 2; i++ {
		if got := unirse(t, h, "XXXXXX", alum); *got != 404 {
			t.Fatalf("intento %d: esperaba 404, got %d", i+1, *got)
		}
	}
	rec := doJSON(t, h, "POST", "/classrooms/join", `{"code":"XXXXXX"}`, alum)
	if rec.Code != 429 || errCode(t, rec) != "rate_limited" {
		t.Fatalf("tercer intento: esperaba 429 rate_limited, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestAulas_LeaveYBajaDeAlumno(t *testing.T) {
	h, _ := newTestHandler(t)
	docID := seedUser(t, h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	doc := sesionDeRol(t, h, docID, model.SessionKindStaff)
	alumID := seedUser(t, h, "a@escuela.edu.ar", "", model.RoleAlumno, false)
	alum := sesionDeRol(t, h, alumID, model.SessionKindPWA)
	otroID := seedUser(t, h, "b@escuela.edu.ar", "", model.RoleAlumno, false)
	otro := sesionDeRol(t, h, otroID, model.SessionKindPWA)

	aula := crearAula(t, h, doc, "Aula")
	aulaID := aula["id"].(string)
	code := aula["join_code"].(string)
	unirse(t, h, code, alum)
	unirse(t, h, code, otro)

	// leave: 204, y puede volver a entrar después
	rec := doJSON(t, h, "DELETE", "/classrooms/"+aulaID+"/membership/me", "", alum)
	if rec.Code != 204 {
		t.Fatalf("leave: esperaba 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "DELETE", "/classrooms/"+aulaID+"/membership/me", "", alum)
	if rec.Code != 404 || errCode(t, rec) != "not_found" {
		t.Fatalf("leave sin membresía: esperaba 404, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := unirse(t, h, code, alum); *got != 200 {
		t.Fatalf("rejoin tras leave: esperaba 200, got %d", *got)
	}

	// students lista ambos
	rec = doJSON(t, h, "GET", "/classrooms/"+aulaID+"/students", "", doc)
	var list struct {
		Students []struct {
			UserID           string `json:"user_id"`
			Status           string `json:"status"`
			TotalSubmissions int    `json:"total_submissions"`
		} `json:"students"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Students) != 2 {
		t.Fatalf("students esperaba 2, got %d (%s)", len(list.Students), rec.Body.String())
	}
	for _, s := range list.Students {
		if s.Status != "active" || s.TotalSubmissions != 0 {
			t.Fatalf("fila de student malformada: %+v", s)
		}
	}

	// baja por parte del docente: 204, cuenta intacta, puede reentrar
	rec = doJSON(t, h, "DELETE", "/classrooms/"+aulaID+"/students/"+alumID, "", doc)
	if rec.Code != 204 {
		t.Fatalf("baja: esperaba 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "DELETE", "/classrooms/"+aulaID+"/students/"+alumID, "", doc)
	if rec.Code != 404 {
		t.Fatalf("baja repetida: esperaba 404, got %d", rec.Code)
	}
	if u, err := h.store.GetUser(t.Context(), alumID); err != nil || u == nil {
		t.Fatal("la cuenta del alumno NO debe borrarse en una baja")
	}
	if got := unirse(t, h, code, alum); *got != 200 {
		t.Fatalf("rejoin tras baja: esperaba 200, got %d", *got)
	}

	// alumno NO puede dar de baja a otro
	rec = doJSON(t, h, "DELETE", "/classrooms/"+aulaID+"/students/"+otroID, "", alum)
	if rec.Code != 403 {
		t.Fatalf("baja por alumno: esperaba 403, got %d", rec.Code)
	}
}
