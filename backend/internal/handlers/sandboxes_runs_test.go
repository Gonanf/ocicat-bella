package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Máquina de estados §7 vía polls del fake runner ---

func TestEstadosDelRun_ViaPolls(t *testing.T) {
	f := seedSandboxes(t, 4, 8)
	sb := crearSandboxOK(t, f.h, f.alum, `{"template_id":"python/numpy","mode":"job"}`)
	runID := sb["run_id"].(string)

	want := []string{"queued", "downloading_image", "starting", "ready", "running", "succeeded", "cleaned"}
	got := []string{"queued"}
	for i := 1; i < len(want); i++ {
		out := stepHasta(t, f.h, f.alum, runID, want[i], 1)
		got = append(got, out["status"].(string))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("secuencia: esperaba %v, got %v", want, got)
		}
	}

	final := runStatusDe(t, f.h, f.alum, runID)
	if ec, ok := final["exit_code"].(float64); !ok || ec != 0 {
		t.Fatalf("exit_code esperaba 0, got %v", final["exit_code"])
	}
	hist, ok := final["history"].([]any)
	if !ok || len(hist) != 1 {
		t.Fatalf("history esperaba 1 entrada, got %v", final["history"])
	}
	entry := hist[0].(map[string]any)
	if entry["n"].(float64) != 1 || entry["outcome"] != "success" {
		t.Fatalf("history entry: %v", entry)
	}

	// modo service queda running con service_url (§7)
	svc := crearSandboxOK(t, f.h, f.alum, `{"template_id":"bun/react"}`)
	out := stepHasta(t, f.h, f.alum, svc["run_id"].(string), "running", 6)
	f.h.Step(t.Context()) // sigue running, no pasa a succeeded
	después := runStatusDe(t, f.h, f.alum, svc["run_id"].(string))
	if después["status"] != "running" {
		t.Fatalf("service debe quedar running, got %v", después["status"])
	}
	if url, _ := out["service_url"].(string); !strings.HasPrefix(url, "/s/") {
		t.Fatalf("service_url esperaba /s/{id}, got %q", url)
	}
}

// --- stop → cleaned; el registro historical persiste (§7) ---

func TestStopRun(t *testing.T) {
	f := seedSandboxes(t, 4, 8)
	sb := crearSandboxOK(t, f.h, f.alum, `{"template_id":"python/numpy"}`)
	runID := sb["run_id"].(string)
	stepHasta(t, f.h, f.alum, runID, "running", 5)

	cases := []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"invitado", f.guest, 403},
		{"otro alumno", f.fuera, 403},
		{"docente del aula", f.doc, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, f.h, "POST", "/runs/"+runID+"/stop", "", tc.cookie)
			if rec.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}

	var out map[string]any
	rec := doJSON(t, f.h, "POST", "/runs/"+runID+"/stop", "", f.alum) // autor
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["status"] != "cleaned" {
		t.Fatalf("stop autor: esperaba {status:cleaned}, got %s", rec.Body.String())
	}
	if got := runStatusDe(t, f.h, f.alum, runID)["status"]; got != "cleaned" {
		t.Fatalf("post-stop esperaba cleaned, got %v", got)
	}
	detalle := doJSON(t, f.h, "GET", "/sandboxes/"+sb["sandbox_id"].(string), "", f.alum)
	if detalle.Code != 200 { // historical: registro persiste
		t.Fatalf("registro historical persiste: esperaba 200, got %d", detalle.Code)
	}
}

// --- ephemeral → 404 tras limpieza (§7) ---

