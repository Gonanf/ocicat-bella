package handlers

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/errors"
	"github.com/Gonanf/ocicat-bella/backend/internal/middleware"
	"github.com/Gonanf/ocicat-bella/backend/internal/model"
	"github.com/Gonanf/ocicat-bella/backend/internal/store"
)

// qrDomain resuelve el dominio para la qr_url ([C3] anti-quishing):
// dominio oficial configurado; en dev sin OFFICIAL_DOMAIN, el Host de la request.
func (h *Handler) qrDomain(r *http.Request) string {
	if h.cfg.OfficialDomain != "" {
		return h.cfg.OfficialDomain
	}
	return r.Host
}

// requirePWAAlumno exige alumno con sesión pwa emparejada (§2.3 scan/confirm/deny).
func requirePWAAlumno(next http.Handler) http.Handler {
	return middleware.RequireRole(model.RoleAlumno)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sess := middleware.SessionFromContext(r.Context()); sess == nil || sess.Kind != model.SessionKindPWA {
			errors.WriteCode(w, errors.CodeForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// QRStart maneja POST /auth/qr/start (§2.3): la PC crea la sesión de emparejamiento.
func (h *Handler) QRStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceLabel string `json:"device_label"`
	}
	if err := decodeJSON(r, &req); err != nil && err != io.EOF {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}
	label := strings.TrimSpace(req.DeviceLabel)
	if len(label) > 200 {
		errors.WriteCode(w, errors.CodeValidationError, "device_label demasiado largo")
		return
	}

	ip := middleware.ExtractIP(r)
	if retryAfter, ok := h.qrStartRL.Allow(ip); !ok {
		middleware.WriteTooManyRequests(w, retryAfter)
		return
	}

	now := time.Now()
	p := &model.PairingSession{
		PairingID:   newID(),
		QRToken:     newToken(),
		DeviceLabel: label,
		OriginIP:    ip,
		Status:      model.PairingWaiting,
		CreatedAt:   now,
		ExpiresAt:   now.Add(model.QRTTL),
	}
	if err := h.store.CreatePairingSession(r.Context(), p); err != nil {
		writeInternal(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"pairing_id":    p.PairingID,
		"qr_url":        schemeFromRequest(r) + "://" + h.qrDomain(r) + "/qr/" + p.QRToken,
		"expires_in":    int(model.QRTTL.Seconds()),
		"refresh_after": model.QRRefreshAfter,
	})
}

// QRStatus maneja GET /auth/qr/{pairing_id}/status (§2.3). GET no muta estado:
// el claim ocurre solo en /confirm; acá solo se emite la cookie ya creada.
func (h *Handler) QRStatus(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.GetPairingSession(r.Context(), r.PathValue("pairing_id"))
	if err != nil {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	}

	status := p.EffectiveStatus(time.Now())
	if status != model.PairingConfirmed {
		writeJSON(w, http.StatusOK, map[string]string{"status": string(status)})
		return
	}

	sess, user, err := h.store.GetSession(r.Context(), p.SessionID)
	if err != nil || sess == nil || user == nil {
		writeInternal(w)
		return
	}
	middleware.SetSessionCookie(w, sess, h.cfg.CookieDomain, h.cfg.IsProduction())
	writeJSON(w, http.StatusOK, map[string]any{
		"status": string(status),
		"user":   map[string]string{"name": user.Name, "role": string(user.Role)},
	})
}

// QRScan maneja POST /auth/qr/scan (§2.3): el celular PWA registra el escaneo.
func (h *Handler) QRScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QRToken string `json:"qr_token"`
	}
	if err := decodeJSON(r, &req); err != nil || req.QRToken == "" {
		errors.WriteCode(w, errors.CodeValidationError)
		return
	}

	me := middleware.UserFromContext(r.Context())
	now := time.Now()

	p, err := h.store.GetPairingSessionByToken(r.Context(), req.QRToken)
	if err == store.ErrNotFound {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	} else if err != nil {
		writeInternal(w)
		return
	}

	switch p.EffectiveStatus(now) {
	case model.PairingExpired:
		errors.WriteCode(w, errors.CodeTokenExpired, "este código ya no sirve, mirá el nuevo")
		return
	case model.PairingScanned:
		if p.UserID == me.ID {
			h.writeScanResponse(w, p) // reintentos de red: idempotente para el mismo usuario
			return
		}
		errors.WriteCode(w, errors.CodeTokenExpired, "este código ya no sirve, mirá el nuevo")
		return
	case model.PairingConfirmed, model.PairingDenied:
		errors.WriteCode(w, errors.CodeTokenExpired, "este código ya no sirve, mirá el nuevo")
		return
	}

	if err := h.store.ScanPairingSession(r.Context(), p.PairingID, me.ID, now); err != nil {
		if err == store.ErrAlreadyClaimed {
			// carrera con otro scan: releer y decidir
			if fresh, gerr := h.store.GetPairingSession(r.Context(), p.PairingID); gerr == nil &&
				fresh.Status == model.PairingScanned && fresh.UserID == me.ID {
				h.writeScanResponse(w, fresh)
				return
			}
		}
		errors.WriteCode(w, errors.CodeTokenExpired, "este código ya no sirve, mirá el nuevo")
		return
	}
	h.writeScanResponse(w, p)
}

