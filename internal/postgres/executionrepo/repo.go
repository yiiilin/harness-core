package executionrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

type AttemptRepo struct{ db postgres.DBTX }
type ActionRepo struct{ db postgres.DBTX }
type VerificationRepo struct{ db postgres.DBTX }
type ArtifactRepo struct{ db postgres.DBTX }
type RuntimeHandleRepo struct{ db postgres.DBTX }

func NewAttemptStore(db postgres.DBTX) *AttemptRepo           { return &AttemptRepo{db: db} }
func NewActionStore(db postgres.DBTX) *ActionRepo             { return &ActionRepo{db: db} }
func NewVerificationStore(db postgres.DBTX) *VerificationRepo { return &VerificationRepo{db: db} }
func NewArtifactStore(db postgres.DBTX) *ArtifactRepo         { return &ArtifactRepo{db: db} }
func NewRuntimeHandleStore(db postgres.DBTX) *RuntimeHandleRepo {
	return &RuntimeHandleRepo{db: db}
}

func (r *AttemptRepo) Create(spec execution.Attempt) execution.Attempt {
	ctx := context.Background()
	stepJSON, _ := json.Marshal(spec.Step)
	metadataJSON, _ := json.Marshal(spec.Metadata)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO attempts (
  attempt_id, session_id, task_id, step_id, approval_id, trace_id, status, step_json, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`, spec.AttemptID, spec.SessionID, nullable(spec.TaskID), nullable(spec.StepID), nullable(spec.ApprovalID), nullable(spec.TraceID), string(spec.Status), string(stepJSON), nullableJSON(metadataJSON), spec.StartedAt, nullableInt64(spec.FinishedAt))
	if err != nil {
		panic(err)
	}
	return spec
}

func (r *AttemptRepo) Get(id string) (execution.Attempt, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT attempt_id, session_id, task_id, step_id, approval_id, trace_id, status, step_json, metadata_json, started_at, finished_at
FROM attempts WHERE attempt_id = $1
`, id)
	return scanAttempt(row.Scan)
}

