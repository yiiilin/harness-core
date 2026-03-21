package runtime

import (
	"context"
	"reflect"

	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
)

const (
	capabilityViewContextKey    = "capability_view"
	capabilityViewMetadataKey   = "capability_view_id"
	capabilityPlanReasonKey     = "capability_plan_reason"
	capabilityFreezeReasonValue = "planner frozen capability view"
)

func (s *Service) freezeCapabilityView(ctx context.Context, sessionID, taskID string) (capability.View, error) {
	if s.CapabilityFreezer == nil {
		return capability.View{}, nil
	}
	return s.CapabilityFreezer.Freeze(ctx, sessionID, taskID)
}

func attachCapabilityViewToContext(pkg ContextPackage, view capability.View) ContextPackage {
	if view.ViewID == "" {
		return pkg
	}
	if pkg.Extras == nil {
		pkg.Extras = map[string]any{}
	}
	entries := make([]map[string]any, 0, len(view.Entries))
	for _, entry := range view.Entries {
		entries = append(entries, map[string]any{
			"tool_name":       entry.ToolName,
			"version":         entry.Version,
			"capability_type": entry.CapabilityType,
			"risk_level":      entry.RiskLevel,
			"metadata":        entry.Metadata,
		})
	}
	pkg.Extras[capabilityViewContextKey] = map[string]any{
		"view_id":   view.ViewID,
		"frozen_at": view.FrozenAt,
		"entries":   entries,
	}
	return pkg
}

func pinStepToCapabilityView(step plan.StepSpec, view capability.View) (plan.StepSpec, error) {
	if view.ViewID != "" {
		if step.Metadata == nil {
			step.Metadata = map[string]any{}
		}
		step.Metadata[capabilityViewMetadataKey] = view.ViewID
		step.Metadata[capabilityPlanReasonKey] = capabilityFreezeReasonValue
	}
	if step.Action.ToolName == "" {
		return step, nil
	}
	entry, ok := findFrozenCapabilityEntry(view, step.Action.ToolName, step.Action.ToolVersion)
	if ok && step.Action.ToolVersion == "" {
		step.Action.ToolVersion = entry.Version
	}
	return step, nil
}

func findFrozenCapabilityEntry(view capability.View, toolName, requestedVersion string) (capability.Snapshot, bool) {
	var matched capability.Snapshot
	found := false
	for _, entry := range view.Entries {
		if entry.ToolName != toolName {
			continue
		}
		if requestedVersion != "" {
			if entry.Version == requestedVersion {
				return entry, true
			}
			continue
		}
		matched = entry
		found = true
	}
	return matched, found
}

func capabilityViewIDFromStep(step plan.StepSpec) string {
	if len(step.Metadata) == 0 {
		return ""
	}
	viewID, _ := step.Metadata[capabilityViewMetadataKey].(string)
	return viewID
}

