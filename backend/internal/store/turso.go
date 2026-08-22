package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// TursoStore implementa Store sobre la API SQL-over-HTTP de Turso (POST /v2/pipeline).
// Cliente propio con net/http + JSON (protocolo confirmado en docs.turso.tech/sdk/http/reference):
// sin CGo (go-libsql descartado) ni libsql-client-go (deprecado). Cero dependencias nuevas.
//
// Cada operación es un pipeline stateless que termina en {"type":"close"}: sin baton,
// cada llamada abre y cierra conexión. Suficiente para el volumen de una escuela MVP;
// si hiciera falta menos latencia, reusar el baton devuelto por el servidor.

type TursoStore struct {
	pipelineURL string
	token       string
	hc          *http.Client
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS schools(
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	global_code TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users(
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	disabled INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS sessions(
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id),
	kind TEXT NOT NULL,
	device_label TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	last_seen_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS magic_tokens(
	token TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id),
	context TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	used_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_magic_tokens_user_id ON magic_tokens(user_id);
`

// NewTursoStore crea el store contra Turso y ejecuta el schema (idempotente).
// dbURL acepta libsql://… o https://…
func NewTursoStore(ctx context.Context, dbURL, authToken string) (*TursoStore, error) {
	url := dbURL
	switch {
	case strings.HasPrefix(url, "libsql://"):
		url = "https://" + strings.TrimPrefix(url, "libsql://")
	case !strings.HasPrefix(url, "http"):
		return nil, fmt.Errorf("TURSO_DATABASE_URL inválida: %q", dbURL)
	}
	url = strings.TrimRight(url, "/") + "/v2/pipeline"

	ts := &TursoStore{
		pipelineURL: url,
		token:       authToken,
		hc:          &http.Client{Timeout: 10 * time.Second},
	}

	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := ts.exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("migración: %w", err)
		}
	}
	return ts, nil
}

// --- Protocolo pipeline ---

type sqlVal struct {
	Type  string `json:"type"` // null | integer | float | text
	Value string `json:"value,omitempty"`
}

type stmtIn struct {
	SQL  string   `json:"sql"`
	Args []sqlVal `json:"args,omitempty"`
}

type pipeReq struct {
	Requests []pipeAction `json:"requests"`
}

type pipeAction struct {
	Type string  `json:"type"`
	Stmt *stmtIn `json:"stmt,omitempty"`
}

type pipeResp struct {
	Results []pipeResult `json:"results"`
}

type pipeErr struct {
	Message string `json:"message"`
}

type pipeResult struct {
	Type     string   `json:"type"` // ok | error
	Error    *pipeErr `json:"error,omitempty"`
	Response *struct {
		Result *struct {
			Rows             [][]sqlVal `json:"rows"`
			AffectedRowCount int        `json:"affected_row_count"`
		} `json:"result,omitempty"`
	} `json:"response,omitempty"`
}

// run ejecuta statements en un pipeline y siempre cierra la conexión.
func (t *TursoStore) run(ctx context.Context, stmts ...stmtIn) ([]pipeResult, error) {
	reqs := make([]pipeAction, 0, len(stmts)+1)
	for i := range stmts {
		reqs = append(reqs, pipeAction{Type: "execute", Stmt: &stmts[i]})
	}
	reqs = append(reqs, pipeAction{Type: "close"})

	body, err := json.Marshal(pipeReq{Requests: reqs})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.pipelineURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("turso pipeline: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("turso pipeline HTTP %d: %s", resp.StatusCode, b)
	}

	var out pipeResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("turso pipeline decode: %w", err)
	}
	if len(out.Results) < len(stmts) {
		return nil, fmt.Errorf("turso pipeline: %d resultados para %d statements", len(out.Results), len(stmts))
	}
	for _, r := range out.Results[:len(stmts)] {
		if r.Type == "error" && r.Error != nil {
			return nil, fmt.Errorf("turso: %s", r.Error.Message)
		}
		if r.Type == "error" {
			return nil, fmt.Errorf("turso: error sin detalle")
		}
	}
	return out.Results, nil
}

func (t *TursoStore) exec(ctx context.Context, sql string, args ...sqlVal) (int, error) {
	results, err := t.run(ctx, stmtIn{SQL: sql, Args: args})
	if err != nil {
		return 0, err
	}
	res := results[0].Response.Result
	if res == nil {
		return 0, nil
	}
	return res.AffectedRowCount, nil
}

func (t *TursoStore) query(ctx context.Context, sql string, args ...sqlVal) ([][]sqlVal, error) {
	results, err := t.run(ctx, stmtIn{SQL: sql, Args: args})
	if err != nil {
		return nil, err
	}
	return results[0].Response.Result.Rows, nil
}

func textArg(s string) sqlVal    { return sqlVal{Type: "text", Value: s} }
func intArg(b bool) sqlVal       { return sqlVal{Type: "integer", Value: boolInt(b)} }
func timeArg(t time.Time) sqlVal { return sqlVal{Type: "text", Value: t.UTC().Format(time.RFC3339)} }

func boolInt(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// cell devuelve el valor textual de una celda y si era NULL.
func cell(row []sqlVal, i int) (string, bool) {
	if i >= len(row) || row[i].Type == "null" {
		return "", true
	}
	return row[i].Value, false
}

func cellTime(row []sqlVal, i int) (time.Time, error) {
	s, isNull := cell(row, i)
	if isNull || s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func scanUser(row []sqlVal) *model.User {
	disabledStr, _ := cell(row, 5)
	hash, _ := cell(row, 3)
	email, _ := cell(row, 2)
	name, _ := cell(row, 1)
	id, _ := cell(row, 0)
	return &model.User{
		ID:           id,
		Name:         name,
		Email:        email,
		PasswordHash: hash,
		Role:         model.Role(cellStr(row, 4)),
		Disabled:     disabledStr == "1",
	}
}

func cellStr(row []sqlVal, i int) string { s, _ := cell(row, i); return s }

const sessionCols = "id, user_id, kind, device_label, created_at, last_seen_at, expires_at"

func scanSession(row []sqlVal) (*model.Session, error) {
	createdAt, err := cellTime(row, 4)
	if err != nil {
		return nil, err
	}
	lastSeenAt, err := cellTime(row, 5)
	if err != nil {
		return nil, err
	}
	expiresAt, err := cellTime(row, 6)
	if err != nil {
		return nil, err
	}
	return &model.Session{
		ID:          cellStr(row, 0),
		UserID:      cellStr(row, 1),
		Kind:        model.SessionKind(cellStr(row, 2)),
		DeviceLabel: cellStr(row, 3),
		CreatedAt:   createdAt,
		LastSeenAt:  lastSeenAt,
		ExpiresAt:   expiresAt,
	}, nil
}

func (t *TursoStore) GetSession(ctx context.Context, sessionID string) (*model.Session, *model.User, error) {
	rows, err := t.query(ctx,
		`SELECT s.`+sessionCols+`, u.id, u.name, u.email, u.password_hash, u.role, u.disabled
		 FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.id = ?`, textArg(sessionID))
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return nil, nil, ErrNotFound
	}
	sess, err := scanSession(rows[0][:7])
	if err != nil {
		return nil, nil, err
	}
	user := scanUser(rows[0][7:])
	return sess, user, nil
}

func (t *TursoStore) CreateSession(ctx context.Context, session *model.Session) error {
	_, err := t.exec(ctx,
		`INSERT INTO sessions (`+sessionCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		textArg(session.ID), textArg(session.UserID), textArg(string(session.Kind)),
		textArg(session.DeviceLabel), timeArg(session.CreatedAt), timeArg(session.LastSeenAt),
		timeArg(session.ExpiresAt))
	return err
}

func (t *TursoStore) TouchSession(ctx context.Context, sessionID string, now time.Time) error {
	_, err := t.exec(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id = ?`, timeArg(now), textArg(sessionID))
	return err
}

func (t *TursoStore) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := t.exec(ctx, `DELETE FROM sessions WHERE id = ?`, textArg(sessionID))
	return err
}

func (t *TursoStore) ListSessionsByUser(ctx context.Context, userID string) ([]model.Session, error) {
	rows, err := t.query(ctx,
		`SELECT `+sessionCols+` FROM sessions WHERE user_id = ? ORDER BY created_at DESC`, textArg(userID))
	if err != nil {
		return nil, err
	}
	out := make([]model.Session, 0, len(rows))
	for _, row := range rows {
		sess, err := scanSession(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *sess)
	}
	return out, nil
}

const userCols = "id, name, email, password_hash, role, disabled"

func (t *TursoStore) GetUser(ctx context.Context, userID string) (*model.User, error) {
	rows, err := t.query(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, textArg(userID))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanUser(rows[0]), nil
}

func (t *TursoStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	rows, err := t.query(ctx, `SELECT `+userCols+` FROM users WHERE email = ?`, textArg(email))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanUser(rows[0]), nil
}

func (t *TursoStore) CreateUser(ctx context.Context, user *model.User) error {
	_, err := t.exec(ctx,
		`INSERT INTO users (`+userCols+`) VALUES (?, ?, ?, ?, ?, ?)`,
		textArg(user.ID), textArg(user.Name), textArg(user.Email),
		textArg(user.PasswordHash), textArg(string(user.Role)), intArg(user.Disabled))
	return err
}

func (t *TursoStore) CreateMagicToken(ctx context.Context, token *model.MagicToken) error {
	var usedAt sqlVal
	if !token.UsedAt.IsZero() {
		usedAt = timeArg(token.UsedAt)
	} else {
		usedAt = sqlVal{Type: "null"}
	}
	_, err := t.exec(ctx,
		`INSERT INTO magic_tokens (token, user_id, context, created_at, expires_at, used_at) VALUES (?, ?, ?, ?, ?, ?)`,
		textArg(token.Token), textArg(token.UserID), textArg(string(token.Context)),
		timeArg(token.CreatedAt), timeArg(token.ExpiresAt), usedAt)
	return err
}

// ConsumeMagicToken marca el token usado en un único UPDATE atómico (single-use garantizado
// por SQLite); si no afectó filas, re-consulta para distinguir expirado vs ya usado.
func (t *TursoStore) ConsumeMagicToken(ctx context.Context, token string, now time.Time) (*model.MagicToken, error) {
	affected, err := t.exec(ctx,
		`UPDATE magic_tokens SET used_at = ? WHERE token = ? AND used_at IS NULL AND expires_at > ?`,
		timeArg(now), textArg(token), timeArg(now))
	if err != nil {
		return nil, err
	}
	if affected == 1 {
		rows, err := t.query(ctx,
			`SELECT token, user_id, context, created_at, expires_at, used_at FROM magic_tokens WHERE token = ?`,
			textArg(token))
		if err != nil || len(rows) == 0 {
			return nil, err
		}
		return scanMagicToken(rows[0])
	}

	rows, err := t.query(ctx,
		`SELECT token, user_id, context, created_at, expires_at, used_at FROM magic_tokens WHERE token = ?`,
		textArg(token))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	mt, err := scanMagicToken(rows[0])
	if err != nil {
		return nil, err
	}
	if !mt.UsedAt.IsZero() {
		return nil, ErrTokenUsed
	}
	return nil, ErrTokenExpired
}

func scanMagicToken(row []sqlVal) (*model.MagicToken, error) {
	createdAt, err := cellTime(row, 3)
	if err != nil {
		return nil, err
	}
	expiresAt, err := cellTime(row, 4)
	if err != nil {
		return nil, err
	}
	usedAt, err := cellTime(row, 5)
	if err != nil {
		return nil, err
	}
	return &model.MagicToken{
		Token:     cellStr(row, 0),
		UserID:    cellStr(row, 1),
		Context:   model.MagicContext(cellStr(row, 2)),
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		UsedAt:    usedAt,
	}, nil
}

func (t *TursoStore) IsConfigured(ctx context.Context) (bool, error) {
	rows, err := t.query(ctx, `SELECT COUNT(*) FROM users WHERE role = 'director'`)
	if err != nil {
		return false, err
	}
	countStr, isNull := cell(rows[0], 0)
	n := 0
	if !isNull {
		_, _ = fmt.Sscanf(countStr, "%d", &n)
	}
	return n > 0, nil
}

func (t *TursoStore) CreateSchool(ctx context.Context, school *model.School) error {
	_, err := t.exec(ctx,
		`INSERT INTO schools (id, name, global_code, created_at) VALUES (?, ?, ?, ?)`,
		textArg(school.ID), textArg(school.Name), textArg(school.GlobalCode), timeArg(school.CreatedAt))
	return err
}

func (t *TursoStore) GetSchool(ctx context.Context) (*model.School, error) {
	rows, err := t.query(ctx, `SELECT id, name, global_code, created_at FROM schools LIMIT 1`)
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
	return &model.School{
		ID:         cellStr(rows[0], 0),
		Name:       cellStr(rows[0], 1),
		GlobalCode: cellStr(rows[0], 2),
		CreatedAt:  createdAt,
	}, nil
}
