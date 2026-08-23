package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"

	"golang.org/x/crypto/bcrypt"
)

func newTestHandler(t *testing.T) (*Handler, *[]string) {
	t.Helper()
	cfg := &config.Config{RateLimitRPH: 10000, Env: "development"}
	h := NewRouter(cfg, store.NewMemStore())
	links := &[]string{}
	h.SetMagicLinkSender(func(email, link string) {
		*links = append(*links, link)
	})
	return h, links
}

func doJSON(t *testing.T, h http.Handler, method, target string, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body no es error JSON: %v (%s)", err, rec.Body.String())
	}
	return resp.Error.Code
}

func seedUser(t *testing.T, h *Handler, email, password string, role model.Role, disabled bool) string {
	t.Helper()
	hash := ""
	if password != "" {
		b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		hash = string(b)
	}
	u := &model.User{
		ID:           newID(),
		Name:         "Test User",
		Email:        email,
		Role:         role,
		Disabled:     disabled,
		PasswordHash: hash,
	}
	if err := h.store.CreateUser(t.Context(), u); err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// tokenDeLink extrae el token de la URL capturada por el mailer fake.
func tokenDeLink(link string) string {
	i := strings.Index(link, "?token=")
	if i < 0 {
		return ""
	}
	return link[i+len("?token="):]
}

func cookieSesion(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

// --- Setup wizard (§1) ---

func TestSetupWizard_FlujoCompleto_YDobleSetup409(t *testing.T) {
	h, _ := newTestHandler(t)

	rec := doJSON(t, h, "GET", "/setup/status", "")
	if rec.Code != 200 {
		t.Fatalf("status inicial: esperaba 200, got %d", rec.Code)
	}
	var st struct {
		Configured bool `json:"configured"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Configured {
		t.Fatal("instancia nueva debería estar sin configurar")
	}

	rec = doJSON(t, h, "POST", "/setup/school", `{"name":"EPET 24"}`)
	if rec.Code != 201 {
		t.Fatalf("crear escuela: esperaba 201, got %d", rec.Code)
	}
	var school struct {
		SchoolID string `json:"school_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &school)
	if school.SchoolID == "" {
		t.Fatal("school_id vacío")
	}

	// wizard retomable: segunda llamada devuelve la escuela existente
	rec = doJSON(t, h, "POST", "/setup/school", `{"name":"EPET 24"}`)
	if rec.Code != 200 {
		t.Fatalf("escuela retomable: esperaba 200, got %d", rec.Code)
	}

	// password débil → validation_error y NO crea el admin
	rec = doJSON(t, h, "POST", "/setup/admin", `{"name":"Ana","email":"ana@escuela.edu.ar","password":"corta"}`)
	if rec.Code != 400 || errCode(t, rec) != "validation_error" {
		t.Fatalf("password débil: esperaba 400 validation_error, got %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, "POST", "/setup/admin", `{"name":"Ana","email":"ana@escuela.edu.ar","password":"secreta123"}`)
	if rec.Code != 201 {
		t.Fatalf("admin: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if cookieSesion(rec) == nil {
		t.Fatal("admin debería abrir sesión staff con cookie")
	}

	// doble setup → 409 en ambos endpoints
	rec = doJSON(t, h, "POST", "/setup/school", `{"name":"Otra"}`)
	if rec.Code != 409 || errCode(t, rec) != "setup_already_done" {
		t.Fatalf("school duplicado: esperaba 409 setup_already_done, got %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "POST", "/setup/admin", `{"name":"Pepe","email":"pepe@escuela.edu.ar","password":"larga1234"}`)
	if rec.Code != 409 || errCode(t, rec) != "setup_already_done" {
		t.Fatalf("admin duplicado: esperaba 409 setup_already_done, got %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, "GET", "/setup/status", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.Configured {
		t.Fatal("tras crear admin la instancia debería estar configurada")
	}
}

// --- Login dos pasos (§2.1) ---

func TestLoginEmailStep_DeteccionDeRol(t *testing.T) {
	h, _ := newTestHandler(t)
	seedUser(t, h, "ana@escuela.edu.ar", "secreta123", model.RoleDocente, false)

	rec := doJSON(t, h, "POST", "/auth/login/email", `{"email":"ana@escuela.edu.ar"}`)
	var resp struct {
		Next string `json:"next"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Next != "password" {
		t.Fatalf("docente → password, got %q", resp.Next)
	}

	// alumno o inexistente → magic_link
	rec = doJSON(t, h, "POST", "/auth/login/email", `{"email":"nadie@x.com"}`)
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Next != "magic_link" {
		t.Fatalf("inexistente → magic_link, got %q", resp.Next)
	}
}

func TestLoginPassword_Happy_BadCreds_AntiEnumeracion_Disabled(t *testing.T) {
	h, _ := newTestHandler(t)
	seedUser(t, h, "ana@escuela.edu.ar", "secreta123", model.RoleDocente, false)

	// credenciales malas → 401 genérico
	rec := doJSON(t, h, "POST", "/auth/login/password", `{"email":"ana@escuela.edu.ar","password":"incorrecta"}`)
	if rec.Code != 401 || errCode(t, rec) != "invalid_credentials" {
		t.Fatalf("pass mala: esperaba 401 invalid_credentials, got %d %s", rec.Code, rec.Body.String())
	}

	// anti-enumeración: body idéntico para email inexistente
	recInexistente := doJSON(t, h, "POST", "/auth/login/password", `{"email":"fantasma@x.com","password":"loquesea"}`)
	if recInexistente.Code != 401 || errCode(t, recInexistente) != "invalid_credentials" {
		t.Fatalf("email inexistente: esperaba 401 invalid_credentials, got %d", recInexistente.Code)
	}
	if recInexistente.Body.String() != rec.Body.String() {
		t.Fatalf("respuesta de email inexistente difiere (enumeración):\n%s\n%s", rec.Body.String(), recInexistente.Body.String())
	}

	// feliz → 200 + cookie staff
	rec = doJSON(t, h, "POST", "/auth/login/password", `{"email":"ana@escuela.edu.ar","password":"secreta123"}`)
	if rec.Code != 200 {
		t.Fatalf("login feliz: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	c := cookieSesion(rec)
	if c == nil {
		t.Fatal("sin cookie de sesión")
	}

	// sesión creada es staff y /auth/me responde con el usuario
	sess, user, err := h.store.GetSession(t.Context(), c.Value)
	if err != nil || sess.Kind != model.SessionKindStaff {
		t.Fatalf("sesión staff esperada, got %v kind=%v", err, func() any {
			if sess != nil {
				return sess.Kind
			}
			return nil
		}())
	}
	if user.Email != "ana@escuela.edu.ar" {
		t.Fatalf("usuario incorrecto: %q", user.Email)
	}
	rec = doJSON(t, h, "GET", "/auth/me", "", c)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ana@escuela.edu.ar") {
		t.Fatalf("/auth/me: esperaba 200 con usuario, got %d %s", rec.Code, rec.Body.String())
	}

	// docente desactivado + password correcta → 403 account_disabled
	seedUser(t, h, "beto@escuela.edu.ar", "clavelarga", model.RoleDocente, true)
	rec = doJSON(t, h, "POST", "/auth/login/password", `{"email":"beto@escuela.edu.ar","password":"clavelarga"}`)
	if rec.Code != 403 || errCode(t, rec) != "account_disabled" {
		t.Fatalf("disabled: esperaba 403 account_disabled, got %d %s", rec.Code, rec.Body.String())
	}

	// anónimo en /auth/me → 401 unauthenticated
	rec = doJSON(t, h, "GET", "/auth/me", "")
	if rec.Code != 401 || errCode(t, rec) != "unauthenticated" {
		t.Fatalf("/auth/me anónimo: esperaba 401, got %d %s", rec.Code, rec.Body.String())
	}
}

// --- Magic links (§2.2) ---

func pedirMagicLink(t *testing.T, h *Handler, links *[]string, email string, ctx model.MagicContext) string {
	t.Helper()
	n0 := len(*links)
	body, _ := json.Marshal(map[string]string{"email": email, "context": string(ctx)})
	rec := doJSON(t, h, "POST", "/auth/magic-link", string(body))
	if rec.Code != 202 {
		t.Fatalf("magic-link: esperaba 202, got %d (%s)", rec.Code, rec.Body.String())
	}
	if n0 == len(*links) {
		t.Fatal("el mailer no fue invocado")
	}
	return tokenDeLink((*links)[len(*links)-1])
}

func TestMagicLink_SingleUse_TTL_YContextoInvalido(t *testing.T) {
	h, links := newTestHandler(t)
	seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)

	token := pedirMagicLink(t, h, links, "juan@escuela.edu.ar", model.MagicContextLoginPC)
	if len(token) < 20 {
		t.Fatalf("token demasiado corto (≥128 bits): %q", token)
	}

	// primer consume OK → pc_temporal
	rec := doJSON(t, h, "GET", "/auth/consume?token="+token, "")
	if rec.Code != 200 {
		t.Fatalf("primer consume: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var consumed struct {
		SessionKind string `json:"session_kind"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &consumed)
	if consumed.SessionKind != "pc_temporal" {
		t.Fatalf("context login_pc → pc_temporal, got %q", consumed.SessionKind)
	}

	// single-use: segunda vez → 410 token_used
	rec = doJSON(t, h, "GET", "/auth/consume?token="+token, "")
	if rec.Code != 410 || errCode(t, rec) != "token_used" {
		t.Fatalf("reconsume: esperaba 410 token_used, got %d %s", rec.Code, rec.Body.String())
	}

	// TTL vencido → 410 token_expired
	expirado := &model.MagicToken{
		Token:     newToken(),
		UserID:    seedUser(t, h, "marta@escuela.edu.ar", "", model.RoleAlumno, false),
		Context:   model.MagicContextPairingPWA,
		CreatedAt: time.Now().Add(-11 * time.Minute),
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}
	if err := h.store.CreateMagicToken(t.Context(), expirado); err != nil {
		t.Fatal(err)
	}
	rec = doJSON(t, h, "GET", "/auth/consume?token="+expirado.Token, "")
	if rec.Code != 410 || errCode(t, rec) != "token_expired" {
		t.Fatalf("expirado: esperaba 410 token_expired, got %d %s", rec.Code, rec.Body.String())
	}

	// token malformado/inexistente → 400 invalid_token
	rec = doJSON(t, h, "GET", "/auth/consume?token=noexiste", "")
	if rec.Code != 400 || errCode(t, rec) != "invalid_token" {
		t.Fatalf("invalid: esperaba 400 invalid_token, got %d %s", rec.Code, rec.Body.String())
	}

	// contexto inválido → 400; email inexistente → igual 202 (anti-enumeración)
	rec = doJSON(t, h, "POST", "/auth/magic-link", `{"email":"juan@escuela.edu.ar","context":"hack"}`)
	if rec.Code != 400 {
		t.Fatalf("contexto inválido: esperaba 400, got %d", rec.Code)
	}
	rec = doJSON(t, h, "POST", "/auth/magic-link", `{"email":"fantasma@x.com","context":"login_pc"}`)
	if rec.Code != 202 {
		t.Fatalf("email inexistente: esperaba 202, got %d", rec.Code)
	}
}

// [C2]-observación: consume desde PC con pc_temporal activa invalida la previa.
func TestConsume_InvalidaPCTemporalPrevia(t *testing.T) {
	h, links := newTestHandler(t)
	seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)

	token1 := pedirMagicLink(t, h, links, "juan@escuela.edu.ar", model.MagicContextLoginPC)
	rec1 := doJSON(t, h, "GET", "/auth/consume?token="+token1, "")
	if rec1.Code != 200 {
		t.Fatalf("consume 1: %d", rec1.Code)
	}
	previa := cookieSesion(rec1)
	if previa == nil {
		t.Fatal("consume 1 sin cookie")
	}

	token2 := pedirMagicLink(t, h, links, "juan@escuela.edu.ar", model.MagicContextLoginPC)
	rec2 := doJSON(t, h, "GET", "/auth/consume?token="+token2, "", previa)
	if rec2.Code != 200 {
		t.Fatalf("consume 2: %d (%s)", rec2.Code, rec2.Body.String())
	}
	nueva := cookieSesion(rec2)
	if nueva == nil || nueva.Value == previa.Value {
		t.Fatal("consume 2 debería emitir una sesión nueva")
	}

	// la sesión previa quedó invalidada
	if _, _, err := h.store.GetSession(t.Context(), previa.Value); err != store.ErrNotFound {
		t.Fatalf("la pc_temporal previa debería estar eliminada, got err=%v", err)
	}

	// logout revoca la actual y limpia cookie
	recLogout := doJSON(t, h, "DELETE", "/sessions/current", "", nueva)
	if recLogout.Code != 204 {
		t.Fatalf("logout: esperaba 204, got %d", recLogout.Code)
	}
	if _, _, err := h.store.GetSession(t.Context(), nueva.Value); err != store.ErrNotFound {
		t.Fatal("logout debería revocar la sesión")
	}
}

// [C5]: pairing_pwa respeta el límite de ~3 dispositivos y consume el token igual.
func TestConsume_DeviceLimit_PairingPWA(t *testing.T) {
	h, links := newTestHandler(t)
	userID := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)

	now := time.Now()
	for i := 0; i < model.MaxPWADevices; i++ {
		sess := &model.Session{
			ID: newID(), UserID: userID, Kind: model.SessionKindPWA,
			CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(model.PWASlidingDuration),
		}
		if err := h.store.CreateSession(t.Context(), sess); err != nil {
			t.Fatal(err)
		}
	}

	token := pedirMagicLink(t, h, links, "juan@escuela.edu.ar", model.MagicContextPairingPWA)
	rec := doJSON(t, h, "GET", "/auth/consume?token="+token, "")
	if rec.Code != 409 || errCode(t, rec) != "device_limit_reached" {
		t.Fatalf("device limit: esperaba 409 device_limit_reached, got %d %s", rec.Code, rec.Body.String())
	}

	// el token queda consumido a pesar del 409
	rec = doJSON(t, h, "GET", "/auth/consume?token="+token, "")
	if rec.Code != 410 || errCode(t, rec) != "token_used" {
		t.Fatalf("token tras límite: esperaba 410 token_used, got %d %s", rec.Code, rec.Body.String())
	}
}
