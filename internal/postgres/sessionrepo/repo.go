package sessionrepo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

type Repo struct {
	db postgres.DBTX
}

func New(db postgres.DBTX) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Create(title, goal string) session.State {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	id := uuid.NewString()
	metadata := map[string]any{}
	metaJSON, _ := json.Marshal(metadata)
	row := r.db.QueryRowContext(ctx, `
INSERT INTO sessions (
  session_id, task_id, parent_session_id, title, goal, phase, current_step_id, summary, retry_count, metadata_json, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING session_id, task_id, parent_session_id, title, goal, phase, current_step_id, summary, retry_count, metadata_json, created_at, updated_at
`, id, nil, nil, title, goal, string(session.PhaseReceived), nil, nil, 0, string(metaJSON), now, now)
	st, err := scanState(row.Scan)
	if err != nil {
		panic(err)
	}
	return st
}

func (r *Repo) Get(id string) (session.State, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT session_id, task_id, parent_session_id, title, goal, phase, current_step_id, summary, retry_count, metadata_json, created_at, updated_at
FROM sessions WHERE session_id = $1
`, id)
	return scanState(row.Scan)
}

func (r *Repo) Update(next session.State) error {
	ctx := context.Background()
	next.UpdatedAt = time.Now().UnixMilli()
	metaJSON, err := json.Marshal(next.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE sessions
SET task_id = $2,
    parent_session_id = $3,
    title = $4,
    goal = $5,
    phase = $6,
    current_step_id = $7,
    summary = $8,
    retry_count = $9,
    metadata_json = $10,
    updated_at = $11
WHERE session_id = $1
`, next.SessionID, nullable(next.TaskID), nullable(next.ParentSessionID), next.Title, nullable(next.Goal), string(next.Phase), nullable(next.CurrentStepID), nullable(next.Summary), next.RetryCount, string(metaJSON), next.UpdatedAt)
	return err
}

func (r *Repo) List() []session.State {
	ctx := context.Background()
	rows, err := r.db.QueryContext(ctx, `
SELECT session_id, task_id, parent_session_id, title, goal, phase, current_step_id, summary, retry_count, metadata_json, created_at, updated_at
FROM sessions
ORDER BY updated_at DESC
`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := []session.State{}
	for rows.Next() {
		st, err := scanState(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return out
}

type scanner func(dest ...any) error

func scanState(scan scanner) (session.State, error) {
	var st session.State
	var taskID, parentID, goal, currentStepID, summary sqlNullString
	var phase string
	var metaRaw string
	if err := scan(&st.SessionID, &taskID, &parentID, &st.Title, &goal, &phase, &currentStepID, &summary, &st.RetryCount, &metaRaw, &st.CreatedAt, &st.UpdatedAt); err != nil {
		return session.State{}, translateErr(err)
	}
	st.TaskID = taskID.String
	st.ParentSessionID = parentID.String
	st.Goal = goal.String
	st.Phase = session.Phase(phase)
	st.CurrentStepID = currentStepID.String
	st.Summary = summary.String
	if metaRaw != "" {
		_ = json.Unmarshal([]byte(metaRaw), &st.Metadata)
	}
	if st.Metadata == nil {
		st.Metadata = map[string]any{}
	}
	return st, nil
}

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
	return session.ErrSessionNotFound
}
