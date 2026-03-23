package contextrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	hcontextsummary "github.com/yiiilin/harness-core/pkg/harness/contextsummary"
)

type SummaryRepo struct{ db postgres.DBTX }

func New(db postgres.DBTX) *SummaryRepo { return &SummaryRepo{db: db} }

func (r *SummaryRepo) Create(spec hcontextsummary.Summary) (hcontextsummary.Summary, error) {
	if spec.SummaryID == "" {
		spec.SummaryID = "ctx_" + uuid.NewString()
	}
	if spec.CreatedAt == 0 {
		spec.CreatedAt = time.Now().UnixMilli()
	}
	ctx := context.Background()
	summaryJSON, _ := json.Marshal(spec.Summary)
	metadataJSON, _ := json.Marshal(spec.Metadata)
	row := r.db.QueryRowContext(ctx, `
INSERT INTO context_summaries (
  summary_id, session_id, task_id, trigger, supersedes_summary_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING sequence
`, spec.SummaryID, nullable(spec.SessionID), nullable(spec.TaskID), nullable(string(spec.Trigger)), nullable(spec.SupersedesSummaryID), nullable(spec.Strategy), nullableJSON(summaryJSON), nullableJSON(metadataJSON), spec.OriginalBytes, spec.CompactedBytes, spec.CreatedAt)
	if err := row.Scan(&spec.Sequence); err != nil {
		return hcontextsummary.Summary{}, err
	}
	return spec, nil
}

func (r *SummaryRepo) Get(id string) (hcontextsummary.Summary, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT summary_id, session_id, task_id, sequence, trigger, supersedes_summary_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
FROM context_summaries
WHERE summary_id = $1
`, id)

	var item hcontextsummary.Summary
	var sessionID, taskID, trigger, supersedesSummaryID, strategy, summaryRaw, metadataRaw sql.NullString
	if err := row.Scan(&item.SummaryID, &sessionID, &taskID, &item.Sequence, &trigger, &supersedesSummaryID, &strategy, &summaryRaw, &metadataRaw, &item.OriginalBytes, &item.CompactedBytes, &item.CreatedAt); err != nil {
		return hcontextsummary.Summary{}, err
	}
	item.SessionID = sessionID.String
	item.TaskID = taskID.String
	item.Trigger = hcontextsummary.Trigger(trigger.String)
	item.SupersedesSummaryID = supersedesSummaryID.String
	item.Strategy = strategy.String
	if summaryRaw.String != "" {
		_ = json.Unmarshal([]byte(summaryRaw.String), &item.Summary)
	}
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &item.Metadata)
	}
	return item, nil
}

func (r *SummaryRepo) List(sessionID string) ([]hcontextsummary.Summary, error) {
	ctx := context.Background()
	query := `
SELECT summary_id, session_id, task_id, sequence, trigger, supersedes_summary_id, strategy, summary_json, metadata_json, original_bytes, compacted_bytes, created_at
FROM context_summaries
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY created_at ASC, sequence ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []hcontextsummary.Summary{}
	for rows.Next() {
		var item hcontextsummary.Summary
		var rawSessionID, taskID, trigger, supersedesSummaryID, strategy, summaryRaw, metadataRaw sql.NullString
		if err := rows.Scan(&item.SummaryID, &rawSessionID, &taskID, &item.Sequence, &trigger, &supersedesSummaryID, &strategy, &summaryRaw, &metadataRaw, &item.OriginalBytes, &item.CompactedBytes, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.SessionID = rawSessionID.String
		item.TaskID = taskID.String
		item.Trigger = hcontextsummary.Trigger(trigger.String)
		item.SupersedesSummaryID = supersedesSummaryID.String
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
		return nil, err
	}
	return out, nil
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
