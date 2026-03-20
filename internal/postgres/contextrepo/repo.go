package contextrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

type SummaryRepo struct{ db postgres.DBTX }

func New(db postgres.DBTX) *SummaryRepo { return &SummaryRepo{db: db} }

func (r *SummaryRepo) Create(spec hruntime.ContextSummary) hruntime.ContextSummary {
	if spec.SummaryID == "" {
		spec.SummaryID = "ctx_" + uuid.NewString()
	}
	if spec.CreatedAt == 0 {
		spec.CreatedAt = time.Now().UnixMilli()
	}
	ctx := context.Background()
	summaryJSON, _ := json.Marshal(spec.Summary)
	metadataJSON, _ := json.Marshal(spec.Metadata)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO context_summaries (
  summary_id, session_id, task_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, spec.SummaryID, nullable(spec.SessionID), nullable(spec.TaskID), nullable(spec.Strategy), nullableJSON(summaryJSON), nullableJSON(metadataJSON), spec.OriginalBytes, spec.CompactedBytes, spec.CreatedAt)
	if err != nil {
		panic(err)
	}
	return spec
}

func (r *SummaryRepo) Get(id string) (hruntime.ContextSummary, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT summary_id, session_id, task_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
FROM context_summaries
WHERE summary_id = $1
`, id)

	var item hruntime.ContextSummary
	var sessionID, taskID, strategy, summaryRaw, metadataRaw sql.NullString
	if err := row.Scan(&item.SummaryID, &sessionID, &taskID, &strategy, &summaryRaw, &metadataRaw, &item.OriginalBytes, &item.CompactedBytes, &item.CreatedAt); err != nil {
		return hruntime.ContextSummary{}, err
	}
	item.SessionID = sessionID.String
	item.TaskID = taskID.String
	item.Strategy = strategy.String
	if summaryRaw.String != "" {
		_ = json.Unmarshal([]byte(summaryRaw.String), &item.Summary)
	}
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &item.Metadata)
	}
	return item, nil
}

func (r *SummaryRepo) List(sessionID string) []hruntime.ContextSummary {
	ctx := context.Background()
	query := `
SELECT summary_id, session_id, task_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
FROM context_summaries
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY created_at ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	out := []hruntime.ContextSummary{}
	for rows.Next() {
		var item hruntime.ContextSummary
		var rawSessionID, taskID, strategy, summaryRaw, metadataRaw sql.NullString
		if err := rows.Scan(&item.SummaryID, &rawSessionID, &taskID, &strategy, &summaryRaw, &metadataRaw, &item.OriginalBytes, &item.CompactedBytes, &item.CreatedAt); err != nil {
			panic(err)
		}
		item.SessionID = rawSessionID.String
		item.TaskID = taskID.String
		item.Strategy = strategy.String
		if summaryRaw.String != "" {
			_ = json.Unmarshal([]byte(summaryRaw.String), &item.Summary)
		}
		if metadataRaw.String != "" {
			_ = json.Unmarshal([]byte(metadataRaw.String), &item.Metadata)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return out
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableJSON(raw []byte) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}
