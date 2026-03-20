package planningrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
)

type Repo struct {
	db postgres.DBTX
}

func New(db postgres.DBTX) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Create(spec planning.Record) (planning.Record, error) {
	if spec.PlanningID == "" {
		spec.PlanningID = "pln_" + uuid.NewString()
	}
	if spec.StartedAt == 0 {
		spec.StartedAt = time.Now().UnixMilli()
	}
	if spec.FinishedAt == 0 {
		spec.FinishedAt = spec.StartedAt
	}
	metadataJSON, _ := json.Marshal(spec.Metadata)
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO planning_records (
  planning_id, session_id, task_id, status, reason, error, plan_id, plan_revision, capability_view_id, context_summary_id, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`, spec.PlanningID, spec.SessionID, nullable(spec.TaskID), string(spec.Status), nullable(spec.Reason), nullable(spec.Error), nullable(spec.PlanID), spec.PlanRevision, nullable(spec.CapabilityViewID), nullable(spec.ContextSummaryID), nullableJSON(metadataJSON), spec.StartedAt, nullableInt64(spec.FinishedAt))
	if err != nil {
		return planning.Record{}, err
	}
	return spec, nil
}

func (r *Repo) Get(id string) (planning.Record, error) {
	row := r.db.QueryRowContext(context.Background(), `
SELECT planning_id, session_id, task_id, status, reason, error, plan_id, plan_revision, capability_view_id, context_summary_id, metadata_json, started_at, finished_at
FROM planning_records
WHERE planning_id = $1
`, id)
	return scanRecord(row.Scan)
}

func (r *Repo) List(sessionID string) ([]planning.Record, error) {
	query := `
SELECT planning_id, session_id, task_id, status, reason, error, plan_id, plan_revision, capability_view_id, context_summary_id, metadata_json, started_at, finished_at
FROM planning_records
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY started_at ASC, plan_revision ASC, planning_id ASC"
	rows, err := r.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []planning.Record{}
	for rows.Next() {
		item, err := scanRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt == out[j].StartedAt {
			if out[i].PlanRevision == out[j].PlanRevision {
				return out[i].PlanningID < out[j].PlanningID
			}
			return out[i].PlanRevision < out[j].PlanRevision
		}
		return out[i].StartedAt < out[j].StartedAt
	})
	return out, nil
}

type scanner func(dest ...any) error

type sqlNullString struct {
	String string
	Valid  bool
}

type sqlNullInt64 struct {
	Int64 int64
	Valid bool
}

func (n *sqlNullString) Scan(value any) error {
	if value == nil {
		n.String = ""
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case string:
		n.String = v
	case []byte:
		n.String = string(v)
	default:
		n.String = ""
	}
	n.Valid = true
	return nil
}

func (n *sqlNullInt64) Scan(value any) error {
	if value == nil {
		n.Int64 = 0
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case int64:
		n.Int64 = v
	case int:
		n.Int64 = int64(v)
	case float64:
		n.Int64 = int64(v)
	default:
		n.Int64 = 0
	}
	n.Valid = true
	return nil
}

func scanRecord(scan scanner) (planning.Record, error) {
	var item planning.Record
	var status string
	var taskID, reason, errText, planID, capabilityViewID, contextSummaryID, metadataRaw sqlNullString
	var finishedAt sqlNullInt64
	if err := scan(&item.PlanningID, &item.SessionID, &taskID, &status, &reason, &errText, &planID, &item.PlanRevision, &capabilityViewID, &contextSummaryID, &metadataRaw, &item.StartedAt, &finishedAt); err != nil {
		return planning.Record{}, translateErr(err)
	}
	item.TaskID = taskID.String
	item.Status = planning.Status(status)
	item.Reason = reason.String
	item.Error = errText.String
	item.PlanID = planID.String
	item.CapabilityViewID = capabilityViewID.String
	item.ContextSummaryID = contextSummaryID.String
	item.FinishedAt = finishedAt.Int64
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &item.Metadata)
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item, nil
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

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func translateErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return planning.ErrPlanningRecordNotFound
	}
	return err
}