func (s *Service) createPlanWithCapabilityView(ctx context.Context, sessionID, changeReason string, steps []plan.StepSpec, view capability.View, planningRecord planning.Record) (plan.Spec, error) {
	sess, err := s.Sessions.Get(sessionID)
	if err != nil {
		return plan.Spec{}, err
	}

	var created plan.Spec
	create := func(planStore plan.Store, snapshotStore capability.SnapshotStore, planningStore planning.Store, sink EventSink) error {
		if err := ensurePlanRevisionBudgetInStore(planStore, sessionID, s.LoopBudgets); err != nil {
			return err
		}
		var err error
		created, err = planStore.Create(sessionID, changeReason, steps)
		if err != nil {
			return err
		}
		if err := persistCapabilityViewInStore(snapshotStore, created, view); err != nil {
			return err
		}
		if planningStore != nil {
			planningRecord.SessionID = created.SessionID
			if planningRecord.TaskID == "" {
				planningRecord.TaskID = sess.TaskID
			}
			planningRecord.PlanID = created.PlanID
			planningRecord.PlanRevision = created.Revision
			if planningRecord.FinishedAt == 0 {
				planningRecord.FinishedAt = s.nowMilli()
			}
			if _, err := planningStore.Create(planningRecord); err != nil {
				return err
			}
		}
		event := newLifecycleEventAt(s.nowMilli(), audit.EventPlanGenerated, sessionID, sess.TaskID, map[string]any{
			"plan_id":       created.PlanID,
			"planning_id":   planningRecord.PlanningID,
			"revision":      created.Revision,
			"change_reason": created.ChangeReason,
			"step_count":    len(created.Steps),
		})
		event.PlanningID = planningRecord.PlanningID
		return s.emitEventsWithSink(ctx, sink, []audit.Event{event})
	}

	if s.Runner != nil {
		err := s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			repoSet := s.repositoriesWithFallback(repos)
			return create(repoSet.Plans, repoSet.CapabilitySnapshots, repoSet.PlanningRecords, s.eventSinkForRepos(repos))
		})
		return created, err
	}

	if err := ensurePlanRevisionBudgetInStore(s.Plans, sessionID, s.LoopBudgets); err != nil {
		return plan.Spec{}, err
	}
	created, err = s.Plans.Create(sessionID, changeReason, steps)
	if err != nil {
		return plan.Spec{}, err
	}
	if err := persistCapabilityViewInStore(s.CapabilitySnapshots, created, view); err != nil {
		return plan.Spec{}, err
	}
	if s.PlanningRecords != nil {
		planningRecord.SessionID = created.SessionID
		if planningRecord.TaskID == "" {
			planningRecord.TaskID = sess.TaskID
		}
		planningRecord.PlanID = created.PlanID
		planningRecord.PlanRevision = created.Revision
		if planningRecord.FinishedAt == 0 {
			planningRecord.FinishedAt = s.nowMilli()
		}
		if _, err := s.PlanningRecords.Create(planningRecord); err != nil {
			return plan.Spec{}, err
		}
	}
	event := newLifecycleEventAt(s.nowMilli(), audit.EventPlanGenerated, sessionID, sess.TaskID, map[string]any{
		"plan_id":       created.PlanID,
		"planning_id":   planningRecord.PlanningID,
		"revision":      created.Revision,
		"change_reason": created.ChangeReason,
		"step_count":    len(created.Steps),
	})
	event.PlanningID = planningRecord.PlanningID
	_ = s.emitEventsWithSink(ctx, s.EventSink, []audit.Event{event})
	return created, nil
}

func persistCapabilityViewInStore(store capability.SnapshotStore, pl plan.Spec, view capability.View) error {
	if store == nil || view.ViewID == "" {
		return nil
	}
	for _, entry := range view.Entries {
		snapshot := entry
		snapshot.SnapshotID = ""
		snapshot.SessionID = pl.SessionID
		snapshot.PlanID = pl.PlanID
		snapshot.ViewID = view.ViewID
		snapshot.Scope = capability.SnapshotScopePlan
		snapshot.StepID = ""
		if _, err := store.Create(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) frozenCapabilityEntryForStep(sessionID string, step plan.StepSpec) (capability.Snapshot, bool, error) {
	viewID := capabilityViewIDFromStep(step)
	if viewID == "" || step.Action.ToolName == "" {
		return capability.Snapshot{}, false, nil
	}
	if s.CapabilitySnapshots == nil {
		return capability.Snapshot{}, false, capability.ErrCapabilityViewNotFound
	}
	items, err := s.CapabilitySnapshots.List(sessionID)
	if err != nil {
		return capability.Snapshot{}, false, err
	}
	foundView := false
	for _, item := range items {
		if item.Scope != capability.SnapshotScopePlan || item.ViewID != viewID {
			continue
		}
		foundView = true
		if item.ToolName != step.Action.ToolName {
			continue
		}
		if step.Action.ToolVersion != "" && item.Version != step.Action.ToolVersion {
			continue
		}
		return item, true, nil
	}
	if !foundView {
		return capability.Snapshot{}, false, capability.ErrCapabilityViewNotFound
	}
	return capability.Snapshot{}, false, capability.ErrCapabilityViewDrift
}

func validateFrozenCapabilityResolution(expected capability.Snapshot, resolution capability.Resolution) error {
	if expected.ToolName != resolution.Definition.ToolName {
		return capability.ErrCapabilityViewDrift
	}
	if expected.Version != resolution.Definition.Version {
		return capability.ErrCapabilityViewDrift
	}
	if expected.CapabilityType != resolution.Definition.CapabilityType {
		return capability.ErrCapabilityViewDrift
	}
	if expected.RiskLevel != resolution.Definition.RiskLevel {
		return capability.ErrCapabilityViewDrift
	}
	if !reflect.DeepEqual(expected.Metadata, resolution.Definition.Metadata) {
		return capability.ErrCapabilityViewDrift
	}
	return nil
}
