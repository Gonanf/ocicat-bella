package store

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// Implementación MemStore de la gestión escolar §9 (docentes, import CSV,
// auditoría). Turso equivalente en turso_school.go.

func (m *MemStore) UpdateUser(ctx context.Context, u *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.ID]; !ok {
		return ErrNotFound
	}
	uCopy := *u
	m.users[u.ID] = &uCopy
	return nil
}

func (m *MemStore) ListUsersByRole(ctx context.Context, role model.Role) ([]model.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []model.User{}
	for _, u := range m.users {
		if u.Role == role {
			out = append(out, *u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out, nil
}

func (m *MemStore) CreateImportToken(ctx context.Context, tok *model.ImportToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tCopy := *tok
	m.importToks[tok.Token] = &tCopy
	return nil
}

func (m *MemStore) GetImportToken(ctx context.Context, token string) (*model.ImportToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tok, ok := m.importToks[token]
	if !ok {
		return nil, ErrNotFound
	}
	if time.Now().After(tok.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	tCopy := *tok
	return &tCopy, nil
}

func (m *MemStore) CreateAuditEntry(ctx context.Context, e *model.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditLog = append(m.auditLog, *e)
	return nil
}

func (m *MemStore) ListAuditEntries(ctx context.Context, actorID, action string, from, to time.Time) ([]model.AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []model.AuditEntry{}
	for _, e := range m.auditLog {
		if actorID != "" && !strings.EqualFold(e.ActorID, actorID) {
			continue
		}
		if action != "" && e.Action != action {
			continue
		}
		if !from.IsZero() && e.CreatedAt.Before(from) {
			continue
		}
		if !to.IsZero() && e.CreatedAt.After(to) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
