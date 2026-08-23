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

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// subirMaterial arma el multipart y hace POST al target.
func subirMaterial(t *testing.T, h *Handler, target, filename, content string, fields map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if filename != "" {
		fw, err := mw.CreateFormFile("file", filename)
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
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type fixtureMateriales struct {
	h         *Handler
	doc       *http.Cookie
	doc2      *http.Cookie
	dir       *http.Cookie
	alumA     *http.Cookie // miembro del aula A
	alumFuera *http.Cookie // sin aulas
	guest     *http.Cookie
	aulaAID   string
	pub       string // id material public
	sch       string // school
	clsA      string // classroom aula A
	clsB      string // classroom aula B
}

func seedFixtureMateriales(t *testing.T) *fixtureMateriales {
	f := &fixtureMateriales{}
	f.h, _ = newTestHandler(t)
	h := f.h

	docID := seedUser(t, h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	f.doc = sesionDeRol(t, h, docID, model.SessionKindStaff)
	doc2ID := seedUser(t, h, "doc2@escuela.edu.ar", "p", model.RoleDocente, false)
	f.doc2 = sesionDeRol(t, h, doc2ID, model.SessionKindStaff)
	dirID := seedUser(t, h, "dir@escuela.edu.ar", "p", model.RoleDirector, false)
	f.dir = sesionDeRol(t, h, dirID, model.SessionKindStaff)
	alumAID := seedUser(t, h, "miembro@escuela.edu.ar", "", model.RoleAlumno, false)
	f.alumA = sesionDeRol(t, h, alumAID, model.SessionKindPWA)
	fueraID := seedUser(t, h, "fuera@escuela.edu.ar", "", model.RoleAlumno, false)
	f.alumFuera = sesionDeRol(t, h, fueraID, model.SessionKindPWA)

	aulaA := crearAula(t, h, f.doc, "Aula A")
	f.aulaAID = aulaA["id"].(string)
	aulaB := crearAula(t, h, f.doc2, "Aula B")

	if got := unirse(t, h, aulaA["join_code"].(string), f.alumA); *got != 200 {
		t.Fatalf("join alumno A: got %d", *got)
	}

	subir := func(aulaID, filename, vis, subject string) string {
		t.Helper()
		rec := subirMaterial(t, h, "/classrooms/"+aulaID+"/materials", filename, "contenido "+filename,
			map[string]string{"title": strings.TrimSuffix(filename, ".pdf"), "visibility": vis, "subject": subject}, f.doc)
		if rec.Code != 201 {
			t.Fatalf("subir %s en %s: esperaba 201, got %d (%s)", filename, aulaID, rec.Code, rec.Body.String())
		}
		var m struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m.ID
	}
	// clsB lo sube doc2 en su aula
	recB := subirMaterial(t, h, "/classrooms/"+aulaB["id"].(string)+"/materials", "b.pdf", "contenido b",
		map[string]string{"title": "b", "visibility": "classroom"}, f.doc2)
	var mb struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(recB.Body.Bytes(), &mb)

	f.pub = subir(f.aulaAID, "publico.pdf", "public", "matematica")
	f.sch = subir(f.aulaAID, "escuela.pdf", "school", "historia")
	f.clsA = subir(f.aulaAID, "clase.pdf", "classroom", "matematica")
	f.clsB = mb.ID

	// sesión de invitado con código global (necesita escuela activa)
	seedEscuela(t, h, "Escuela Test", "ESCUELA-TEST1", true)
	rec := doJSON(t, h, "POST", "/guest/sessions", `{"code":"ESCUELA-TEST1"}`)
	if rec.Code != 201 {
		t.Fatalf("guest session: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	f.guest = cookieSesion(rec)
	if f.guest == nil {
		t.Fatal("guest session sin Set-Cookie")
	}
	return f
}

// Matriz de visibilidad §6: quién ve qué, en metadata y listado.
func TestMateriales_VisibilidadPorQuienPregunta(t *testing.T) {
	f := seedFixtureMateriales(t)

	cases := []struct {
		name       string
		cookie     *http.Cookie // nil = anónimo
		scope      string       // "" | public | mine | classroom:A|classroom:B
		subject    string
		wantTitles []string
	}{
		{"anónimo → solo public", nil, "", "", []string{"publico"}},
		{"anónimo scope=public → public", nil, "public", "", []string{"publico"}},
		{"anónimo scope=mine → nada", nil, "mine", "", nil},
		{"anónimo classroom:A → solo public", nil, "classroom:" + f.aulaAID, "", []string{"publico"}},
		{"invitado → public+school", f.guest, "", "", []string{"publico", "escuela"}},
		{"invitado classroom:A → public+school", f.guest, "classroom:" + f.aulaAID, "", []string{"publico", "escuela"}},
		{"miembro de A → pub+school+clsA", f.alumA, "", "", []string{"publico", "escuela", "clase"}},
		{"alumno externo → pub+school", f.alumFuera, "", "", []string{"publico", "escuela"}},
		{"docente dueño → sus 3", f.doc, "", "", []string{"publico", "escuela", "clase"}},
		{"docente ajeno → pub+school+clsB propia", f.doc2, "", "", []string{"publico", "escuela", "b"}},
		{"director → todo", f.dir, "", "", []string{"publico", "escuela", "clase", "b"}},
		{"scope mine docente dueño", f.doc, "mine", "", []string{"publico", "escuela", "clase"}},
		{"subject filtra para miembro", f.alumA, "", "matematica", []string{"publico", "clase"}},
		{"subject filtra para anónimo", nil, "", "historia", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := "/materials?scope=" + tc.scope + "&subject=" + tc.subject
			req := httptest.NewRequest("GET", target, nil)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			rec := httptest.NewRecorder()
			f.h.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
			}
			var resp struct {
				Materials []struct {
					Title            string `json:"title"`
					Type             string `json:"type"`
					PreviewAvailable bool   `json:"preview_available"`
				} `json:"materials"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, m := range resp.Materials {
				got[m.Title] = true
				if m.Type != "pdf" || !m.PreviewAvailable {
					t.Fatalf("campos §6 incompletos: %+v", m)
				}
			}
			if len(got) != len(tc.wantTitles) {
				t.Fatalf("esperaba %v, got %v", tc.wantTitles, got)
			}
			for _, want := range tc.wantTitles {
				if !got[want] {
					t.Fatalf("faltaba %q en %v", want, got)
				}
			}
		})
	}

	// metadata puntual: 404 indistinguible cuando no hay acceso
	meta := []struct {
		name   string
		cookie *http.Cookie
		id     func(f *fixtureMateriales) string
		want   int
	}{
		{"anónimo ve public", nil, func(f *fixtureMateriales) string { return f.pub }, 200},
		{"anónimo NO ve school", nil, func(f *fixtureMateriales) string { return f.sch }, 404},
		{"anónimo NO ve classroom", nil, func(f *fixtureMateriales) string { return f.clsA }, 404},
		{"invitado ve school", f.guest, func(f *fixtureMateriales) string { return f.sch }, 200},
		{"invitado NO ve classroom", f.guest, func(f *fixtureMateriales) string { return f.clsA }, 404},
		{"externo NO ve classroom", f.alumFuera, func(f *fixtureMateriales) string { return f.clsA }, 404},
		{"inexistente 404 igual", f.dir, func(*fixtureMateriales) string { return "noexiste" }, 404},
	}
	for _, tc := range meta {
		t.Run("meta: "+tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/materials/"+tc.id(f), nil)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			rec := httptest.NewRecorder()
			f.h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
			if tc.want == 404 && errCode(t, rec) != "not_found" {
				t.Fatalf("esperaba not_found, got %q", errCode(t, rec))
			}
		})
	}
}

func TestMateriales_UploadValidaciones(t *testing.T) {
	f := seedFixtureMateriales(t)

	cases := []struct {
		name     string
		cookie   *http.Cookie
		filename string
		fields   map[string]string
		size     int
		wantCode int
		wantErr  string
	}{
		{"sin file → 400", f.doc, "", map[string]string{"title": "x", "visibility": "public"}, 0, 400, "validation_error"},
		{"sin title → 400", f.doc, "a.pdf", map[string]string{"visibility": "public"}, 10, 400, "validation_error"},
		{"visibility inválida → 400", f.doc, "a.pdf", map[string]string{"title": "x", "visibility": "world"}, 10, 400, "validation_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := strings.Repeat("x", tc.size)
			rec := subirMaterial(t, f.h, "/classrooms/"+f.aulaAID+"/materials", tc.filename, content, tc.fields, tc.cookie)
			if rec.Code != tc.wantCode || errCode(t, rec) != tc.wantErr {
				t.Fatalf("esperaba %d %q, got %d (%s)", tc.wantCode, tc.wantErr, rec.Code, rec.Body.String())
			}
		})
	}

	// archivo grande explícito
	rec := subirMaterial(t, f.h, "/classrooms/"+f.aulaAID+"/materials", "grande.pdf",
		strings.Repeat("x", 11<<20), map[string]string{"title": "x", "visibility": "public"}, f.doc)
	if rec.Code != 400 || errCode(t, rec) != "file_too_large" {
		t.Fatalf(">10MB: esperaba 400 file_too_large, got %d (%s)", rec.Code, rec.Body.String())
	}

	// alumno no sube materiales
	rec = subirMaterial(t, f.h, "/classrooms/"+f.aulaAID+"/materials", "a.pdf", "x",
		map[string]string{"title": "x", "visibility": "public"}, f.alumA)
	if rec.Code != 403 {
		t.Fatalf("alumno subiendo: esperaba 403, got %d", rec.Code)
	}
}

func TestMateriales_FileStreamYRange(t *testing.T) {
	f := seedFixtureMateriales(t)

	getFile := func(id, rangeHeader, query string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "/materials/"+id+"/file"+query, nil)
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, req)
		return rec
	}

	// descarga completa inline
	rec := getFile(f.pub, "", "", nil)
	if rec.Code != 200 || rec.Body.Len() != len("contenido publico.pdf") {
		t.Fatalf("file completo: esperaba 200/%d bytes, got %d/%d", len("contenido publico.pdf"), rec.Code, rec.Body.Len())
	}

	// Range básico bytes=0-9 → 206 con Content-Range
	full := "contenido publico.pdf"
	rec = getFile(f.pub, "bytes=0-9", "", nil)
	if rec.Code != 206 {
		t.Fatalf("range: esperaba 206, got %d (%s)", rec.Code, rec.Body.String())
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 0-9/"+strconv.Itoa(len(full)) {
		t.Fatalf("Content-Range inesperado: %q", cr)
	}
	if rec.Body.String() != full[:10] {
		t.Fatalf("rango incorrecto: %q", rec.Body.String())
	}

	// Range sobre cola
	rec = getFile(f.pub, "bytes=-5", "", nil)
	if rec.Code != 206 || rec.Body.String() != full[len(full)-5:] {
		t.Fatalf("sufijo: esperaba últimos 5 bytes, got %d %q", rec.Code, rec.Body.String())
	}

	// ?download=1 fuerza attachment
	rec = getFile(f.pub, "", "?download=1", nil)
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "publico.pdf") {
		t.Fatalf("download=1 debe forzar attachment con nombre: %q", cd)
	}

	// acceso denegado → 404 indistinguible (material classroom para externo)
	rec = getFile(f.clsA, "", "", f.alumFuera)
	if rec.Code != 404 {
		t.Fatalf("file de classroom para externo: esperaba 404, got %d", rec.Code)
	}
	// y el miembro sí lo baja
	rec = getFile(f.clsA, "", "", f.alumA)
	if rec.Code != 200 {
		t.Fatalf("file de classroom para miembro: esperaba 200, got %d", rec.Code)
	}
}

func TestMateriales_PatchDeleteAutorODirector(t *testing.T) {
	f := seedFixtureMateriales(t)

	// PATCH por autor
	rec := doJSON(t, f.h, "PATCH", "/materials/"+f.clsA, `{"title":"Clase v2","visibility":"school"}`, f.doc)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"visibility":"school"`) {
		t.Fatalf("patch autor: esperaba 200 school, got %d (%s)", rec.Code, rec.Body.String())
	}

	// visibility inválida en PATCH
	rec = doJSON(t, f.h, "PATCH", "/materials/"+f.clsA, `{"visibility":"mundo"}`, f.doc)
	if rec.Code != 400 {
		t.Fatalf("patch visibility inválida: esperaba 400, got %d", rec.Code)
	}

	// PATCH por director (no autor) permitido
	rec = doJSON(t, f.h, "PATCH", "/materials/"+f.clsA, `{"subject":"geografia"}`, f.dir)
	if rec.Code != 200 {
		t.Fatalf("patch director: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// PATCH por otro docente que SÍ puede verlo (school ahora): 403
	rec = doJSON(t, f.h, "PATCH", "/materials/"+f.clsA, `{"title":"robado"}`, f.doc2)
	if rec.Code != 403 {
		t.Fatalf("patch ajeno: esperaba 403, got %d (%s)", rec.Code, rec.Body.String())
	}

	// DELETE por autor → 204; luego GET da 404
	rec = doJSON(t, f.h, "DELETE", "/materials/"+f.clsA, "", f.doc)
	if rec.Code != 204 {
		t.Fatalf("delete autor: esperaba 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, f.h, "GET", "/materials/"+f.clsA, "", f.dir)
	if rec.Code != 404 {
		t.Fatalf("material borrado: esperaba 404, got %d", rec.Code)
	}
}
