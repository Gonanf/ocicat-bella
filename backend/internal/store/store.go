package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// Common store errors
var (
	ErrNotFound = fmt.Errorf("resource not found")
)

// Store defines the storage operations required by the backend.
// In PR1 this is backed by an in-memory stub; Turso SQL implementation will be added in PR2.
type Store interface {
	// Sessions
	GetSession(ctx context.Context, sessionID string) (*model.Session, *model.User, error)
	CreateSession(ctx context.Context, session *model.Session) error
	TouchSession(ctx context.Context, sessionID string, now time.Time) error
	DeleteSession(ctx context.Context, sessionID string) error

	// Users
	GetUser(ctx context.Context, userID string) (*model.User, error)
	CreateUser(ctx context.Context, user *model.User) error
}

// MemStore is a thread-safe in-memory implementation of Store for development and testing.
type MemStore struct {
	mu       sync.RWMutex
	users    map[string]*model.User
	sessions map[string]*model.Session
}

// NewMemStore creates an initialized MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		users:    make(map[string]*model.User),
		sessions: make(map[string]*model.Session),
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