func TestEphemeral_404TrasLimpieza(t *testing.T) {
	f := seedSandboxes(t, 4, 8)
	sb := crearSandboxOK(t, f.h, f.alum,
		`{"template_id":"python/numpy","retention":"ephemeral"}`)
	sbxID := sb["sandbox_id"].(string)

	if got := doJSON(t, f.h, "GET", "/sandboxes/"+sbxID, "", f.alum).Code; got != 200 {
		t.Fatalf("ephemeral vivo: esperaba 200, got %d", got)
	}
	for i := 0; i < 10; i++ {
		f.h.Step(t.Context())
		if got := doJSON(t, f.h, "GET", "/sandboxes/"+sbxID, "", f.alum).Code; got == 404 {
			break
		}
	}
	runID := sb["run_id"].(string)
	if got := doJSON(t, f.h, "GET", "/sandboxes/"+sbxID, "", f.alum).Code; got != 404 {
		t.Fatalf("ephemeral limpiado: esperaba 404, got %d", got)
	}
	if got := doJSON(t, f.h, "GET", "/runs/"+runID, "", f.alum).Code; got != 404 {
		t.Fatalf("run de ephemeral limpiado: esperaba 404, got %d", got)
	}
	lista := doJSON(t, f.h, "GET", "/sandboxes", "", f.alum)
	if strings.Contains(lista.Body.String(), sbxID) {
		t.Fatal("ephemeral limpiado no debe aparecer en el listado")
	}
}

// --- GET /runs/{id}/logs: SSE canónico §7 con el fake runner ---

func TestLogsSSE(t *testing.T) {
	f := seedSandboxes(t, 4, 8)

	corriendo := crearSandboxOK(t, f.h, f.alum,
		`{"template_id":"python/numpy","retention":"historical","visibility":"public"}`)
	stepHasta(t, f.h, f.alum, corriendo["run_id"].(string), "running", 5)

	// invitado + run NO terminado → prohibido
	rec := doJSON(t, f.h, "GET", "/runs/"+corriendo["run_id"].(string)+"/logs", "", f.guest)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("invitado sobre run vivo: esperaba 403, got %d", rec.Code)
	}

	stepHasta(t, f.h, f.alum, corriendo["run_id"].(string), "cleaned", 4)

	// terminado: autor ve SSE completo
	rec = doJSON(t, f.h, "GET", "/runs/"+corriendo["run_id"].(string)+"/logs", "", f.alum)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs autor: esperaba 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type esperaba text/event-stream, got %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"event: log", `"line":"[runner]`, "event: status", "event: exit", `"exit_code":0`} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE sin %q:\n%s", want, body)
		}
	}

	// invitado sí ve logs de un histórico público terminado
	rec = doJSON(t, f.h, "GET", "/runs/"+corriendo["run_id"].(string)+"/logs", "", f.guest)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "event: exit") {
		t.Fatalf("invitado logs históricos públicos: esperaba 200 con exit, got %d", rec.Code)
	}
}

// --- deliver popula last_test_result con el run submission_test (FASE 6/§5.2) ---

