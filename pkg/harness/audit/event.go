package audit

const (
	EventTaskCreated     = "task.created"
	EventSessionCreated  = "session.created"
	EventPlanGenerated   = "plan.generated"
	EventStepStarted     = "step.started"
	EventToolCalled      = "tool.called"
	EventToolCompleted   = "tool.completed"
	EventToolFailed      = "tool.failed"
	EventVerifyCompleted = "verify.completed"
	EventStateChanged    = "state.changed"
	EventPolicyDenied    = "policy.denied"
	EventTaskCompleted   = "task.completed"
	EventTaskFailed      = "task.failed"
)

type Event struct {
	EventID   string         `json:"event_id"`
	Type      string         `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	StepID    string         `json:"step_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt int64          `json:"created_at"`
}
