package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

func entrarComoInvitado(t *testing.T, h *Handler, code string) (*httptest.ResponseRecorder, *http.Cookie) {
	t.Helper()
	rec := doJSON(t, h, "POST", "/guest/sessions", `{"code":"`+code+`"}`)
	return rec, cookieSesion(rec)
}

func TestEscuela_CodigoGlobalEInvitados(t *testing.T) {
	h, _ := newTestHandler(t)
	seedEscuela(t, h, "Escuela EPET 24", "ESCUELA-7K2M", true)

	dirID := seedUser(t, h, "dir@escuela.edu.ar", "p", model.RoleDirector, false)
	dir := sesionDeRol(t, h, dirID, model.SessionKindStaff)
	docID := seedUser(t, h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	doc := sesionDeRol(t, h, docID, model.SessionKindStaff)

	// GET /school: solo director, con forma completa §9.1
	rec := doJSON(t, h, "GET", "/school", "", dir)
	if rec.Code != 200 {
		t.Fatalf("GET /school director: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var school struct {
		Name       string `json:"name"`
		GlobalCode struct {
			Code   string `json:"code"`
			Active bool   `json:"active"`
		} `json:"global_code"`
		Stats struct {
			Teachers        int `json:"teachers"`
			Classrooms      int `json:"classrooms"`
			Students        int `json:"students"`
			PublicMaterials int `json:"public_materials"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &school); err != nil {
		t.Fatal(err)
	}
	if school.Name != "Escuela EPET 24" || !school.GlobalCode.Active ||
		school.GlobalCode.Code != "ESCUELA-7K2M" || school.Stats.Teachers != 1 {
		t.Fatalf("forma GET /school incorrecta: %+v", school)
	}
	rec = doJSON(t, h, "GET", "/school", "", doc)
	if rec.Code != 403 {
		t.Fatalf("GET /school docente: esperaba 403, got %d", rec.Code)
	}
	rec = doJSON(t, h, "GET", "/school", "")
	if rec.Code != 403 && rec.Code != 401 {
		t.Fatalf("GET /school anónimo: esperaba 401/403, got %d", rec.Code)
	}

	// invitado entra con el código global
	rec, guestCookie := entrarComoInvitado(t, h, "escuela-7k2m") // case-insensitive
	if rec.Code != 201 || guestCookie == nil {
		t.Fatalf("guest session: esperaba 201+cookie, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"school_name":"Escuela EPET 24"`) {
		t.Fatalf("guest session sin school_name: %s", rec.Body.String())
	}

	// código malo → 404 invalid_code
	recBad, _ := entrarComoInvitado(t, h, "ESCUELA-XXXX")
	if recBad.Code != 404 || errCode(t, recBad) != "invalid_code" {
		t.Fatalf("código malo: esperaba 404 invalid_code, got %d (%s)", recBad.Code, recBad.Body.String())
	}

	// GET /school/public es público
	req := httptest.NewRequest("GET", "/school/public", nil)
	pubRec := httptest.NewRecorder()
	h.ServeHTTP(pubRec, req)
	if pubRec.Code != 200 || !strings.Contains(pubRec.Body.String(), "EPET") {
		t.Fatalf("/school/public: esperaba 200 con nombre, got %d (%s)", pubRec.Code, pubRec.Body.String())
	}

	// invitado lee materiales school pero mutación → guest_read_only
	aula := crearAula(t, h, doc, "Aula con invitado mirando")
	subir := subirMaterial(t, h, "/classrooms/"+aula["id"].(string)+"/materials",
		"esc.pdf", "%PDF-esc", map[string]string{"title": "esc", "visibility": "school"}, doc)
	if subir.Code != 201 {
		t.Fatalf("setup material school: got %d (%s)", subir.Code, subir.Body.String())
	}

	listaInv := doJSON(t, h, "GET", "/materials?scope=public", "", guestCookie)
	if listaInv.Code != 200 || !strings.Contains(listaInv.Body.String(), "esc") {
		t.Fatalf("invitado debe ver material school en listado público: %d (%s)", listaInv.Code, listaInv.Body.String())
	}

	cases := []struct {
		name     string
		method   string
		target   string
		body     string
		wantCode int
		wantErr  string
	}{
		{"crear aula → guest_read_only", "POST", "/classrooms", `{"name":"x"}`, 403, "guest_read_only"},
		{"join → guest_read_only", "POST", "/classrooms/join", `{"code":"ZZZZZZ"}`, 403, "guest_read_only"},
		{"upload material → guest_read_only", "POST", "/classrooms/" + aula["id"].(string) + "/materials", "", 403, "guest_read_only"},
		{"patch material → guest_read_only", "PATCH", "/materials/x", `{}`, 403, "guest_read_only"},
		{"delete material → guest_read_only", "DELETE", "/materials/x", "", 403, "guest_read_only"},
		{"rotate join code → guest_read_only", "POST", "/classrooms/" + aula["id"].(string) + "/join_code/rotate", "", 403, "guest_read_only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc
			if r.method == "POST" && strings.HasPrefix(r.target, "/classrooms/"+aula["id"].(string)+"/materials") {
				sub := subirMaterial(t, h, r.target, "x.pdf", "x", map[string]string{"title": "x", "visibility": "public"}, guestCookie)
				if sub.Code != r.wantCode || errCode(t, sub) != r.wantErr {
					t.Fatalf("esperaba %d %q, got %d (%s)", r.wantCode, r.wantErr, sub.Code, sub.Body.String())
				}
				return
			}
			rec := doJSON(t, h, r.method, r.target, r.body, guestCookie)
			if rec.Code != r.wantCode || (r.wantErr != "" && errCode(t, rec) != r.wantErr) {
				t.Fatalf("esperaba %d %q, got %d (%s)", r.wantCode, r.wantErr, rec.Code, rec.Body.String())
			}
		})
	}

	// anónimo también choca con guest_read_only al intentar actuar
	recAnon := doJSON(t, h, "POST", "/classrooms", `{"name":"x"}`)
	if recAnon.Code != 403 || errCode(t, recAnon) != "guest_read_only" {
		t.Fatalf("anónimo creando aula: esperaba guest_read_only, got %d (%s)", recAnon.Code, recAnon.Body.String())
	}
}

// Regenerar el código global: el viejo muere al instante (futuros invitados
// reciben invalid_code) y los invitados ya logueados quedan afuera.
func TestEscuela_RegeneradoMataInvitados(t *testing.T) {
	h, _ := newTestHandler(t)
	seedEscuela(t, h, "Escuela Test", "ESCUELA-VIEJA", true)

	dirID := seedUser(t, h, "dir@escuela.edu.ar", "p", model.RoleDirector, false)
	dir := sesionDeRol(t, h, dirID, model.SessionKindStaff)
	docID := seedUser(t, h, "doc@escuela.edu.ar", "p", model.RoleDocente, false)
	doc := sesionDeRol(t, h, docID, model.SessionKindStaff)

	rec, guestCookie := entrarComoInvitado(t, h, "ESCUELA-VIEJA")
	if rec.Code != 201 || guestCookie == nil {
		t.Fatalf("setup invitado: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	// un material school que el invitado ve mientras su sesión vive
	aula := crearAula(t, h, doc, "Aula")
	sub := subirMaterial(t, h, "/classrooms/"+aula["id"].(string)+"/materials", "sch.pdf", "x",
		map[string]string{"title": "sch", "visibility": "school"}, doc)
	if sub.Code != 201 {
		t.Fatalf("setup material school: got %d (%s)", sub.Code, sub.Body.String())
	}
	antes := doJSON(t, h, "GET", "/materials", "", guestCookie)
	if !strings.Contains(antes.Body.String(), "sch") {
		t.Fatalf("invitado debe ver material school antes de regenerar: %s", antes.Body.String())
	}

	// regenerar
	regen := doJSON(t, h, "POST", "/school/global-code/regenerate", "", dir)
	if regen.Code != 200 {
		t.Fatalf("regenerate: esperaba 200, got %d (%s)", regen.Code, regen.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(regen.Body.Bytes(), &resp); err != nil || resp.Code == "" || resp.Code == "ESCUELA-VIEJA" {
		t.Fatalf("regenerate sin código nuevo: %v (%s)", err, regen.Body.String())
	}

	// el viejo ya no deja entrar a futuros invitados
	recOld, _ := entrarComoInvitado(t, h, "ESCUELA-VIEJA")
	if recOld.Code != 404 || errCode(t, recOld) != "invalid_code" {
		t.Fatalf("código viejo tras regenerar: esperaba 404 invalid_code, got %d (%s)", recOld.Code, recOld.Body.String())
	}

	// …y la sesión del invitado viejo murió: ahora ve como anónimo (sin school)
	desps := doJSON(t, h, "GET", "/materials", "", guestCookie)
	if desps.Code != 200 || strings.Contains(desps.Body.String(), "sch") {
		t.Fatalf("invitado viejo debe quedar afuera (ver solo public): %d (%s)", desps.Code, desps.Body.String())
	}

	// el nuevo código funciona y GET /school lo refleja activo
	recNew, _ := entrarComoInvitado(t, h, resp.Code)
	if recNew.Code != 201 {
		t.Fatalf("código nuevo: esperaba 201, got %d (%s)", recNew.Code, recNew.Body.String())
	}
	school := doJSON(t, h, "GET", "/school", "", dir)
	if !strings.Contains(school.Body.String(), `"code":"`+resp.Code+`"`) {
		t.Fatalf("GET /school debe mostrar el código nuevo: %s", school.Body.String())
	}

	// docente no puede regenerar
	recDoc := doJSON(t, h, "POST", "/school/global-code/regenerate", "", doc)
	if recDoc.Code != 403 {
		t.Fatalf("regenerate docente: esperaba 403, got %d", recDoc.Code)
	}
}

// Desactivar (DELETE) apaga el acceso de invitados hasta regenerar.
func TestEscuela_DesactivarCodigoGlobal(t *testing.T) {
	h, _ := newTestHandler(t)
	seedEscuela(t, h, "Escuela Test", "ESCUELA-ACTIVA", true)

	dirID := seedUser(t, h, "dir@escuela.edu.ar", "p", model.RoleDirector, false)
	dir := sesionDeRol(t, h, dirID, model.SessionKindStaff)

	rec, _ := entrarComoInvitado(t, h, "ESCUELA-ACTIVA")
	if rec.Code != 201 {
		t.Fatalf("precondición invitado: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	del := doJSON(t, h, "DELETE", "/school/global-code", "", dir)
	if del.Code != 204 {
		t.Fatalf("disable: esperaba 204, got %d (%s)", del.Code, del.Body.String())
	}

	// nadie entra como invitado ahora
	recOff, _ := entrarComoInvitado(t, h, "ESCUELA-ACTIVA")
	if recOff.Code != 404 || errCode(t, recOff) != "invalid_code" {
		t.Fatalf("join tras desactivar: esperaba 404 invalid_code, got %d (%s)", recOff.Code, recOff.Body.String())
	}

	// estado visible en GET /school: active=false
	school := doJSON(t, h, "GET", "/school", "", dir)
	if !strings.Contains(school.Body.String(), `"active":false`) {
		t.Fatalf("GET /school debe mostrar active=false: %s", school.Body.String())
	}

	// se puede volver a activar regenerando
	regen := doJSON(t, h, "POST", "/school/global-code/regenerate", "", dir)
	if regen.Code != 200 {
		t.Fatalf("reactivar: esperaba 200, got %d (%s)", regen.Code, regen.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(regen.Body.Bytes(), &resp)
	recOn, _ := entrarComoInvitado(t, h, resp.Code)
	if recOn.Code != 201 {
		t.Fatalf("invitado tras reactivar: esperaba 201, got %d (%s)", recOn.Code, recOn.Body.String())
	}
}

// Rate limit POST /guest/sessions: 20/h por IP.
func TestEscuela_GuestSessionRateLimit(t *testing.T) {
	h, _ := newTestHandler(t)
	h.guestRL = middleware.NewIPRateLimiter(2, 2)
	seedEscuela(t, h, "Escuela Test", "ESCUELA-RL", true)

	for i := 0; i < 2; i++ {
		rec, _ := entrarComoInvitado(t, h, "CODIGO-MALO1")
		if rec.Code != 404 {
			t.Fatalf("intento %d: esperaba 404, got %d", i+1, rec.Code)
		}
	}
	rec, _ := entrarComoInvitado(t, h, "CODIGO-MALO1")
	if rec.Code != 429 || errCode(t, rec) != "rate_limited" {
		t.Fatalf("tercer intento: esperaba 429 rate_limited, got %d (%s)", rec.Code, rec.Body.String())
	}
}
