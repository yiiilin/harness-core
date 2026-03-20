package capabilityrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/capabilityrepo"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
)

func TestSnapshotRepoCreateAndListReturnStorageErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := capabilityrepo.New(db)
	createBoom := errors.New("insert failed")
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO capability_snapshots (
  snapshot_id, session_id, task_id, plan_id, step_id, view_id, scope, tool_name, version, capability_type, risk_level, metadata_json, resolved_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`)).WillReturnError(createBoom)
	if _, err := repo.Create(capability.Snapshot{SnapshotID: "cap1", ToolName: "shell.exec", ResolvedAt: 1}); !errors.Is(err, createBoom) {
		t.Fatalf("expected create storage error, got %v", err)
	}

	listBoom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT snapshot_id, session_id, task_id, plan_id, step_id, view_id, scope, tool_name, version, capability_type, risk_level, metadata_json, resolved_at
FROM capability_snapshots
WHERE session_id = $1
ORDER BY resolved_at ASC`)).WithArgs("sess1").WillReturnError(listBoom)
	if _, err := repo.List("sess1"); !errors.Is(err, listBoom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}
