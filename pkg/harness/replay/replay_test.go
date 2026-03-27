package replay

import (
	"encoding/json"
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
		blocked: []execution.BlockedRuntimeProjection{
			{
				Runtime: execution.BlockedRuntime{
					BlockedRuntimeID: "blocked-1",
					SessionID:        "session-42",
				},
			},
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
	if len(projection.BlockedRuntimes) != 1 || projection.BlockedRuntimes[0].Runtime.BlockedRuntimeID != "blocked-1" {
		t.Fatalf("expected blocked runtime projections to be included, got %#v", projection.BlockedRuntimes)
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
				CycleID:   "cycle-1",
				SessionID: "session-1",
				RuntimeHandles: []execution.RuntimeHandle{{
					HandleID: "via-get",
					Metadata: map[string]any{
						execution.InteractiveMetadataKeyEnabled:      true,
						execution.InteractiveMetadataKeySupportsView: true,
					},
				}},
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
	if len(projection.Cycles[0].InteractiveRuntimes) != 1 || projection.Cycles[0].InteractiveRuntimes[0].Handle.HandleID != "via-get" {
		t.Fatalf("projection did not include derived interactive runtimes: %#v", projection.Cycles[0])
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

func TestReaderProjectsProgramLineage(t *testing.T) {
	metadata := execution.ApplyTargetMetadata(map[string]any{
		execution.ProgramMetadataKeyID:           "prog_lineage",
		execution.ProgramMetadataKeyGroupID:      "group_lineage",
		execution.ProgramMetadataKeyNodeID:       "node_apply",
		execution.ProgramMetadataKeyParentStepID: "parent_step",
		execution.ProgramMetadataKeyDependsOn:    []string{"node_prepare"},
	}, execution.Target{TargetID: "host-a", Kind: "host"}, 1, 1)

	reader := &fakeExecutionFactReader{
		cycles: []execution.ExecutionCycle{{
			CycleID:   "cycle-1",
			SessionID: "session-1",
			Actions: []execution.ActionRecord{{
				ActionID:  "action-1",
				SessionID: "session-1",
				CycleID:   "cycle-1",
				Metadata:  metadata,
			}},
		}},
	}

	projection, err := NewReader(reader).SessionProjection("session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projection.Cycles) != 1 {
		t.Fatalf("expected one cycle projection, got %#v", projection.Cycles)
	}
	if projection.Cycles[0].Program == nil || !projection.Cycles[0].Program.HasLineage() {
		t.Fatalf("expected structured program lineage on cycle projection, got %#v", projection.Cycles[0])
	}
	if projection.Cycles[0].Program.ProgramID != "prog_lineage" || projection.Cycles[0].Program.GroupID != "group_lineage" || projection.Cycles[0].Program.NodeID != "node_apply" {
		t.Fatalf("unexpected cycle program lineage: %#v", projection.Cycles[0].Program)
	}
	if !reflect.DeepEqual(projection.Cycles[0].Program.DependsOn, []string{"node_prepare"}) {
		t.Fatalf("unexpected cycle dependency lineage: %#v", projection.Cycles[0].Program)
	}
	if len(projection.Cycles[0].TargetSlices) != 1 {
		t.Fatalf("expected one target slice, got %#v", projection.Cycles[0].TargetSlices)
	}
	if projection.Cycles[0].TargetSlices[0].Program == nil || projection.Cycles[0].TargetSlices[0].Program.NodeID != "node_apply" || projection.Cycles[0].TargetSlices[0].Program.GroupID != "group_lineage" {
		t.Fatalf("expected target slice to retain program lineage, got %#v", projection.Cycles[0].TargetSlices[0])
	}
}

func TestExecutionCycleProjectionMarshalOmitsEmptyProgramLineage(t *testing.T) {
	data, err := json.Marshal(ExecutionCycleProjection{})
	if err != nil {
		t.Fatalf("marshal execution cycle projection: %v", err)
	}
	if string(data) != "{\"cycle\":{\"cycle_id\":\"\",\"session_id\":\"\"}}" {
		t.Fatalf("unexpected execution cycle projection json: %s", data)
	}
}

func TestReaderProjectsApprovalLinkageAndInteractiveRuntimeLineage(t *testing.T) {
	handleMetadata := execution.ApplyInteractiveRuntimeMetadata(map[string]any{
		execution.ProgramMetadataKeyID:      "prog_interactive",
		execution.ProgramMetadataKeyGroupID: "group_interactive",
		execution.ProgramMetadataKeyNodeID:  "node_interactive",
		execution.TargetMetadataKeyID:       "target-1",
		execution.TargetMetadataKeyKind:     "host",
	}, &execution.InteractiveCapabilities{View: true}, &execution.InteractiveObservation{
		Status: "active",
	}, nil)

	reader := &fakeExecutionFactReader{
		cycles: []execution.ExecutionCycle{{
			CycleID:    "cycle-1",
			SessionID:  "session-1",
			StepID:     "step_interactive",
			ApprovalID: "approval-1",
			RuntimeHandles: []execution.RuntimeHandle{{
				HandleID:  "handle-1",
				SessionID: "session-1",
				AttemptID: "attempt-1",
				CycleID:   "cycle-1",
				Status:    execution.RuntimeHandleActive,
				Metadata:  handleMetadata,
			}},
		}},
	}

	projection, err := NewReader(reader).ExecutionCycleProjection("session-1", "cycle-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projection.ApprovalLinkage == nil || projection.ApprovalLinkage.ApprovalID != "approval-1" || projection.ApprovalLinkage.StepID != "step_interactive" {
		t.Fatalf("expected approval linkage on cycle projection, got %#v", projection)
	}
	if len(projection.InteractiveRuntimes) != 1 || projection.InteractiveRuntimes[0].Lineage == nil {
		t.Fatalf("expected interactive runtime lineage on cycle projection, got %#v", projection.InteractiveRuntimes)
	}
	if projection.InteractiveRuntimes[0].Lineage.HandleID != "handle-1" || projection.InteractiveRuntimes[0].Lineage.AttemptID != "attempt-1" || projection.InteractiveRuntimes[0].Lineage.CycleID != "cycle-1" {
		t.Fatalf("unexpected runtime handle linkage: %#v", projection.InteractiveRuntimes[0].Lineage)
	}
	if projection.InteractiveRuntimes[0].Lineage.Program == nil || projection.InteractiveRuntimes[0].Lineage.Program.ProgramID != "prog_interactive" || projection.InteractiveRuntimes[0].Lineage.Program.NodeID != "node_interactive" {
		t.Fatalf("expected interactive runtime program lineage, got %#v", projection.InteractiveRuntimes[0].Lineage)
	}
}

type fakeExecutionFactReader struct {
	cycles    []execution.ExecutionCycle
	events    []audit.Event
	blocked   []execution.BlockedRuntimeProjection
	getReturn map[string]execution.ExecutionCycle
	getCalls  []string
}

func (f *fakeExecutionFactReader) ListExecutionCycles(sessionID string) ([]execution.ExecutionCycle, error) {
	return f.cycles, nil
}

func (f *fakeExecutionFactReader) ListAuditEvents(sessionID string) ([]audit.Event, error) {
	return f.events, nil
}

func (f *fakeExecutionFactReader) ListBlockedRuntimeProjections() ([]execution.BlockedRuntimeProjection, error) {
	return f.blocked, nil
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
