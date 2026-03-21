package replay

import (
	"reflect"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func TestReaderProjectsSession(t *testing.T) {
	reader := &fakeExecutionFactReader{
		cycles: []execution.ExecutionCycle{
			{
				CycleID:        "cycle-1",
				SessionID:      "session-42",
				StartedAt:      100,
				RuntimeHandles: []execution.RuntimeHandle{{HandleID: "handle-1"}},
			},
			{
				CycleID:   "cycle-2",
				SessionID: "session-42",
				StartedAt: 200,
			},
		},
		events: []audit.Event{
			{EventID: "event-2", Sequence: 2, CycleID: "cycle-1", CreatedAt: 2},
			{EventID: "event-3", Sequence: 3, CycleID: "cycle-2", CreatedAt: 3},
			{EventID: "event-1", Sequence: 1, CycleID: "cycle-1", CreatedAt: 1},
			{EventID: "event-orphan", Sequence: 4, CycleID: "", CreatedAt: 4},
		},
	}

	readerGetter := NewReader(reader)
	projection, err := readerGetter.SessionProjection("session-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if projection.SessionID != "session-42" {
		t.Fatalf("session id mismatch: %s", projection.SessionID)
	}

	if got := eventSequences(projection.Events); !reflect.DeepEqual([]int64{1, 2, 3, 4}, got) {
		t.Fatalf("event sequences = %v, want %v", got, []int64{1, 2, 3, 4})
	}
	if got := eventSequences(projection.UnmatchedEvents); !reflect.DeepEqual([]int64{4}, got) {
		t.Fatalf("unmatched event sequences = %v, want %v", got, []int64{4})
	}

	if len(projection.Cycles) != 2 {
		t.Fatalf("expected 2 cycles, got %d", len(projection.Cycles))
	}

	firstCycle := projection.Cycles[0]
	if firstCycle.Cycle.CycleID != "cycle-1" {
		t.Fatalf("first cycle id = %s", firstCycle.Cycle.CycleID)
	}

	if got := eventSequences(firstCycle.Events); !reflect.DeepEqual([]int64{1, 2}, got) {
		t.Fatalf("first cycle events = %v", got)
	}

	if len(firstCycle.Cycle.RuntimeHandles) != 1 || firstCycle.Cycle.RuntimeHandles[0].HandleID != "handle-1" {
		t.Fatalf("runtime handles missing from projection")
	}
}

func TestReaderUsesGetExecutionCycleWhenAvailable(t *testing.T) {
	reader := &fakeExecutionFactReader{
		cycles: []execution.ExecutionCycle{{CycleID: "cycle-1", SessionID: "session-1"}},
		events: []audit.Event{{EventID: "event-1", Sequence: 1, CycleID: "cycle-1", CreatedAt: 1}},
		getReturn: map[string]execution.ExecutionCycle{
			"cycle-1": {
				CycleID:        "cycle-1",
				SessionID:      "session-1",
				RuntimeHandles: []execution.RuntimeHandle{{HandleID: "via-get"}},
			},
		},
	}

	readerGetter := NewReader(reader)
	projection, err := readerGetter.SessionProjection("session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reader.getCalls) != 1 || reader.getCalls[0] != "cycle-1" {
		t.Fatalf("get was not invoked: %v", reader.getCalls)
	}

	if len(projection.Cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d", len(projection.Cycles))
	}

	handles := projection.Cycles[0].Cycle.RuntimeHandles
	if len(handles) != 1 || handles[0].HandleID != "via-get" {
		t.Fatalf("projection did not include runtime handles from GetExecutionCycle")
	}
}

func TestReaderProjectsSingleExecutionCycle(t *testing.T) {
	reader := &fakeExecutionFactReader{
		cycles: []execution.ExecutionCycle{{CycleID: "cycle-1", SessionID: "session-1"}},
		events: []audit.Event{
			{EventID: "event-2", Sequence: 2, CycleID: "cycle-1", CreatedAt: 2},
			{EventID: "event-1", Sequence: 1, CycleID: "cycle-1", CreatedAt: 1},
			{EventID: "event-3", Sequence: 3, CycleID: "", CreatedAt: 3},
		},
	}

	projection, err := NewReader(reader).ExecutionCycleProjection("session-1", "cycle-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if projection.Cycle.CycleID != "cycle-1" {
		t.Fatalf("cycle id mismatch: %s", projection.Cycle.CycleID)
	}
	if got := eventSequences(projection.Events); !reflect.DeepEqual([]int64{1, 2}, got) {
		t.Fatalf("cycle event sequences = %v, want %v", got, []int64{1, 2})
	}
}

type fakeExecutionFactReader struct {
	cycles    []execution.ExecutionCycle
	events    []audit.Event
	getReturn map[string]execution.ExecutionCycle
	getCalls  []string
}

func (f *fakeExecutionFactReader) ListExecutionCycles(sessionID string) ([]execution.ExecutionCycle, error) {
	return f.cycles, nil
}

func (f *fakeExecutionFactReader) ListAuditEvents(sessionID string) ([]audit.Event, error) {
	return f.events, nil
}

func (f *fakeExecutionFactReader) GetExecutionCycle(sessionID, cycleID string) (execution.ExecutionCycle, error) {
	f.getCalls = append(f.getCalls, cycleID)
	if c, ok := f.getReturn[cycleID]; ok {
		return c, nil
	}
	for _, cycle := range f.cycles {
		if cycle.CycleID == cycleID {
			return cycle, nil
		}
	}
	return execution.ExecutionCycle{}, execution.ErrExecutionCycleNotFound
}

func eventSequences(events []audit.Event) []int64 {
	seq := make([]int64, len(events))
	for i, ev := range events {
		seq[i] = ev.Sequence
	}
	return seq
}
