package sessionrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func (r *Repo) Create(title, goal string) (session.State, error) {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	id := uuid.NewString()
	metadata := map[string]any{}
	metaJSON, _ := json.Marshal(metadata)
	row := r.db.QueryRowContext(ctx, `
INSERT INTO sessions (
  session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, metadata_json, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
RETURNING session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, metadata_json, version, created_at, updated_at
`, id, nil, title, goal, string(session.PhaseReceived), nil, nil, 0, string(session.ExecutionIdle), nil, nil, nil, nil, nil, now, nil, string(metaJSON), 1, now, now)
	st, err := scanState(row.Scan)
	if err != nil {
		return session.State{}, err
	}
	return st, nil
}

func (r *Repo) Get(id string) (session.State, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, metadata_json, version, created_at, updated_at
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
	result, err := r.db.ExecContext(ctx, `
UPDATE sessions
SET task_id = $2,
    title = $3,
    goal = $4,
    phase = $5,
    current_step_id = $6,
    summary = $7,
    retry_count = $8,
    execution_state = $9,
    in_flight_step_id = $10,
    pending_approval_id = $11,
    lease_id = $12,
    lease_claimed_at = $13,
    lease_expires_at = $14,
    last_heartbeat_at = $15,
    interrupted_at = $16,
    metadata_json = $17,
    version = $18,
    updated_at = $19
WHERE session_id = $1 AND version = $20
`, next.SessionID, nullable(next.TaskID), next.Title, nullable(next.Goal), string(next.Phase), nullable(next.CurrentStepID), nullable(next.Summary), next.RetryCount, string(next.ExecutionState), nullable(next.InFlightStepID), nullable(next.PendingApprovalID), nullable(next.LeaseID), nullableInt64(next.LeaseClaimedAt), nullableInt64(next.LeaseExpiresAt), nullableInt64(next.LastHeartbeatAt), nullableInt64(next.InterruptedAt), string(metaJSON), next.Version, next.UpdatedAt, next.Version-1)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows > 0 {
		return nil
	}
	return r.classifyUpdateErr(ctx, next.SessionID)
}

func (r *Repo) ClaimNext(mode session.ClaimMode, leaseID string, claimedAt, expiresAt int64) (session.State, bool, error) {
	ctx := context.Background()
	condition := claimCondition(mode)
	if condition == "" {
		return session.State{}, false, nil
	}
	row := r.db.QueryRowContext(ctx, `
UPDATE sessions
SET lease_id = $1,
    lease_claimed_at = $2,
    lease_expires_at = $3,
    last_heartbeat_at = $2,
    version = version + 1,
    updated_at = $2
WHERE session_id = (
  SELECT session_id
  FROM sessions
  WHERE `+condition+`
    AND (lease_id IS NULL OR lease_expires_at IS NULL OR lease_expires_at <= $2)
  ORDER BY created_at ASC, session_id ASC
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
RETURNING session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, metadata_json, version, created_at, updated_at
`, leaseID, claimedAt, expiresAt)
	st, err := scanState(row.Scan)
	if errors.Is(err, session.ErrSessionNotFound) {
		return session.State{}, false, nil
	}
	if err != nil {
		return session.State{}, false, err
	}
	return st, true, nil
}

