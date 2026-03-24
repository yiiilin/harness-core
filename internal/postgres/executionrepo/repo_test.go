package executionrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/executionrepo"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestAttemptRepoGetReturnsStorageError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := executionrepo.NewAttemptStore(db)
	boom := errors.New("database offline")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT attempt_id, session_id, task_id, step_id, approval_id, cycle_id, trace_id, status, step_json, metadata_json, started_at, finished_at
FROM attempts WHERE attempt_id = $1
`)).WithArgs("att1").WillReturnError(boom)

	_, err = repo.Get("att1")
	if !errors.Is(err, boom) {
		t.Fatalf("expected underlying storage error, got %v", err)
	}
}

func TestAttemptRepoCreateAndListReturnStorageErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := executionrepo.NewAttemptStore(db)
	createBoom := errors.New("insert failed")
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO attempts (
  attempt_id, session_id, task_id, step_id, approval_id, cycle_id, trace_id, status, step_json, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`)).WillReturnError(createBoom)
	if _, err := repo.Create(execution.Attempt{AttemptID: "att1", SessionID: "sess1", Status: execution.AttemptBlocked, StartedAt: 1}); !errors.Is(err, createBoom) {
		t.Fatalf("expected create storage error, got %v", err)
	}

	listBoom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT attempt_id, session_id, task_id, step_id, approval_id, cycle_id, trace_id, status, step_json, metadata_json, started_at, finished_at
FROM attempts
WHERE session_id = $1
ORDER BY started_at ASC, COALESCE(step_id, '') ASC, attempt_id ASC`)).WithArgs("sess1").WillReturnError(listBoom)
	if _, err := repo.List("sess1"); !errors.Is(err, listBoom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}
