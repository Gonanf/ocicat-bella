package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"

	"golang.org/x/crypto/bcrypt"
)

// dummyHash iguala el timing entre email inexistente y password incorrecta
// (§2.1: comparación constante aunque el email no exista).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("ocicat-timing-equalizer"), bcrypt.DefaultCost)

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// newToken genera un token base64url de 256 bits (§2.2 exige ≥128 bits).
func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func writeInternal(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": "internal_error", "message": "Error interno del servidor"},
	})
}

// schemeFromRequest deduce el esquema para armar links absolutos del magic link.
func schemeFromRequest(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// LoginEmailStep maneja POST /auth/login/email (§2.1 paso 1). Siempre 200.
func (h *Handler) LoginEmailStep(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Email == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	if retryAfter, ok := h.loginEmailRL.Allow(middleware.ExtractIP(r)); !ok {
		middleware.WriteTooManyRequests(w, retryAfter)
		return
	}

	next := "magic_link"
	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err == nil && (user.Role == model.RoleDocente || user.Role == model.RoleDirector) {
		next = "password"
	}
	writeJSON(w, http.StatusOK, map[string]string{"next": next})
}

// LoginPassword maneja POST /auth/login/password (§2.1 paso 2).
// 401 genérico anti-enumeración; 403 account_disabled sólo tras password correcta.
func (h *Handler) LoginPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Email == "" || req.Password == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	user, lookupErr := h.store.GetUserByEmail(r.Context(), req.Email)

	hash := dummyHash
	if lookupErr == nil && user.PasswordHash != "" {
		hash = []byte(user.PasswordHash)
	}
	bcryptErr := bcrypt.CompareHashAndPassword(hash, []byte(req.Password))
	if bcryptErr != nil || lookupErr != nil {
		key := strings.ToLower(req.Email) + "|" + middleware.ExtractIP(r)
		if !h.loginFailRL.RecordFailure(key) {
			middleware.WriteTooManyRequests(w, 900)
			return
		}
		errors.WriteCode(w, errors.CodeInvalidCredentials)
		return
	}
	if user.Disabled {
		errors.WriteCode(w, errors.CodeAccountDisabled)
		return
	}
	if user.Role != model.RoleDocente && user.Role != model.RoleDirector {
		errors.WriteCode(w, errors.CodeInvalidCredentials)
		return
	}

	now := time.Now()
	sess := &model.Session{
		ID:         newID(),
		UserID:     user.ID,
		Kind:       model.SessionKindStaff,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(model.StaffSlidingDuration),
	}
	if err := h.store.CreateSession(r.Context(), sess); err != nil {
		writeInternal(w)
		return
	}
	middleware.SetSessionCookie(w, sess, h.cfg.CookieDomain, h.cfg.IsProduction())
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

// MagicLinkRequest maneja POST /auth/magic-link (§2.2): 202 SIEMPRE,
// exista o no la cuenta ([C2] anti-enumeración).
func (h *Handler) MagicLinkRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email   string             `json:"email"`
		Context model.MagicContext `json:"context"`
	}
	if err := decodeJSON(r, &req); err != nil ||
		req.Email == "" || !strings.Contains(req.Email, "@") ||
		(req.Context != model.MagicContextPairingPWA && req.Context != model.MagicContextLoginPC) {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	ip := middleware.ExtractIP(r)
	if retryAfter, ok := h.magicIPRL.Allow(ip); !ok {
		middleware.WriteTooManyRequests(w, retryAfter)
		return
	}
	if retryAfter, ok := h.magicEmailRL.Allow(strings.ToLower(req.Email)); !ok {
		middleware.WriteTooManyRequests(w, retryAfter)
		return
	}

	if user, err := h.store.GetUserByEmail(r.Context(), req.Email); err == nil && !user.Disabled {
		now := time.Now()
		token := &model.MagicToken{
			Token:     newToken(),
			UserID:    user.ID,
			Context:   req.Context,
			CreatedAt: now,
			ExpiresAt: now.Add(model.MagicLinkTTL),
		}
		if err := h.store.CreateMagicToken(r.Context(), token); err != nil {
			// fallo interno: no enviamos nada, pero la respuesta sigue 202 ([C2])
			writeJSON(w, http.StatusAccepted, magicLinkResponse())
			return
		}
		link := schemeFromRequest(r) + "://" + r.Host + "/auth/consume?token=" + token.Token
		h.sendMagicLink(user.Email, link)
	}
	writeJSON(w, http.StatusAccepted, magicLinkResponse())
}