func (r *Repo) RenewLease(sessionID, leaseID string, now, expiresAt int64) (session.State, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
UPDATE sessions
SET lease_expires_at = $3,
    last_heartbeat_at = $4,
    version = version + 1,
    updated_at = $4
WHERE session_id = $1
  AND lease_id = $2
  AND lease_expires_at > $4
RETURNING session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, metadata_json, version, created_at, updated_at
`, sessionID, leaseID, expiresAt, now)
	st, err := scanState(row.Scan)
	if err == nil {
		return st, nil
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		return session.State{}, err
	}
	return session.State{}, r.classifyLeaseErr(ctx, sessionID)
}

func (r *Repo) ReleaseLease(sessionID, leaseID string, now int64) (session.State, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
UPDATE sessions
SET lease_id = NULL,
    lease_claimed_at = NULL,
    lease_expires_at = NULL,
    version = version + 1,
    updated_at = $3
WHERE session_id = $1
  AND lease_id = $2
  AND lease_expires_at > $3
RETURNING session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, metadata_json, version, created_at, updated_at
`, sessionID, leaseID, now)
	st, err := scanState(row.Scan)
	if err == nil {
		return st, nil
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		return session.State{}, err
	}
	return session.State{}, r.classifyLeaseErr(ctx, sessionID)
}

func (r *Repo) List() ([]session.State, error) {
	ctx := context.Background()
	rows, err := r.db.QueryContext(ctx, `
SELECT session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, metadata_json, version, created_at, updated_at
FROM sessions
ORDER BY updated_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []session.State{}
	for rows.Next() {
		st, err := scanState(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
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

func scanState(scan scanner) (session.State, error) {
	var st session.State
	var taskID, goal, currentStepID, summary, executionState, inFlightStepID, pendingApprovalID, leaseID sqlNullString
	var leaseClaimedAt, leaseExpiresAt, lastHeartbeatAt, interruptedAt sqlNullInt64
	var phase string
	var metaRaw string
	if err := scan(&st.SessionID, &taskID, &st.Title, &goal, &phase, &currentStepID, &summary, &st.RetryCount, &executionState, &inFlightStepID, &pendingApprovalID, &leaseID, &leaseClaimedAt, &leaseExpiresAt, &lastHeartbeatAt, &interruptedAt, &metaRaw, &st.Version, &st.CreatedAt, &st.UpdatedAt); err != nil {
		return session.State{}, translateErr(err)
	}
	st.TaskID = taskID.String
	st.Goal = goal.String
	st.Phase = session.Phase(phase)
	st.CurrentStepID = currentStepID.String
	st.Summary = summary.String
	st.ExecutionState = session.ExecutionState(executionState.String)
	st.InFlightStepID = inFlightStepID.String
	st.PendingApprovalID = pendingApprovalID.String
	st.LeaseID = leaseID.String
	st.LeaseClaimedAt = leaseClaimedAt.Int64
	st.LeaseExpiresAt = leaseExpiresAt.Int64
	st.LastHeartbeatAt = lastHeartbeatAt.Int64
	st.InterruptedAt = interruptedAt.Int64
	if metaRaw != "" {
		_ = json.Unmarshal([]byte(metaRaw), &st.Metadata)
	}
	if st.Metadata == nil {
		st.Metadata = map[string]any{}
	}
	return st, nil
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

func translateErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return session.ErrSessionNotFound
	}
	return err
}

func (r *Repo) classifyUpdateErr(ctx context.Context, sessionID string) error {
	row := r.db.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE session_id = $1`, sessionID)
	var exists int
	if err := row.Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session.ErrSessionNotFound
		}
		return err
	}
	return session.ErrSessionVersionConflict
}

func (r *Repo) classifyLeaseErr(ctx context.Context, sessionID string) error {
	row := r.db.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE session_id = $1`, sessionID)
	var exists int
	if err := row.Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session.ErrSessionNotFound
		}
		return err
	}
	return session.ErrSessionLeaseNotHeld
}

func claimCondition(mode session.ClaimMode) string {
	switch mode {
	case session.ClaimModeRunnable:
		return "phase NOT IN ('complete', 'failed', 'aborted') AND execution_state = 'idle' AND (pending_approval_id IS NULL OR pending_approval_id = '')"
	case session.ClaimModeRecoverable:
		return "phase NOT IN ('complete', 'failed', 'aborted') AND execution_state IN ('in_flight', 'interrupted') AND (pending_approval_id IS NULL OR pending_approval_id = '')"
	default:
		return ""
	}
}
