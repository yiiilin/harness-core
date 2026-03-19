package audit

type Event struct {
	EventID   string         `json:"event_id"`
	Type      string         `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	StepID    string         `json:"step_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt int64          `json:"created_at"`
}
