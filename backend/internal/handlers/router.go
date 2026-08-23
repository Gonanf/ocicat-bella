package handlers

import (
	"net/http"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

// Handler wraps the ServeMux router and dependencies.
type Handler struct {
	cfg         *config.Config
	store       store.Store
	mux         *http.ServeMux
	rateLimiter *middleware.IPRateLimiter
}

// NewRouter initializes the ServeMux with Go 1.22+ routing patterns and global middlewares.
func NewRouter(cfg *config.Config, s store.Store) http.Handler {
	h := &Handler{
		cfg:         cfg,
		store:       s,
		mux:         http.NewServeMux(),
		rateLimiter: middleware.NewIPRateLimiter(cfg.RateLimitRPH, 20),
	}

	h.registerRoutes()

	// Build global middleware chain:
	// RateLimiting -> SessionMiddleware -> ServeMux
	var handler http.Handler = h.mux
	handler = middleware.SessionMiddleware(s)(handler)
	handler = middleware.RateLimitMiddleware(h.rateLimiter)(handler)

	return handler
}

func (h *Handler) registerRoutes() {
	// Healthcheck endpoint
	h.mux.HandleFunc("GET /healthz", Healthz)
}
