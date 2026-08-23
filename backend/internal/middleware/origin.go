package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
)

// OriginMiddleware valida el header Origin en toda mutación (§0.2 CSRF):
// si viene y no coincide con el dominio oficial (o el Host de la request en
// dev sin OFFICIAL_DOMAIN) → 403 forbidden. Sin Origin pasa (curl/tests).
func OriginMiddleware(officialDomain string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
			default:
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			u, err := url.Parse(origin)
			if err != nil || u.Host == "" {
				errors.WriteCode(w, errors.CodeForbidden)
				return
			}
			if strings.EqualFold(u.Host, r.Host) ||
				(officialDomain != "" && strings.EqualFold(u.Host, officialDomain)) {
				next.ServeHTTP(w, r)
				return
			}
			errors.WriteCode(w, errors.CodeForbidden)
		})
	}
}
