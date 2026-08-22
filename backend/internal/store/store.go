package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// Store sentinel errors for magic-token consumption (§2.2) and QR pairing claims (§2.3).
var (
	ErrNotFound       = fmt.Errorf("resource not found")
	ErrTokenExpired   = fmt.Errorf("token expired")
	ErrTokenUsed      = fmt.Errorf("token already used")
	ErrAlreadyClaimed = fmt.Errorf("pairing already claimed")
)

// Store defines the storage operations required by the backend.
// Backed by MemStore (tests/dev) or TursoStore (Turso HTTP API v2, see turso.go).
type Store interface {
	// Sessions
	GetSession(ctx context.Context, sessionID string) (*model.Session, *model.User, error)
	CreateSession(ctx context.Context, session *model.Session) error
	TouchSession(ctx context.Context, sessionID string, now time.Time) error
	DeleteSession(ctx context.Context, sessionID string) error
	ListSessionsByUser(ctx context.Context, userID string) ([]model.Session, error)

	// Users
	GetUser(ctx context.Context, userID string) (*model.User, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	CreateUser(ctx context.Context, user *model.User) error

	// Magic link tokens (single-use atómico: consume marca used_at y valida TTL en una operación)
	CreateMagicToken(ctx context.Context, token *model.MagicToken) error
	ConsumeMagicToken(ctx context.Context, token string, now time.Time) (*model.MagicToken, error)

	// Pairing sessions QR inverso (§2.3): transiciones atómicas single-use.
	// Scan/Claim/Deny devuelven ErrNotFound | ErrTokenExpired | ErrAlreadyClaimed
	// cuando la transición no aplica al estado actual.
	CreatePairingSession(ctx context.Context, p *model.PairingSession) error
	GetPairingSession(ctx context.Context, pairingID string) (*model.PairingSession, error)
	GetPairingSessionByToken(ctx context.Context, qrToken string) (*model.PairingSession, error)
	ScanPairingSession(ctx context.Context, pairingID, userID string, now time.Time) error
	// ClaimPairingSession marca scanned→confirmed exactamente una vez y crea la
	// sesión pc_temporal del alumno en la misma operación; claims concurrentes:
	// uno gana, el resto ErrAlreadyClaimed.
	ClaimPairingSession(ctx context.Context, pairingID, userID string, sess *model.Session, now time.Time) error
	DenyPairingSession(ctx context.Context, pairingID, userID string, now time.Time) error

	// Setup wizard (§1): una única escuela por instancia
	IsConfigured(ctx context.Context) (bool, error)
	CreateSchool(ctx context.Context, school *model.School) error
	GetSchool(ctx context.Context) (*model.School, error)
	SetSchoolGlobalCode(ctx context.Context, code string, active bool) error

	// Classrooms y membresías (§3)
	CreateClassroom(ctx context.Context, c *model.Classroom) error
	GetClassroom(ctx context.Context, id string) (*model.Classroom, error)
	GetClassroomByCode(ctx context.Context, code string) (*model.Classroom, error)
	UpdateClassroom(ctx context.Context, c *model.Classroom) error
	ListClassrooms(ctx context.Context) ([]model.Classroom, error)

	AddMember(ctx context.Context, m *model.Membership) error
	GetMember(ctx context.Context, classroomID, userID string) (*model.Membership, error)
	RemoveMember(ctx context.Context, classroomID, userID string) error
	ListMembershipsByUser(ctx context.Context, userID string) ([]model.Membership, error)
	ListClassroomStudents(ctx context.Context, classroomID string) ([]model.StudentInfo, error)

	// Materials (§6): contenido en Material.Data
	CreateMaterial(ctx context.Context, m *model.Material) error
	GetMaterial(ctx context.Context, id string) (*model.Material, error)
	UpdateMaterial(ctx context.Context, m *model.Material) error
	DeleteMaterial(ctx context.Context, id string) error
	ListMaterials(ctx context.Context) ([]model.Material, error)

	// Consignas (§4): Deleted=true tras soft-delete; Get devuelve aunque esté borrada.
	CreateAssignment(ctx context.Context, a *model.Assignment) error
	GetAssignment(ctx context.Context, id string) (*model.Assignment, error)
	UpdateAssignment(ctx context.Context, a *model.Assignment) error
	ListAssignments(ctx context.Context) ([]model.Assignment, error)

	// Adjuntos de consigna (§4): sin AssignmentID son huérfanos purgables por el GC.
	CreateAttachment(ctx context.Context, at *model.Attachment) error
	GetAttachment(ctx context.Context, id string) (*model.Attachment, error)
	DeleteAttachment(ctx context.Context, id string) error
	ListAttachments(ctx context.Context) ([]model.Attachment, error)

	// Entregas (§5): submission = UN intento; UpdateSubmission persiste también
	// la transición draft→delivered con su snapshot ya copiado.
	CreateSubmission(ctx context.Context, s *model.Submission) error
	GetSubmission(ctx context.Context, id string) (*model.Submission, error)
	UpdateSubmission(ctx context.Context, s *model.Submission) error
	ListSubmissionsByAssignment(ctx context.Context, assignmentID string) ([]model.Submission, error)

	// Escuela (§9.1/§10)
	SchoolStats(ctx context.Context) (*model.SchoolStats, error)
	RevokeGuestAccess(ctx context.Context) error
}

// MemStore is a thread-safe in-memory implementation of Store for development and testing.
type MemStore struct {
	mu          sync.RWMutex
	users       map[string]*model.User
	sessions    map[string]*model.Session
	tokens      map[string]*model.MagicToken
	pairings    map[string]*model.PairingSession // clave: PairingID
	school      *model.School
	classrooms  map[string]*model.Classroom
	memberships map[string]*model.Membership // clave: memberKey(classroomID, userID)
	materials   map[string]*model.Material
	assignments map[string]*model.Assignment
	attachments map[string]*model.Attachment
	submissions map[string]*model.Submission
}

// NewMemStore creates an initialized MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		users:       make(map[string]*model.User),
		sessions:    make(map[string]*model.Session),
		tokens:      make(map[string]*model.MagicToken),
		pairings:    make(map[string]*model.PairingSession),
		classrooms:  make(map[string]*model.Classroom),
		memberships: make(map[string]*model.Membership),
		materials:   make(map[string]*model.Material),
		assignments: make(map[string]*model.Assignment),
		attachments: make(map[string]*model.Attachment),
		submissions: make(map[string]*model.Submission),
	}
}

