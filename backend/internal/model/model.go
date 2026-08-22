package model

import (
	"time"
)

// Role represents user roles in the hierarchy:
// director -> docente -> alumno -> invitado -> anonimo
type Role string

const (
	RoleDirector Role = "director"
	RoleDocente  Role = "docente"
	RoleAlumno   Role = "alumno"
	RoleInvitado Role = "invitado"
	RoleAnonimo  Role = "anonimo"
)

// SessionKind represents session classes defined in docs/api-v1.md §0.2.
type SessionKind string

const (
	// SessionKindPWA is a persistent pairing session for students (30 days sliding).
	SessionKindPWA SessionKind = "pwa"
	// SessionKindStaff is a persistent session for teachers/directors (7 days sliding).
	SessionKindStaff SessionKind = "staff"
	// SessionKindPCTemporal is an ephemeral session for shared school PCs (TTL <= 60 min, inactivity <= 10 min).
	SessionKindPCTemporal SessionKind = "pc_temporal"
)

// User represents an authenticated user in the system.
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Role     Role   `json:"role"`
	Disabled bool   `json:"disabled"`
}

// Session represents an active user session.
type Session struct {
	ID          string      `json:"id"`
	UserID      string      `json:"user_id"`
	Kind        SessionKind `json:"kind"`
	DeviceLabel string      `json:"device_label,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	LastSeenAt  time.Time   `json:"last_seen_at"`
	ExpiresAt   time.Time   `json:"expires_at"`
}

const (
	// PCTemporalMaxTTL is the absolute maximum lifetime for a pc_temporal session (60 min).
	PCTemporalMaxTTL = 60 * time.Minute
	// PCTemporalInactivityTimeout is the maximum idle time for a pc_temporal session (10 min).
	PCTemporalInactivityTimeout = 10 * time.Minute

	// PWASlidingDuration is the sliding expiration for pwa sessions (30 days).
	PWASlidingDuration = 30 * 24 * time.Hour
	// StaffSlidingDuration is the sliding expiration for staff sessions (7 days).
	StaffSlidingDuration = 7 * 24 * time.Hour
)

// IsExpired checks whether a session has expired according to its class rules.
func (s *Session) IsExpired(now time.Time) bool {
	if s == nil {
		return true
	}

	// Check absolute expiration
	if !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt) {
		return true
	}

	// For pc_temporal, check inactivity timeout
	if s.Kind == SessionKindPCTemporal {
		if !s.LastSeenAt.IsZero() && now.Sub(s.LastSeenAt) > PCTemporalInactivityTimeout {
			return true
		}
		if !s.CreatedAt.IsZero() && now.Sub(s.CreatedAt) > PCTemporalMaxTTL {
			return true
		}
	}

	return false
}
