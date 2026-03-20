package observability

import (
	"context"
	"sync"
)

type Snapshot struct {
	StepRuns          int   `json:"step_runs"`
	StepSuccess       int   `json:"step_success"`
	StepFailure       int   `json:"step_failure"`
	PolicyDenied      int   `json:"policy_denied"`
	VerifyFailure     int   `json:"verify_failure"`
	ActionFailure     int   `json:"action_failure"`
	PlanningRuns      int   `json:"planning_runs"`
	PlanningFailure   int   `json:"planning_failure"`
	ApprovalRequested int   `json:"approval_requested"`
	ApprovalApproved  int   `json:"approval_approved"`
	ApprovalRejected  int   `json:"approval_rejected"`
	RecoveryRuns      int   `json:"recovery_runs"`
	SessionAborts     int   `json:"session_aborts"`
	LeaseClaims       int   `json:"lease_claims"`
	LeaseRenewals     int   `json:"lease_renewals"`
	LeaseReleases     int   `json:"lease_releases"`
	TotalDurationMS   int64 `json:"total_duration_ms"`
}

type MetricSample struct {
	Name       string            `json:"name"`
	Labels     map[string]string `json:"labels,omitempty"`
	Fields     map[string]any    `json:"fields,omitempty"`
	RecordedAt int64             `json:"recorded_at"`
}

type TraceSpan struct {
	Name           string         `json:"name"`
	TraceID        string         `json:"trace_id,omitempty"`
	SpanID         string         `json:"span_id,omitempty"`
	ParentID       string         `json:"parent_id,omitempty"`
	SessionID      string         `json:"session_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	PlanningID     string         `json:"planning_id,omitempty"`
	ApprovalID     string         `json:"approval_id,omitempty"`
	LeaseID        string         `json:"lease_id,omitempty"`
	StepID         string         `json:"step_id,omitempty"`
	AttemptID      string         `json:"attempt_id,omitempty"`
	ActionID       string         `json:"action_id,omitempty"`
	VerificationID string         `json:"verification_id,omitempty"`
	CausationID    string         `json:"causation_id,omitempty"`
	StartedAt      int64          `json:"started_at"`
	FinishedAt     int64          `json:"finished_at"`
	Attributes     map[string]any `json:"attributes,omitempty"`
}

type MetricsExporter interface {
	ExportMetric(ctx context.Context, sample MetricSample) error
}

type TraceExporter interface {
	ExportTrace(ctx context.Context, span TraceSpan) error
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
	case "planning.cycle":
		r.s.PlanningRuns++
		if success, _ := fields["success"].(bool); !success {
			r.s.PlanningFailure++
		}
	case "approval.request":
		r.s.ApprovalRequested++
	case "approval.response":
		switch fields["reply"] {
		case "reject":
			r.s.ApprovalRejected++
		default:
			r.s.ApprovalApproved++
		}
	case "session.recover":
		r.s.RecoveryRuns++
	case "session.abort":
		r.s.SessionAborts++
	case "lease.claim":
		if claimed, _ := fields["claimed"].(bool); claimed {
			r.s.LeaseClaims++
		}
	case "lease.renew":
		r.s.LeaseRenewals++
	case "lease.release":
		r.s.LeaseReleases++
	}
}

func (r *MemoryRecorder) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.s
}
