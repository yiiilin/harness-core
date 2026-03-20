package approvalrepo

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/approval"
)

type Repo struct {
	db postgres.DBTX
}

func New(db postgres.DBTX) *Repo {
	return &Repo{db: db}
}

func (r *Repo) CreatePending(req approval.Request) approval.Record {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	stepJSON, _ := json.Marshal(req.Step)
	metadataJSON, _ := json.Marshal(req.Metadata)
	row := r.db.QueryRowContext(ctx, `
INSERT INTO approvals (
  approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, created_at, updated_at
`, newID(), req.SessionID, nullable(req.TaskID), nullable(req.StepID), nullable(req.ToolName), nullable(req.Reason), nullable(req.MatchedRule), string(approval.StatusPending), nil, string(stepJSON), nullableJSON(metadataJSON), now, nil, nil, now, now)
	rec, err := scanRecord(row.Scan)
	if err != nil {
		panic(err)
	}
	return rec
}

func (r *Repo) Get(id string) (approval.Record, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, created_at, updated_at
FROM approvals WHERE approval_id = $1
`, id)
	return scanRecord(row.Scan)
}

func (r *Repo) Update(next approval.Record) error {
	ctx := context.Background()
	next.UpdatedAt = time.Now().UnixMilli()
	stepJSON, err := json.Marshal(next.Step)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(next.Metadata)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE approvals
SET session_id = $2,
    task_id = $3,
    step_id = $4,
    tool_name = $5,
    reason = $6,
    matched_rule = $7,
    status = $8,
    reply = $9,
    step_json = $10,
    metadata_json = $11,
    requested_at = $12,
    responded_at = $13,
    consumed_at = $14,
    updated_at = $15
WHERE approval_id = $1
`, next.ApprovalID, next.SessionID, nullable(next.TaskID), nullable(next.StepID), nullable(next.ToolName), nullable(next.Reason), nullable(next.MatchedRule), string(next.Status), nullable(string(next.Reply)), string(stepJSON), nullableJSON(metadataJSON), next.RequestedAt, nullableInt64(next.RespondedAt), nullableInt64(next.ConsumedAt), next.UpdatedAt)
	return err
}

func (r *Repo) List(sessionID string) []approval.Record {
	ctx := context.Background()
	query := `
SELECT approval_id, session_id, task_id, step_id, tool_name, reason, matched_rule, status, reply, step_json, metadata_json, requested_at, responded_at, consumed_at, created_at, updated_at
FROM approvals
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY requested_at ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	out := []approval.Record{}
	for rows.Next() {
		rec, err := scanRecord(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RequestedAt == out[j].RequestedAt {
			return out[i].ApprovalID < out[j].ApprovalID
		}
		return out[i].RequestedAt < out[j].RequestedAt
	})
	return out
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

func scanRecord(scan scanner) (approval.Record, error) {
	var rec approval.Record
	var taskID, stepID, toolName, reason, matchedRule, reply, metadataRaw sqlNullString
	var respondedAt, consumedAt sqlNullInt64
	var status, stepRaw string
	if err := scan(&rec.ApprovalID, &rec.SessionID, &taskID, &stepID, &toolName, &reason, &matchedRule, &status, &reply, &stepRaw, &metadataRaw, &rec.RequestedAt, &respondedAt, &consumedAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return approval.Record{}, translateErr(err)
	}
	rec.TaskID = taskID.String
	rec.StepID = stepID.String
	rec.ToolName = toolName.String
	rec.Reason = reason.String
	rec.MatchedRule = matchedRule.String
	rec.Status = approval.Status(status)
	rec.Reply = approval.Reply(reply.String)
	rec.RespondedAt = respondedAt.Int64
	rec.ConsumedAt = consumedAt.Int64
	_ = json.Unmarshal([]byte(stepRaw), &rec.Step)
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &rec.Metadata)
	}
	if rec.Metadata == nil {
		rec.Metadata = map[string]any{}
	}
	return rec, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableJSON(raw []byte) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}

func newID() string {
	return uuid.NewString()
}

func translateErr(err error) error {
	if err == nil {
		return nil
	}
	return approval.ErrApprovalNotFound
}
