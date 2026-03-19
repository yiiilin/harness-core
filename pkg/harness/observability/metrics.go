package observability

import "sync"

type Snapshot struct {
	StepRuns        int   `json:"step_runs"`
	StepSuccess     int   `json:"step_success"`
	StepFailure     int   `json:"step_failure"`
	PolicyDenied    int   `json:"policy_denied"`
	VerifyFailure   int   `json:"verify_failure"`
	ActionFailure   int   `json:"action_failure"`
	TotalDurationMS int64 `json:"total_duration_ms"`
}

type Recorder interface {
	Record(name string, fields map[string]any)
	Snapshot() Snapshot
}

type MemoryRecorder struct {
	mu sync.RWMutex
	s  Snapshot
}

func NewMemoryRecorder() *MemoryRecorder { return &MemoryRecorder{} }

func (r *MemoryRecorder) Record(name string, fields map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch name {
	case "step.run":
		r.s.StepRuns++
		if v, _ := fields["duration_ms"].(int64); v > 0 {
			r.s.TotalDurationMS += v
		}
		if success, _ := fields["success"].(bool); success {
			r.s.StepSuccess++
		} else {
			r.s.StepFailure++
		}
		if denied, _ := fields["policy_denied"].(bool); denied {
			r.s.PolicyDenied++
		}
		if verifyFailed, _ := fields["verify_failed"].(bool); verifyFailed {
			r.s.VerifyFailure++
		}
		if actionFailed, _ := fields["action_failed"].(bool); actionFailed {
			r.s.ActionFailure++
		}
	}
}

func (r *MemoryRecorder) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.s
}
