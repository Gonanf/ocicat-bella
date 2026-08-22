package middleware

import (
	"net/http"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// RequireAuth ensures that the request has an active, authenticated user.
// Returns 401 unauthenticated if user is nil, or 403 account_disabled if disabled.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			errors.WriteCode(w, errors.CodeUnauthenticated)
			return
		}

		if user.Disabled {
			errors.WriteCode(w, errors.CodeAccountDisabled)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireRole ensures that the authenticated user has one of the allowed roles.
// Hierarchy rules: Director inherits all Docente permissions (§0.1).
func RequireRole(allowedRoles ...model.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil {
				errors.WriteCode(w, errors.CodeUnauthenticated)
				return
			}

			if user.Disabled {
				errors.WriteCode(w, errors.CodeAccountDisabled)
				return
			}

			for _, role := range allowedRoles {
				// Director passes all teacher (docente) checks (§0.1, §13.9.2)
				if user.Role == role || (user.Role == model.RoleDirector && role == model.RoleDocente) {
					next.ServeHTTP(w, r)
					return
				}
			}

			errors.WriteCode(w, errors.CodeForbidden)
		})
	}
}

// RequireNonGuest blocks guest (or anonymous) users from mutating resources (CU-1, CU-14).
func RequireNonGuest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil || user.Role == model.RoleInvitado || user.Role == model.RoleAnonimo {
			errors.WriteCode(w, errors.CodeGuestReadOnly)
			return
		}

		next.ServeHTTP(w, r)
	})
}
