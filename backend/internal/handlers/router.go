package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/config"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
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
	qrStartRL     *middleware.IPRateLimiter // 60/h por IP (§0.3)
	joinRL        *middleware.IPRateLimiter // POST /classrooms/join: 20/h por IP (§0.3)
	guestRL       *middleware.IPRateLimiter // POST /guest/sessions: 20/h por IP (§0.3/§10)
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
		rateLimiter:  middleware.NewIPRateLimiter(cfg.RateLimitRPH, 200),
		loginEmailRL: middleware.NewIPRateLimiter(10, 10),
		magicIPRL:    middleware.NewIPRateLimiter(10, 10),
		magicEmailRL: middleware.NewIPRateLimiter(3, 3),
		qrStartRL:    middleware.NewIPRateLimiter(60, 20),
		joinRL:       middleware.NewIPRateLimiter(20, 20),
		guestRL:      middleware.NewIPRateLimiter(20, 20),
		loginFailRL:  middleware.NewFailLimiter(5, 15*60*time.Second),
	}
	// DEV: sin SMTP configurado, el link queda en el log del servidor
	h.sendMagicLink = func(email, link string) {
		log.Printf("[magic-link] para %s: %s", email, link)
	}

	h.registerRoutes()

	// Build global middleware chain:
	// RateLimiting -> SessionMiddleware -> OriginCheck -> ServeMux
	var handler http.Handler = h.mux
	handler = middleware.OriginMiddleware(cfg.OfficialDomain)(handler)
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

	// QR inverso CU-16 (§2.3)
	h.mux.HandleFunc("POST /auth/qr/start", h.QRStart)
	h.mux.HandleFunc("GET /auth/qr/{pairing_id}/status", h.QRStatus)
	h.mux.Handle("POST /auth/qr/scan", requirePWAAlumno(http.HandlerFunc(h.QRScan)))
	h.mux.Handle("POST /auth/qr/{pairing_id}/confirm", requirePWAAlumno(http.HandlerFunc(h.QRConfirm)))
	h.mux.Handle("POST /auth/qr/{pairing_id}/deny", requirePWAAlumno(http.HandlerFunc(h.QRDeny)))

	// Sesiones y dispositivos (§2.4)
	h.mux.Handle("GET /sessions", middleware.RequireAuth(http.HandlerFunc(h.ListSessions)))
	h.mux.Handle("DELETE /sessions/{id}", middleware.RequireAuth(http.HandlerFunc(h.DeleteSession)))
	h.mux.Handle("POST /sessions/close-others", middleware.RequireAuth(http.HandlerFunc(h.CloseOthers)))

	h.registerFase4Routes()
	h.registerFase5Routes()
}

// atajos de middleware para las rutas de Fase 4.
func requireStaff(next http.Handler) http.Handler {
	// docente o director (director pasa checks de docente, §0.1)
	return middleware.RequireRole(model.RoleDocente)(next)
}

func requireAlumno(next http.Handler) http.Handler {
	return middleware.RequireRole(model.RoleAlumno)(next)
}

func requireDirector(next http.Handler) http.Handler {
	return middleware.RequireRole(model.RoleDirector)(next)
}

