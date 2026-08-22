package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// Implementación Turso de las operaciones de aulas (§3), materiales (§6)
// y código global de escuela (§9.1/§10).

func cellInt(row []sqlVal, i int) int {
	s, isNull := cell(row, i)
	if isNull {
		return 0
	}
	n := 0
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

// --- Classrooms ---

const classroomCols = "id, teacher_id, name, course, shift, join_code, archived, created_at"

func scanClassroom(row []sqlVal) (*model.Classroom, error) {
	createdAt, err := cellTime(row, 7)
	if err != nil {
		return nil, err
	}
	return &model.Classroom{
		ID:        cellStr(row, 0),
		TeacherID: cellStr(row, 1),
		Name:      cellStr(row, 2),
		Course:    cellStr(row, 3),
		Shift:     cellStr(row, 4),
		JoinCode:  cellStr(row, 5),
		Archived:  cellInt(row, 6) == 1,
		CreatedAt: createdAt,
	}, nil
}

func (t *TursoStore) CreateClassroom(ctx context.Context, c *model.Classroom) error {
	_, err := t.exec(ctx,
		`INSERT INTO classrooms (`+classroomCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		textArg(c.ID), textArg(c.TeacherID), textArg(c.Name), textArg(c.Course), textArg(c.Shift),
		textArg(c.JoinCode), intArg(c.Archived), timeArg(c.CreatedAt))
	return err
}

func (t *TursoStore) getClassroom(ctx context.Context, where string, arg sqlVal) (*model.Classroom, error) {
	rows, err := t.query(ctx, `SELECT `+classroomCols+` FROM classrooms WHERE `+where, arg)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanClassroom(rows[0])
}

func (t *TursoStore) GetClassroom(ctx context.Context, id string) (*model.Classroom, error) {
	return t.getClassroom(ctx, `id = ?`, textArg(id))
}

func (t *TursoStore) GetClassroomByCode(ctx context.Context, code string) (*model.Classroom, error) {
	return t.getClassroom(ctx, `join_code = ? AND archived = 0`, textArg(strings.ToUpper(code)))
}

func (t *TursoStore) UpdateClassroom(ctx context.Context, c *model.Classroom) error {
	n, err := t.exec(ctx,
		`UPDATE classrooms SET teacher_id = ?, name = ?, course = ?, shift = ?, join_code = ?, archived = ?
		 WHERE id = ?`,
		textArg(c.TeacherID), textArg(c.Name), textArg(c.Course), textArg(c.Shift),
		textArg(c.JoinCode), intArg(c.Archived), textArg(c.ID))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *TursoStore) ListClassrooms(ctx context.Context) ([]model.Classroom, error) {
	rows, err := t.query(ctx,
		`SELECT `+classroomCols+` FROM classrooms WHERE archived = 0 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	out := make([]model.Classroom, 0, len(rows))
	for _, row := range rows {
		c, err := scanClassroom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// --- Memberships ---

const membershipCols = "classroom_id, user_id, status, created_at"

func scanMembership(row []sqlVal) (*model.Membership, error) {
	createdAt, err := cellTime(row, 3)
	if err != nil {
		return nil, err
	}
	return &model.Membership{
		ClassroomID: cellStr(row, 0),
		UserID:      cellStr(row, 1),
		Status:      model.MembershipStatus(cellStr(row, 2)),
		CreatedAt:   createdAt,
	}, nil
}

func (t *TursoStore) AddMember(ctx context.Context, mem *model.Membership) error {
	_, err := t.exec(ctx,
		`INSERT INTO memberships (`+membershipCols+`) VALUES (?, ?, ?, ?)`,
		textArg(mem.ClassroomID), textArg(mem.UserID), textArg(string(mem.Status)), timeArg(mem.CreatedAt))
	return err
}

func (t *TursoStore) GetMember(ctx context.Context, classroomID, userID string) (*model.Membership, error) {
	rows, err := t.query(ctx,
		`SELECT `+membershipCols+` FROM memberships WHERE classroom_id = ? AND user_id = ?`,
		textArg(classroomID), textArg(userID))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanMembership(rows[0])
}

func (t *TursoStore) RemoveMember(ctx context.Context, classroomID, userID string) error {
	_, err := t.exec(ctx,
		`DELETE FROM memberships WHERE classroom_id = ? AND user_id = ?`,
		textArg(classroomID), textArg(userID))
	return err
}

func (t *TursoStore) ListMembershipsByUser(ctx context.Context, userID string) ([]model.Membership, error) {
	rows, err := t.query(ctx,
		`SELECT `+membershipCols+` FROM memberships WHERE user_id = ? ORDER BY created_at`, textArg(userID))
	if err != nil {
		return nil, err
	}
	out := make([]model.Membership, 0, len(rows))
	for _, row := range rows {
		mem, err := scanMembership(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *mem)
	}
	return out, nil
}

func (t *TursoStore) ListClassroomStudents(ctx context.Context, classroomID string) ([]model.StudentInfo, error) {
	rows, err := t.query(ctx,
		`SELECT m.user_id, u.name, u.email, m.status
		 FROM memberships m JOIN users u ON u.id = m.user_id
		 WHERE m.classroom_id = ? ORDER BY u.name`, textArg(classroomID))
	if err != nil {
		return nil, err
	}
	out := make([]model.StudentInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.StudentInfo{
			UserID: cellStr(row, 0),
			Name:   cellStr(row, 1),
			Email:  cellStr(row, 2),
			Status: model.MembershipStatus(cellStr(row, 3)),
		})
	}
	return out, nil
}

// --- Materials (§6): ponytail: data_b64 TEXT; disco bajo data dir si el volumen lo pide. ---

const materialCols = "id, classroom_id, uploaded_by, title, filename, content_type, size, visibility, subject, data_b64, created_at"

func scanMaterial(row []sqlVal) (*model.Material, error) {
	createdAt, err := cellTime(row, 10)
	if err != nil {
		return nil, err
	}
	dataB64 := cellStr(row, 9)
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return nil, fmt.Errorf("material %s: decode: %w", cellStr(row, 0), err)
	}
	return &model.Material{
		ID:          cellStr(row, 0),
		ClassroomID: cellStr(row, 1),
		UploadedBy:  cellStr(row, 2),
		Title:       cellStr(row, 3),
		Filename:    cellStr(row, 4),
		ContentType: cellStr(row, 5),
		Size:        int64(cellInt(row, 6)),
		Visibility:  model.Visibility(cellStr(row, 7)),
		Subject:     cellStr(row, 8),
		Data:        data,
		CreatedAt:   createdAt,
	}, nil
}

func materialArgs(mt *model.Material) []sqlVal {
	return []sqlVal{
		textArg(mt.ID), textArg(mt.ClassroomID), textArg(mt.UploadedBy), textArg(mt.Title),
		textArg(mt.Filename), textArg(mt.ContentType), sqlVal{Type: "integer", Value: fmt.Sprintf("%d", mt.Size)},
		textArg(string(mt.Visibility)), textArg(mt.Subject),
		textArg(base64.StdEncoding.EncodeToString(mt.Data)), timeArg(mt.CreatedAt),
	}
}

func (t *TursoStore) CreateMaterial(ctx context.Context, mt *model.Material) error {
	_, err := t.exec(ctx,
		`INSERT INTO materials (`+materialCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		materialArgs(mt)...)
	return err
}

func (t *TursoStore) GetMaterial(ctx context.Context, id string) (*model.Material, error) {
	rows, err := t.query(ctx, `SELECT `+materialCols+` FROM materials WHERE id = ?`, textArg(id))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanMaterial(rows[0])
}

func (t *TursoStore) UpdateMaterial(ctx context.Context, mt *model.Material) error {
	args := append([]sqlVal{
		textArg(mt.Title), textArg(mt.Filename), textArg(mt.ContentType),
		sqlVal{Type: "integer", Value: fmt.Sprintf("%d", mt.Size)},
		textArg(string(mt.Visibility)), textArg(mt.Subject),
	}, sqlVal{Type: "text", Value: base64.StdEncoding.EncodeToString(mt.Data)}, sqlVal{Type: "text", Value: mt.ID})
	n, err := t.exec(ctx,
		`UPDATE materials SET title = ?, filename = ?, content_type = ?, size = ?, visibility = ?, subject = ?, data_b64 = ?
		 WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *TursoStore) DeleteMaterial(ctx context.Context, id string) error {
	_, err := t.exec(ctx, `DELETE FROM materials WHERE id = ?`, textArg(id))
	return err
}

func (t *TursoStore) ListMaterials(ctx context.Context) ([]model.Material, error) {
	rows, err := t.query(ctx,
		`SELECT `+materialCols+` FROM materials ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	out := make([]model.Material, 0, len(rows))
	for _, row := range rows {
		mt, err := scanMaterial(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *mt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// --- Escuela: stats y revocación de invitados ---

func (t *TursoStore) SetSchoolGlobalCode(ctx context.Context, code string, active bool) error {
	n, err := t.exec(ctx,
		`UPDATE schools SET global_code = ?, global_code_active = ?`,
		textArg(code), intArg(active))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *TursoStore) SchoolStats(ctx context.Context) (*model.SchoolStats, error) {
	rows, err := t.query(ctx,
		`SELECT
			(SELECT COUNT(*) FROM users WHERE role = 'docente'),
			(SELECT COUNT(*) FROM classrooms WHERE archived = 0),
			(SELECT COUNT(*) FROM users WHERE role = 'alumno'),
			(SELECT COUNT(*) FROM materials WHERE visibility = 'public')`)
	if err != nil {
		return nil, err
	}
	return &model.SchoolStats{
		Teachers:        cellInt(rows[0], 0),
		Classrooms:      cellInt(rows[0], 1),
		Students:        cellInt(rows[0], 2),
		PublicMaterials: cellInt(rows[0], 3),
	}, nil
}

// RevokeGuestAccess borra en un pipeline las sesiones de invitados y sus
// usuarios efímeros: el código viejo deja afuera también a quienes ya entraron.
func (t *TursoStore) RevokeGuestAccess(ctx context.Context) error {
	_, err := t.run(ctx,
		stmtIn{SQL: `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE role = 'invitado')`},
		stmtIn{SQL: `DELETE FROM users WHERE role = 'invitado'`},
	)
	return err
}
