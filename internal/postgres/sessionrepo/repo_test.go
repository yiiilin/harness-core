package sessionrepo_test

import (
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

	createRows := sqlmock.NewRows([]string{"session_id", "task_id", "parent_session_id", "title", "goal", "phase", "current_step_id", "summary", "retry_count", "execution_state", "in_flight_step_id", "last_heartbeat_at", "interrupted_at", "metadata_json", "created_at", "updated_at"}).
		AddRow("sess1", nil, nil, "demo", "goal", "received", nil, nil, 0, "idle", nil, int64(1), nil, "{}", int64(1), int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO sessions (
  session_id, task_id, parent_session_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, last_heartbeat_at, interrupted_at, metadata_json, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING session_id, task_id, parent_session_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, last_heartbeat_at, interrupted_at, metadata_json, created_at, updated_at
`)).WillReturnRows(createRows)
	created := repo.Create("demo", "goal")
	if created.SessionID != "sess1" {
		t.Fatalf("expected sess1, got %s", created.SessionID)
	}

	getRows := sqlmock.NewRows([]string{"session_id", "task_id", "parent_session_id", "title", "goal", "phase", "current_step_id", "summary", "retry_count", "execution_state", "in_flight_step_id", "last_heartbeat_at", "interrupted_at", "metadata_json", "created_at", "updated_at"}).
		AddRow("sess1", "task1", nil, "demo", "goal", "plan", nil, nil, 1, "idle", nil, int64(2), nil, "{}", int64(1), int64(2))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT session_id, task_id, parent_session_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, last_heartbeat_at, interrupted_at, metadata_json, created_at, updated_at
FROM sessions WHERE session_id = $1
`)).WithArgs("sess1").WillReturnRows(getRows)
	got, err := repo.Get("sess1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TaskID != "task1" || got.Phase != session.Phase("plan") {
		t.Fatalf("unexpected session: %#v", got)
	}

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE sessions
SET task_id = $2,
    parent_session_id = $3,
    title = $4,
    goal = $5,
    phase = $6,
    current_step_id = $7,
    summary = $8,
    retry_count = $9,
    execution_state = $10,
    in_flight_step_id = $11,
    last_heartbeat_at = $12,
    interrupted_at = $13,
    metadata_json = $14,
    updated_at = $15
WHERE session_id = $1
`)).WillReturnResult(sqlmock.NewResult(0, 1))
	got.Summary = "done"
	if err := repo.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}

	listRows := sqlmock.NewRows([]string{"session_id", "task_id", "parent_session_id", "title", "goal", "phase", "current_step_id", "summary", "retry_count", "execution_state", "in_flight_step_id", "last_heartbeat_at", "interrupted_at", "metadata_json", "created_at", "updated_at"}).
		AddRow("sess1", "task1", nil, "demo", "goal", "plan", nil, "done", 1, "idle", nil, int64(3), nil, "{}", int64(1), int64(3))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT session_id, task_id, parent_session_id, title, goal, phase, current_step_id, summary, retry_count, execution_state, in_flight_step_id, last_heartbeat_at, interrupted_at, metadata_json, created_at, updated_at
FROM sessions
ORDER BY updated_at DESC
`)).WillReturnRows(listRows)
	items := repo.List()
	if len(items) != 1 || items[0].Summary != "done" {
		t.Fatalf("unexpected list result: %#v", items)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
