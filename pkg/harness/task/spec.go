package task

type Spec struct {
	TaskID      string         `json:"task_id"`
	TaskType    string         `json:"task_type"`
	Goal        string         `json:"goal"`
	Constraints map[string]any `json:"constraints,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}
