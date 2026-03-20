package capabilityrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type SnapshotRepo struct{ db postgres.DBTX }

func New(db postgres.DBTX) *SnapshotRepo { return &SnapshotRepo{db: db} }

func (r *SnapshotRepo) Create(spec capability.Snapshot) capability.Snapshot {
	if spec.SnapshotID == "" {
		spec.SnapshotID = "cap_" + uuid.NewString()
	}
	if spec.ResolvedAt == 0 {
		spec.ResolvedAt = time.Now().UnixMilli()
	}
	ctx := context.Background()
	metadataJSON, _ := json.Marshal(spec.Metadata)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO capability_snapshots (
  snapshot_id, session_id, task_id, step_id, tool_name, version, capability_type, risk_level, metadata_json, resolved_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`, spec.SnapshotID, nullable(spec.SessionID), nullable(spec.TaskID), nullable(spec.StepID), spec.ToolName, nullable(spec.Version), nullable(spec.CapabilityType), nullable(string(spec.RiskLevel)), nullableJSON(metadataJSON), spec.ResolvedAt)
	if err != nil {
		panic(err)
	}
	return spec
}

func (r *SnapshotRepo) Get(id string) (capability.Snapshot, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT snapshot_id, session_id, task_id, step_id, tool_name, version, capability_type, risk_level, metadata_json, resolved_at
FROM capability_snapshots
WHERE snapshot_id = $1
`, id)

	var item capability.Snapshot
	var sessionID, taskID, stepID, version, capabilityType, riskLevel, metadataRaw sql.NullString
	if err := row.Scan(&item.SnapshotID, &sessionID, &taskID, &stepID, &item.ToolName, &version, &capabilityType, &riskLevel, &metadataRaw, &item.ResolvedAt); err != nil {
		return capability.Snapshot{}, err
	}
	item.SessionID = sessionID.String
	item.TaskID = taskID.String
	item.StepID = stepID.String
	item.Version = version.String
	item.CapabilityType = capabilityType.String
	item.RiskLevel = tool.RiskLevel(riskLevel.String)
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &item.Metadata)
	}
	return item, nil
}

func (r *SnapshotRepo) List(sessionID string) []capability.Snapshot {
	ctx := context.Background()
	query := `
SELECT snapshot_id, session_id, task_id, step_id, tool_name, version, capability_type, risk_level, metadata_json, resolved_at
FROM capability_snapshots
`
	args := []any{}
	if sessionID != "" {
		query += "WHERE session_id = $1\n"
		args = append(args, sessionID)
	}
	query += "ORDER BY resolved_at ASC"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	out := []capability.Snapshot{}
	for rows.Next() {
		var item capability.Snapshot
		var rawSessionID, taskID, stepID, version, capabilityType, riskLevel, metadataRaw sql.NullString
		if err := rows.Scan(&item.SnapshotID, &rawSessionID, &taskID, &stepID, &item.ToolName, &version, &capabilityType, &riskLevel, &metadataRaw, &item.ResolvedAt); err != nil {
			panic(err)
		}
		item.SessionID = rawSessionID.String
		item.TaskID = taskID.String
		item.StepID = stepID.String
		item.Version = version.String
		item.CapabilityType = capabilityType.String
		item.RiskLevel = tool.RiskLevel(riskLevel.String)
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
