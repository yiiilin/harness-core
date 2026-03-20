package approvalrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/approvalrepo"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
)

func TestApprovalRepoCreateGetUpdateList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := approvalrepo.New(db)

	createRows := sqlmock.NewRows([]string{"approval_id", "session_id", "task_id", "step_id", "tool_name", "reason", "matched_rule", "status", "reply", "step_json", "metadata_json", "requested_at", "responded_at", "consumed_at", "version", "created_at", "updated_at"}).
		AddRow("apv1", "sess1", nil, "step_1", "shell.exec", "needs approval", "test/ask", "pending", nil, `{}`, `{}`, int64(1), nil, nil, int64(1), int64(1), int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO approvals (
  approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, version, created_at, updated_at
`)).WillReturnRows(createRows)

	created, err := repo.CreatePending(approval.Request{
		SessionID:   "sess1",
		StepID:      "step_1",
		ToolName:    "shell.exec",
		Reason:      "needs approval",
		MatchedRule: "test/ask",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1, got %d", created.Version)
	}

	getRows := sqlmock.NewRows([]string{"approval_id", "session_id", "task_id", "step_id", "tool_name", "reason", "matched_rule", "status", "reply", "step_json", "metadata_json", "requested_at", "responded_at", "consumed_at", "version", "created_at", "updated_at"}).
		AddRow("apv1", "sess1", nil, "step_1", "shell.exec", "needs approval", "test/ask", "pending", nil, `{}`, `{}`, int64(1), nil, nil, int64(1), int64(1), int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, version, created_at, updated_at
FROM approvals WHERE approval_id = $1
`)).WithArgs("apv1").WillReturnRows(getRows)

	got, err := repo.Get("apv1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.StepID != "step_1" || got.ToolName != "shell.exec" {
		t.Fatalf("unexpected approval: %#v", got)
	}

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE approvals
SET session_id = $2,
    task_id = $3,
    step_id = $4,
    tool_name = $5,
    reason = $6,
    matched_rule = $7,
    status = $8,
    reply = $9,
    step_json = $10,
    metadata_json = $11,
    requested_at = $12,
    responded_at = $13,
    consumed_at = $14,
    version = $15,
    updated_at = $16
WHERE approval_id = $1 AND version = $17
`)).WillReturnResult(sqlmock.NewResult(0, 1))

	got.Status = approval.StatusApproved
	got.Reply = approval.ReplyOnce
	got.Version++
	if err := repo.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}

	listRows := sqlmock.NewRows([]string{"approval_id", "session_id", "task_id", "step_id", "tool_name", "reason", "matched_rule", "status", "reply", "step_json", "metadata_json", "requested_at", "responded_at", "consumed_at", "version", "created_at", "updated_at"}).
		AddRow("apv1", "sess1", nil, "step_1", "shell.exec", "needs approval", "test/ask", "approved", "once", `{}`, `{}`, int64(1), int64(2), nil, int64(2), int64(1), int64(2))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, version, created_at, updated_at
FROM approvals
WHERE session_id = $1
ORDER BY requested_at ASC`)).WithArgs("sess1").WillReturnRows(listRows)

	items, err := repo.List("sess1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].Status != approval.StatusApproved || items[0].Version != 2 {
		t.Fatalf("unexpected list result: %#v", items)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

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
  approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, version, created_at, updated_at
`)).WillReturnError(createBoom)
	if _, err := repo.CreatePending(approval.Request{SessionID: "sess1"}); !errors.Is(err, createBoom) {
		t.Fatalf("expected create storage error, got %v", err)
	}

	listBoom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, version, created_at, updated_at
FROM approvals
WHERE session_id = $1
ORDER BY requested_at ASC`)).WithArgs("sess1").WillReturnError(listBoom)
	if _, err := repo.List("sess1"); !errors.Is(err, listBoom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}

func TestApprovalRepoUpdateReturnsVersionConflictOrNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := approvalrepo.New(db)
	record := approval.Record{
		ApprovalID:  "apv1",
		SessionID:   "sess1",
		Status:      approval.StatusApproved,
		Reply:       approval.ReplyOnce,
		RequestedAt: 1,
		CreatedAt:   1,
		UpdatedAt:   1,
		Version:     2,
	}

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE approvals
SET session_id = $2,
    task_id = $3,
    step_id = $4,
    tool_name = $5,
    reason = $6,
    matched_rule = $7,
    status = $8,
    reply = $9,
    step_json = $10,
    metadata_json = $11,
    requested_at = $12,
    responded_at = $13,
    consumed_at = $14,
    version = $15,
    updated_at = $16
WHERE approval_id = $1 AND version = $17
`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM approvals WHERE approval_id = $1`)).
		WithArgs("apv1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
	if err := repo.Update(record); !errors.Is(err, approval.ErrApprovalVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE approvals
SET session_id = $2,
    task_id = $3,
    step_id = $4,
    tool_name = $5,
    reason = $6,
    matched_rule = $7,
    status = $8,
    reply = $9,
    step_json = $10,
    metadata_json = $11,
    requested_at = $12,
    responded_at = $13,
    consumed_at = $14,
    version = $15,
    updated_at = $16
WHERE approval_id = $1 AND version = $17
`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM approvals WHERE approval_id = $1`)).
		WithArgs("apv1").
		WillReturnError(sqlmock.ErrCancelled)
	if err := repo.Update(record); !errors.Is(err, approval.ErrApprovalNotFound) && !errors.Is(err, sqlmock.ErrCancelled) {
		t.Fatalf("expected not found probe failure or not found, got %v", err)
	}
}
