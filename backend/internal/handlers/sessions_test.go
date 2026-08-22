package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

func TestSesiones_Listado_BorradoPuntual_CloseOthers(t *testing.T) {
	h, _ := newTestHandler(t)
	userID := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)
	otro := seedUser(t, h, "marta@escuela.edu.ar", "", model.RoleAlumno, false)

	celular1 := sesionPWA(t, h, userID)
	celular2 := sesionPWA(t, h, userID)
	sesionAjena := sesionPWA(t, h, otro)

	// listado: solo las propias, con los campos §2.4
	rec := doJSON(t, h, "GET", "/sessions", "", celular1)
	if rec.Code != 200 {
		t.Fatalf("GET /sessions: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var lista struct {
		Sessions []struct {
			ID          string `json:"id"`
			Kind        string `json:"kind"`
			DeviceLabel string `json:"device_label"`
			CreatedAt   string `json:"created_at"`
			LastSeenAt  string `json:"last_seen_at"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatal(err)
	}
	if len(lista.Sessions) != 2 {
		t.Fatalf("esperaba 2 sesiones propias, got %d: %+v", len(lista.Sessions), lista.Sessions)
	}
	for _, s := range lista.Sessions {
		if s.Kind != "pwa" || s.CreatedAt == "" || s.LastSeenAt == "" {
			t.Fatalf("campos §2.4 incompletos: %+v", s)
		}
	}

	// borrar sesión ajena → 404 (indistinguible de inexistente)
	rec = doJSON(t, h, "DELETE", "/sessions/"+sesionAjena.Value, "", celular1)
	if rec.Code != 404 || errCode(t, rec) != "not_found" {
		t.Fatalf("sesión ajena: esperaba 404 not_found, got %d %s", rec.Code, rec.Body.String())
	}
	if _, _, err := h.store.GetSession(t.Context(), sesionAjena.Value); err != nil {
		t.Fatal("la sesión ajena no debe haberse borrado")
	}

	// borrar propia → 204 y desaparece
	rec = doJSON(t, h, "DELETE", "/sessions/"+celular2.Value, "", celular1)
	if rec.Code != 204 {
		t.Fatalf("borrar propia: esperaba 204, got %d", rec.Code)
	}
	if _, _, err := h.store.GetSession(t.Context(), celular2.Value); err == nil {
		t.Fatal("la sesión debería estar revocada")
	}

	// close-others: revoca todas menos la actual, sin tocar ajenas
	sesionExtra := sesionPWA(t, h, userID)
	rec = doJSON(t, h, "POST", "/sessions/close-others", "", celular1)
	if rec.Code != 204 {
		t.Fatalf("close-others: esperaba 204, got %d (%s)", rec.Code, rec.Body.String())
	}
	if _, _, err := h.store.GetSession(t.Context(), celular1.Value); err != nil {
		t.Fatal("la sesión actual debe sobrevivir a close-others")
	}
	if _, _, err := h.store.GetSession(t.Context(), sesionExtra.Value); err == nil {
		t.Fatal("close-others debería revocar las demás sesiones propias")
	}
	if _, _, err := h.store.GetSession(t.Context(), sesionAjena.Value); err != nil {
		t.Fatal("close-others no debe tocar sesiones de otros usuarios")
	}
}

// [C5]: pc_temporal NO consume el cupo de ~3 dispositivos pwa.
func TestQR_PCTemporalNoConsumeCupoDeDispositivos(t *testing.T) {
	h, links := newTestHandler(t)
	userID := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)

	now := time.Now()
	ids := make([]string, model.MaxPWADevices)
	for i := range ids {
		sess := &model.Session{
			ID: newID(), UserID: userID, Kind: model.SessionKindPWA,
			CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(model.PWASlidingDuration),
		}
		if err := h.store.CreateSession(t.Context(), sess); err != nil {
			t.Fatal(err)
		}
		ids[i] = sess.ID
	}
	celular := &http.Cookie{Name: middleware.SessionCookieName, Value: ids[0]}

	pairingID, qrURL := iniciarQR(t, h)
	doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, tokenDeURL(qrURL)), celular)
	rec := doJSON(t, h, "POST", "/auth/qr/"+pairingID+"/confirm", "", celular)
	if rec.Code != 200 {
		t.Fatalf("confirm con cupo lleno ([C5] pc_temporal no consume): esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// la PC obtiene sesión usable
	rec = doJSON(t, h, "GET", "/auth/qr/"+pairingID+"/status", "")
	pc := cookieSesion(rec)
	if pc == nil {
		t.Fatal("confirmed sin cookie para la PC")
	}
	sess, _, err := h.store.GetSession(t.Context(), pc.Value)
	if err != nil || sess.Kind != model.SessionKindPCTemporal {
		t.Fatalf("sesión pc_temporal esperada, got err=%v", err)
	}

	// GET /sessions del alumno ahora muestra las 3 pwa + 1 pc_temporal
	rec = doJSON(t, h, "GET", "/sessions", "", celular)
	var lista struct {
		Sessions []struct {
			Kind string `json:"kind"`
		} `json:"sessions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &lista)
	kinds := map[string]int{}
	for _, s := range lista.Sessions {
		kinds[s.Kind]++
	}
	if kinds["pwa"] != 3 || kinds["pc_temporal"] != 1 {
		t.Fatalf("listado incorrecto: %+v", kinds)
	}

	// y el cupo sigue lleno: un pairing_pwa nuevo sigue bloqueado
	token := pedirMagicLink(t, h, links, "juan@escuela.edu.ar", model.MagicContextPairingPWA)
	rec = doJSON(t, h, "GET", "/auth/consume?token="+token, "")
	if rec.Code != 409 || errCode(t, rec) != "device_limit_reached" {
		t.Fatalf("cupo intacto: esperaba 409 device_limit_reached, got %d %s", rec.Code, rec.Body.String())
	}
}

// Desvincular un celular (borrar su sesión pwa) libera cupo para emparejar otro.
func TestSesiones_DesvincularLiberaCupo(t *testing.T) {
	h, links := newTestHandler(t)
	userID := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)

	now := time.Now()
	ids := make([]string, model.MaxPWADevices)
	for i := range ids {
		sess := &model.Session{
			ID: newID(), UserID: userID, Kind: model.SessionKindPWA,
			CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(model.PWASlidingDuration),
		}
		if err := h.store.CreateSession(t.Context(), sess); err != nil {
			t.Fatal(err)
		}
		ids[i] = sess.ID
	}

	token := pedirMagicLink(t, h, links, "juan@escuela.edu.ar", model.MagicContextPairingPWA)
	rec := doJSON(t, h, "GET", "/auth/consume?token="+token, "")
	if rec.Code != 409 || errCode(t, rec) != "device_limit_reached" {
		t.Fatalf("pre-condición cupo lleno: esperaba 409, got %d %s", rec.Code, rec.Body.String())
	}

	// desde otra sesión propia, desvincular el primer celular
	actuando := &http.Cookie{Name: middleware.SessionCookieName, Value: ids[1]}
	rec = doJSON(t, h, "DELETE", "/sessions/"+ids[0], "", actuando)
	if rec.Code != 204 {
		t.Fatalf("desvincular: esperaba 204, got %d (%s)", rec.Code, rec.Body.String())
	}

	token = pedirMagicLink(t, h, links, "juan@escuela.edu.ar", model.MagicContextPairingPWA)
	rec = doJSON(t, h, "GET", "/auth/consume?token="+token, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"session_kind":"pwa"`) {
		t.Fatalf("emparejar tras desvincular: esperaba 200 pwa, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// --- Origin en mutaciones (§0.2 CSRF, gap PR2) ---

func conOrigin(h http.Handler, method, target, body, origin string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestOrigin_TablaMutaciones(t *testing.T) {
	h, _ := newTestHandler(t) // sin OFFICIAL_DOMAIN: se acepta Origin == Host

	cases := []struct {
		name     string
		method   string
		target   string
		body     string
		origin   string
		wantCode int
		wantErr  string
	}{
		{"POST origin malo → 403", "POST", "/auth/qr/start", `{}`, "https://evil.example", 403, "forbidden"},
		{"DELETE origin malo → 403", "DELETE", "/sessions/x", "", "http://example.com.evil.com", 403, "forbidden"},
		{"PUT origin malo → 403", "PUT", "/cualquiera", "{}", "https://evil.example:8080", 403, "forbidden"},
		{"PATCH origin malo → 403", "PATCH", "/cualquiera", "{}", "https://evil.example", 403, "forbidden"},
		{"POST sin Origin pasa (curl/tests)", "POST", "/auth/qr/start", `{}`, "", 201, ""},
		{"POST origin == Host pasa en dev", "POST", "/auth/qr/start", `{}`, "http://example.com", 201, ""},
		{"GET nunca se filtra por Origin", "GET", "/healthz", "", "https://evil.example", 200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := conOrigin(h, tc.method, tc.target, tc.body, tc.origin)
			if rec.Code != tc.wantCode {
				t.Fatalf("esperaba %d, got %d (%s)", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.wantErr != "" && errCode(t, rec) != tc.wantErr {
				t.Fatalf("esperaba error %q, got %s", tc.wantErr, rec.Body.String())
			}
		})
	}
}

func TestOrigin_DominioOficialConfigurado(t *testing.T) {
	cfg := &config.Config{RateLimitRPH: 10000, Env: "development", OfficialDomain: "ocicat.test"}
	h := NewRouter(cfg, store.NewMemStore())

	// Origin == dominio oficial pasa...
	rec := conOrigin(h, "POST", "/auth/qr/start", `{}`, "https://ocicat.test")
	if rec.Code != 201 {
		t.Fatalf("dominio oficial: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	// ...y la qr_url usa ese dominio ([C3] anti-quishing), no el Host
	var resp struct {
		QRURL string `json:"qr_url"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !strings.HasPrefix(resp.QRURL, "http://ocicat.test/qr/") {
		t.Fatalf("qr_url debe usar OFFICIAL_DOMAIN: %q", resp.QRURL)
	}

	// Origin de otro dominio (aun si el Host coincide) → 403
	rec = conOrigin(h, "POST", "/auth/qr/start", `{}`, "https://otro.test")
	if rec.Code != 403 || errCode(t, rec) != "forbidden" {
		t.Fatalf("dominio ajeno: esperaba 403 forbidden, got %d %s", rec.Code, rec.Body.String())
	}
}
