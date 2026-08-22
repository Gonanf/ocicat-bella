package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

// Handler wraps the ServeMux router and dependencies.
type Handler struct {
	cfg   *config.Config
	store store.Store
	mux   *http.ServeMux
	root  http.Handler // cadena completa: RateLimit -> Session -> mux

	rateLimiter   *middleware.IPRateLimiter
	loginEmailRL  *middleware.IPRateLimiter // 10/h por IP (§0.3)
	magicIPRL     *middleware.IPRateLimiter // 10/h por IP (§0.3)
	magicEmailRL  *middleware.IPRateLimiter // 3/h por email (§0.3)
	loginFailRL   *middleware.FailLimiter   // 5 fallos/15min por email+IP (§0.3)
	sendMagicLink func(email, link string)  // seam de envío de email; inyectable en tests
}

// SetMagicLinkSender reemplaza el envío de emails (tests / SMTP real).
func (h *Handler) SetMagicLinkSender(fn func(email, link string)) {
	h.sendMagicLink = fn
}

// ServeHTTP delega a la cadena completa de middlewares.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.root.ServeHTTP(w, r)
}

// NewRouter initializes the ServeMux with Go 1.22+ routing patterns and global middlewares.
func NewRouter(cfg *config.Config, s store.Store) *Handler {
	h := &Handler{
		cfg:          cfg,
		store:        s,
		mux:          http.NewServeMux(),
		rateLimiter:  middleware.NewIPRateLimiter(cfg.RateLimitRPH, 20),
		loginEmailRL: middleware.NewIPRateLimiter(10, 10),
		magicIPRL:    middleware.NewIPRateLimiter(10, 10),
		magicEmailRL: middleware.NewIPRateLimiter(3, 3),
		loginFailRL:  middleware.NewFailLimiter(5, 15*60*time.Second),
	}
	// DEV: sin SMTP configurado, el link queda en el log del servidor
	h.sendMagicLink = func(email, link string) {
		log.Printf("[magic-link] para %s: %s", email, link)
	}

	h.registerRoutes()

	// Build global middleware chain:
	// RateLimiting -> SessionMiddleware -> ServeMux
	var handler http.Handler = h.mux
	handler = middleware.SessionMiddleware(s)(handler)
	handler = middleware.RateLimitMiddleware(h.rateLimiter)(handler)

	h.root = handler
	return h
}

func (h *Handler) registerRoutes() {
	h.mux.HandleFunc("GET /healthz", Healthz)

	// Setup wizard (§1)
	h.mux.HandleFunc("GET /setup/status", h.SetupStatus)
	h.mux.HandleFunc("POST /setup/school", h.SetupSchool)
	h.mux.HandleFunc("POST /setup/admin", h.SetupAdmin)

	// Auth (§2)
	h.mux.HandleFunc("POST /auth/login/email", h.LoginEmailStep)
	h.mux.HandleFunc("POST /auth/login/password", h.LoginPassword)
	h.mux.HandleFunc("POST /auth/magic-link", h.MagicLinkRequest)
	h.mux.HandleFunc("GET /auth/consume", h.ConsumeMagicLink)
	h.mux.Handle("GET /auth/me", middleware.RequireAuth(http.HandlerFunc(h.Me)))
	h.mux.Handle("DELETE /sessions/current", middleware.RequireAuth(http.HandlerFunc(h.Logout)))
}
