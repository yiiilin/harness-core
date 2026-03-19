package taskrepo_test

import (
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/taskrepo"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func TestTaskRepoCreateGetUpdateList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := taskrepo.New(db)

	createRows := sqlmock.NewRows([]string{"task_id", "task_type", "goal", "status", "session_id", "constraints_json", "metadata_json", "created_at", "updated_at"}).
		AddRow("task1", "demo", "goal", "received", nil, "{}", "{}", int64(1), int64(1))
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO tasks (
  task_id, task_type, goal, status, session_id, constraints_json, metadata_json, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING task_id, task_type, goal, status, session_id, constraints_json, metadata_json, created_at, updated_at
`)).WillReturnRows(createRows)
	created := repo.Create(task.Spec{TaskType: "demo", Goal: "goal"})
	if created.TaskID != "task1" {
		t.Fatalf("expected task1, got %s", created.TaskID)
	}

	getRows := sqlmock.NewRows([]string{"task_id", "task_type", "goal", "status", "session_id", "constraints_json", "metadata_json", "created_at", "updated_at"}).
		AddRow("task1", "demo", "goal", "running", "sess1", "{}", "{}", int64(1), int64(2))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT task_id, task_type, goal, status, session_id, constraints_json, metadata_json, created_at, updated_at
FROM tasks WHERE task_id = $1
`)).WithArgs("task1").WillReturnRows(getRows)
	got, err := repo.Get("task1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SessionID != "sess1" || got.Status != task.StatusRunning {
		t.Fatalf("unexpected task: %#v", got)
	}

	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE tasks
SET task_type = $2,
    goal = $3,
    status = $4,
    session_id = $5,
    constraints_json = $6,
    metadata_json = $7,
    updated_at = $8
WHERE task_id = $1
`)).WillReturnResult(sqlmock.NewResult(0, 1))
	got.Status = task.StatusCompleted
	if err := repo.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}

	listRows := sqlmock.NewRows([]string{"task_id", "task_type", "goal", "status", "session_id", "constraints_json", "metadata_json", "created_at", "updated_at"}).
		AddRow("task1", "demo", "goal", "completed", "sess1", "{}", "{}", int64(1), int64(3))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT task_id, task_type, goal, status, session_id, constraints_json, metadata_json, created_at, updated_at
FROM tasks
ORDER BY updated_at DESC
`)).WillReturnRows(listRows)
	items := repo.List()
	if len(items) != 1 || items[0].Status != task.StatusCompleted {
		t.Fatalf("unexpected list result: %#v", items)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
