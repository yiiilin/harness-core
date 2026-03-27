package replay

import (
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

// SessionReader is the minimal public read surface needed to build a
// session-scoped replay view.
type SessionReader interface {
	ListExecutionCycles(sessionID string) ([]execution.ExecutionCycle, error)
	ListAuditEvents(sessionID string) ([]audit.Event, error)
}

type BlockedRuntimeProjectionReader interface {
	ListBlockedRuntimeProjections() ([]execution.BlockedRuntimeProjection, error)
}

// CycleReader is the optional public read surface used to hydrate a listed
// execution cycle via GetExecutionCycle when available.
type CycleReader interface {
	GetExecutionCycle(sessionID, cycleID string) (execution.ExecutionCycle, error)
}

// ExecutionFactReader is a convenience union for callers that expose both
// session-level listing and per-cycle reads.
type ExecutionFactReader interface {
	SessionReader
	CycleReader
}

// Reader wraps public execution-fact reads with replay/debug-oriented
// projection helpers.
type Reader struct {
	sessionReader SessionReader
	cycleReader   CycleReader
	blockedReader BlockedRuntimeProjectionReader
}

// SessionProjection groups execution cycles with ordered audit events for one
// session-oriented replay/debug view.
type SessionProjection struct {
	SessionID       string                               `json:"session_id"`
	Cycles          []ExecutionCycleProjection           `json:"cycles,omitempty"`
	BlockedRuntimes []execution.BlockedRuntimeProjection `json:"blocked_runtimes,omitempty"`
	Events          []audit.Event                        `json:"events,omitempty"`
	UnmatchedEvents []audit.Event                        `json:"unmatched_events,omitempty"`
}

// ExecutionCycleProjection pairs an execution cycle with its related ordered
// audit events.
type ExecutionCycleProjection struct {
	Cycle               execution.ExecutionCycle       `json:"cycle"`
	Program             *execution.ProgramLineage      `json:"program,omitempty"`
	ApprovalLinkage     *execution.ApprovalLinkage     `json:"approval_linkage,omitempty"`
	TargetSlices        []execution.TargetSlice        `json:"target_slices,omitempty"`
	InteractiveRuntimes []execution.InteractiveRuntime `json:"interactive_runtimes,omitempty"`
	Events              []audit.Event                  `json:"events,omitempty"`
}
