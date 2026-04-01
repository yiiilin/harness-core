package runtime_test

import (
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func mustCreateSession(tb testing.TB, rt *hruntime.Service, title, goal string) session.State {
	tb.Helper()
	sess, err := rt.CreateSession(title, goal)
	if err != nil {
		tb.Fatalf("create session: %v", err)
	}
	return sess
}

func mustCreateTask(tb testing.TB, rt *hruntime.Service, spec task.Spec) task.Record {
	tb.Helper()
	rec, err := rt.CreateTask(spec)
	if err != nil {
		tb.Fatalf("create task: %v", err)
	}
	return rec
}

func mustListAuditEvents(tb testing.TB, rt *hruntime.Service, sessionID string) []audit.Event {
	tb.Helper()
	items, err := rt.ListAuditEvents(sessionID)
	if err != nil {
		tb.Fatalf("list audit events: %v", err)
	}
	return items
}

func mustFindAuditEventType(tb testing.TB, events []audit.Event, eventType string) audit.Event {
	tb.Helper()
	for _, event := range events {
		if event.Type == eventType {
			return event
		}
	}
	tb.Fatalf("expected audit event %q, got %#v", eventType, events)
	return audit.Event{}
}

func mustListPlans(tb testing.TB, rt *hruntime.Service, sessionID string) []plan.Spec {
	tb.Helper()
	items, err := rt.ListPlans(sessionID)
	if err != nil {
		tb.Fatalf("list plans: %v", err)
	}
	return items
}

func mustPlanByRevision(tb testing.TB, rt *hruntime.Service, sessionID string, revision int) plan.Spec {
	tb.Helper()
	items := mustListPlans(tb, rt, sessionID)
	for _, item := range items {
		if item.Revision == revision {
			return item
		}
	}
	tb.Fatalf("expected plan revision %d, got %#v", revision, items)
	return plan.Spec{}
}

func mustListAttempts(tb testing.TB, rt *hruntime.Service, sessionID string) []execution.Attempt {
	tb.Helper()
	items, err := rt.ListAttempts(sessionID)
	if err != nil {
		tb.Fatalf("list attempts: %v", err)
	}
	return items
}

func mustListActions(tb testing.TB, rt *hruntime.Service, sessionID string) []execution.ActionRecord {
	tb.Helper()
	items, err := rt.ListActions(sessionID)
	if err != nil {
		tb.Fatalf("list actions: %v", err)
	}
	return items
}

func mustListVerifications(tb testing.TB, rt *hruntime.Service, sessionID string) []execution.VerificationRecord {
	tb.Helper()
	items, err := rt.ListVerifications(sessionID)
	if err != nil {
		tb.Fatalf("list verifications: %v", err)
	}
	return items
}

func mustListArtifacts(tb testing.TB, rt *hruntime.Service, sessionID string) []execution.Artifact {
	tb.Helper()
	items, err := rt.ListArtifacts(sessionID)
	if err != nil {
		tb.Fatalf("list artifacts: %v", err)
	}
	return items
}

func mustListRuntimeHandles(tb testing.TB, rt *hruntime.Service, sessionID string) []execution.RuntimeHandle {
	tb.Helper()
	items, err := rt.ListRuntimeHandles(sessionID)
	if err != nil {
		tb.Fatalf("list runtime handles: %v", err)
	}
	return items
}

func mustListCapabilitySnapshots(tb testing.TB, rt *hruntime.Service, sessionID string) []capability.Snapshot {
	tb.Helper()
	items, err := rt.ListCapabilitySnapshots(sessionID)
	if err != nil {
		tb.Fatalf("list capability snapshots: %v", err)
	}
	return items
}

func mustListPlanningRecords(tb testing.TB, rt *hruntime.Service, sessionID string) []planning.Record {
	tb.Helper()
	items, err := rt.ListPlanningRecords(sessionID)
	if err != nil {
		tb.Fatalf("list planning records: %v", err)
	}
	return items
}

func mustListRecoverableSessions(tb testing.TB, rt *hruntime.Service) []session.State {
	tb.Helper()
	items, err := rt.ListRecoverableSessions()
	if err != nil {
		tb.Fatalf("list recoverable sessions: %v", err)
	}
	return items
}

func withExplicitPlannerProjection(opts hruntime.Options) hruntime.Options {
	if opts.RuntimePolicy.Planner.Projection.Mode == "" {
		opts.RuntimePolicy.Planner.Projection = hruntime.PlannerProjectionPolicy{Mode: hruntime.PlannerProjectionRaw}
	}
	return opts
}
