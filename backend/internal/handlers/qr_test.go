package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// seedPairing inserta un pairing directamente en el store (para casos de expiración).
func seedPairing(t *testing.T, h *Handler, userID string, status model.PairingStatus, startedAgo time.Duration) *model.PairingSession {
	t.Helper()
	now := time.Now()
	p := &model.PairingSession{
		PairingID:   newID(),
		QRToken:     newToken(),
		UserID:      userID,
		DeviceLabel: "LAB-PC07",
		OriginIP:    "10.0.0.7",
		Status:      status,
		CreatedAt:   now.Add(startedAgo),
		ExpiresAt:   now.Add(startedAgo).Add(model.QRTTL),
	}
	if err := h.store.CreatePairingSession(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func iniciarQR(t *testing.T, h *Handler) (pairingID, qrURL string) {
	t.Helper()
	rec := doJSON(t, h, "POST", "/auth/qr/start", `{"device_label":"LAB-PC07"}`)
	if rec.Code != 201 {
		t.Fatalf("qr/start: esperaba 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		PairingID    string `json:"pairing_id"`
		QRURL        string `json:"qr_url"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshAfter int    `json:"refresh_after"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ExpiresIn != 90 || resp.RefreshAfter != 60 {
		t.Fatalf("TTL inválido: expires_in=%d refresh_after=%d", resp.ExpiresIn, resp.RefreshAfter)
	}
	return resp.PairingID, resp.QRURL
}

func tokenDeURL(qrURL string) string {
	i := strings.Index(qrURL, "/qr/")
	if i < 0 {
		return ""
	}
	return qrURL[i+len("/qr/"):]
}

// sesionPWA crea una sesión pwa persistente para userID y devuelve su cookie
// (simula el celular ya emparejado vía magic link [C2]).
func sesionPWA(t *testing.T, h *Handler, userID string) *http.Cookie {
	t.Helper()
	now := time.Now()
	sess := &model.Session{
		ID:         newID(),
		UserID:     userID,
		Kind:       model.SessionKindPWA,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(model.PWASlidingDuration),
	}
	if err := h.store.CreateSession(t.Context(), sess); err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: middleware.SessionCookieName, Value: sess.ID}
}

func scanPairingID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		PairingID string `json:"pairing_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return resp.PairingID
}

func TestQRFeliz_FlujoCompleto(t *testing.T) {
	h, _ := newTestHandler(t)
	userID := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)
	celular := sesionPWA(t, h, userID)

	pairingID, qrURL := iniciarQR(t, h)
	qrToken := tokenDeURL(qrURL)

	// [C3]: token ≥128 bits base64url e independiente del pairing_id
	if len(qrToken) < 20 {
		t.Fatalf("qr_token demasiado corto: %q", qrToken)
	}
	if qrToken == pairingID || pairingID == "" {
		t.Fatal("pairing_id y qr_token deben ser independientes")
	}
	i := strings.Index(qrURL, "://")
	host := qrURL[i+3:]
	if !strings.HasPrefix(host, "example.com/qr/") {
		t.Fatalf("qr_url debería usar el host de la request en dev: %q", qrURL)
	}

	// PC poll → waiting
	rec := doJSON(t, h, "GET", "/auth/qr/"+pairingID+"/status", "")
	var st struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Status != "waiting" {
		t.Fatalf("status inicial: esperaba waiting, got %q", st.Status)
	}

	// scan desde la PWA
	rec = doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, qrToken), celular)
	if rec.Code != 200 {
		t.Fatalf("scan: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var scanResp struct {
		PairingID   string `json:"pairing_id"`
		DeviceLabel string `json:"device_label"`
		RequestedAt string `json:"requested_at"`
		OriginIP    string `json:"origin_ip"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &scanResp)
	if scanResp.PairingID == "" || scanResp.DeviceLabel != "LAB-PC07" ||
		scanResp.RequestedAt == "" || scanResp.OriginIP == "" {
		t.Fatalf("[C3] scan debe devolver identidad de la PC: %+v", scanResp)
	}

	// PC poll → scanned
	rec = doJSON(t, h, "GET", "/auth/qr/"+pairingID+"/status", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Status != "scanned" {
		t.Fatalf("status tras scan: esperaba scanned, got %q", st.Status)
	}

	// confirm desde el mismo celular
	rec = doJSON(t, h, "POST", "/auth/qr/"+pairingID+"/confirm", "", celular)
	if rec.Code != 200 {
		t.Fatalf("confirm: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// PC poll → confirmed + Set-Cookie pc_temporal + user {name, role} (§2.3)
	rec = doJSON(t, h, "GET", "/auth/qr/"+pairingID+"/status", "")
	if rec.Code != 200 {
		t.Fatalf("status confirmado: esperaba 200, got %d", rec.Code)
	}
	var conf struct {
		Status string `json:"status"`
		User   struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"user"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &conf)
	if conf.Status != "confirmed" || conf.User.Role != "alumno" || conf.User.Name == "" {
		t.Fatalf("confirmed con user: got %+v", conf)
	}
	pc := cookieSesion(rec)
	if pc == nil {
		t.Fatal("confirmed debería incluir Set-Cookie de sesión")
	}
	sess, user, err := h.store.GetSession(t.Context(), pc.Value)
	if err != nil || sess.Kind != model.SessionKindPCTemporal || user.Email != "juan@escuela.edu.ar" {
		t.Fatalf("sesión pc_temporal esperada, got err=%v kind=%v user=%v", err, sess.Kind, user.Email)
	}
	if _, _, err = h.store.GetSession(t.Context(), celular.Value); err != nil {
		t.Fatal("la sesión PWA no debe verse afectada por el claim")
	}

	// single-use: segundo confirm → 409 already_claimed; re-scan → 410
	rec = doJSON(t, h, "POST", "/auth/qr/"+pairingID+"/confirm", "", celular)
	if rec.Code != 409 || errCode(t, rec) != "already_claimed" {
		t.Fatalf("doble confirm: esperaba 409 already_claimed, got %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, qrToken), celular)
	if rec.Code != 410 || errCode(t, rec) != "token_expired" {
		t.Fatalf("re-scan usado: esperaba 410 token_expired, got %d %s", rec.Code, rec.Body.String())
	}

	// la cookie emitida abre sesión en la PC
	rec = doJSON(t, h, "GET", "/auth/me", "", pc)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "juan@escuela.edu.ar") {
		t.Fatalf("/auth/me con cookie QR: esperaba 200, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestQR_Expiracion_YEstados(t *testing.T) {
	h, _ := newTestHandler(t)
	userID := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)
	celular := sesionPWA(t, h, userID)

	// vencido sin scan: status expired y scan → 410
	p := seedPairing(t, h, "", model.PairingWaiting, -(model.QRTTL + time.Second))
	rec := doJSON(t, h, "GET", "/auth/qr/"+p.PairingID+"/status", "")
	var st struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Status != "expired" {
		t.Fatalf("expired esperado en status, got %q", st.Status)
	}
	rec = doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, p.QRToken), celular)
	if rec.Code != 410 || errCode(t, rec) != "token_expired" {
		t.Fatalf("scan vencido: esperaba 410 token_expired, got %d %s", rec.Code, rec.Body.String())
	}

	// vencido entre scan y confirm ([C3]: TTL 90s también aplica al confirm)
	p2 := seedPairing(t, h, userID, model.PairingScanned, -(model.QRTTL + time.Second))
	rec = doJSON(t, h, "POST", "/auth/qr/"+p2.PairingID+"/confirm", "", celular)
	if rec.Code != 410 || errCode(t, rec) != "token_expired" {
		t.Fatalf("confirm vencido: esperaba 410 token_expired, got %d %s", rec.Code, rec.Body.String())
	}

	// token inexistente → 404
	rec = doJSON(t, h, "POST", "/auth/qr/scan", `{"qr_token":"noexiste1234567890"}`, celular)
	if rec.Code != 404 || errCode(t, rec) != "not_found" {
		t.Fatalf("scan inexistente: esperaba 404 not_found, got %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "GET", "/auth/qr/noexiste/status", "")
	if rec.Code != 404 {
		t.Fatalf("status inexistente: esperaba 404, got %d", rec.Code)
	}
}

func TestQRDeny_NuncaIniciaSesion(t *testing.T) {
	h, _ := newTestHandler(t)
	userID := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)
	celular := sesionPWA(t, h, userID)

	pairingID, qrURL := iniciarQR(t, h)
	doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, tokenDeURL(qrURL)), celular)

	rec := doJSON(t, h, "POST", "/auth/qr/"+pairingID+"/deny", "", celular)
	if rec.Code != 200 {
		t.Fatalf("deny: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, "GET", "/auth/qr/"+pairingID+"/status", "")
	var st struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Status != "denied" {
		t.Fatalf("status tras deny: esperaba denied, got %q", st.Status)
	}

	// deny es terminal: confirm tardío → 409 y NUNCA hay Set-Cookie ni sesión nueva
	rec = doJSON(t, h, "POST", "/auth/qr/"+pairingID+"/confirm", "", celular)
	if rec.Code != 409 || errCode(t, rec) != "already_claimed" {
		t.Fatalf("confirm tras deny: esperaba 409 already_claimed, got %d %s", rec.Code, rec.Body.String())
	}
	if cookieSesion(rec) != nil {
		t.Fatal("denied nunca debe emitir cookie de sesión")
	}
}

func TestQR_ClaimConcurrente_UnSoloGanador(t *testing.T) {
	h, _ := newTestHandler(t)
	userID := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)
	celular := sesionPWA(t, h, userID)

	pairingID, qrURL := iniciarQR(t, h)
	doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, tokenDeURL(qrURL)), celular)

	const n = 8
	var wg sync.WaitGroup
	codigos := make([]int, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := doJSON(t, h, "POST", "/auth/qr/"+pairingID+"/confirm", "", celular)
			codigos[i] = rec.Code
		}(i)
	}
	wg.Wait()

	ganadores := 0
	for _, c := range codigos {
		switch c {
		case 200:
			ganadores++
		case http.StatusConflict:
		default:
			t.Fatalf("código inesperado en claim concurrente: %d", c)
		}
	}
	if ganadores != 1 {
		t.Fatalf("exactamente un claim debe ganar, ganaron %d: %v", ganadores, codigos)
	}
	sessions, _ := h.store.ListSessionsByUser(t.Context(), userID)
	pc := 0
	for _, s := range sessions {
		if s.Kind == model.SessionKindPCTemporal {
			pc++
		}
	}
	if pc != 1 {
		t.Fatalf("una sola pc_temporal esperada, hay %d", pc)
	}
}

func TestQR_AutorizacionScanYConfirm(t *testing.T) {
	h, _ := newTestHandler(t)
	juan := seedUser(t, h, "juan@escuela.edu.ar", "", model.RoleAlumno, false)
	marta := seedUser(t, h, "marta@escuela.edu.ar", "", model.RoleAlumno, false)
	seedUser(t, h, "ana@escuela.edu.ar", "clavelarga", model.RoleDocente, false)
	celularJuan := sesionPWA(t, h, juan)
	celularMarta := sesionPWA(t, h, marta)

	// login docente → sesión staff (kind incorrecto para escanear)
	rec := doJSON(t, h, "POST", "/auth/login/password", `{"email":"ana@escuela.edu.ar","password":"clavelarga"}`)
	if rec.Code != 200 {
		t.Fatalf("login docente: %d", rec.Code)
	}
	staff := cookieSesion(rec)

	_, qrURL := iniciarQR(t, h)
	qrToken := tokenDeURL(qrURL)

	// sin sesión → 401; staff (no alumno) → 403
	rec = doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, qrToken))
	if rec.Code != 401 || errCode(t, rec) != "unauthenticated" {
		t.Fatalf("scan anónimo: esperaba 401, got %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, qrToken), staff)
	if rec.Code != 403 || errCode(t, rec) != "forbidden" {
		t.Fatalf("scan staff: esperaba 403 forbidden, got %d %s", rec.Code, rec.Body.String())
	}

	// marta roba el scan; juan ya no puede confirmar (no es quien escaneó)
	rec = doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, qrToken), celularMarta)
	if rec.Code != 200 {
		t.Fatalf("scan de marta: esperaba 200, got %d", rec.Code)
	}
	rec = doJSON(t, h, "POST", "/auth/qr/"+scanPairingID(t, rec)+"/confirm", "", celularJuan)
	if rec.Code != 403 || errCode(t, rec) != "forbidden" {
		t.Fatalf("confirm de no-scanner: esperaba 403 forbidden, got %d %s", rec.Code, rec.Body.String())
	}

	// deny por quien no escaneó → 403
	pairingID2, qrURL2 := iniciarQR(t, h)
	doJSON(t, h, "POST", "/auth/qr/scan", fmt.Sprintf(`{"qr_token":%q}`, tokenDeURL(qrURL2)), celularMarta)
	rec = doJSON(t, h, "POST", "/auth/qr/"+pairingID2+"/deny", "", celularJuan)
	if rec.Code != 403 {
		t.Fatalf("deny de no-scanner: esperaba 403, got %d", rec.Code)
	}
}
