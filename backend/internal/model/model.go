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
	// SessionKindGuest is the read-only session obtained with the school global
	// code (§10). Duración no especificada en el contrato: reusamos la ventana staff.
	SessionKindGuest SessionKind = "guest"
)

// User represents an authenticated user in the system.
type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email,omitempty"`
	Role         Role   `json:"role"`
	Disabled     bool   `json:"disabled"`
	PasswordHash string `json:"-"` // bcrypt; vacío para alumnos (solo magic link)
}

// School is the single school instance created by the setup wizard (§1).
// GlobalCodeActive=false tras DELETE /school/global-code (§9.1): el código
// queda guardado pero nadie entra como invitado hasta regenerarlo.
type School struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	GlobalCode       string    `json:"global_code"`
	GlobalCodeActive bool      `json:"-"`
	CreatedAt        time.Time `json:"-"`
}

// Visibility de un material (§6): public < school < classroom.
type Visibility string

const (
	VisPublic    Visibility = "public"    // anónimos, invitados y miembros
	VisSchool    Visibility = "school"    // invitados con código global + miembros
	VisClassroom Visibility = "classroom" // solo miembros del aula (+director)
)

// Classroom is one aula (§3). JoinCode es el código de invitación rotable.
type Classroom struct {
	ID        string    `json:"id"`
	TeacherID string    `json:"teacher_id"`
	Name      string    `json:"name"`
	Course    string    `json:"course,omitempty"`
	Shift     string    `json:"shift,omitempty"`
	JoinCode  string    `json:"join_code"`
	Archived  bool      `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type MembershipStatus string

const (
	MemberInvited MembershipStatus = "invited"
	MemberActive  MembershipStatus = "active"
)

// Membership vincula un alumno a un aula. PK compuesta (ClassroomID, UserID).
type Membership struct {
	ClassroomID string           `json:"classroom_id"`
	UserID      string           `json:"user_id"`
	Status      MembershipStatus `json:"status"`
	CreatedAt   time.Time        `json:"-"`
}

// Material is a library file (§6); contenido en Data ([]byte) — ponytail:
// bytes en store, disco bajo data dir si el volumen lo pide.
type Material struct {
	ID          string
	ClassroomID string // aula donde fue creado (§13.9.2)
	UploadedBy  string
	Title       string
	Filename    string
	ContentType string
	Size        int64
	Visibility  Visibility
	Subject     string
	Data        []byte
	CreatedAt   time.Time
}

// StudentInfo es la fila de GET /classrooms/{id}/students (§3).
type StudentInfo struct {
	UserID           string           `json:"user_id"`
	Name             string           `json:"name"`
	Email            string           `json:"email"`
	Status           MembershipStatus `json:"status"`
	TotalSubmissions int              `json:"total_submissions"` // 0 hasta Fase entregas
}

// SchoolStats alimenta GET /school (§9.1).
type SchoolStats struct {
	Teachers        int `json:"teachers"`
	Classrooms      int `json:"classrooms"`
	Students        int `json:"students"`
	PublicMaterials int `json:"public_materials"`
}

// MagicContext is the request context of a magic link (§2.2).
type MagicContext string

const (
	// MagicContextPairingPWA pairs a student phone; única vía a sesión pwa persistente ([C2]).
	MagicContextPairingPWA MagicContext = "pairing_pwa"
	// MagicContextLoginPC abre una sesión pc_temporal en la PC que consume.
	MagicContextLoginPC MagicContext = "login_pc"
)

// MagicToken is a single-use short-lived token delivered by email (§2.2).
type MagicToken struct {
	Token     string
	UserID    string
	Context   MagicContext
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    time.Time // zero = sin consumir
}

const (
	// MagicLinkTTL is the maximum lifetime of a magic link token (≤10 min, §2.2).
	MagicLinkTTL = 10 * time.Minute
	// MaxPWADevices is the device pairing limit per student ([C5], ~3 dispositivos).
	MaxPWADevices = 3
)

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

// PairingStatus de un pairing QR inverso (§2.3). "expired" nunca se persiste:
// EffectiveStatus lo deriva de ExpiresAt; confirmed/denied son terminales.
type PairingStatus string

const (
	PairingWaiting   PairingStatus = "waiting"
	PairingScanned   PairingStatus = "scanned"
	PairingConfirmed PairingStatus = "confirmed"
	PairingDenied    PairingStatus = "denied"
	PairingExpired   PairingStatus = "expired"
)

const (
	// QRTTL is the pairing session lifetime ([C3]: ≤90s single-use).
	QRTTL = 90 * time.Second
	// QRRefreshAfter es la regeneración sugerida para la PC (segundos, §2.3).
	QRRefreshAfter = 60
)

// PairingSession is one inverse-QR login attempt on a shared PC (CU-16).
type PairingSession struct {
	PairingID   string // opaco, para poll de la PC; independiente del QRToken
	QRToken     string // ≥128 bits base64url; lo que codifica el QR
	UserID      string // cuenta del celular tras /scan; vacío mientras waiting
	DeviceLabel string // autodeclarado por la PC ([C3]: decorativo)
	OriginIP    string // IP de quien creó el QR; se muestra en el celular [C3]
	Status      PairingStatus
	CreatedAt   time.Time
	ExpiresAt   time.Time
	SessionID   string // sesión pc_temporal emitida al confirmar
}

// EffectiveStatus resuelve el estado visible: los terminales son definitivos;
// vencido gana sobre waiting/scanned.
func (p *PairingSession) EffectiveStatus(now time.Time) PairingStatus {
	switch p.Status {
	case PairingConfirmed, PairingDenied:
		return p.Status
	}
	if now.After(p.ExpiresAt) {
		return PairingExpired
	}
	return p.Status
}
