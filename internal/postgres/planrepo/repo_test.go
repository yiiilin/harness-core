package planrepo_test

import (
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/planrepo"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

func TestPlanRepoCreateGetUpdateList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := planrepo.New(db)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT plan_id, session_id, revision, status, change_reason, created_at, updated_at
FROM plans WHERE session_id = $1
ORDER BY revision ASC
`)).WithArgs("sess1").WillReturnRows(sqlmock.NewRows([]string{"plan_id", "session_id", "revision", "status", "change_reason", "created_at", "updated_at"}))

	createPlanRows := sqlmock.NewRows([]string{"plan_id", "session_id", "revision", "status", "change_reason", "created_at", "updated_at"}).
		AddRow("plan1", "sess1", 1, "active", "initial", int64(1), int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO plans (
  plan_id, session_id, revision, status, change_reason, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING plan_id, session_id, revision, status, change_reason, created_at, updated_at
`)).WillReturnRows(createPlanRows)
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO plan_steps (
  plan_id, step_index, step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`)).WillReturnResult(sqlmock.NewResult(0, 1))
	createdPlanRows := sqlmock.NewRows([]string{"plan_id", "session_id", "revision", "status", "change_reason", "created_at", "updated_at"}).
		AddRow("plan1", "sess1", 1, "active", "initial", int64(1), int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT plan_id, session_id, revision, status, change_reason, created_at, updated_at
FROM plans WHERE plan_id = $1
`)).WithArgs("plan1").WillReturnRows(createdPlanRows)
	createdStepRows := sqlmock.NewRows([]string{"step_id", "title", "action_json", "verify_json", "on_fail_json", "status", "attempt", "reason", "metadata_json", "started_at", "finished_at"}).
		AddRow("step1", "title", `{"tool_name":"shell.exec","args":{}}`, `{"mode":"all","checks":[]}`, `{"strategy":"abort"}`, "pending", 0, nil, "{}", nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
FROM plan_steps WHERE plan_id = $1
ORDER BY step_index ASC
`)).WithArgs("plan1").WillReturnRows(createdStepRows)

	pl := repo.Create("sess1", "initial", []plan.StepSpec{{
		StepID: "step1",
		Title:  "title",
		Action: action.Spec{ToolName: "shell.exec", Args: map[string]any{}},
		Verify: verify.Spec{Mode: verify.ModeAll, Checks: []verify.Check{}},
		OnFail: plan.OnFailSpec{Strategy: "abort"},
	}})
	if pl.PlanID != "plan1" {
		t.Fatalf("expected plan1, got %s", pl.PlanID)
	}

	getPlanRows := sqlmock.NewRows([]string{"plan_id", "session_id", "revision", "status", "change_reason", "created_at", "updated_at"}).
		AddRow("plan1", "sess1", 1, "active", "initial", int64(1), int64(2))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT plan_id, session_id, revision, status, change_reason, created_at, updated_at
FROM plans WHERE plan_id = $1
`)).WithArgs("plan1").WillReturnRows(getPlanRows)
	getStepRows := sqlmock.NewRows([]string{"step_id", "title", "action_json", "verify_json", "on_fail_json", "status", "attempt", "reason", "metadata_json", "started_at", "finished_at"}).
		AddRow("step1", "title", `{"tool_name":"shell.exec","args":{}}`, `{"mode":"all","checks":[]}`, `{"strategy":"abort"}`, "pending", 0, nil, "{}", nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
FROM plan_steps WHERE plan_id = $1
ORDER BY step_index ASC
`)).WithArgs("plan1").WillReturnRows(getStepRows)
	got, err := repo.Get("plan1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SessionID != "sess1" || got.Revision != 1 || len(got.Steps) != 1 {
		t.Fatalf("unexpected plan: %#v", got)
	}

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE plans
SET session_id = $2,
    revision = $3,
    status = $4,
    change_reason = $5,
    updated_at = $6
WHERE plan_id = $1
`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM plan_steps WHERE plan_id = $1`)).WithArgs("plan1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO plan_steps (
  plan_id, step_index, step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`)).WillReturnResult(sqlmock.NewResult(0, 1))
	got.Status = plan.StatusCompleted
	got.Steps[0].Status = plan.StepCompleted
	if err := repo.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}

	listPlanRows := sqlmock.NewRows([]string{"plan_id", "session_id", "revision", "status", "change_reason", "created_at", "updated_at"}).
		AddRow("plan1", "sess1", 1, "completed", "initial", int64(1), int64(3))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT plan_id, session_id, revision, status, change_reason, created_at, updated_at
FROM plans WHERE session_id = $1
ORDER BY revision ASC
`)).WithArgs("sess1").WillReturnRows(listPlanRows)
	listGetPlanRows := sqlmock.NewRows([]string{"plan_id", "session_id", "revision", "status", "change_reason", "created_at", "updated_at"}).
		AddRow("plan1", "sess1", 1, "completed", "initial", int64(1), int64(3))
	listStepRows := sqlmock.NewRows([]string{"step_id", "title", "action_json", "verify_json", "on_fail_json", "status", "attempt", "reason", "metadata_json", "started_at", "finished_at"}).
		AddRow("step1", "title", `{"tool_name":"shell.exec","args":{}}`, `{"mode":"all","checks":[]}`, `{"strategy":"abort"}`, "completed", 0, nil, "{}", nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT plan_id, session_id, revision, status, change_reason, created_at, updated_at
FROM plans WHERE plan_id = $1
`)).WithArgs("plan1").WillReturnRows(listGetPlanRows)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
FROM plan_steps WHERE plan_id = $1
ORDER BY step_index ASC
`)).WithArgs("plan1").WillReturnRows(listStepRows)
	items := repo.ListBySession("sess1")
	if len(items) != 1 || items[0].Status != plan.StatusCompleted {
		t.Fatalf("unexpected list result: %#v", items)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPlanRepoGetPreservesStoredStepOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := planrepo.New(db)
	planRows := sqlmock.NewRows([]string{"plan_id", "session_id", "revision", "status", "change_reason", "created_at", "updated_at"}).
		AddRow("plan1", "sess1", 1, "active", "ordered", int64(1), int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT plan_id, session_id, revision, status, change_reason, created_at, updated_at
FROM plans WHERE plan_id = $1
`)).WithArgs("plan1").WillReturnRows(planRows)
	stepRows := sqlmock.NewRows([]string{"step_id", "title", "action_json", "verify_json", "on_fail_json", "status", "attempt", "reason", "metadata_json", "started_at", "finished_at"}).
		AddRow("step_10", "ten", `{"tool_name":"shell.exec","args":{}}`, `{"mode":"all","checks":[]}`, `{"strategy":"abort"}`, "pending", 0, nil, "{}", nil, nil).
		AddRow("step_2", "two", `{"tool_name":"shell.exec","args":{}}`, `{"mode":"all","checks":[]}`, `{"strategy":"abort"}`, "pending", 0, nil, "{}", nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
FROM plan_steps WHERE plan_id = $1
ORDER BY step_index ASC
`)).WithArgs("plan1").WillReturnRows(stepRows)

	got, err := repo.Get("plan1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("expected two steps, got %#v", got.Steps)
	}
	if got.Steps[0].StepID != "step_10" || got.Steps[1].StepID != "step_2" {
		t.Fatalf("expected persisted order to be preserved, got %#v", got.Steps)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
