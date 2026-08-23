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
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email,omitempty"`
	Role         Role      `json:"role"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"created_at,omitempty"` // §9.2 lista de docentes
	PasswordHash string    `json:"-"`                    // bcrypt; vacío para alumnos (solo magic link)
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
// Settings de sandbox (§10.2/G4): AllowedTemplates vacío = catálogo completo
// (default); CustomDockerfileEnabled OFF por defecto.
type Classroom struct {
	ID        string    `json:"id"`
	TeacherID string    `json:"teacher_id"`
	Name      string    `json:"name"`
	Course    string    `json:"course,omitempty"`
	Shift     string    `json:"shift,omitempty"`
	JoinCode  string    `json:"join_code"`
	Archived  bool      `json:"-"`
	CreatedAt time.Time `json:"created_at"`

	AllowedTemplates       []string `json:"-"`
	CustomDockerfileEnabled bool     `json:"-"`
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

// AssignmentStats alimenta GET /assignments/{id}/stats y las métricas del
// listado docente (§4): {total_students, delivered, tested_ok, test_errors,
// late, missing} sobre la ÚLTIMA entrega de cada alumno.
type AssignmentStats struct {
	TotalStudents int `json:"total_students"`
	Delivered     int `json:"delivered"`
	TestedOK      int `json:"tested_ok"`
	TestErrors    int `json:"test_errors"`
	Late          int `json:"late"`
	Missing       int `json:"missing"`
}

// SchoolStats alimenta GET /school (§9.1).
type SchoolStats struct {
	Teachers        int `json:"teachers"`
	Classrooms      int `json:"classrooms"`
	Students        int `json:"students"`
	PublicMaterials int `json:"public_materials"`
}

// ImportToken acredita el paso 1 (preview dry_run) del import CSV §9.3;
// el commit exige un token vigente del mismo aula. TTL corto.
type ImportToken struct {
	Token       string
	ClassroomID string
	CreatedBy   string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// ImportTTL ventana para pasar de preview a commit (§9.3 dos pasos).
const ImportTTL = 30 * time.Minute

// MaxImportRows es el límite de filas por import CSV (§9.3).
const MaxImportRows = 200

// AuditEntry es una acción sensible inmutable (§9.5): impersonaciones,
// rotaciones de códigos, altas/bajas y cambios de credenciales.
type AuditEntry struct {
	ID           string    `json:"id"`
	ActorID      string    `json:"actor_id"`
	Action       string    `json:"action"`
	TargetUserID string    `json:"target_user_id,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	IP           string    `json:"ip,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
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
	// ImpersonatedBy ≠ 0: sesión de impersonación administrativa (§9.5);
	// el usuario que actúa es UserID pero el banner/audit muestran el director.
	ImpersonatedBy string `json:"impersonated_by,omitempty"`
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

// --- Consignas y entregas (§4, §5) ---

// Runtime es el entorno de ejecución de la consigna (catálogo §10.2).
type Runtime string

const (
	RuntimePython  Runtime = "python"
	RuntimeWeb     Runtime = "web"
	RuntimeCpp     Runtime = "cpp"
	RuntimeArduino Runtime = "arduino"
)

func ValidRuntime(rt Runtime) bool {
	return rt == RuntimePython || rt == RuntimeWeb || rt == RuntimeCpp || rt == RuntimeArduino
}

// AttemptsMode: unlimited | limited{max} | one (§4).
type AttemptsMode string

const (
	AttemptsUnlimited AttemptsMode = "unlimited"
	AttemptsLimited   AttemptsMode = "limited"
	AttemptsOne       AttemptsMode = "one"
)

// AttemptsConfig guarda la config vigente; Max aplica solo a limited.
type AttemptsConfig struct {
	Mode AttemptsMode `json:"mode"`
	Max  int          `json:"max,omitempty"` // 0 salvo limited
}

// MaxAttempts resuelve el tope efectivo: unlimited → sin tope (-1).
func (a AttemptsConfig) MaxAttempts() int {
	switch a.Mode {
	case AttemptsOne:
		return 1
	case AttemptsLimited:
		return a.Max
	default:
		return -1
	}
}

// LatePolicy: allowed (tardías marcadas) | closed (cierra al vencer) (§4).
type LatePolicy string

const (
	LateAllowed LatePolicy = "allowed"
	LateClosed  LatePolicy = "closed"
)

// Assignment is one consigna (§4). Vive dentro del aula; publicación inmediata.
// Deleted=true tras DELETE (soft-delete §4): entregas conservadas.
type Assignment struct {
	ID            string
	ClassroomID   string
	CreatedBy     string
	Title         string
	Instructions  string
	AttachmentIDs []string
	Runtime       Runtime
	DueAt         time.Time // zero = sin vencimiento
	Attempts      AttemptsConfig
	LatePolicy    LatePolicy
	Deleted       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Attachment is one file adjunto a una consigna (§4). Sin AssignmentID es
// huérfano: el GC lo purga a las 24 h.
type Attachment struct {
	ID           string
	Filename     string
	ContentType  string
	Size         int64
	Data         []byte
	UploadedBy   string
	AssignmentID string // vacío hasta que el create de assignment lo reclame
	CreatedAt    time.Time
}

// SubmissionState: draft → delivered (+flags derivados late/tested_ok/test_error, §5).
type SubmissionState string

const (
	SubDraft     SubmissionState = "draft"
	SubDelivered SubmissionState = "delivered"
)

// SubmissionFile is one file dentro de una submission; Data en store como Material.
type SubmissionFile struct {
	ID   string
	Name string
	Size int64
	Data []byte
}

// TestResult queda null hasta FASE 6 (sandboxes); campo y modelo listos (§5.2).
type TestResult struct {
	RunID    string `json:"run_id"`
	ExitCode int    `json:"exit_code"`
}

// Submission = UN intento (§5). Al entregar se congela: Files se copia a
// SnapshotFiles y nada vuelve a mutarla (reintento = nueva draft).
type Submission struct {
	ID             string
	AssignmentID   string
	StudentID      string
	AttemptNumber  int
	State          SubmissionState
	Files          []SubmissionFile // vivos mientras draft
	SnapshotFiles  []SubmissionFile // copia inmutable al deliver
	LastTestResult *TestResult      // populado si hubo run submission_test terminado (FASE 6)
	DeliveredAt    time.Time
	Late           bool
	TestedOK       bool // exit_code == 0
	TestError      bool // exit_code != 0
	CreatedAt      time.Time
}

// --- Sandboxes y runs (§7) ---

type SandboxMode string

const (
	ModeJob     SandboxMode = "job"
	ModeService SandboxMode = "service"
)

type SandboxPurpose string

const (
	PurposeStandalone      SandboxPurpose = "standalone"
	PurposeSubmissionTest  SandboxPurpose = "submission_test"
)

type SandboxRetention string

const (
	RetentionHistorical SandboxRetention = "historical" // registro re-accedible/re-instanciable
	RetentionEphemeral  SandboxRetention = "ephemeral"  // 404 tras limpieza
)

// RunStatus une la máquina de estados §2.3/§13.2.
type RunStatus string

const (
	RunQueued           RunStatus = "queued"
	RunDownloadingImage RunStatus = "downloading_image"
	RunStarting         RunStatus = "starting"
	RunReady            RunStatus = "ready"
	RunRunning          RunStatus = "running"
	RunSucceeded        RunStatus = "succeeded"
	RunFailed           RunStatus = "failed"
	RunCleaned          RunStatus = "cleaned"
)

// Terminal reporta si el run ya terminó (invitados solo ven estos).
func (s RunStatus) Terminal() bool {
	return s == RunSucceeded || s == RunFailed || s == RunCleaned
}

// ActiveContainer: el contenedor está prendido (cuenta contra presupuesto §0.3).
func (s RunStatus) ActiveContainer() bool {
	return s == RunDownloadingImage || s == RunStarting || s == RunReady || s == RunRunning
}

type LogLine struct {
	Stream string `json:"stream"` // stdout | stderr
	Line   string `json:"line"`
}

// Sandbox es el registro de un entorno (§7); los runs son sus ejecuciones #N.
// Cleaned marca un ephemeral ya limpiado → handlers responden 404.
type Sandbox struct {
	ID            string
	TemplateID    string
	CreatedBy     string
	ClassroomID   string // aula de contexto (submission_test o primera membresía)
	Mode          SandboxMode
	StartCommand  string
	PackagesExtra string
	Purpose       SandboxPurpose
	SubmissionID  string
	Retention     SandboxRetention
	Visibility    Visibility // public/school habilitan lectura de invitados/anónimos (§7)
	Cleaned       bool       // ephemeral limpiado
	CreatedAt     time.Time
}

type RunHistoryEntry struct {
	N       int    `json:"n"`
	Outcome string `json:"outcome"` // success | error | timeout
}

type Run struct {
	ID         string
	SandboxID  string
	N          int // ejecución #N del sandbox
	Status     RunStatus
	ExitCode   *int
	ServiceURL string // modo service: /s/{sandbox_id} al llegar a running (Q5 abierta)
	StartedAt  time.Time // cero mientras queued
	History    []RunHistoryEntry
	Logs       []LogLine
	CreatedAt  time.Time
}