func (h *Handler) writeScanResponse(w http.ResponseWriter, p *model.PairingSession) {
	writeJSON(w, http.StatusOK, map[string]any{
		"pairing_id":   p.PairingID,
		"device_label": p.DeviceLabel,
		"requested_at": p.CreatedAt.UTC().Format(time.RFC3339),
		"origin_ip":    p.OriginIP,
	})
}

// QRConfirm maneja POST /auth/qr/{pairing_id}/confirm (§2.3): claim atómico
// scanned→confirmed exactamente una vez; emite la sesión pc_temporal que
// /status entregará por Set-Cookie.
func (h *Handler) QRConfirm(w http.ResponseWriter, r *http.Request) {
	me := middleware.UserFromContext(r.Context())

	p, err := h.store.GetPairingSession(r.Context(), r.PathValue("pairing_id"))
	if err == store.ErrNotFound {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	} else if err != nil {
		writeInternal(w)
		return
	}
	if p.UserID != me.ID {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}

	now := time.Now()
	sess := &model.Session{
		ID:         newID(),
		UserID:     me.ID,
		Kind:       model.SessionKindPCTemporal,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(model.PCTemporalMaxTTL),
	}
	err = h.store.ClaimPairingSession(r.Context(), p.PairingID, me.ID, sess, now)
	switch err {
	case nil:
		writeJSON(w, http.StatusOK, map[string]any{})
	case store.ErrAlreadyClaimed:
		errors.WriteCode(w, errors.CodeAlreadyClaimed)
	case store.ErrTokenExpired:
		errors.WriteCode(w, errors.CodeTokenExpired)
	case store.ErrNotFound:
		errors.WriteCode(w, errors.CodeNotFound)
	default:
		writeInternal(w)
	}
}

// QRDeny maneja POST /auth/qr/{pairing_id}/deny (§2.3): rechazo explícito;
// la PC ve denied y nunca inicia sesión.
func (h *Handler) QRDeny(w http.ResponseWriter, r *http.Request) {
	me := middleware.UserFromContext(r.Context())

	p, err := h.store.GetPairingSession(r.Context(), r.PathValue("pairing_id"))
	if err == store.ErrNotFound {
		errors.WriteCode(w, errors.CodeNotFound)
		return
	} else if err != nil {
		writeInternal(w)
		return
	}
	if p.UserID != me.ID {
		errors.WriteCode(w, errors.CodeForbidden)
		return
	}

	switch err := h.store.DenyPairingSession(r.Context(), p.PairingID, me.ID, time.Now()); err {
	case nil, store.ErrAlreadyClaimed:
		writeJSON(w, http.StatusOK, map[string]any{})
	case store.ErrTokenExpired:
		errors.WriteCode(w, errors.CodeTokenExpired)
	case store.ErrNotFound:
		errors.WriteCode(w, errors.CodeNotFound)
	default:
		writeInternal(w)
	}
}