func (r *AttemptRepo) Update(next execution.Attempt) error {
	ctx := context.Background()
	stepJSON, err := json.Marshal(next.Step)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(next.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE attempts
SET session_id = $2,
    task_id = $3,
    step_id = $4,
    approval_id = $5,
    trace_id = $6,
    status = $7,
    step_json = $8,
    metadata_json = $9,
    started_at = $10,
    finished_at = $11
WHERE attempt_id = $1
`, next.AttemptID, next.SessionID, nullable(next.TaskID), nullable(next.StepID), nullable(next.ApprovalID), nullable(next.TraceID), string(next.Status), string(stepJSON), nullableJSON(metadataJSON), next.StartedAt, nullableInt64(next.FinishedAt))
	return err
}

func (r *AttemptRepo) List(sessionID string) []execution.Attempt {
	ctx := context.Background()
	query := `
SELECT attempt_id, session_id, task_id, step_id, approval_id, trace_id, status, step_json, metadata_json, started_at, finished_at
FROM attempts
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY started_at ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := []execution.Attempt{}
	for rows.Next() {
		item, err := scanAttempt(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return out
}

func (r *ActionRepo) Create(spec execution.ActionRecord) execution.ActionRecord {
	ctx := context.Background()
	resultJSON, _ := json.Marshal(spec.Result)
	metadataJSON, _ := json.Marshal(spec.Metadata)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO action_records (
  action_id, attempt_id, session_id, task_id, step_id, tool_name, trace_id, causation_id, status, result_json, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`, spec.ActionID, spec.AttemptID, spec.SessionID, nullable(spec.TaskID), nullable(spec.StepID), nullable(spec.ToolName), nullable(spec.TraceID), nullable(spec.CausationID), string(spec.Status), string(resultJSON), nullableJSON(metadataJSON), spec.StartedAt, nullableInt64(spec.FinishedAt))
	if err != nil {
		panic(err)
	}
	return spec
}

func (r *ActionRepo) Get(id string) (execution.ActionRecord, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT action_id, attempt_id, session_id, task_id, step_id, tool_name, trace_id, causation_id, status, result_json, metadata_json, started_at, finished_at
FROM action_records WHERE action_id = $1
`, id)
	return scanAction(row.Scan)
}

func (r *ActionRepo) Update(next execution.ActionRecord) error {
	ctx := context.Background()
	resultJSON, err := json.Marshal(next.Result)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(next.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE action_records
SET attempt_id = $2,
    session_id = $3,
    task_id = $4,
    step_id = $5,
    tool_name = $6,
    trace_id = $7,
    causation_id = $8,
    status = $9,
    result_json = $10,
    metadata_json = $11,
    started_at = $12,
    finished_at = $13
WHERE action_id = $1
`, next.ActionID, next.AttemptID, next.SessionID, nullable(next.TaskID), nullable(next.StepID), nullable(next.ToolName), nullable(next.TraceID), nullable(next.CausationID), string(next.Status), string(resultJSON), nullableJSON(metadataJSON), next.StartedAt, nullableInt64(next.FinishedAt))
	return err
}

func (r *ActionRepo) List(sessionID string) []execution.ActionRecord {
	ctx := context.Background()
	query := `
SELECT action_id, attempt_id, session_id, task_id, step_id, tool_name, trace_id, causation_id, status, result_json, metadata_json, started_at, finished_at
FROM action_records
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY started_at ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := []execution.ActionRecord{}
	for rows.Next() {
		item, err := scanAction(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return out
}

func (r *VerificationRepo) Create(spec execution.VerificationRecord) execution.VerificationRecord {
	ctx := context.Background()
	specJSON, _ := json.Marshal(spec.Spec)
	resultJSON, _ := json.Marshal(spec.Result)
	metadataJSON, _ := json.Marshal(spec.Metadata)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO verification_records (
  verification_id, attempt_id, session_id, task_id, step_id, action_id, trace_id, causation_id, status, spec_json, result_json, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
`, spec.VerificationID, spec.AttemptID, spec.SessionID, nullable(spec.TaskID), nullable(spec.StepID), nullable(spec.ActionID), nullable(spec.TraceID), nullable(spec.CausationID), string(spec.Status), string(specJSON), string(resultJSON), nullableJSON(metadataJSON), spec.StartedAt, nullableInt64(spec.FinishedAt))
	if err != nil {
		panic(err)
	}
	return spec
}

func (r *VerificationRepo) Get(id string) (execution.VerificationRecord, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT verification_id, attempt_id, session_id, task_id, step_id, action_id, trace_id, causation_id, status, spec_json, result_json, metadata_json, started_at, finished_at
FROM verification_records WHERE verification_id = $1
`, id)
	return scanVerification(row.Scan)
}

func (r *VerificationRepo) Update(next execution.VerificationRecord) error {
	ctx := context.Background()
	specJSON, err := json.Marshal(next.Spec)
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(next.Result)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(next.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE verification_records
SET attempt_id = $2,
    session_id = $3,
    task_id = $4,
    step_id = $5,
    action_id = $6,
    trace_id = $7,
    causation_id = $8,
    status = $9,
    spec_json = $10,
    result_json = $11,
    metadata_json = $12,
    started_at = $13,
    finished_at = $14
WHERE verification_id = $1
`, next.VerificationID, next.AttemptID, next.SessionID, nullable(next.TaskID), nullable(next.StepID), nullable(next.ActionID), nullable(next.TraceID), nullable(next.CausationID), string(next.Status), string(specJSON), string(resultJSON), nullableJSON(metadataJSON), next.StartedAt, nullableInt64(next.FinishedAt))
	return err
}

func (r *VerificationRepo) List(sessionID string) []execution.VerificationRecord {
	ctx := context.Background()
	query := `
SELECT verification_id, attempt_id, session_id, task_id, step_id, action_id, trace_id, causation_id, status, spec_json, result_json, metadata_json, started_at, finished_at
FROM verification_records
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY started_at ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := []execution.VerificationRecord{}
	for rows.Next() {
		item, err := scanVerification(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return out
}

func (r *ArtifactRepo) Create(spec execution.Artifact) execution.Artifact {
	ctx := context.Background()
	payloadJSON, _ := json.Marshal(spec.Payload)
	metadataJSON, _ := json.Marshal(spec.Metadata)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO artifacts (
  artifact_id, session_id, task_id, step_id, attempt_id, action_id, verification_id, trace_id, name, kind, payload_json, metadata_json, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`, spec.ArtifactID, spec.SessionID, nullable(spec.TaskID), nullable(spec.StepID), nullable(spec.AttemptID), nullable(spec.ActionID), nullable(spec.VerificationID), nullable(spec.TraceID), nullable(spec.Name), nullable(spec.Kind), nullableJSON(payloadJSON), nullableJSON(metadataJSON), spec.CreatedAt)
	if err != nil {
		panic(err)
	}
	return spec
}

func (r *ArtifactRepo) Get(id string) (execution.Artifact, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT artifact_id, session_id, task_id, step_id, attempt_id, action_id, verification_id, trace_id, name, kind, payload_json, metadata_json, created_at
FROM artifacts WHERE artifact_id = $1
`, id)
	return scanArtifact(row.Scan)
}

func (r *ArtifactRepo) Update(next execution.Artifact) error {
	ctx := context.Background()
	payloadJSON, err := json.Marshal(next.Payload)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(next.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE artifacts
SET session_id = $2,
    task_id = $3,
    step_id = $4,
    attempt_id = $5,
    action_id = $6,
    verification_id = $7,
    trace_id = $8,
    name = $9,
    kind = $10,
    payload_json = $11,
    metadata_json = $12,
    created_at = $13
WHERE artifact_id = $1
`, next.ArtifactID, next.SessionID, nullable(next.TaskID), nullable(next.StepID), nullable(next.AttemptID), nullable(next.ActionID), nullable(next.VerificationID), nullable(next.TraceID), nullable(next.Name), nullable(next.Kind), nullableJSON(payloadJSON), nullableJSON(metadataJSON), next.CreatedAt)
	return err
}

func (r *ArtifactRepo) List(sessionID string) []execution.Artifact {
	ctx := context.Background()
	query := `
SELECT artifact_id, session_id, task_id, step_id, attempt_id, action_id, verification_id, trace_id, name, kind, payload_json, metadata_json, created_at
FROM artifacts
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY created_at ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := []execution.Artifact{}
	for rows.Next() {
		item, err := scanArtifact(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

func (r *RuntimeHandleRepo) Create(spec execution.RuntimeHandle) execution.RuntimeHandle {
	ctx := context.Background()
	metadataJSON, _ := json.Marshal(spec.Metadata)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO runtime_handles (
  handle_id, session_id, task_id, attempt_id, trace_id, kind, value, metadata_json, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`, spec.HandleID, spec.SessionID, nullable(spec.TaskID), nullable(spec.AttemptID), nullable(spec.TraceID), nullable(spec.Kind), nullable(spec.Value), nullableJSON(metadataJSON), spec.CreatedAt, spec.UpdatedAt)
	if err != nil {
		panic(err)
	}
	return spec
}

func (r *RuntimeHandleRepo) Get(id string) (execution.RuntimeHandle, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT handle_id, session_id, task_id, attempt_id, trace_id, kind, value, metadata_json, created_at, updated_at
FROM runtime_handles WHERE handle_id = $1
`, id)
	return scanRuntimeHandle(row.Scan)
}

func (r *RuntimeHandleRepo) Update(next execution.RuntimeHandle) error {
	ctx := context.Background()
	metadataJSON, err := json.Marshal(next.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE runtime_handles
SET session_id = $2,
    task_id = $3,
    attempt_id = $4,
    trace_id = $5,
    kind = $6,
    value = $7,
    metadata_json = $8,
    created_at = $9,
    updated_at = $10
WHERE handle_id = $1
`, next.HandleID, next.SessionID, nullable(next.TaskID), nullable(next.AttemptID), nullable(next.TraceID), nullable(next.Kind), nullable(next.Value), nullableJSON(metadataJSON), next.CreatedAt, next.UpdatedAt)
	return err
}

func (r *RuntimeHandleRepo) List(sessionID string) []execution.RuntimeHandle {
	ctx := context.Background()
	query := `
SELECT handle_id, session_id, task_id, attempt_id, trace_id, kind, value, metadata_json, created_at, updated_at
FROM runtime_handles
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY created_at ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := []execution.RuntimeHandle{}
	for rows.Next() {
		item, err := scanRuntimeHandle(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return out
}

type scanner func(dest ...any) error

type sqlNullString struct {
	String string
	Valid  bool
}

type sqlNullInt64 struct {
	Int64 int64
	Valid bool
}

func (n *sqlNullString) Scan(value any) error {
	if value == nil {
		n.String = ""
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case string:
		n.String = v
	case []byte:
		n.String = string(v)
	default:
		n.String = ""
	}
	n.Valid = true
	return nil
}

func (n *sqlNullInt64) Scan(value any) error {
	if value == nil {
		n.Int64 = 0
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case int64:
		n.Int64 = v
	case int:
		n.Int64 = int64(v)
	case float64:
		n.Int64 = int64(v)
	default:
		n.Int64 = 0
	}
	n.Valid = true
	return nil
}

func scanAttempt(scan scanner) (execution.Attempt, error) {
	var rec execution.Attempt
	var taskID, stepID, approvalID, traceID, metadataRaw sqlNullString
	var status, stepRaw string
	var finishedAt sqlNullInt64
	if err := scan(&rec.AttemptID, &rec.SessionID, &taskID, &stepID, &approvalID, &traceID, &status, &stepRaw, &metadataRaw, &rec.StartedAt, &finishedAt); err != nil {
		return execution.Attempt{}, translateErr(err)
	}
	rec.TaskID = taskID.String
	rec.StepID = stepID.String
	rec.ApprovalID = approvalID.String
	rec.TraceID = traceID.String
	rec.Status = execution.AttemptStatus(status)
	rec.FinishedAt = finishedAt.Int64
	_ = json.Unmarshal([]byte(stepRaw), &rec.Step)
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &rec.Metadata)
	}
	if rec.Metadata == nil {
		rec.Metadata = map[string]any{}
	}
	return rec, nil
}

func scanAction(scan scanner) (execution.ActionRecord, error) {
	var rec execution.ActionRecord
	var taskID, stepID, toolName, traceID, causationID, metadataRaw sqlNullString
	var status, resultRaw string
	var finishedAt sqlNullInt64
	if err := scan(&rec.ActionID, &rec.AttemptID, &rec.SessionID, &taskID, &stepID, &toolName, &traceID, &causationID, &status, &resultRaw, &metadataRaw, &rec.StartedAt, &finishedAt); err != nil {
		return execution.ActionRecord{}, translateErr(err)
	}
	rec.TaskID = taskID.String
	rec.StepID = stepID.String
	rec.ToolName = toolName.String
	rec.TraceID = traceID.String
	rec.CausationID = causationID.String
	rec.Status = execution.ActionStatus(status)
	rec.FinishedAt = finishedAt.Int64
	_ = json.Unmarshal([]byte(resultRaw), &rec.Result)
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &rec.Metadata)
	}
	if rec.Metadata == nil {
		rec.Metadata = map[string]any{}
	}
	return rec, nil
}

func scanVerification(scan scanner) (execution.VerificationRecord, error) {
	var rec execution.VerificationRecord
	var taskID, stepID, actionID, traceID, causationID, metadataRaw sqlNullString
	var status, specRaw, resultRaw string
	var finishedAt sqlNullInt64
	if err := scan(&rec.VerificationID, &rec.AttemptID, &rec.SessionID, &taskID, &stepID, &actionID, &traceID, &causationID, &status, &specRaw, &resultRaw, &metadataRaw, &rec.StartedAt, &finishedAt); err != nil {
		return execution.VerificationRecord{}, translateErr(err)
	}
	rec.TaskID = taskID.String
	rec.StepID = stepID.String
	rec.ActionID = actionID.String
	rec.TraceID = traceID.String
	rec.CausationID = causationID.String
	rec.Status = execution.VerificationStatus(status)
	rec.FinishedAt = finishedAt.Int64
	_ = json.Unmarshal([]byte(specRaw), &rec.Spec)
	_ = json.Unmarshal([]byte(resultRaw), &rec.Result)
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &rec.Metadata)
	}
	if rec.Metadata == nil {
		rec.Metadata = map[string]any{}
	}
	return rec, nil
}

func scanArtifact(scan scanner) (execution.Artifact, error) {
	var rec execution.Artifact
	var taskID, stepID, attemptID, actionID, verificationID, traceID, name, kind, payloadRaw, metadataRaw sqlNullString
	if err := scan(&rec.ArtifactID, &rec.SessionID, &taskID, &stepID, &attemptID, &actionID, &verificationID, &traceID, &name, &kind, &payloadRaw, &metadataRaw, &rec.CreatedAt); err != nil {
		return execution.Artifact{}, translateErr(err)
	}
	rec.TaskID = taskID.String
	rec.StepID = stepID.String
	rec.AttemptID = attemptID.String
	rec.ActionID = actionID.String
	rec.VerificationID = verificationID.String
	rec.TraceID = traceID.String
	rec.Name = name.String
	rec.Kind = kind.String
	if payloadRaw.String != "" {
		_ = json.Unmarshal([]byte(payloadRaw.String), &rec.Payload)
	}
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &rec.Metadata)
	}
	if rec.Payload == nil {
		rec.Payload = map[string]any{}
	}
	if rec.Metadata == nil {
		rec.Metadata = map[string]any{}
	}
	return rec, nil
}

func scanRuntimeHandle(scan scanner) (execution.RuntimeHandle, error) {
	var rec execution.RuntimeHandle
	var taskID, attemptID, traceID, kind, value, metadataRaw sqlNullString
	if err := scan(&rec.HandleID, &rec.SessionID, &taskID, &attemptID, &traceID, &kind, &value, &metadataRaw, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return execution.RuntimeHandle{}, translateErr(err)
	}
	rec.TaskID = taskID.String
	rec.AttemptID = attemptID.String
	rec.TraceID = traceID.String
	rec.Kind = kind.String
	rec.Value = value.String
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &rec.Metadata)
	}
	if rec.Metadata == nil {
		rec.Metadata = map[string]any{}
	}
	return rec, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableJSON(raw []byte) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}

func translateErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return execution.ErrRecordNotFound
	}
	return err
}