// GetSession retrieves a session and its associated user.
func (m *MemStore) GetSession(ctx context.Context, sessionID string) (*model.Session, *model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil, ErrNotFound
	}

	user, ok := m.users[session.UserID]
	if !ok {
		return nil, nil, ErrNotFound
	}

	// Return copies to prevent race conditions on mutation
	sCopy := *session
	uCopy := *user
	return &sCopy, &uCopy, nil
}

// CreateSession stores a new session.
func (m *MemStore) CreateSession(ctx context.Context, session *model.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sCopy := *session
	m.sessions[session.ID] = &sCopy
	return nil
}

// TouchSession updates the last seen timestamp of a session.
func (m *MemStore) TouchSession(ctx context.Context, sessionID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return ErrNotFound
	}

	session.LastSeenAt = now
	return nil
}

// DeleteSession removes a session.
func (m *MemStore) DeleteSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
	return nil
}

// GetUser retrieves a user by ID.
func (m *MemStore) GetUser(ctx context.Context, userID string) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.users[userID]
	if !ok {
		return nil, ErrNotFound
	}

	uCopy := *user
	return &uCopy, nil
}

// CreateUser stores a new user.
func (m *MemStore) CreateUser(ctx context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	uCopy := *user
	m.users[user.ID] = &uCopy
	return nil
}

// GetUserByEmail retrieves a user by email address.
func (m *MemStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			uCopy := *u
			return &uCopy, nil
		}
	}
	return nil, ErrNotFound
}

