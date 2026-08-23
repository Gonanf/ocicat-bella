package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// ErrDuplicate señala colisión de código/campo único; el handler reintenta
// generando otro código.
var ErrDuplicate = fmt.Errorf("duplicate unique field")

func memberKey(classroomID, userID string) string {
	return classroomID + "\x00" + userID
}

func copyClassroom(c *model.Classroom) *model.Classroom { x := *c; return &x }

// --- Classrooms (§3) ---

func (m *MemStore) CreateClassroom(ctx context.Context, c *model.Classroom) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.classrooms {
		if strings.EqualFold(existing.JoinCode, c.JoinCode) {
			return ErrDuplicate
		}
	}
	m.classrooms[c.ID] = copyClassroom(c)
	return nil
}

func (m *MemStore) GetClassroom(ctx context.Context, id string) (*model.Classroom, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.classrooms[id]
	if !ok {
		return nil, ErrNotFound
	}
	return copyClassroom(c), nil
}

func (m *MemStore) GetClassroomByCode(ctx context.Context, code string) (*model.Classroom, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, c := range m.classrooms {
		if !c.Archived && strings.EqualFold(c.JoinCode, code) {
			return copyClassroom(c), nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemStore) UpdateClassroom(ctx context.Context, c *model.Classroom) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.classrooms[c.ID]
	if !ok {
		return ErrNotFound
	}
	for id, other := range m.classrooms {
		if id != c.ID && strings.EqualFold(other.JoinCode, c.JoinCode) {
			return ErrDuplicate
		}
	}
	cCopy := *c
	*existing = cCopy
	return nil
}

func (m *MemStore) ListClassrooms(ctx context.Context) ([]model.Classroom, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []model.Classroom
	for _, c := range m.classrooms {
		if !c.Archived {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// --- Memberships ---

func (m *MemStore) AddMember(ctx context.Context, mem *model.Membership) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := memberKey(mem.ClassroomID, mem.UserID)
	if _, exists := m.memberships[key]; exists {
		return ErrDuplicate
	}
	cp := *mem
	m.memberships[key] = &cp
	return nil
}

func (m *MemStore) GetMember(ctx context.Context, classroomID, userID string) (*model.Membership, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mem, ok := m.memberships[memberKey(classroomID, userID)]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *mem
	return &cp, nil
}

func (m *MemStore) RemoveMember(ctx context.Context, classroomID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.memberships, memberKey(classroomID, userID))
	return nil
}

func (m *MemStore) ListMembershipsByUser(ctx context.Context, userID string) ([]model.Membership, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []model.Membership
	for _, mem := range m.memberships {
		if mem.UserID == userID {
			out = append(out, *mem)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemStore) ListClassroomStudents(ctx context.Context, classroomID string) ([]model.StudentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []model.StudentInfo
	for _, mem := range m.memberships {
		if mem.ClassroomID != classroomID {
			continue
		}
		u, ok := m.users[mem.UserID]
		if !ok {
			continue
		}
		out = append(out, model.StudentInfo{
			UserID: u.ID,
			Name:   u.Name,
			Email:  u.Email,
			Status: mem.Status,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// --- Materials (§6): ponytail: contenido como []byte en memoria/DB base64;
// pasar a disco bajo data dir si los archivos crecen. ---

func copyMaterial(mt *model.Material) *model.Material {
	x := *mt
	x.Data = append([]byte(nil), mt.Data...)
	return &x
}

func (m *MemStore) CreateMaterial(ctx context.Context, mt *model.Material) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.materials[mt.ID] = copyMaterial(mt)
	return nil
}

func (m *MemStore) GetMaterial(ctx context.Context, id string) (*model.Material, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mt, ok := m.materials[id]
	if !ok {
		return nil, ErrNotFound
	}
	return copyMaterial(mt), nil
}

func (m *MemStore) UpdateMaterial(ctx context.Context, mt *model.Material) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.materials[mt.ID]
	if !ok {
		return ErrNotFound
	}
	*existing = *copyMaterial(mt)
	return nil
}

func (m *MemStore) DeleteMaterial(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.materials, id)
	return nil
}

func (m *MemStore) ListMaterials(ctx context.Context) ([]model.Material, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []model.Material
	for _, mt := range m.materials {
		out = append(out, *copyMaterial(mt))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// --- Escuela: código global (§9.1), stats y revocación de invitados (§10) ---

func (m *MemStore) SetSchoolGlobalCode(ctx context.Context, code string, active bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.school == nil {
		return ErrNotFound
	}
	if active && code != "" {
		m.school.GlobalCode = code
	}
	m.school.GlobalCodeActive = active
	return nil
}

func (m *MemStore) SchoolStats(ctx context.Context) (*model.SchoolStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &model.SchoolStats{}
	for _, u := range m.users {
		switch u.Role {
		case model.RoleDocente:
			stats.Teachers++
		case model.RoleAlumno:
			stats.Students++
		}
	}
	for _, c := range m.classrooms {
		if !c.Archived {
			stats.Classrooms++
		}
	}
	for _, mt := range m.materials {
		if mt.Visibility == model.VisPublic {
			stats.PublicMaterials++
		}
	}
	return stats, nil
}

// RevokeGuestAccess borra sesiones de invitados y sus usuarios efímeros:
// regenerar/desactivar el código global deja afuera a quienes entraron con él.
func (m *MemStore) RevokeGuestAccess(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	guests := map[string]bool{}
	for id, u := range m.users {
		if u.Role == model.RoleInvitado {
			guests[id] = true
		}
	}
	for sid, s := range m.sessions {
		if guests[s.UserID] {
			delete(m.sessions, sid)
		}
	}
	for id := range guests {
		delete(m.users, id)
	}
	return nil
}
