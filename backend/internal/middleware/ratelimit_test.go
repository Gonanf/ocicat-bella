package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
)

func TestRateLimitMiddleware_Triggers429WithRetryAfter(t *testing.T) {
	// Create limiter with 1 request per hour and burst of 2
	limiter := NewIPRateLimiter(1, 2)
	mw := RateLimitMiddleware(limiter)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	// 1st request -> should pass
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "192.168.1.100:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("expected 1st request to pass with 200, got %d", rec1.Code)
	}

	// 2nd request -> should pass (burst 2)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "192.168.1.100:12345"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 2nd request to pass with 200, got %d", rec2.Code)
	}

	// 3rd request -> burst exhausted, should return 429
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "192.168.1.100:12345"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 3rd request to be 429 rate_limited, got %d", rec3.Code)
	}

	// Check Retry-After header
	retryAfterStr := rec3.Header().Get("Retry-After")
	if retryAfterStr == "" {
		t.Fatal("expected Retry-After header to be present")
	}

	retryAfter, err := strconv.Atoi(retryAfterStr)
	if err != nil || retryAfter <= 0 {
		t.Errorf("expected positive integer Retry-After header, got %q (err: %v)", retryAfterStr, err)
	}

	// Check response body structure
	var resp errors.ErrorResponse
	if err := json.Unmarshal(rec3.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json error response: %v", err)
	}

	if resp.Error == nil || resp.Error.Code != errors.CodeRateLimited {
		t.Errorf("expected error code %s, got %+v", errors.CodeRateLimited, resp.Error)
	}
}

func TestRateLimitMiddleware_DifferentIPsAreIsolated(t *testing.T) {
	limiter := NewIPRateLimiter(1, 1)
	mw := RateLimitMiddleware(limiter)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP 1 first request -> 200
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("expected IP 1 first request 200, got %d", rec1.Code)
	}

	// IP 1 second request -> 429
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("expected IP 1 second request 429, got %d", rec2.Code)
	}

	// IP 2 first request -> 200 (isolated from IP 1)
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "10.0.0.2:1234"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("expected IP 2 first request 200, got %d", rec3.Code)
	}
}
