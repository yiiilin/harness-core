package planrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

type Repo struct {
	db postgres.DBTX
}

func New(db postgres.DBTX) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Create(sessionID, changeReason string, steps []plan.StepSpec) plan.Spec {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	revision := 1
	if latest, ok := r.LatestBySession(sessionID); ok {
		revision = latest.Revision + 1
	}
	generatedPlanID := uuid.NewString()
	row := r.db.QueryRowContext(ctx, `
INSERT INTO plans (
  plan_id, session_id, revision, status, change_reason, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING plan_id, session_id, revision, status, change_reason, created_at, updated_at
`, generatedPlanID, sessionID, revision, string(plan.StatusActive), nullable(changeReason), now, now)
	created, err := scanPlan(row.Scan)
	if err != nil {
		panic(err)
	}
	for i := range steps {
		if steps[i].Status == "" {
			steps[i].Status = plan.StepPending
		}
		actionJSON, _ := json.Marshal(steps[i].Action)
		verifyJSON, _ := json.Marshal(steps[i].Verify)
		onFailJSON, _ := json.Marshal(steps[i].OnFail)
		metadataJSON, _ := json.Marshal(steps[i].Metadata)
		_, err := r.db.ExecContext(ctx, `
INSERT INTO plan_steps (
  plan_id, step_index, step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`, created.PlanID, i, steps[i].StepID, steps[i].Title, string(actionJSON), string(verifyJSON), nullableJSON(onFailJSON), string(steps[i].Status), steps[i].Attempt, nullable(steps[i].Reason), nullableJSON(metadataJSON), nullableInt64(steps[i].StartedAt), nullableInt64(steps[i].FinishedAt))
		if err != nil {
			panic(err)
		}
	}
	full, err := r.Get(created.PlanID)
	if err != nil {
		panic(err)
	}
	return full
}

func (r *Repo) Get(id string) (plan.Spec, error) {
	ctx := context.Background()
	row := r.db.QueryRowContext(ctx, `
SELECT plan_id, session_id, revision, status, change_reason, created_at, updated_at
FROM plans WHERE plan_id = $1
`, id)
	pl, err := scanPlan(row.Scan)
	if err != nil {
		return plan.Spec{}, err
	}
	stepsRows, err := r.db.QueryContext(ctx, `
SELECT step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
FROM plan_steps WHERE plan_id = $1
ORDER BY step_index ASC
`, id)
	if err != nil {
		return plan.Spec{}, err
	}
	defer stepsRows.Close()
	for stepsRows.Next() {
		step, err := scanStep(stepsRows.Scan)
		if err != nil {
			return plan.Spec{}, err
		}
		pl.Steps = append(pl.Steps, step)
	}
	if err := stepsRows.Err(); err != nil {
		return plan.Spec{}, err
	}
	return pl, nil
}

func (r *Repo) ListBySession(sessionID string) []plan.Spec {
	ctx := context.Background()
	rows, err := r.db.QueryContext(ctx, `
SELECT plan_id, session_id, revision, status, change_reason, created_at, updated_at
FROM plans WHERE session_id = $1
ORDER BY revision ASC
`, sessionID)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	out := []plan.Spec{}
	for rows.Next() {
		pl, err := scanPlan(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, pl)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	for i := range out {
		full, err := r.Get(out[i].PlanID)
		if err != nil {
			panic(err)
		}
		out[i] = full
	}
	return out
}

func (r *Repo) LatestBySession(sessionID string) (plan.Spec, bool) {
	items := r.ListBySession(sessionID)
	if len(items) == 0 {
		return plan.Spec{}, false
	}
	return items[len(items)-1], true
}

func (r *Repo) Update(next plan.Spec) error {
	ctx := context.Background()
	next.UpdatedAt = time.Now().UnixMilli()
	_, err := r.db.ExecContext(ctx, `
UPDATE plans
SET session_id = $2,
    revision = $3,
    status = $4,
    change_reason = $5,
    updated_at = $6
WHERE plan_id = $1
`, next.PlanID, next.SessionID, next.Revision, string(next.Status), nullable(next.ChangeReason), next.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM plan_steps WHERE plan_id = $1`, next.PlanID)
	if err != nil {
		return err
	}
	for i := range next.Steps {
		actionJSON, _ := json.Marshal(next.Steps[i].Action)
		verifyJSON, _ := json.Marshal(next.Steps[i].Verify)
		onFailJSON, _ := json.Marshal(next.Steps[i].OnFail)
		metadataJSON, _ := json.Marshal(next.Steps[i].Metadata)
		_, err := r.db.ExecContext(ctx, `
INSERT INTO plan_steps (
  plan_id, step_index, step_id, title, action_json, verify_json, on_fail_json, status, attempt, reason, metadata_json, started_at, finished_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`, next.PlanID, i, next.Steps[i].StepID, next.Steps[i].Title, string(actionJSON), string(verifyJSON), nullableJSON(onFailJSON), string(next.Steps[i].Status), next.Steps[i].Attempt, nullable(next.Steps[i].Reason), nullableJSON(metadataJSON), nullableInt64(next.Steps[i].StartedAt), nullableInt64(next.Steps[i].FinishedAt))
		if err != nil {
			return err
		}
	}
	return nil
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

func scanPlan(scan scanner) (plan.Spec, error) {
	var pl plan.Spec
	var status string
	var changeReason sqlNullString
	if err := scan(&pl.PlanID, &pl.SessionID, &pl.Revision, &status, &changeReason, &pl.CreatedAt, &pl.UpdatedAt); err != nil {
		return plan.Spec{}, translateErr(err)
	}
	pl.Status = plan.Status(status)
	pl.ChangeReason = changeReason.String
	pl.Steps = []plan.StepSpec{}
	return pl, nil
}

func scanStep(scan scanner) (plan.StepSpec, error) {
	var st plan.StepSpec
	var actionRaw, verifyRaw string
	var onFailRaw, reasonRaw, metadataRaw sqlNullString
	var status string
	var startedAt, finishedAt sqlNullInt64
	if err := scan(&st.StepID, &st.Title, &actionRaw, &verifyRaw, &onFailRaw, &status, &st.Attempt, &reasonRaw, &metadataRaw, &startedAt, &finishedAt); err != nil {
		return plan.StepSpec{}, translateErr(err)
	}
	st.Status = plan.StepStatus(status)
	st.Reason = reasonRaw.String
	st.StartedAt = startedAt.Int64
	st.FinishedAt = finishedAt.Int64
	_ = json.Unmarshal([]byte(actionRaw), &st.Action)
	_ = json.Unmarshal([]byte(verifyRaw), &st.Verify)
	if onFailRaw.String != "" {
		_ = json.Unmarshal([]byte(onFailRaw.String), &st.OnFail)
	}
	if metadataRaw.String != "" {
		_ = json.Unmarshal([]byte(metadataRaw.String), &st.Metadata)
	}
	if st.Metadata == nil {
		st.Metadata = map[string]any{}
	}
	return st, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableJSON(b []byte) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
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
		return plan.ErrPlanNotFound
	}
	return err
}
