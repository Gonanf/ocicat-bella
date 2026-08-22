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

// Store sentinel errors for magic-token consumption (§2.2).
var (
	ErrNotFound     = fmt.Errorf("resource not found")
	ErrTokenExpired = fmt.Errorf("token expired")
	ErrTokenUsed    = fmt.Errorf("token already used")
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

	// Setup wizard (§1): una única escuela por instancia
	IsConfigured(ctx context.Context) (bool, error)
	CreateSchool(ctx context.Context, school *model.School) error
	GetSchool(ctx context.Context) (*model.School, error)
}

// MemStore is a thread-safe in-memory implementation of Store for development and testing.
type MemStore struct {
	mu       sync.RWMutex
	users    map[string]*model.User
	sessions map[string]*model.Session
	tokens   map[string]*model.MagicToken
	school   *model.School
}

// NewMemStore creates an initialized MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		users:    make(map[string]*model.User),
		sessions: make(map[string]*model.Session),
		tokens:   make(map[string]*model.MagicToken),
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
