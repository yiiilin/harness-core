package verify

type Result struct {
	Success bool           `json:"success"`
	Details map[string]any `json:"details,omitempty"`
	Reason  string         `json:"reason,omitempty"`
}
