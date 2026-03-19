package session

type Phase string

type ExecutionState string

const (
	PhaseReceived Phase = "received"
	PhasePrepare  Phase = "prepare"
	PhasePlan     Phase = "plan"
	PhaseExecute  Phase = "execute"
	PhaseVerify   Phase = "verify"
	PhaseRecover  Phase = "recover"
	PhaseComplete Phase = "complete"
	PhaseFailed   Phase = "failed"
	PhaseAborted  Phase = "aborted"
)

const (
	ExecutionIdle        ExecutionState = "idle"
	ExecutionInFlight    ExecutionState = "in_flight"
	ExecutionInterrupted ExecutionState = "interrupted"
)

type State struct {
	SessionID       string         `json:"session_id"`
	TaskID          string         `json:"task_id,omitempty"`
	ParentSessionID string         `json:"parent_session_id,omitempty"`
	Title           string         `json:"title"`
	Goal            string         `json:"goal,omitempty"`
	Phase           Phase          `json:"phase"`
	CurrentStepID   string         `json:"current_step_id,omitempty"`
	Summary         string         `json:"summary,omitempty"`
	RetryCount      int            `json:"retry_count"`
	ExecutionState  ExecutionState `json:"execution_state"`
	InFlightStepID  string         `json:"in_flight_step_id,omitempty"`
	LastHeartbeatAt int64          `json:"last_heartbeat_at,omitempty"`
	InterruptedAt   int64          `json:"interrupted_at,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       int64          `json:"created_at"`
	UpdatedAt       int64          `json:"updated_at"`
}