// ListSessionsByUser returns all sessions of a user, newest first.
func (m *MemStore) ListSessionsByUser(ctx context.Context, userID string) ([]model.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []model.Session
	for _, s := range m.sessions {
		if s.UserID == userID {
			out = append(out, *s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// CreateMagicToken stores a magic link token.
func (m *MemStore) CreateMagicToken(ctx context.Context, token *model.MagicToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tCopy := *token
	m.tokens[token.Token] = &tCopy
	return nil
}

// ConsumeMagicToken atomically validates TTL + single-use and marks the token used.
func (m *MemStore) ConsumeMagicToken(ctx context.Context, token string, now time.Time) (*model.MagicToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.tokens[token]
	if !ok {
		return nil, ErrNotFound
	}
	if !t.UsedAt.IsZero() {
		return nil, ErrTokenUsed
	}
	if now.After(t.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	t.UsedAt = now

	tCopy := *t
	return &tCopy, nil
}

// IsConfigured reports whether the setup wizard has completed (a director exists).
func (m *MemStore) IsConfigured(ctx context.Context) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, u := range m.users {
		if u.Role == model.RoleDirector {
			return true, nil
		}
	}
	return false, nil
}

// CreateSchool stores the school instance; errors if one already exists.
func (m *MemStore) CreateSchool(ctx context.Context, school *model.School) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.school != nil {
		return fmt.Errorf("school already exists")
	}
	sCopy := *school
	m.school = &sCopy
	return nil
}

// GetSchool returns the school instance.
func (m *MemStore) GetSchool(ctx context.Context) (*model.School, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.school == nil {
		return nil, ErrNotFound
	}
	sCopy := *m.school
	return &sCopy, nil
}

// --- Pairing sessions QR inverso (§2.3) ---

func copyPairing(p *model.PairingSession) *model.PairingSession {
	c := *p
	return &c
}

func (m *MemStore) CreatePairingSession(ctx context.Context, p *model.PairingSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pairings[p.PairingID] = copyPairing(p)
	return nil
}

func (m *MemStore) GetPairingSession(ctx context.Context, pairingID string) (*model.PairingSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.pairings[pairingID]
	if !ok {
		return nil, ErrNotFound
	}
	return copyPairing(p), nil
}

func (m *MemStore) GetPairingSessionByToken(ctx context.Context, qrToken string) (*model.PairingSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.pairings {
		if p.QRToken == qrToken {
			return copyPairing(p), nil
		}
	}
	return nil, ErrNotFound
}

// transition aplica una transición condicional bajo lock y clasifica el fallo.
func (m *MemStore) transition(p *model.PairingSession, from model.PairingStatus, userID string, now time.Time) error {
	if now.After(p.ExpiresAt) {
		return ErrTokenExpired
	}
	if p.Status != from || (userID != "" && p.UserID != userID) {
		return ErrAlreadyClaimed
	}
	return nil
}

func (m *MemStore) ScanPairingSession(ctx context.Context, pairingID, userID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pairings[pairingID]
	if !ok {
		return ErrNotFound
	}
	if err := m.transition(p, model.PairingWaiting, "", now); err != nil {
		return err
	}
	p.Status = model.PairingScanned
	p.UserID = userID
	return nil
}

func (m *MemStore) ClaimPairingSession(ctx context.Context, pairingID, userID string, sess *model.Session, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pairings[pairingID]
	if !ok {
		return ErrNotFound
	}
	if err := m.transition(p, model.PairingScanned, userID, now); err != nil {
		return err
	}
	p.Status = model.PairingConfirmed
	sCopy := *sess
	m.sessions[sess.ID] = &sCopy
	p.SessionID = sess.ID
	return nil
}

func (m *MemStore) DenyPairingSession(ctx context.Context, pairingID, userID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pairings[pairingID]
	if !ok {
		return ErrNotFound
	}
	if err := m.transition(p, model.PairingScanned, userID, now); err != nil {
		return err
	}
	p.Status = model.PairingDenied
	return nil
}
