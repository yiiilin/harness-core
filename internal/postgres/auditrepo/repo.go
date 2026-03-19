package auditrepo

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/internal/postgres"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
)

type Repo struct {
	db postgres.DBTX
}

func New(db postgres.DBTX) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Emit(event audit.Event) error {
	ctx := context.Background()
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = time.Now().UnixMilli()
	}
	payloadJSON, _ := json.Marshal(event.Payload)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO audit_events (
  event_id, type, session_id, step_id, payload_json, created_at
) VALUES ($1, $2, $3, $4, $5, $6)
`, event.EventID, event.Type, nullable(event.SessionID), nullable(event.StepID), nullableJSON(payloadJSON), event.CreatedAt)
	return err
}

func (r *Repo) List(sessionID string) []audit.Event {
	ctx := context.Background()
	query := `
SELECT event_id, type, session_id, step_id, payload_json, created_at
FROM audit_events
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
	out := []audit.Event{}
	for rows.Next() {
		evt, err := scanEvent(rows.Scan)
		if err != nil {
			panic(err)
		}
		out = append(out, evt)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			return out[i].EventID < out[j].EventID
		}
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out
}

type scanner func(dest ...any) error

type sqlNullString struct {
	String string
	Valid  bool
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

func scanEvent(scan scanner) (audit.Event, error) {
	var evt audit.Event
	var sessionID, stepID, payloadRaw sqlNullString
	if err := scan(&evt.EventID, &evt.Type, &sessionID, &stepID, &payloadRaw, &evt.CreatedAt); err != nil {
		return audit.Event{}, err
	}
	evt.SessionID = sessionID.String
	evt.StepID = stepID.String
	if payloadRaw.String != "" {
		_ = json.Unmarshal([]byte(payloadRaw.String), &evt.Payload)
	}
	if evt.Payload == nil {
		evt.Payload = map[string]any{}
	}
	return evt, nil
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
