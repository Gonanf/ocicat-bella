package store

import (
	"context"
	"sort"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// MemStore de consignas (§4), adjuntos y entregas (§5).
// ponytail: contenido como []byte en memoria (base64 en Turso); disco bajo
// data dir si los archivos crecen, igual que Material.

func copyAssignment(a *model.Assignment) *model.Assignment {
	x := *a
	x.AttachmentIDs = append([]string(nil), a.AttachmentIDs...)
	return &x
}

func copyFiles(fs []model.SubmissionFile) []model.SubmissionFile {
	out := make([]model.SubmissionFile, len(fs))
	for i := range fs {
		out[i] = fs[i]
		out[i].Data = append([]byte(nil), fs[i].Data...)
	}
	return out
}

func copySubmission(s *model.Submission) *model.Submission {
	x := *s
	x.Files = copyFiles(s.Files)
	x.SnapshotFiles = copyFiles(s.SnapshotFiles)
	if s.LastTestResult != nil {
		tr := *s.LastTestResult
		x.LastTestResult = &tr
	}
	return &x
}

// --- Assignments (§4) ---

func (m *MemStore) CreateAssignment(ctx context.Context, a *model.Assignment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assignments[a.ID] = copyAssignment(a)
	return nil
}

func (m *MemStore) GetAssignment(ctx context.Context, id string) (*model.Assignment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.assignments[id]
	if !ok {
		return nil, ErrNotFound
	}
	return copyAssignment(a), nil
}

func (m *MemStore) UpdateAssignment(ctx context.Context, a *model.Assignment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.assignments[a.ID]
	if !ok {
		return ErrNotFound
	}
	*existing = *copyAssignment(a)
	return nil
}

func (m *MemStore) ListAssignments(ctx context.Context) ([]model.Assignment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []model.Assignment
	for _, a := range m.assignments {
		out = append(out, *copyAssignment(a))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// --- Attachments (§4) ---

func copyAttachment(at *model.Attachment) *model.Attachment {
	x := *at
	x.Data = append([]byte(nil), at.Data...)
	return &x
}

func (m *MemStore) CreateAttachment(ctx context.Context, at *model.Attachment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attachments[at.ID] = copyAttachment(at)
	return nil
}

func (m *MemStore) GetAttachment(ctx context.Context, id string) (*model.Attachment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	at, ok := m.attachments[id]
	if !ok {
		return nil, ErrNotFound
	}
	return copyAttachment(at), nil
}

func (m *MemStore) DeleteAttachment(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.attachments, id)
	return nil
}

func (m *MemStore) ListAttachments(ctx context.Context) ([]model.Attachment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []model.Attachment
	for _, at := range m.attachments {
		out = append(out, *copyAttachment(at))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// --- Submissions (§5): cada submission es UN intento; deliver congela. ---

func (m *MemStore) CreateSubmission(ctx context.Context, s *model.Submission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submissions[s.ID] = copySubmission(s)
	return nil
}

func (m *MemStore) GetSubmission(ctx context.Context, id string) (*model.Submission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.submissions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return copySubmission(s), nil
}

func (m *MemStore) UpdateSubmission(ctx context.Context, s *model.Submission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.submissions[s.ID]
	if !ok {
		return ErrNotFound
	}
	*existing = *copySubmission(s)
	return nil
}

func (m *MemStore) ListSubmissionsByAssignment(ctx context.Context, assignmentID string) ([]model.Submission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []model.Submission
	for _, s := range m.submissions {
		if s.AssignmentID == assignmentID {
			out = append(out, *copySubmission(s))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AttemptNumber != out[j].AttemptNumber {
			return out[i].AttemptNumber < out[j].AttemptNumber
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}
