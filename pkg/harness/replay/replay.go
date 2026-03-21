package replay

import (
	"errors"
	"sort"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

// ErrNilReader indicates that a replay helper was called without a usable
// public execution-fact reader.
var ErrNilReader = errors.New("replay reader is nil")

// NewReader wraps a public execution-fact reader with replay/debug helpers.
// If the source also supports GetExecutionCycle, listed cycles are hydrated
// through that read before projection.
func NewReader(source SessionReader) *Reader {
	if source == nil {
		return &Reader{}
	}
	reader := &Reader{sessionReader: source}
	if getter, ok := source.(CycleReader); ok {
		reader.cycleReader = getter
	}
	return reader
}

// SessionProjection loads public execution cycles and audit events for one
// session and projects them into a replay/debug-friendly view.
func (r *Reader) SessionProjection(sessionID string) (SessionProjection, error) {
	if r == nil || r.sessionReader == nil {
		return SessionProjection{}, ErrNilReader
	}

	cycles, err := r.sessionReader.ListExecutionCycles(sessionID)
	if err != nil {
		return SessionProjection{}, err
	}
	cycles, err = r.hydrateCycles(sessionID, cycles)
	if err != nil {
		return SessionProjection{}, err
	}

	events, err := r.sessionReader.ListAuditEvents(sessionID)
	if err != nil {
		return SessionProjection{}, err
	}

	return BuildSessionProjection(sessionID, cycles, events), nil
}

// ExecutionCycleProjection loads one public execution cycle plus session audit
// events and projects the cycle-correlated events into a replay/debug view.
func (r *Reader) ExecutionCycleProjection(sessionID, cycleID string) (ExecutionCycleProjection, error) {
	if r == nil || r.sessionReader == nil {
		return ExecutionCycleProjection{}, ErrNilReader
	}

	cycle, err := getCycle(r.sessionReader, r.cycleReader, sessionID, cycleID)
	if err != nil {
		return ExecutionCycleProjection{}, err
	}

	events, err := r.sessionReader.ListAuditEvents(sessionID)
	if err != nil {
		return ExecutionCycleProjection{}, err
	}

	return BuildExecutionCycleProjection(cycle, events), nil
}

// LoadSessionProjection is a convenience wrapper for one-shot session
// projections.
func LoadSessionProjection(reader SessionReader, sessionID string) (SessionProjection, error) {
	return NewReader(reader).SessionProjection(sessionID)
}

// LoadCycleProjection is a convenience wrapper for one-shot execution-cycle
// projections.
func LoadCycleProjection(reader SessionReader, sessionID, cycleID string) (ExecutionCycleProjection, error) {
	return NewReader(reader).ExecutionCycleProjection(sessionID, cycleID)
}

// BuildSessionProjection groups ordered audit events under their execution
// cycles while preserving unmatched session-level events.
func BuildSessionProjection(sessionID string, cycles []execution.ExecutionCycle, auditEvents []audit.Event) SessionProjection {
	orderedCycles := orderedExecutionCycles(cycles)
	projection := SessionProjection{
		SessionID: firstNonEmpty(sessionID, sessionIDFromCycles(orderedCycles), sessionIDFromEvents(auditEvents)),
		Cycles:    make([]ExecutionCycleProjection, len(orderedCycles)),
		Events:    orderedAuditEvents(auditEvents),
	}

	cycleIndex := make(map[string]int, len(orderedCycles))
	for i, cycle := range orderedCycles {
		projection.Cycles[i] = ExecutionCycleProjection{Cycle: cloneExecutionCycle(cycle)}
		if cycle.CycleID != "" {
			cycleIndex[cycle.CycleID] = i
		}
	}

	for _, event := range projection.Events {
		if idx, ok := cycleIndex[event.CycleID]; ok && event.CycleID != "" {
			projection.Cycles[idx].Events = append(projection.Cycles[idx].Events, event)
			continue
		}
		projection.UnmatchedEvents = append(projection.UnmatchedEvents, event)
	}

	return projection
}

// BuildExecutionCycleProjection keeps a logical execution cycle together with
// its ordered cycle-correlated audit events.
func BuildExecutionCycleProjection(cycle execution.ExecutionCycle, auditEvents []audit.Event) ExecutionCycleProjection {
	projection := ExecutionCycleProjection{Cycle: cloneExecutionCycle(cycle)}
	for _, event := range orderedAuditEvents(auditEvents) {
		if cycle.CycleID == "" || event.CycleID != cycle.CycleID {
			continue
		}
		projection.Events = append(projection.Events, event)
	}
	return projection
}

func (r *Reader) hydrateCycles(sessionID string, cycles []execution.ExecutionCycle) ([]execution.ExecutionCycle, error) {
	out := orderedExecutionCycles(cycles)
	if r.cycleReader == nil {
		return out, nil
	}
	for i := range out {
		if out[i].CycleID == "" {
			continue
		}
		cycle, err := r.cycleReader.GetExecutionCycle(sessionID, out[i].CycleID)
		switch {
		case err == nil:
			out[i] = cloneExecutionCycle(cycle)
		case errors.Is(err, execution.ErrExecutionCycleNotFound):
			continue
		default:
			return nil, err
		}
	}
	return out, nil
}

func getCycle(sessionReader SessionReader, cycleReader CycleReader, sessionID, cycleID string) (execution.ExecutionCycle, error) {
	if cycleReader != nil {
		return cycleReader.GetExecutionCycle(sessionID, cycleID)
	}

	cycles, err := sessionReader.ListExecutionCycles(sessionID)
	if err != nil {
		return execution.ExecutionCycle{}, err
	}
	for _, cycle := range cycles {
		if cycle.CycleID == cycleID {
			return cloneExecutionCycle(cycle), nil
		}
	}
	return execution.ExecutionCycle{}, execution.ErrExecutionCycleNotFound
}

func orderedExecutionCycles(cycles []execution.ExecutionCycle) []execution.ExecutionCycle {
	out := make([]execution.ExecutionCycle, len(cycles))
	for i := range cycles {
		out[i] = cloneExecutionCycle(cycles[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartedAt != out[j].StartedAt {
			return out[i].StartedAt < out[j].StartedAt
		}
		return out[i].CycleID < out[j].CycleID
	})
	return out
}

func orderedAuditEvents(events []audit.Event) []audit.Event {
	out := append([]audit.Event(nil), events...)
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i]
		right := out[j]
		if left.Sequence > 0 && right.Sequence > 0 && left.Sequence != right.Sequence {
			return left.Sequence < right.Sequence
		}
		if left.CreatedAt != right.CreatedAt {
			return left.CreatedAt < right.CreatedAt
		}
		if left.EventID != right.EventID {
			return left.EventID < right.EventID
		}
		return left.Type < right.Type
	})
	return out
}

func cloneExecutionCycle(cycle execution.ExecutionCycle) execution.ExecutionCycle {
	cloned := cycle
	cloned.Attempts = append([]execution.Attempt(nil), cycle.Attempts...)
	cloned.Actions = append([]execution.ActionRecord(nil), cycle.Actions...)
	cloned.Verifications = append([]execution.VerificationRecord(nil), cycle.Verifications...)
	cloned.Artifacts = append([]execution.Artifact(nil), cycle.Artifacts...)
	cloned.RuntimeHandles = append([]execution.RuntimeHandle(nil), cycle.RuntimeHandles...)
	return cloned
}

func sessionIDFromCycles(cycles []execution.ExecutionCycle) string {
	for _, cycle := range cycles {
		if cycle.SessionID != "" {
			return cycle.SessionID
		}
	}
	return ""
}

func sessionIDFromEvents(events []audit.Event) string {
	for _, event := range events {
		if event.SessionID != "" {
			return event.SessionID
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
