package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

func TestSessionMiddleware_Anonymous(t *testing.T) {
	memStore := store.NewMemStore()
	mw := SessionMiddleware(memStore)

	var ctxUser *model.User
	var ctxSess *model.Session

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxUser = UserFromContext(r.Context())
		ctxSess = SessionFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ctxUser != nil {
		t.Errorf("expected nil user for anonymous request, got %+v", ctxUser)
	}
	if ctxSess != nil {
		t.Errorf("expected nil session for anonymous request, got %+v", ctxSess)
	}
}

func TestSessionMiddleware_ValidSessions(t *testing.T) {
	memStore := store.NewMemStore()
	ctx := context.Background()

	// Seed User
	user := &model.User{
		ID:       "usr_123",
		Name:     "Ana García",
		Email:    "ana@escuela.edu.ar",
		Role:     model.RoleDocente,
		Disabled: false,
	}
	_ = memStore.CreateUser(ctx, user)

	// Seed Staff Session
	now := time.Now()
	staffSess := &model.Session{
		ID:         "sess_staff_123",
		UserID:     user.ID,
		Kind:       model.SessionKindStaff,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(model.StaffSlidingDuration),
	}
	_ = memStore.CreateSession(ctx, staffSess)

	mw := SessionMiddleware(memStore)

	var ctxUser *model.User
	var ctxSess *model.Session

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxUser = UserFromContext(r.Context())
		ctxSess = SessionFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/classrooms", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: staffSess.ID,
	})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ctxUser == nil || ctxUser.ID != user.ID {
		t.Fatalf("expected user %s in context, got %+v", user.ID, ctxUser)
	}
	if ctxSess == nil || ctxSess.ID != staffSess.ID {
		t.Fatalf("expected session %s in context, got %+v", staffSess.ID, ctxSess)
	}
}

func TestSessionMiddleware_PCTemporalRules(t *testing.T) {
	memStore := store.NewMemStore()
	ctx := context.Background()

	user := &model.User{
		ID:   "usr_student",
		Name: "Juan Perez",
		Role: model.RoleAlumno,
	}
	_ = memStore.CreateUser(ctx, user)

	now := time.Now()

	// 1. Expired by TTL (>60m)
	expiredTTL := &model.Session{
		ID:         "sess_ttl_expired",
		UserID:     user.ID,
		Kind:       model.SessionKindPCTemporal,
		CreatedAt:  now.Add(-65 * time.Minute),
		LastSeenAt: now.Add(-5 * time.Minute),
		ExpiresAt:  now.Add(-5 * time.Minute),
	}
	_ = memStore.CreateSession(ctx, expiredTTL)

	// 2. Expired by inactivity (>10m idle)
	expiredIdle := &model.Session{
		ID:         "sess_idle_expired",
		UserID:     user.ID,
		Kind:       model.SessionKindPCTemporal,
		CreatedAt:  now.Add(-20 * time.Minute),
		LastSeenAt: now.Add(-12 * time.Minute),
		ExpiresAt:  now.Add(40 * time.Minute),
	}
	_ = memStore.CreateSession(ctx, expiredIdle)

	// 3. Active pc_temporal session
	activeSession := &model.Session{
		ID:         "sess_active",
		UserID:     user.ID,
		Kind:       model.SessionKindPCTemporal,
		CreatedAt:  now.Add(-15 * time.Minute),
		LastSeenAt: now.Add(-2 * time.Minute),
		ExpiresAt:  now.Add(45 * time.Minute),
	}
	_ = memStore.CreateSession(ctx, activeSession)

	mw := SessionMiddleware(memStore)

	tests := []struct {
		name         string
		cookieVal    string
		expectAuthed bool
	}{
		{"TTL expired", expiredTTL.ID, false},
		{"Inactivity expired", expiredIdle.ID, false},
		{"Active session", activeSession.ID, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var authed bool
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				u := UserFromContext(r.Context())
				authed = (u != nil)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tc.cookieVal})
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if authed != tc.expectAuthed {
				t.Errorf("expected authed=%v, got %v", tc.expectAuthed, authed)
			}
		})
	}
}

func TestAuthGuards(t *testing.T) {
	t.Run("RequireAuth without user", func(t *testing.T) {
		handler := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 unauthenticated, got %d", rec.Code)
		}
	})

	t.Run("RequireAuth with disabled user", func(t *testing.T) {
		handler := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		disabledUser := &model.User{ID: "u1", Role: model.RoleDocente, Disabled: true}
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req = req.WithContext(WithUser(req.Context(), disabledUser))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 account_disabled, got %d", rec.Code)
		}
	})

	t.Run("RequireRole hierarchy - director passes docente check", func(t *testing.T) {
		handler := RequireRole(model.RoleDocente)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		director := &model.User{ID: "u_dir", Role: model.RoleDirector}
		req := httptest.NewRequest(http.MethodGet, "/classrooms", nil)
		req = req.WithContext(WithUser(req.Context(), director))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("director should pass docente check, got status %d", rec.Code)
		}
	})

	t.Run("RequireRole alumno blocked from docente endpoint", func(t *testing.T) {
		handler := RequireRole(model.RoleDocente)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		alumno := &model.User{ID: "u_alum", Role: model.RoleAlumno}
		req := httptest.NewRequest(http.MethodGet, "/classrooms", nil)
		req = req.WithContext(WithUser(req.Context(), alumno))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("alumno should be forbidden, got status %d", rec.Code)
		}
	})

	t.Run("RequireNonGuest blocks invitado", func(t *testing.T) {
		handler := RequireNonGuest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		guest := &model.User{ID: "u_guest", Role: model.RoleInvitado}
		req := httptest.NewRequest(http.MethodPost, "/submissions", nil)
		req = req.WithContext(WithUser(req.Context(), guest))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("guest should be blocked with 403 guest_read_only, got %d", rec.Code)
		}
	})
}
