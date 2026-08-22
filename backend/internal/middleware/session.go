package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

// SessionCookieName is the standard cookie name defined in docs/api-v1.md §0.2.
const SessionCookieName = "ocicat_session"

type contextKey string

const (
	userContextKey    contextKey = "ocicat_user"
	sessionContextKey contextKey = "ocicat_session"
)

// WithUser returns a new context with the given user attached.
func WithUser(ctx context.Context, user *model.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext retrieves the user from context, or nil if anonymous.
func UserFromContext(ctx context.Context) *model.User {
	if u, ok := ctx.Value(userContextKey).(*model.User); ok {
		return u
	}
	return nil
}

// WithSession returns a new context with the given session attached.
func WithSession(ctx context.Context, session *model.Session) context.Context {
	return context.WithValue(ctx, sessionContextKey, session)
}

// SessionFromContext retrieves the session from context, or nil if none.
func SessionFromContext(ctx context.Context) *model.Session {
	if s, ok := ctx.Value(sessionContextKey).(*model.Session); ok {
		return s
	}
	return nil
}

// SessionMiddleware extracts and validates the session cookie from requests,
// attaching the active User and Session to the request context.
// If no session cookie is present or if the session is expired/invalid,
// the request proceeds with a nil user in context (anonymous).
func SessionMiddleware(s store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				// Anonymous request
				next.ServeHTTP(w, r)
				return
			}

			now := time.Now()
			sess, user, err := s.GetSession(r.Context(), cookie.Value)
			if err != nil || sess == nil || user == nil {
				// Invalid or not found session
				next.ServeHTTP(w, r)
				return
			}

			// Validate session lifetime and inactivity rules (§0.2)
			if sess.IsExpired(now) {
				// Expired session -> treat as anonymous
				next.ServeHTTP(w, r)
				return
			}

			// Best-effort update of last seen timestamp
			_ = s.TouchSession(r.Context(), sess.ID, now)
			sess.LastSeenAt = now

			ctx := WithUser(r.Context(), user)
			ctx = WithSession(ctx, sess)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SetSessionCookie configures the session cookie according to §0.2 rules:
// - HttpOnly: true
// - Secure: enabled in production or over HTTPS
// - SameSite: Strict
// - Path: "/"
// - pc_temporal: no Max-Age (session cookie, erased on browser close)
// - pwa: 30 days sliding Max-Age
// - staff: 7 days sliding Max-Age
func SetSessionCookie(w http.ResponseWriter, session *model.Session, domain string, isProduction bool) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Domain:   domain,
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteStrictMode,
	}

	switch session.Kind {
	case model.SessionKindPCTemporal:
		// pc_temporal: no Max-Age / Expires set (session cookie)
		cookie.MaxAge = 0
	case model.SessionKindPWA:
		cookie.MaxAge = int(model.PWASlidingDuration.Seconds())
		cookie.Expires = time.Now().Add(model.PWASlidingDuration)
	case model.SessionKindStaff:
		cookie.MaxAge = int(model.StaffSlidingDuration.Seconds())
		cookie.Expires = time.Now().Add(model.StaffSlidingDuration)
	}

	http.SetCookie(w, cookie)
}

// ClearSessionCookie clears the session cookie from the client.
func ClearSessionCookie(w http.ResponseWriter, domain string, isProduction bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   domain,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteStrictMode,
	})
}
