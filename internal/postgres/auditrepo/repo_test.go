package auditrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/auditrepo"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
)

func TestAuditRepoEmitAndList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := auditrepo.New(db)

	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO audit_events (
  event_id, type, session_id, task_id, planning_id, step_id, attempt_id, action_id, trace_id, causation_id, payload_json, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`)).WillReturnResult(sqlmock.NewResult(0, 1))
	err = repo.Emit(audit.Event{Type: audit.EventToolCalled, SessionID: "sess1", TaskID: "task1", PlanningID: "pln1", StepID: "step1", AttemptID: "attempt1", ActionID: "action1", TraceID: "trace1", CausationID: "attempt1", Payload: map[string]any{"tool_name": "shell.exec"}, CreatedAt: 1})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	listRows := sqlmock.NewRows([]string{"event_id", "type", "session_id", "task_id", "planning_id", "step_id", "attempt_id", "action_id", "trace_id", "causation_id", "payload_json", "created_at"}).
		AddRow("evt1", "tool.called", "sess1", "task1", "pln1", "step1", "attempt1", "action1", "trace1", "attempt1", `{"tool_name":"shell.exec"}`, int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT event_id, type, session_id, task_id, planning_id, step_id, attempt_id, action_id, trace_id, causation_id, payload_json, created_at
FROM audit_events
WHERE session_id = $1
ORDER BY created_at ASC`)).WithArgs("sess1").WillReturnRows(listRows)
	items, err := repo.List("sess1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 event, got %d", len(items))
	}
	if items[0].Type != audit.EventToolCalled || items[0].SessionID != "sess1" {
		t.Fatalf("unexpected event: %#v", items[0])
	}

	allRows := sqlmock.NewRows([]string{"event_id", "type", "session_id", "task_id", "planning_id", "step_id", "attempt_id", "action_id", "trace_id", "causation_id", "payload_json", "created_at"}).
		AddRow("evt1", "tool.called", "sess1", "task1", "pln1", "step1", "attempt1", "action1", "trace1", "attempt1", `{"tool_name":"shell.exec"}`, int64(1)).
		AddRow("evt2", "verify.completed", "sess1", "task1", "pln1", "step1", "attempt1", "action1", "trace1", "action1", `{"success":true}`, int64(2))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT event_id, type, session_id, task_id, planning_id, step_id, attempt_id, action_id, trace_id, causation_id, payload_json, created_at
FROM audit_events
ORDER BY created_at ASC`)).WillReturnRows(allRows)
	all, err := repo.List("")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAuditRepoListReturnsStorageError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := auditrepo.New(db)
	boom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT event_id, type, session_id, task_id, planning_id, step_id, attempt_id, action_id, trace_id, causation_id, payload_json, created_at
FROM audit_events
WHERE session_id = $1
ORDER BY created_at ASC`)).WithArgs("sess1").WillReturnError(boom)
	if _, err := repo.List("sess1"); !errors.Is(err, boom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}
