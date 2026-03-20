package approvalrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/approvalrepo"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
)

func TestApprovalRepoCreateAndListReturnStorageErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := approvalrepo.New(db)
	createBoom := errors.New("insert failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO approvals (
  approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, created_at, updated_at
`)).WillReturnError(createBoom)
	if _, err := repo.CreatePending(approval.Request{SessionID: "sess1"}); !errors.Is(err, createBoom) {
		t.Fatalf("expected create storage error, got %v", err)
	}

	listBoom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, created_at, updated_at
FROM approvals
WHERE session_id = $1
ORDER BY requested_at ASC`)).WithArgs("sess1").WillReturnError(listBoom)
	if _, err := repo.List("sess1"); !errors.Is(err, listBoom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}
