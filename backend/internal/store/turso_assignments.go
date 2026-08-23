package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Gonanf/ocicat-bella/backend/internal/model"
)

// Implementación Turso de consignas (§4), adjuntos y entregas (§5).
// ponytail: archivos y snapshot como columnas JSON con data_b64; disco bajo
// data dir si el volumen lo pide, igual que Material.

func filesArg(fs []model.SubmissionFile) sqlVal {
	type wire struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		DataB6 string `json:"data_b64"`
	}
	out := make([]wire, len(fs))
	for i := range fs {
		out[i] = wire{fs[i].ID, fs[i].Name, fs[i].Size, base64.StdEncoding.EncodeToString(fs[i].Data)}
	}
	b, _ := json.Marshal(out)
	return textArg(string(b))
}

func scanFiles(s string) []model.SubmissionFile {
	if s == "" || s == "[]" {
		return nil
	}
	var wires []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		DataB6 string `json:"data_b64"`
	}
	if json.Unmarshal([]byte(s), &wires) != nil {
		return nil
	}
	out := make([]model.SubmissionFile, 0, len(wires))
	for _, w := range wires {
		data, err := base64.StdEncoding.DecodeString(w.DataB6)
		if err != nil {
			continue
		}
		out = append(out, model.SubmissionFile{ID: w.ID, Name: w.Name, Size: w.Size, Data: data})
	}
	return out
}

func idsArg(ids []string) sqlVal {
	b, _ := json.Marshal(ids)
	return textArg(string(b))
}

