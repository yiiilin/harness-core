package task

type Status string

const (
	StatusReceived  Status = "received"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusAborted   Status = "aborted"
)

type Spec struct {
	TaskID      string         `json:"task_id"`
	TaskType    string         `json:"task_type"`
	Goal        string         `json:"goal"`
	Constraints map[string]any `json:"constraints,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Record struct {
	TaskID      string         `json:"task_id"`
	TaskType    string         `json:"task_type"`
	Goal        string         `json:"goal"`
	Status      Status         `json:"status"`
	SessionID   string         `json:"session_id,omitempty"`
	Constraints map[string]any `json:"constraints,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   int64          `json:"created_at"`
	UpdatedAt   int64          `json:"updated_at"`
}
