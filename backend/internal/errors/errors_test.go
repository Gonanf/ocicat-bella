package errors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllErrorCodesInTable(t *testing.T) {
	expectedCodes := []struct {
		code   Code
		status int
	}{
		{CodeValidationError, http.StatusBadRequest},
		{CodeFileTooLarge, http.StatusBadRequest},
		{CodeInvalidToken, http.StatusBadRequest},
		{CodeInvalidCredentials, http.StatusBadRequest},
		{CodeUnauthenticated, http.StatusUnauthorized},
		{CodeForbidden, http.StatusForbidden},
		{CodeGuestReadOnly, http.StatusForbidden},
		{CodeAccountDisabled, http.StatusForbidden},
		{CodeNotFound, http.StatusNotFound},
		{CodeInvalidCode, http.StatusNotFound},
		{CodeEmailAlreadyExists, http.StatusConflict},
		{CodeDeviceLimitReached, http.StatusConflict},
		{CodeAlreadyMember, http.StatusConflict},
		{CodeAlreadyClaimed, http.StatusConflict},
		{CodeAttemptsExhausted, http.StatusConflict},
		{CodeDeadlinePassed, http.StatusConflict},
		{CodeSetupAlreadyDone, http.StatusConflict},
		{CodeAttemptConflict, http.StatusConflict},
		{CodeTokenUsed, http.StatusGone},
		{CodeTokenExpired, http.StatusGone},
		{CodeRateLimited, http.StatusTooManyRequests},
		{CodeCapacityUnavailable, http.StatusServiceUnavailable},
	}

	for _, tc := range expectedCodes {
		t.Run(string(tc.code), func(t *testing.T) {
			spec, exists := Table[tc.code]
			if !exists {
				t.Fatalf("error code %s not found in Table", tc.code)
			}
			if spec.HTTPStatus != tc.status {
				t.Errorf("expected HTTP status %d for code %s, got %d", tc.status, tc.code, spec.HTTPStatus)
			}
			if spec.Message == "" {
				t.Errorf("empty default message for code %s", tc.code)
			}

			// Test New constructor
			apiErr := New(tc.code)
			if apiErr.Code != tc.code {
				t.Errorf("expected code %s, got %s", tc.code, apiErr.Code)
			}
			if apiErr.HTTPStatus != tc.status {
				t.Errorf("expected status %d, got %d", tc.status, apiErr.HTTPStatus)
			}
		})
	}
}

func TestWriteAPIError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteCode(rec, CodeInvalidCode, "código personalizado")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type application/json; charset=utf-8, got %s", contentType)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error object in response, got nil")
	}

	if resp.Error.Code != CodeInvalidCode {
		t.Errorf("expected code %s, got %s", CodeInvalidCode, resp.Error.Code)
	}

	if resp.Error.Message != "código personalizado" {
		t.Errorf("expected custom message, got %s", resp.Error.Message)
	}
}

func TestUnknownCodeFallback(t *testing.T) {
	apiErr := New("unknown_nonexistent_code")
	if apiErr.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("expected 500 for unknown error code, got %d", apiErr.HTTPStatus)
	}
}
