package executionrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/executionrepo"
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
SELECT attempt_id, session_id, task_id, step_id, approval_id, trace_id, status, step_json, metadata_json, started_at, finished_at
FROM attempts WHERE attempt_id = $1
`)).WithArgs("att1").WillReturnError(boom)

	_, err = repo.Get("att1")
	if !errors.Is(err, boom) {
		t.Fatalf("expected underlying storage error, got %v", err)
	}
}
