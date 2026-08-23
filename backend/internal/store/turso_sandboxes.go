package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// Implementación Turso de sandboxes y runs (§7).

const sandboxCols = "id, template_id, created_by, classroom_id, mode, start_command, packages_extra, purpose, submission_id, retention, visibility, cleaned, created_at"

func scanSandbox(row []sqlVal) *model.Sandbox {
	createdAt, _ := cellTime(row, 12)
	return &model.Sandbox{
		ID:            cellStr(row, 0),
		TemplateID:    cellStr(row, 1),
		CreatedBy:     cellStr(row, 2),
		ClassroomID:   cellStr(row, 3),
		Mode:          model.SandboxMode(cellStr(row, 4)),
		StartCommand:  cellStr(row, 5),
		PackagesExtra: cellStr(row, 6),
		Purpose:       model.SandboxPurpose(cellStr(row, 7)),
		SubmissionID:  cellStr(row, 8),
		Retention:     model.SandboxRetention(cellStr(row, 9)),
		Visibility:    model.Visibility(cellStr(row, 10)),
		Cleaned:       cellInt(row, 11) == 1,
		CreatedAt:     createdAt,
	}
}

func sandboxArgs(sb *model.Sandbox) []sqlVal {
	return []sqlVal{
		textArg(sb.ID), textArg(sb.TemplateID), textArg(sb.CreatedBy), textArg(sb.ClassroomID),
		textArg(string(sb.Mode)), textArg(sb.StartCommand), textArg(sb.PackagesExtra),
		textArg(string(sb.Purpose)), textArg(sb.SubmissionID), textArg(string(sb.Retention)),
		textArg(string(sb.Visibility)), intArg(sb.Cleaned), timeArg(sb.CreatedAt),
	}
}

func (t *TursoStore) CreateSandbox(ctx context.Context, sb *model.Sandbox) error {
	_, err := t.exec(ctx,
		`INSERT INTO sandboxes (`+sandboxCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sandboxArgs(sb)...)
	return err
}

func (t *TursoStore) GetSandbox(ctx context.Context, id string) (*model.Sandbox, error) {
	rows, err := t.query(ctx, `SELECT `+sandboxCols+` FROM sandboxes WHERE id = ?`, textArg(id))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanSandbox(rows[0]), nil
}

func (t *TursoStore) ListSandboxes(ctx context.Context) ([]model.Sandbox, error) {
	rows, err := t.query(ctx, `SELECT `+sandboxCols+` FROM sandboxes ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	out := make([]model.Sandbox, 0, len(rows))
	for _, row := range rows {
		out = append(out, *scanSandbox(row))
	}
	return out, nil
}

func (t *TursoStore) UpdateSandbox(ctx context.Context, sb *model.Sandbox) error {
	args := append(sandboxArgs(sb)[1:], textArg(sb.ID))
	n, err := t.exec(ctx,
		`UPDATE sandboxes SET template_id = ?, created_by = ?, classroom_id = ?, mode = ?,
		 start_command = ?, packages_extra = ?, purpose = ?, submission_id = ?, retention = ?,
		 visibility = ?, cleaned = ?, created_at = ?
		 WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const runCols = "id, sandbox_id, n, status, exit_code, service_url, started_at, logs_json, created_at"

func scanRun(row []sqlVal) (*model.Run, error) {
	startedAt, err := cellTime(row, 6)
	if err != nil {
		return nil, err
	}
	createdAt, err := cellTime(row, 8)
	if err != nil {
		return nil, err
	}
	r := &model.Run{
		ID:         cellStr(row, 0),
		SandboxID:  cellStr(row, 1),
		N:          cellInt(row, 2),
		Status:     model.RunStatus(cellStr(row, 3)),
		ServiceURL: cellStr(row, 5),
		StartedAt:  startedAt,
		CreatedAt:  createdAt,
	}
	if ecStr, isNull := cell(row, 4); !isNull {
		r.ExitCode = new(int)
		_, _ = fmt.Sscanf(ecStr, "%d", r.ExitCode)
	}
	logsJSON := cellStr(row, 7)
	if logsJSON != "" && logsJSON != "[]" {
		var logs []model.LogLine
		if json.Unmarshal([]byte(logsJSON), &logs) == nil {
			r.Logs = logs
		}
	}
	return r, nil
}

// ponytail: History no se persiste; GET /runs/{id} lo reconstruye desde los
// runs del mismo sandbox (ver handlers). Logs como JSON compacto.
func runLogsJSON(logs []model.LogLine) sqlVal {
	b, _ := json.Marshal(logs)
	return textArg(string(b))
}

func runArgs(r *model.Run) []sqlVal {
	var exitCode sqlVal
	if r.ExitCode == nil {
		exitCode = sqlVal{Type: "null"}
	} else {
		exitCode = sqlVal{Type: "integer", Value: fmt.Sprintf("%d", *r.ExitCode)}
	}
	var startedAt sqlVal
	if r.StartedAt.IsZero() {
		startedAt = sqlVal{Type: "null"}
	} else {
		startedAt = timeArg(r.StartedAt)
	}
	return []sqlVal{
		textArg(r.ID), textArg(r.SandboxID),
		sqlVal{Type: "integer", Value: fmt.Sprintf("%d", r.N)},
		textArg(string(r.Status)), exitCode, textArg(r.ServiceURL),
		startedAt, runLogsJSON(r.Logs), timeArg(r.CreatedAt),
	}
}

func (t *TursoStore) CreateRun(ctx context.Context, r *model.Run) error {
	_, err := t.exec(ctx,
		`INSERT INTO runs (`+runCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runArgs(r)...)
	return err
}

func (t *TursoStore) GetRun(ctx context.Context, id string) (*model.Run, error) {
	rows, err := t.query(ctx, `SELECT `+runCols+` FROM runs WHERE id = ?`, textArg(id))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanRun(rows[0])
}

func (t *TursoStore) UpdateRun(ctx context.Context, r *model.Run) error {
	args := append(runArgs(r)[1:], textArg(r.ID))
	n, err := t.exec(ctx,
		`UPDATE runs SET sandbox_id = ?, n = ?, status = ?, exit_code = ?, service_url = ?,
		 started_at = ?, logs_json = ?, created_at = ?
		 WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *TursoStore) ListRunsBySandbox(ctx context.Context, sandboxID string) ([]model.Run, error) {
	rows, err := t.query(ctx,
		`SELECT `+runCols+` FROM runs WHERE sandbox_id = ? ORDER BY n, created_at`, textArg(sandboxID))
	if err != nil {
		return nil, err
	}
	out := make([]model.Run, 0, len(rows))
	for _, row := range rows {
		r, err := scanRun(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, nil
}

func (t *TursoStore) ListRuns(ctx context.Context) ([]model.Run, error) {
	rows, err := t.query(ctx, `SELECT `+runCols+` FROM runs ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	out := make([]model.Run, 0, len(rows))
	for _, row := range rows {
		r, err := scanRun(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, nil
}
