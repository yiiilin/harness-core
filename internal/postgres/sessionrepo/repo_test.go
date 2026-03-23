package sessionrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/sessionrepo"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestSessionRepoCreateGetUpdateList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := sessionrepo.New(db)

	createRows := sqlmock.NewRows([]string{"session_id", "task_id", "title", "goal", "phase", "current_step_id", "summary", "retry_count", "execution_state", "in_flight_step_id", "pending_approval_id", "lease_id", "lease_claimed_at", "lease_expires_at", "last_heartbeat_at", "interrupted_at", "runtime_started_at", "metadata_json", "version", "created_at", "updated_at"}).
		AddRow("sess1", nil, "demo", "goal", "received", nil, nil, 0, "idle", nil, nil, nil, nil, nil, int64(1), nil, nil, "{}", int64(1), int64(1), int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO sessions (
  session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, runtime_started_at, metadata_json, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
RETURNING session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, runtime_started_at, metadata_json, version, created_at, updated_at
`)).WillReturnRows(createRows)
	created, err := repo.Create("demo", "goal")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.SessionID != "sess1" {
		t.Fatalf("expected sess1, got %s", created.SessionID)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1, got %d", created.Version)
	}
	if created.RuntimeStartedAt != 0 {
		t.Fatalf("expected create runtime_started_at to default to zero, got %#v", created)
	}

	getRows := sqlmock.NewRows([]string{"session_id", "task_id", "title", "goal", "phase", "current_step_id", "summary", "retry_count", "execution_state", "in_flight_step_id", "pending_approval_id", "lease_id", "lease_claimed_at", "lease_expires_at", "last_heartbeat_at", "interrupted_at", "runtime_started_at", "metadata_json", "version", "created_at", "updated_at"}).
		AddRow("sess1", "task1", "demo", "goal", "plan", nil, nil, 1, "idle", nil, nil, nil, nil, nil, int64(2), nil, int64(22), "{}", int64(1), int64(1), int64(2))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, runtime_started_at, metadata_json, version, created_at, updated_at
FROM sessions WHERE session_id = $1
`)).WithArgs("sess1").WillReturnRows(getRows)
	got, err := repo.Get("sess1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TaskID != "task1" || got.Phase != session.Phase("plan") {
		t.Fatalf("unexpected session: %#v", got)
	}
	if got.RuntimeStartedAt != 22 {
		t.Fatalf("expected runtime_started_at 22, got %#v", got)
	}

	mock.ExpectExec(regexp.QuoteMeta(`
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
    runtime_started_at = $17,
    metadata_json = $18,
    version = $19,
    updated_at = $20
WHERE session_id = $1 AND version = $21
`)).WillReturnResult(sqlmock.NewResult(0, 1))
	got.Summary = "done"
	got.RuntimeStartedAt = 44
	got.Version++
	if err := repo.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}

	listRows := sqlmock.NewRows([]string{"session_id", "task_id", "title", "goal", "phase", "current_step_id", "summary", "retry_count", "execution_state", "in_flight_step_id", "pending_approval_id", "lease_id", "lease_claimed_at", "lease_expires_at", "last_heartbeat_at", "interrupted_at", "runtime_started_at", "metadata_json", "version", "created_at", "updated_at"}).
		AddRow("sess1", "task1", "demo", "goal", "plan", nil, "done", 1, "idle", nil, nil, nil, nil, nil, int64(3), nil, int64(44), "{}", int64(2), int64(1), int64(3))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, runtime_started_at, metadata_json, version, created_at, updated_at
FROM sessions
ORDER BY updated_at DESC
`)).WillReturnRows(listRows)
	items, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].Summary != "done" {
		t.Fatalf("unexpected list result: %#v", items)
	}
	if items[0].Version != 2 {
		t.Fatalf("expected list version 2, got %#v", items[0])
	}
	if items[0].RuntimeStartedAt != 44 {
		t.Fatalf("expected list runtime_started_at 44, got %#v", items[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSessionRepoGetReturnsStorageError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := sessionrepo.New(db)
	boom := errors.New("storage unavailable")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, runtime_started_at, metadata_json, version, created_at, updated_at
FROM sessions WHERE session_id = $1
`)).WithArgs("sess1").WillReturnError(boom)

	_, err = repo.Get("sess1")
	if !errors.Is(err, boom) {
		t.Fatalf("expected underlying storage error, got %v", err)
	}
}

func TestSessionRepoCreateAndListReturnStorageErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := sessionrepo.New(db)
	createBoom := errors.New("insert failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO sessions (
  session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, runtime_started_at, metadata_json, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
RETURNING session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, runtime_started_at, metadata_json, version, created_at, updated_at
`)).WillReturnError(createBoom)
	if _, err := repo.Create("demo", "goal"); !errors.Is(err, createBoom) {
		t.Fatalf("expected create storage error, got %v", err)
	}

	listBoom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT session_id, task_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, pending_approval_id, lease_id, lease_claimed_at, lease_expires_at, last_heartbeat_at, interrupted_at, runtime_started_at, metadata_json, version, created_at, updated_at
FROM sessions
ORDER BY updated_at DESC
`)).WillReturnError(listBoom)
	if _, err := repo.List(); !errors.Is(err, listBoom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}

func TestSessionRepoUpdateReturnsVersionConflictOrNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := sessionrepo.New(db)
	state := session.State{
		SessionID:        "sess1",
		Title:            "demo",
		Goal:             "goal",
		Phase:            session.PhasePlan,
		ExecutionState:   session.ExecutionIdle,
		LastHeartbeatAt:  1,
		RuntimeStartedAt: 4,
		Metadata:         map[string]any{},
		CreatedAt:        1,
		UpdatedAt:        1,
		Version:          2,
	}

	mock.ExpectExec(regexp.QuoteMeta(`
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
    runtime_started_at = $17,
    metadata_json = $18,
    version = $19,
    updated_at = $20
WHERE session_id = $1 AND version = $21
`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM sessions WHERE session_id = $1`)).
		WithArgs("sess1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
	if err := repo.Update(state); !errors.Is(err, session.ErrSessionVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`
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
    runtime_started_at = $17,
    metadata_json = $18,
    version = $19,
    updated_at = $20
WHERE session_id = $1 AND version = $21
`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM sessions WHERE session_id = $1`)).
		WithArgs("sess1").
		WillReturnError(sqlmock.ErrCancelled)
	if err := repo.Update(state); !errors.Is(err, session.ErrSessionNotFound) && !errors.Is(err, sqlmock.ErrCancelled) {
		t.Fatalf("expected not found probe failure or not found, got %v", err)
	}
}