func (h *Handler) registerFase4Routes() {
	// Aulas (§3)
	guestRO := middleware.RequireNonGuest
	h.mux.Handle("POST /classrooms", guestRO(requireStaff(http.HandlerFunc(h.CreateClassroom))))
	h.mux.Handle("GET /classrooms", middleware.RequireAuth(http.HandlerFunc(h.ListClassrooms)))
	h.mux.Handle("POST /classrooms/join", guestRO(requireAlumno(http.HandlerFunc(h.JoinClassroom))))
	h.mux.Handle("GET /classrooms/{id}", requireStaff(http.HandlerFunc(h.GetClassroomDetail)))
	h.mux.Handle("PATCH /classrooms/{id}", guestRO(requireStaff(http.HandlerFunc(h.PatchClassroom))))
	h.mux.Handle("DELETE /classrooms/{id}", guestRO(requireStaff(http.HandlerFunc(h.DeleteClassroom))))
	h.mux.Handle("GET /classrooms/{id}/join_code", requireStaff(http.HandlerFunc(h.GetJoinCode)))
	h.mux.Handle("POST /classrooms/{id}/join_code/rotate", guestRO(requireStaff(http.HandlerFunc(h.RotateJoinCode))))
	h.mux.Handle("DELETE /classrooms/{id}/membership/me", guestRO(requireAlumno(http.HandlerFunc(h.LeaveClassroom))))
	h.mux.Handle("GET /classrooms/{id}/students", requireStaff(http.HandlerFunc(h.ListStudents)))
	h.mux.Handle("DELETE /classrooms/{id}/students/{user_id}", guestRO(requireStaff(http.HandlerFunc(h.RemoveStudent))))

	// Materiales (§6)
	h.mux.HandleFunc("GET /materials", h.ListMaterials) // pública, filtra por rol
	h.mux.Handle("POST /classrooms/{id}/materials", guestRO(requireStaff(http.HandlerFunc(h.UploadMaterial))))
	h.mux.HandleFunc("GET /materials/{id}", h.GetMaterialMeta)
	h.mux.HandleFunc("GET /materials/{id}/file", h.ServeMaterialFile)
	h.mux.Handle("PATCH /materials/{id}", guestRO(requireStaff(http.HandlerFunc(h.PatchMaterial))))
	h.mux.Handle("DELETE /materials/{id}", guestRO(requireStaff(http.HandlerFunc(h.DeleteMaterial))))

	// Escuela: código global + invitados (§9.1, §10)
	h.mux.Handle("GET /school", requireDirector(http.HandlerFunc(h.SchoolInfo)))
	h.mux.Handle("POST /school/global-code/regenerate", requireDirector(http.HandlerFunc(h.RegenerateGlobalCode)))
	h.mux.Handle("DELETE /school/global-code", requireDirector(http.HandlerFunc(h.DisableGlobalCode)))
	h.mux.HandleFunc("GET /school/public", h.PublicSchoolInfo)
	h.mux.HandleFunc("POST /guest/sessions", h.GuestSession)
}

func (h *Handler) registerFase5Routes() {
	guestRO := middleware.RequireNonGuest

	// Consignas (§4)
	h.mux.Handle("POST /classrooms/{id}/assignments", guestRO(requireStaff(http.HandlerFunc(h.CreateAssignment))))
	h.mux.Handle("GET /classrooms/{id}/assignments", middleware.RequireAuth(http.HandlerFunc(h.ListAssignments)))
	h.mux.Handle("POST /assignments/attachments", guestRO(requireStaff(http.HandlerFunc(h.UploadAttachment))))
	h.mux.Handle("GET /assignments/{id}", middleware.RequireAuth(http.HandlerFunc(h.GetAssignment)))
	h.mux.Handle("PATCH /assignments/{id}", guestRO(requireStaff(http.HandlerFunc(h.PatchAssignment))))
	h.mux.Handle("DELETE /assignments/{id}", guestRO(requireStaff(http.HandlerFunc(h.DeleteAssignment))))
	h.mux.Handle("GET /assignments/{id}/stats", requireStaff(http.HandlerFunc(h.GetAssignmentStats)))

	// Entregas (§5): submission = UN intento
	h.mux.Handle("POST /assignments/{id}/submissions/files", guestRO(requireAlumno(http.HandlerFunc(h.UploadSubmissionFiles))))
	h.mux.Handle("GET /assignments/{id}/submissions/me", requireAlumno(http.HandlerFunc(h.MySubmissions)))
	h.mux.Handle("GET /assignments/{id}/submissions", requireStaff(http.HandlerFunc(h.ListAssignmentSubmissions)))
	h.mux.Handle("POST /submissions/{submission_id}/deliver", guestRO(requireAlumno(http.HandlerFunc(h.DeliverSubmission))))
	h.mux.Handle("GET /submissions/{submission_id}", middleware.RequireAuth(http.HandlerFunc(h.GetSubmission)))
}
