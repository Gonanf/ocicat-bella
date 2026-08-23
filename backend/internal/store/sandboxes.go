package store

import (
	"context"
	"sort"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// MemStore de sandboxes y runs (§7).

func copySandbox(sb *model.Sandbox) *model.Sandbox { x := *sb; return &x }

func copyRun(r *model.Run) *model.Run {
	x := *r
	x.History = append([]model.RunHistoryEntry(nil), r.History...)
	x.Logs = append([]model.LogLine(nil), r.Logs...)
	if r.ExitCode != nil {
		ec := *r.ExitCode
		x.ExitCode = &ec
	}
	return &x
}

func (m *MemStore) CreateSandbox(ctx context.Context, sb *model.Sandbox) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sandboxes[sb.ID] = copySandbox(sb)
	return nil
}

func (m *MemStore) GetSandbox(ctx context.Context, id string) (*model.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sb, ok := m.sandboxes[id]
	if !ok {
		return nil, ErrNotFound
	}
	return copySandbox(sb), nil
}

func (m *MemStore) ListSandboxes(ctx context.Context) ([]model.Sandbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]model.Sandbox, 0, len(m.sandboxes))
	for _, sb := range m.sandboxes {
		out = append(out, *copySandbox(sb))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemStore) UpdateSandbox(ctx context.Context, sb *model.Sandbox) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.sandboxes[sb.ID]
	if !ok {
		return ErrNotFound
	}
	*existing = *copySandbox(sb)
	return nil
}

func (m *MemStore) CreateRun(ctx context.Context, r *model.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[r.ID] = copyRun(r)
	return nil
}

func (m *MemStore) GetRun(ctx context.Context, id string) (*model.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return copyRun(r), nil
}

func (m *MemStore) UpdateRun(ctx context.Context, r *model.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.runs[r.ID]
	if !ok {
		return ErrNotFound
	}
	*existing = *copyRun(r)
	return nil
}

func (m *MemStore) ListRunsBySandbox(ctx context.Context, sandboxID string) ([]model.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []model.Run
	for _, r := range m.runs {
		if r.SandboxID == sandboxID {
			out = append(out, *copyRun(r))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N < out[j].N
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (m *MemStore) ListRuns(ctx context.Context) ([]model.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]model.Run, 0, len(m.runs))
	for _, r := range m.runs {
		out = append(out, *copyRun(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