func scanIDs(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// --- Assignments ---

const assignmentCols = "id, classroom_id, created_by, title, instructions, attachment_ids, runtime, due_at, attempts_mode, attempts_max, late_policy, deleted, created_at, updated_at"

func scanAssignment(row []sqlVal) (*model.Assignment, error) {
	createdAt, err := cellTime(row, 12)
	if err != nil {
		return nil, err
	}
	updatedAt, err := cellTime(row, 13)
	if err != nil {
		return nil, err
	}
	dueAt, err := cellTime(row, 7)
	if err != nil {
		return nil, err
	}
	return &model.Assignment{
		ID:            cellStr(row, 0),
		ClassroomID:   cellStr(row, 1),
		CreatedBy:     cellStr(row, 2),
		Title:         cellStr(row, 3),
		Instructions:  cellStr(row, 4),
		AttachmentIDs: scanIDs(cellStr(row, 5)),
		Runtime:       model.Runtime(cellStr(row, 6)),
		DueAt:         dueAt,
		Attempts: model.AttemptsConfig{
			Mode: model.AttemptsMode(cellStr(row, 8)),
			Max:  cellInt(row, 9),
		},
		LatePolicy: model.LatePolicy(cellStr(row, 10)),
		Deleted:    cellInt(row, 11) == 1,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

func assignmentArgs(a *model.Assignment) []sqlVal {
	var dueAt sqlVal
	if a.DueAt.IsZero() {
		dueAt = sqlVal{Type: "null"}
	} else {
		dueAt = timeArg(a.DueAt)
	}
	return []sqlVal{
		textArg(a.ID), textArg(a.ClassroomID), textArg(a.CreatedBy), textArg(a.Title),
		textArg(a.Instructions), idsArg(a.AttachmentIDs), textArg(string(a.Runtime)),
		dueAt, textArg(string(a.Attempts.Mode)),
		sqlVal{Type: "integer", Value: fmt.Sprintf("%d", a.Attempts.Max)},
		textArg(string(a.LatePolicy)), intArg(a.Deleted), timeArg(a.CreatedAt), timeArg(a.UpdatedAt),
	}
}

func (t *TursoStore) CreateAssignment(ctx context.Context, a *model.Assignment) error {
	_, err := t.exec(ctx,
		`INSERT INTO assignments (`+assignmentCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		assignmentArgs(a)...)
	return err
}

func (t *TursoStore) GetAssignment(ctx context.Context, id string) (*model.Assignment, error) {
	rows, err := t.query(ctx, `SELECT `+assignmentCols+` FROM assignments WHERE id = ?`, textArg(id))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanAssignment(rows[0])
}

func (t *TursoStore) UpdateAssignment(ctx context.Context, a *model.Assignment) error {
	args := append(assignmentArgs(a)[1:], textArg(a.ID))
	n, err := t.exec(ctx,
		`UPDATE assignments SET classroom_id = ?, created_by = ?, title = ?, instructions = ?,
		 attachment_ids = ?, runtime = ?, due_at = ?, attempts_mode = ?, attempts_max = ?,
		 late_policy = ?, deleted = ?, created_at = ?, updated_at = ?
		 WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *TursoStore) ListAssignments(ctx context.Context) ([]model.Assignment, error) {
	rows, err := t.query(ctx, `SELECT `+assignmentCols+` FROM assignments ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	out := make([]model.Assignment, 0, len(rows))
	for _, row := range rows {
		a, err := scanAssignment(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, nil
}

// --- Attachments ---

const attachmentCols = "id, filename, content_type, size, data_b64, uploaded_by, assignment_id, created_at"

func (t *TursoStore) CreateAttachment(ctx context.Context, at *model.Attachment) error {
	_, err := t.exec(ctx,
		`INSERT INTO attachments (`+attachmentCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		textArg(at.ID), textArg(at.Filename), textArg(at.ContentType),
		sqlVal{Type: "integer", Value: fmt.Sprintf("%d", at.Size)},
		textArg(base64.StdEncoding.EncodeToString(at.Data)),
		textArg(at.UploadedBy), textArg(at.AssignmentID), timeArg(at.CreatedAt))
	return err
}

func (t *TursoStore) DeleteAttachment(ctx context.Context, id string) error {
	_, err := t.exec(ctx, `DELETE FROM attachments WHERE id = ?`, textArg(id))
	return err
}

func (t *TursoStore) GetAttachment(ctx context.Context, id string) (*model.Attachment, error) {
	rows, err := t.query(ctx,
		`SELECT id, filename, content_type, size, data_b64, uploaded_by, assignment_id, created_at
		 FROM attachments WHERE id = ?`, textArg(id))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	row := rows[0]
	createdAt, err := cellTime(row, 7)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(cellStr(row, 4))
	if err != nil {
		return nil, fmt.Errorf("attachment %s: decode: %w", cellStr(row, 0), err)
	}
	return &model.Attachment{
		ID:           cellStr(row, 0),
		Filename:     cellStr(row, 1),
		ContentType:  cellStr(row, 2),
		Size:         int64(cellInt(row, 3)),
		Data:         data,
		UploadedBy:   cellStr(row, 5),
		AssignmentID: cellStr(row, 6),
		CreatedAt:    createdAt,
	}, nil
}

func (t *TursoStore) ListAttachments(ctx context.Context) ([]model.Attachment, error) {
	rows, err := t.query(ctx, `SELECT id, filename, content_type, size, data_b64, uploaded_by, assignment_id, created_at FROM attachments ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	out := make([]model.Attachment, 0, len(rows))
	for _, row := range rows {
		createdAt, err := cellTime(row, 7)
		if err != nil {
			return nil, err
		}
		data, err := base64.StdEncoding.DecodeString(cellStr(row, 4))
		if err != nil {
			return nil, fmt.Errorf("attachment %s: decode: %w", cellStr(row, 0), err)
		}
		out = append(out, model.Attachment{
			ID:           cellStr(row, 0),
			Filename:     cellStr(row, 1),
			ContentType:  cellStr(row, 2),
			Size:         int64(cellInt(row, 3)),
			Data:         data,
			UploadedBy:   cellStr(row, 5),
			AssignmentID: cellStr(row, 6),
			CreatedAt:    createdAt,
		})
	}
	return out, nil
}

// --- Submissions ---

const submissionCols = "id, assignment_id, student_id, attempt_number, state, files_json, snapshot_files_json, last_test_result_json, late, tested_ok, test_error, delivered_at, created_at"

func scanSubmission(row []sqlVal) (*model.Submission, error) {
	deliveredAt, err := cellTime(row, 11)
	if err != nil {
		return nil, err
	}
	createdAt, err := cellTime(row, 12)
	if err != nil {
		return nil, err
	}
	s := &model.Submission{
		ID:            cellStr(row, 0),
		AssignmentID:  cellStr(row, 1),
		StudentID:     cellStr(row, 2),
		AttemptNumber: cellInt(row, 3),
		State:         model.SubmissionState(cellStr(row, 4)),
		Files:         scanFiles(cellStr(row, 5)),
		SnapshotFiles: scanFiles(cellStr(row, 6)),
		DeliveredAt:   deliveredAt,
		Late:          cellInt(row, 8) == 1,
		TestedOK:      cellInt(row, 9) == 1,
		TestError:     cellInt(row, 10) == 1,
		CreatedAt:     createdAt,
	}
	if trJSON, isNull := cell(row, 7); !isNull && trJSON != "" {
		tr := &model.TestResult{}
		if json.Unmarshal([]byte(trJSON), tr) == nil {
			s.LastTestResult = tr
		}
	}
	return s, nil
}

func submissionArgs(s *model.Submission) []sqlVal {
	var deliveredAt sqlVal
	if s.DeliveredAt.IsZero() {
		deliveredAt = sqlVal{Type: "null"}
	} else {
		deliveredAt = timeArg(s.DeliveredAt)
	}
	var lastTR sqlVal
	if s.LastTestResult == nil {
		lastTR = sqlVal{Type: "null"}
	} else {
		b, _ := json.Marshal(s.LastTestResult)
		lastTR = textArg(string(b))
	}
	return []sqlVal{
		textArg(s.ID), textArg(s.AssignmentID), textArg(s.StudentID),
		sqlVal{Type: "integer", Value: fmt.Sprintf("%d", s.AttemptNumber)},
		textArg(string(s.State)), filesArg(s.Files), filesArg(s.SnapshotFiles), lastTR,
		intArg(s.Late), intArg(s.TestedOK), intArg(s.TestError),
		deliveredAt, timeArg(s.CreatedAt),
	}
}

func (t *TursoStore) CreateSubmission(ctx context.Context, s *model.Submission) error {
	_, err := t.exec(ctx,
		`INSERT INTO submissions (`+submissionCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		submissionArgs(s)...)
	return err
}

func (t *TursoStore) GetSubmission(ctx context.Context, id string) (*model.Submission, error) {
	rows, err := t.query(ctx, `SELECT `+submissionCols+` FROM submissions WHERE id = ?`, textArg(id))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return scanSubmission(rows[0])
}

func (t *TursoStore) UpdateSubmission(ctx context.Context, s *model.Submission) error {
	args := append(submissionArgs(s)[1:], textArg(s.ID))
	n, err := t.exec(ctx,
		`UPDATE submissions SET assignment_id = ?, student_id = ?, attempt_number = ?, state = ?,
		 files_json = ?, snapshot_files_json = ?, last_test_result_json = ?, late = ?, tested_ok = ?,
		 test_error = ?, delivered_at = ?, created_at = ?
		 WHERE id = ?`, args...)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *TursoStore) ListSubmissionsByAssignment(ctx context.Context, assignmentID string) ([]model.Submission, error) {
	rows, err := t.query(ctx,
		`SELECT `+submissionCols+` FROM submissions WHERE assignment_id = ?
		 ORDER BY attempt_number, created_at`, textArg(assignmentID))
	if err != nil {
		return nil, err
	}
	out := make([]model.Submission, 0, len(rows))
	for _, row := range rows {
		s, err := scanSubmission(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].AttemptNumber < out[j].AttemptNumber
	})
	return out, nil
}
