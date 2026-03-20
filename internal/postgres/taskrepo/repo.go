package taskrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

type Repo struct {
	db postgres.DBTX
}

func New(db postgres.DBTX) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Create(spec task.Spec) (task.Record, error) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	id := spec.TaskID
	if id == "" {
		id = uuid.NewString()
	}
	constraintsJSON, _ := json.Marshal(spec.Constraints)
	metadataJSON, _ := json.Marshal(spec.Metadata)
	row := r.db.QueryRowContext(ctx, `
INSERT INTO tasks (
  task_id, task_type, goal, status, session_id, constraints_json, metadata_json, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING task_id, task_type, goal, status, session_id, constraints_json, metadata_json, created_at, updated_at
`, id, spec.TaskType, spec.Goal, string(task.StatusReceived), nil, string(constraintsJSON), string(metadataJSON), now, now)
	rec, err := scanRecord(row.Scan)
	if err != nil {
		return task.Record{}, err
	}
	return rec, nil
}

func (r *Repo) Get(id string) (task.Record, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT task_id, task_type, goal, status, session_id, constraints_json, metadata_json, created_at, updated_at
FROM tasks WHERE task_id = $1
`, id)
	return scanRecord(row.Scan)
}

func (r *Repo) Update(next task.Record) error {
	ctx := context.Background()
	next.UpdatedAt = time.Now().UnixMilli()
	constraintsJSON, err := json.Marshal(next.Constraints)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(next.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE tasks
SET task_type = $2,
    goal = $3,
    status = $4,
    session_id = $5,
    constraints_json = $6,
    metadata_json = $7,
    updated_at = $8
WHERE task_id = $1
`, next.TaskID, next.TaskType, next.Goal, string(next.Status), nullable(next.SessionID), string(constraintsJSON), string(metadataJSON), next.UpdatedAt)
	return err
}

func (r *Repo) List() ([]task.Record, error) {
	ctx := context.Background()
	rows, err := r.db.QueryContext(ctx, `
SELECT task_id, task_type, goal, status, session_id, constraints_json, metadata_json, created_at, updated_at
FROM tasks
ORDER BY updated_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []task.Record{}
	for rows.Next() {
		rec, err := scanRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type scanner func(dest ...any) error

type sqlNullString struct {
	String string
	Valid  bool
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

func scanRecord(scan scanner) (task.Record, error) {
	var rec task.Record
	var status string
	var sessionID sqlNullString
	var constraintsRaw string
	var metadataRaw string
	if err := scan(&rec.TaskID, &rec.TaskType, &rec.Goal, &status, &sessionID, &constraintsRaw, &metadataRaw, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return task.Record{}, translateErr(err)
	}
	rec.Status = task.Status(status)
	rec.SessionID = sessionID.String
	if constraintsRaw != "" {
		_ = json.Unmarshal([]byte(constraintsRaw), &rec.Constraints)
	}
	if metadataRaw != "" {
		_ = json.Unmarshal([]byte(metadataRaw), &rec.Metadata)
	}
	if rec.Constraints == nil {
		rec.Constraints = map[string]any{}
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

func translateErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return task.ErrTaskNotFound
	}
	return err
}