func TestDeliver_PopulaLastTestResult(t *testing.T) {
	f := seedSandboxes(t, 4, 8)
	asgID := crearConsignaRapida(t, f)
	subID := subirDraft(t, f, asgID)

	testRun := crearSandboxOK(t, f.h, f.alum,
		`{"template_id":"python/numpy","purpose":"submission_test","submission_id":"`+subID+`"}`)
	stepHasta(t, f.h, f.alum, testRun["run_id"].(string), "cleaned", 8)

	del := doJSON(t, f.h, "POST", "/submissions/"+subID+"/deliver", `{"confirm_attempt":1}`, f.alum)
	if del.Code != 201 {
		t.Fatalf("deliver: esperaba 201, got %d (%s)", del.Code, del.Body.String())
	}
	var out struct {
		LastTestResult *struct {
			RunID    string `json:"run_id"`
			ExitCode int    `json:"exit_code"`
		} `json:"last_test_result"`
		TestedOK bool `json:"tested_ok"`
	}
	if err := json.Unmarshal(del.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.LastTestResult == nil ||
		out.LastTestResult.RunID != testRun["run_id"].(string) ||
		out.LastTestResult.ExitCode != 0 {
		t.Fatalf("last_test_result mal populado: %+v", del.Body.String())
	}

	// snapshot inmutable: la entrega guardó el resultado
	got := doJSON(t, f.h, "GET", "/submissions/"+subID, "", f.doc)
	if !strings.Contains(got.Body.String(), `"exit_code":0`) {
		t.Fatalf("snapshot sin resultado de prueba: %s", got.Body.String())
	}
}

// --- GET /sandboxes: listado según quien pregunta (§7) ---

func TestListadoSandboxes_PorRol(t *testing.T) {
	f := seedSandboxes(t, 4, 8)

	privada := crearSandboxOK(t, f.h, f.alum, `{"template_id":"python/numpy"}`)
	publica := crearSandboxOK(t, f.h, f.alum, `{"template_id":"cpp/sqlite","visibility":"public"}`)

	cases := []struct {
		name    string
		cookie  *http.Cookie
		quieres []string
		status  int
	}{
		{"anónimo: sin contexto de escuela → 401", nil, nil, http.StatusUnauthorized},
		{"invitado: ídem", f.guest, []string{publica["sandbox_id"].(string)}, http.StatusOK},
		{"alumno: propios", f.alum, []string{privada["sandbox_id"].(string), publica["sandbox_id"].(string)}, http.StatusOK},
		{"docente: de sus aulas", f.doc, []string{privada["sandbox_id"].(string), publica["sandbox_id"].(string)}, http.StatusOK},
		{"director: todo", f.dir, []string{privada["sandbox_id"].(string), publica["sandbox_id"].(string)}, http.StatusOK},
		{"alumno fuera: nada", f.fuera, nil, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if tc.cookie == nil { // anónimo: doJSON no tolera cookies nil
				rec = doJSON(t, f.h, "GET", "/sandboxes", "")
			} else {
				rec = doJSON(t, f.h, "GET", "/sandboxes", "", tc.cookie)
			}
			if rec.Code != tc.status {
				t.Fatalf("esperaba %d, got %d (%s)", tc.status, rec.Code, rec.Body.String())
			}
			if tc.status != http.StatusOK {
				return
			}
			for _, id := range tc.quieres {
				if !strings.Contains(rec.Body.String(), id) {
					t.Fatalf("listado sin %s: %s", id, rec.Body.String())
				}
			}
			noQuiere := privada["sandbox_id"].(string)
			if tc.cookie == nil || tc.cookie == f.guest || tc.cookie == f.fuera {
				if strings.Contains(rec.Body.String(), noQuiere) {
					t.Fatalf("%s no debía ver la sandbox privada", tc.name)
				}
			}
		})
	}
}

// --- POST /sandboxes/{id}/instantiate: relanza desde el registro (§7) ---

func TestInstantiateSandbox(t *testing.T) {
	f := seedSandboxes(t, 4, 8)
	sb := crearSandboxOK(t, f.h, f.alum, `{"template_id":"python/numpy"}`)
	sbxID := sb["sandbox_id"].(string)
	primerRun := sb["run_id"].(string)
	stepHasta(t, f.h, f.alum, primerRun, "cleaned", 8)

	cases := []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"alumno fuera", f.fuera, 403},
		{"docente del aula", f.doc, 202},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, f.h, "POST", "/sandboxes/"+sbxID+"/instantiate", "", tc.cookie)
			if rec.Code != tc.want {
				t.Fatalf("esperaba %d, got %d (%s)", tc.want, rec.Code, rec.Body.String())
			}
		})
	}

	rec := doJSON(t, f.h, "POST", "/sandboxes/"+sbxID+"/instantiate", "", f.alum)
	if rec.Code != 202 {
		t.Fatalf("instantiate autor: esperaba 202, got %d (%s)", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	nuevo := out["run_id"].(string)
	if nuevo == primerRun {
		t.Fatal("instantiate debió crear un run NUEVO")
	}
	final := stepHasta(t, f.h, f.alum, nuevo, "cleaned", 8)
	// #1 del create + #2 del instantiate docente + #3 del instantiate autor
	hist := final["history"].([]any)
	if len(hist) != 3 {
		t.Fatalf("history esperaba 3 ejecuciones, got %v", hist)
	}
	if hist[2].(map[string]any)["n"].(float64) != 3 {
		t.Fatalf("tercera ejecución debió ser #3: %v", hist[2])
	}
}