func magicLinkResponse() map[string]any {
	return map[string]any{
		"sent":    true,
		"message": "Si el email corresponde a una cuenta, recibís un enlace en unos minutos.",
	}
}

// ConsumeMagicLink maneja GET /auth/consume?token=... (§2.2).
func (h *Handler) ConsumeMagicLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if len(token) < 20 {
		errors.WriteCode(w, errors.CodeInvalidToken)
		return
	}

	mt, err := h.store.ConsumeMagicToken(r.Context(), token, time.Now())
	switch {
	case err == nil:
	case err == store.ErrTokenExpired:
		errors.WriteCode(w, errors.CodeTokenExpired)
		return
	case err == store.ErrTokenUsed:
		errors.WriteCode(w, errors.CodeTokenUsed)
		return
	default:
		errors.WriteCode(w, errors.CodeInvalidToken)
		return
	}

	user, err := h.store.GetUser(r.Context(), mt.UserID)
	if err != nil {
		errors.WriteCode(w, errors.CodeInvalidToken)
		return
	}
	if user.Disabled {
		errors.WriteCode(w, errors.CodeAccountDisabled)
		return
	}

	// [C2]-observación: consume desde una PC con pc_temporal activa invalida la previa
	// antes de emitir la nueva (un link abierto desde webmail no deja sesión huérfana).
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil && cookie.Value != "" {
		if prev, _, gerr := h.store.GetSession(r.Context(), cookie.Value); gerr == nil &&
			prev.Kind == model.SessionKindPCTemporal && !prev.IsExpired(time.Now()) {
			_ = h.store.DeleteSession(r.Context(), prev.ID)
		}
	}

	kind := model.SessionKindPCTemporal
	expiresAt := time.Now().Add(model.PCTemporalMaxTTL)
	if mt.Context == model.MagicContextPairingPWA {
		active := 0
		sessions, lerr := h.store.ListSessionsByUser(r.Context(), user.ID)
		if lerr == nil {
			for i := range sessions {
				if sessions[i].Kind == model.SessionKindPWA && !sessions[i].IsExpired(time.Now()) {
					active++
				}
			}
		}
		if active >= model.MaxPWADevices {
			// el token ya quedó consumido por ConsumeMagicToken (§2.2)
			errors.WriteCode(w, errors.CodeDeviceLimitReached)
			return
		}
		kind = model.SessionKindPWA
		expiresAt = time.Now().Add(model.PWASlidingDuration)
	}

	now := time.Now()
	sess := &model.Session{
		ID:         newID(),
		UserID:     user.ID,
		Kind:       kind,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  expiresAt,
	}
	if err := h.store.CreateSession(r.Context(), sess); err != nil {
		writeInternal(w)
		return
	}
	middleware.SetSessionCookie(w, sess, h.cfg.CookieDomain, h.cfg.IsProduction())
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "session_kind": string(kind)})
}

// Me maneja GET /auth/me: usuario de la sesión actual.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": middleware.UserFromContext(r.Context())})
}

// Logout maneja DELETE /sessions/current (§2.4).
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if sess := middleware.SessionFromContext(r.Context()); sess != nil {
		_ = h.store.DeleteSession(r.Context(), sess.ID)
	}
	middleware.ClearSessionCookie(w, h.cfg.CookieDomain, h.cfg.IsProduction())
	w.WriteHeader(http.StatusNoContent)
}
