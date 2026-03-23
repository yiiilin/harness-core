package contextrepo_test

import (
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/yiiilin/harness-core/internal/postgres/contextrepo"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func TestSummaryRepoPersistsExtendedFieldsAndStableOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := contextrepo.New(db)
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO context_summaries (
  summary_id, session_id, task_id, trigger, supersedes_summary_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING sequence
`)).
		WithArgs("ctx1", "sess1", "task1", string(hruntime.CompactionTriggerExecute), "ctx0", "truncate", sqlmock.AnyArg(), sqlmock.AnyArg(), 100, 40, int64(123)).
		WillReturnRows(sqlmock.NewRows([]string{"sequence"}).AddRow(int64(7)))

	created, err := repo.Create(hruntime.ContextSummary{
		SummaryID:           "ctx1",
		SessionID:           "sess1",
		TaskID:              "task1",
		Trigger:             hruntime.CompactionTriggerExecute,
		SupersedesSummaryID: "ctx0",
		Strategy:            "truncate",
		Summary:             map[string]any{"goal": "demo"},
		Metadata:            map[string]any{"k": "v"},
		OriginalBytes:       100,
		CompactedBytes:      40,
		CreatedAt:           123,
	})
	if err != nil {
		t.Fatalf("create summary: %v", err)
	}
	if created.Sequence != 7 {
		t.Fatalf("expected sequence to be returned from storage, got %#v", created)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT summary_id, session_id, task_id, sequence, trigger, supersedes_summary_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
FROM context_summaries
WHERE summary_id = $1
`)).
		WithArgs("ctx1").
		WillReturnRows(sqlmock.NewRows([]string{"summary_id", "session_id", "task_id", "sequence", "trigger", "supersedes_summary_id", "strategy", "summary_json", "metadata_json", "original_bytes", "compacted_bytes", "created_at"}).
			AddRow("ctx1", "sess1", "task1", int64(7), string(hruntime.CompactionTriggerExecute), "ctx0", "truncate", `{"goal":"demo"}`, `{"k":"v"}`, 100, 40, int64(123)))

	got, err := repo.Get("ctx1")
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if got.Trigger != hruntime.CompactionTriggerExecute || got.SupersedesSummaryID != "ctx0" || got.Sequence != 7 {
		t.Fatalf("expected extended summary fields from storage, got %#v", got)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT summary_id, session_id, task_id, sequence, trigger, supersedes_summary_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
FROM context_summaries
WHERE session_id = $1
ORDER BY created_at ASC, sequence ASC`)).
		WithArgs("sess1").
		WillReturnRows(sqlmock.NewRows([]string{"summary_id", "session_id", "task_id", "sequence", "trigger", "supersedes_summary_id", "strategy", "summary_json", "metadata_json", "original_bytes", "compacted_bytes", "created_at"}).
			AddRow("ctx1", "sess1", "task1", int64(1), string(hruntime.CompactionTriggerPlan), "", "truncate", `{"goal":"demo"}`, `{}`, 200, 50, int64(500)).
			AddRow("ctx2", "sess1", "task1", int64(2), string(hruntime.CompactionTriggerExecute), "ctx1", "truncate", `{"goal":"demo"}`, `{}`, 150, 40, int64(500)))

	items, err := repo.List("sess1")
	if err != nil {
		t.Fatalf("list summaries: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two summaries, got %#v", items)
	}
	if items[0].SummaryID != "ctx1" || items[1].SummaryID != "ctx2" {
		t.Fatalf("expected stable ordering by created_at and sequence, got %#v", items)
	}
	if items[1].Trigger != hruntime.CompactionTriggerExecute || items[1].SupersedesSummaryID != "ctx1" {
		t.Fatalf("expected list to hydrate extended fields, got %#v", items[1])
	}
}

func TestSummaryRepoCreateAndListReturnStorageErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	repo := contextrepo.New(db)
	createBoom := errors.New("insert failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
INSERT INTO context_summaries (
  summary_id, session_id, task_id, trigger, supersedes_summary_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING sequence
`)).WillReturnError(createBoom)
	if _, err := repo.Create(hruntime.ContextSummary{SummaryID: "ctx1", CreatedAt: 1}); !errors.Is(err, createBoom) {
		t.Fatalf("expected create storage error, got %v", err)
	}

	listBoom := errors.New("list failed")
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT summary_id, session_id, task_id, sequence, trigger, supersedes_summary_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
FROM context_summaries
WHERE session_id = $1
ORDER BY created_at ASC, sequence ASC`)).WithArgs("sess1").WillReturnError(listBoom)
	if _, err := repo.List("sess1"); !errors.Is(err, listBoom) {
		t.Fatalf("expected list storage error, got %v", err)
	}
}
