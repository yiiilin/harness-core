package planningrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/planningrepo"
	hplanning "github.com/yiiilin/harness-core/pkg/harness/planning"
)

func TestPlanningRepoCreateGetAndList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := planningrepo.New(db)

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO planning_records (
  planning_id, session_id, task_id, status, reason, error, plan_id, plan_revision, capability_view_id, context_summary_id, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`)).
		WithArgs("planrec_1", "sess1", "task1", "completed", "initial planning", nil, "plan1", 1, "view1", "ctx1", `{"origin":"test"}`, int64(10), int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	created, err := repo.Create(hplanning.Record{
		PlanningID:       "planrec_1",
		SessionID:        "sess1",
		TaskID:           "task1",
		Status:           hplanning.StatusCompleted,
		Reason:           "initial planning",
		PlanID:           "plan1",
		PlanRevision:     1,
		CapabilityViewID: "view1",
		ContextSummaryID: "ctx1",
		Metadata:         map[string]any{"origin": "test"},
		StartedAt:        10,
		FinishedAt:       20,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.PlanningID != "planrec_1" {
		t.Fatalf("expected planning id to be preserved, got %#v", created)
	}

	row := sqlmock.NewRows([]string{"planning_id", "session_id", "task_id", "status", "reason", "error", "plan_id", "plan_revision", "capability_view_id", "context_summary_id", "metadata_json", "started_at", "finished_at"}).
		AddRow("planrec_1", "sess1", "task1", "completed", "initial planning", nil, "plan1", 1, "view1", "ctx1", `{"origin":"test"}`, int64(10), int64(20))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT planning_id, session_id, task_id, status, reason, error, plan_id, plan_revision, capability_view_id, context_summary_id, metadata_json, started_at, finished_at
FROM planning_records
WHERE planning_id = $1
`)).WithArgs("planrec_1").WillReturnRows(row)

	got, err := repo.Get("planrec_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PlanID != "plan1" || got.CapabilityViewID != "view1" || got.ContextSummaryID != "ctx1" {
		t.Fatalf("unexpected planning record: %#v", got)
	}

	rows := sqlmock.NewRows([]string{"planning_id", "session_id", "task_id", "status", "reason", "error", "plan_id", "plan_revision", "capability_view_id", "context_summary_id", "metadata_json", "started_at", "finished_at"}).
		AddRow("planrec_1", "sess1", "task1", "completed", "initial planning", nil, "plan1", 1, "view1", "ctx1", `{"origin":"test"}`, int64(10), int64(20)).
		AddRow("planrec_2", "sess1", "task1", "failed", "replan", "planner failed", nil, 0, "view2", "ctx2", nil, int64(30), int64(40))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT planning_id, session_id, task_id, status, reason, error, plan_id, plan_revision, capability_view_id, context_summary_id, metadata_json, started_at, finished_at
FROM planning_records
WHERE session_id = $1
ORDER BY started_at ASC, plan_revision ASC, planning_id ASC
`)).WithArgs("sess1").WillReturnRows(rows)

	items, err := repo.List("sess1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 || items[1].Status != hplanning.StatusFailed {
		t.Fatalf("unexpected planning record list: %#v", items)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPlanningRepoCreateAndListReturnStorageErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := planningrepo.New(db)

	createBoom := errors.New("create failed")
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO planning_records (
  planning_id, session_id, task_id, status, reason, error, plan_id, plan_revision, capability_view_id, context_summary_id, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`)).WillReturnError(createBoom)

	if _, err := repo.Create(hplanning.Record{PlanningID: "planrec_err", SessionID: "sess1", Status: hplanning.StatusFailed, StartedAt: 1}); !errors.Is(err, createBoom) {
		t.Fatalf("expected create storage error, got %v", err)
	}

	listBoom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT planning_id, session_id, task_id, status, reason, error, plan_id, plan_revision, capability_view_id, context_summary_id, metadata_json, started_at, finished_at
FROM planning_records
WHERE session_id = $1
ORDER BY started_at ASC, plan_revision ASC, planning_id ASC
`)).WithArgs("sess1").WillReturnError(listBoom)

	if _, err := repo.List("sess1"); !errors.Is(err, listBoom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}
