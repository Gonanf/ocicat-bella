package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	Healthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type application/json; charset=utf-8, got %q", contentType)
	}

	var resp map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode healthz response: %v", err)
	}

	if !resp["ok"] {
		t.Errorf("expected ok: true, got %v", resp["ok"])
	}
}

func TestRouter_Integration(t *testing.T) {
	cfg := &config.Config{
		RateLimitRPH: 100,
		Env:          "development",
	}
	memStore := store.NewMemStore()
	router := NewRouter(cfg, memStore)

	// Test GET /healthz
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 on /healthz through router, got %d", rec.Code)
	}

	// Test Method Not Allowed / 404 for unrouted methods
	postReq := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed for POST /healthz, got %d", postRec.Code)
	}
}

func TestRouter_APIv1Prefix(t *testing.T) {
	cfg := &config.Config{RateLimitRPH: 100, Env: "development"}
	router := NewRouter(cfg, store.NewMemStore())

	for _, path := range []string{"/api/v1/healthz", "/api/v1/sandboxes", "/api/v1/templates", "/api/v1/auth/me"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Errorf("%s: expected route to exist, got 404", path)
		}
	}
}
