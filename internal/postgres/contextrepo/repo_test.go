package contextrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/contextrepo"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestSummaryRepoCreateAndListReturnStorageErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := contextrepo.New(db)
	createBoom := errors.New("insert failed")
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO context_summaries (
  summary_id, session_id, task_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`)).WillReturnError(createBoom)
	if _, err := repo.Create(hruntime.ContextSummary{SummaryID: "ctx1", CreatedAt: 1}); !errors.Is(err, createBoom) {
		t.Fatalf("expected create storage error, got %v", err)
	}

	listBoom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT summary_id, session_id, task_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
FROM context_summaries
WHERE session_id = $1
ORDER BY created_at ASC`)).WithArgs("sess1").WillReturnError(listBoom)
	if _, err := repo.List("sess1"); !errors.Is(err, listBoom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}
