package store

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// Implementación Turso de la gestión escolar §9 (docentes, import CSV,
// auditoría). MemStore equivalente en school.go.

func (t *TursoStore) UpdateUser(ctx context.Context, u *model.User) error {
	affected, err := t.exec(ctx,
		`UPDATE users SET name = ?, email = ?, password_hash = ?, role = ?, disabled = ?, created_at = ? WHERE id = ?`,
		textArg(u.Name), textArg(u.Email), textArg(u.PasswordHash),
		textArg(string(u.Role)), intArg(u.Disabled), timeArg(u.CreatedAt), textArg(u.ID))
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *TursoStore) ListUsersByRole(ctx context.Context, role model.Role) ([]model.User, error) {
	rows, err := t.query(ctx, `SELECT `+userCols+` FROM users WHERE role = ?`, textArg(string(role)))
	if err != nil {
		return nil, err
	}
	out := make([]model.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, *scanUser(row))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out, nil
}

func (t *TursoStore) CreateImportToken(ctx context.Context, tok *model.ImportToken) error {
	_, err := t.exec(ctx,
		`INSERT INTO import_tokens (token, classroom_id, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		textArg(tok.Token), textArg(tok.ClassroomID), textArg(tok.CreatedBy),
		timeArg(tok.CreatedAt), timeArg(tok.ExpiresAt))
	return err
}

func (t *TursoStore) GetImportToken(ctx context.Context, token string) (*model.ImportToken, error) {
	rows, err := t.query(ctx,
		`SELECT token, classroom_id, created_by, created_at, expires_at FROM import_tokens WHERE token = ?`,
		textArg(token))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	createdAt, err := cellTime(rows[0], 3)
	if err != nil {
		return nil, err
	}
	expiresAt, err := cellTime(rows[0], 4)
	if err != nil {
		return nil, err
	}
	tok := &model.ImportToken{
		Token:       cellStr(rows[0], 0),
		ClassroomID: cellStr(rows[0], 1),
		CreatedBy:   cellStr(rows[0], 2),
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
	}
	if time.Now().After(tok.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	return tok, nil
}

const auditCols = "id, actor_id, action, target_user_id, reason, ip, created_at"

func scanAuditEntry(row []sqlVal) (*model.AuditEntry, error) {
	createdAt, err := cellTime(row, 6)
	if err != nil {
		return nil, err
	}
	return &model.AuditEntry{
		ID:           cellStr(row, 0),
		ActorID:      cellStr(row, 1),
		Action:       cellStr(row, 2),
		TargetUserID: cellStr(row, 3),
		Reason:       cellStr(row, 4),
		IP:           cellStr(row, 5),
		CreatedAt:    createdAt,
	}, nil
}

func (t *TursoStore) CreateAuditEntry(ctx context.Context, e *model.AuditEntry) error {
	_, err := t.exec(ctx,
		`INSERT INTO audit_log (`+auditCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		textArg(e.ID), textArg(e.ActorID), textArg(e.Action), textArg(e.TargetUserID),
		textArg(e.Reason), textArg(e.IP), timeArg(e.CreatedAt))
	return err
}

func (t *TursoStore) ListAuditEntries(ctx context.Context, actorID, action string, from, to time.Time) ([]model.AuditEntry, error) {
	where := []string{}
	args := []sqlVal{}
	if actorID != "" {
		where = append(where, "actor_id = ?")
		args = append(args, textArg(actorID))
	}
	if action != "" {
		where = append(where, "action = ?")
		args = append(args, textArg(action))
	}
	if !from.IsZero() {
		where = append(where, "created_at >= ?")
		args = append(args, timeArg(from))
	}
	if !to.IsZero() {
		where = append(where, "created_at <= ?")
		args = append(args, timeArg(to))
	}
	sql := `SELECT ` + auditCols + ` FROM audit_log`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := t.query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out := make([]model.AuditEntry, 0, len(rows))
	for _, row := range rows {
		e, err := scanAuditEntry(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
