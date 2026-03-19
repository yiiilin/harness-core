package auditrepo_test

import (
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
  event_id, type, session_id, step_id, payload_json, created_at
) VALUES ($1, $2, $3, $4, $5, $6)
`)).WillReturnResult(sqlmock.NewResult(0, 1))
	err = repo.Emit(audit.Event{Type: audit.EventToolCalled, SessionID: "sess1", StepID: "step1", Payload: map[string]any{"tool_name": "shell.exec"}, CreatedAt: 1})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	listRows := sqlmock.NewRows([]string{"event_id", "type", "session_id", "step_id", "payload_json", "created_at"}).
		AddRow("evt1", "tool.called", "sess1", "step1", `{"tool_name":"shell.exec"}`, int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT event_id, type, session_id, step_id, payload_json, created_at
FROM audit_events
WHERE session_id = $1
ORDER BY created_at ASC`)).WithArgs("sess1").WillReturnRows(listRows)
	items := repo.List("sess1")
	if len(items) != 1 {
		t.Fatalf("expected 1 event, got %d", len(items))
	}
	if items[0].Type != audit.EventToolCalled || items[0].SessionID != "sess1" {
		t.Fatalf("unexpected event: %#v", items[0])
	}

	allRows := sqlmock.NewRows([]string{"event_id", "type", "session_id", "step_id", "payload_json", "created_at"}).
		AddRow("evt1", "tool.called", "sess1", "step1", `{"tool_name":"shell.exec"}`, int64(1)).
		AddRow("evt2", "verify.completed", "sess1", "step1", `{"success":true}`, int64(2))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT event_id, type, session_id, step_id, payload_json, created_at
FROM audit_events
ORDER BY created_at ASC`)).WillReturnRows(allRows)
	all := repo.List("")
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
