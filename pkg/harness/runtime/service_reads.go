package runtime

import (
	"context"
	"errors"
	"sort"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/audit"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/persistence"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/task"
)

func (s *Service) readRepositories(ctx context.Context, fn func(repos persistence.RepositorySet) error) error {
	if s.Runner != nil {
		return s.Runner.Within(ctx, func(repos persistence.RepositorySet) error {
			return fn(s.repositoriesWithFallback(repos))
		})
	}
	return fn(s.repositoriesWithFallback(persistence.RepositorySet{}))
}

func (s *Service) getSessionRecord(ctx context.Context, id string) (session.State, error) {
	var out session.State
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return session.ErrSessionNotFound
		}
		var err error
		out, err = repos.Sessions.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listSessionRecords(ctx context.Context) ([]session.State, error) {
	var out []session.State
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return nil
		}
		var err error
		out, err = repos.Sessions.List()
		return err
	})
	return out, err
}

func (s *Service) getTaskRecord(ctx context.Context, id string) (task.Record, error) {
	var out task.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Tasks == nil {
			return task.ErrTaskNotFound
		}
		var err error
		out, err = repos.Tasks.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listTaskRecords(ctx context.Context) ([]task.Record, error) {
	var out []task.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Tasks == nil {
			return nil
		}
		var err error
		out, err = repos.Tasks.List()
		return err
	})
	return out, err
}

func (s *Service) getPlanRecord(ctx context.Context, planID string) (plan.Spec, error) {
	var out plan.Spec
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Plans == nil {
			return plan.ErrPlanNotFound
		}
		var err error
		out, err = repos.Plans.Get(planID)
		return err
	})
	out = annotatePlanIdentity(out)
	return out, err
}

func (s *Service) listPlanRecords(ctx context.Context, sessionID string) ([]plan.Spec, error) {
	var out []plan.Spec
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Plans == nil {
			return nil
		}
		var err error
		out, err = repos.Plans.ListBySession(sessionID)
		return err
	})
	for i := range out {
		out[i] = annotatePlanIdentity(out[i])
	}
	return out, err
}

func (s *Service) latestPlanForSession(ctx context.Context, sessionID string) (plan.Spec, bool, error) {
	var (
		out plan.Spec
		ok  bool
	)
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Plans == nil {
			return nil
		}
		var err error
		out, ok, err = repos.Plans.LatestBySession(sessionID)
		return err
	})
	out = annotatePlanIdentity(out)
	return out, ok, err
}

func (s *Service) getApprovalRecord(ctx context.Context, id string) (approval.Record, error) {
	var out approval.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Approvals == nil {
			return approval.ErrApprovalNotFound
		}
		var err error
		out, err = repos.Approvals.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listApprovalRecords(ctx context.Context, sessionID string) ([]approval.Record, error) {
	var out []approval.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Approvals == nil {
			return nil
		}
		var err error
		out, err = repos.Approvals.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listAttemptRecords(ctx context.Context, sessionID string) ([]execution.Attempt, error) {
	var out []execution.Attempt
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Attempts == nil {
			return nil
		}
		var err error
		out, err = repos.Attempts.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listActionRecords(ctx context.Context, sessionID string) ([]execution.ActionRecord, error) {
	var out []execution.ActionRecord
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Actions == nil {
			return nil
		}
		var err error
		out, err = repos.Actions.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listVerificationRecords(ctx context.Context, sessionID string) ([]execution.VerificationRecord, error) {
	var out []execution.VerificationRecord
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Verifications == nil {
			return nil
		}
		var err error
		out, err = repos.Verifications.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listArtifactRecords(ctx context.Context, sessionID string) ([]execution.Artifact, error) {
	var out []execution.Artifact
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Artifacts == nil {
			return nil
		}
		var err error
		out, err = repos.Artifacts.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) getRuntimeHandleRecord(ctx context.Context, id string) (execution.RuntimeHandle, error) {
	var out execution.RuntimeHandle
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.RuntimeHandles == nil {
			return execution.ErrRecordNotFound
		}
		var err error
		out, err = repos.RuntimeHandles.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listRuntimeHandleRecords(ctx context.Context, sessionID string) ([]execution.RuntimeHandle, error) {
	var out []execution.RuntimeHandle
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.RuntimeHandles == nil {
			return nil
		}
		var err error
		out, err = repos.RuntimeHandles.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listCapabilitySnapshotRecords(ctx context.Context, sessionID string) ([]capability.Snapshot, error) {
	var out []capability.Snapshot
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.CapabilitySnapshots == nil {
			return nil
		}
		var err error
		out, err = repos.CapabilitySnapshots.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) getBlockedRuntimeRecord(ctx context.Context, sessionID string) (execution.BlockedRuntime, error) {
	var out execution.BlockedRuntime
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return session.ErrSessionNotFound
		}
		state, err := repos.Sessions.Get(sessionID)
		if err != nil {
			return err
		}
		out, err = blockedRuntimeFromStateAndRepos(state, repos)
		return err
	})
	return out, err
}

func (s *Service) getBlockedRuntimeByApprovalRecord(ctx context.Context, approvalID string) (execution.BlockedRuntime, error) {
	var out execution.BlockedRuntime
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		var err error
		out, err = blockedRuntimeByApprovalInRepos(approvalID, repos)
		return err
	})
	return out, err
}

func (s *Service) getBlockedRuntimeByIDRecord(ctx context.Context, blockedRuntimeID string) (execution.BlockedRuntime, error) {
	var out execution.BlockedRuntime
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.BlockedRuntimes != nil {
			rec, err := blockedRuntimeRecordOrErr(repos.BlockedRuntimes, blockedRuntimeID)
			switch {
			case err == nil:
				state, stateErr := repos.Sessions.Get(rec.SessionID)
				if stateErr != nil {
					return stateErr
				}
				out, err = blockedRuntimeFromStateAndGenericRecord(state, rec, repos)
				return err
			case errors.Is(err, execution.ErrBlockedRuntimeNotFound):
			default:
				return err
			}
		}
		var err error
		out, err = blockedRuntimeByApprovalInRepos(blockedRuntimeID, repos)
		return err
	})
	return out, err
}

func (s *Service) getBlockedRuntimeStoredRecord(ctx context.Context, blockedRuntimeID string) (execution.BlockedRuntimeRecord, error) {
	var out execution.BlockedRuntimeRecord
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		rec, err := blockedRuntimeRecordOrErr(repos.BlockedRuntimes, blockedRuntimeID)
		switch {
		case err == nil:
			out = cloneBlockedRuntimeRecord(rec)
			return nil
		case !errors.Is(err, execution.ErrBlockedRuntimeNotFound):
			return err
		}
		if repos.Approvals == nil {
			return execution.ErrBlockedRuntimeNotFound
		}
		approvalRec, err := repos.Approvals.Get(blockedRuntimeID)
		if err != nil {
			if errors.Is(err, approval.ErrApprovalNotFound) {
				return execution.ErrBlockedRuntimeNotFound
			}
			return err
		}
		out = blockedRuntimeRecordFromApproval(approvalRec)
		return nil
	})
	return out, err
}

func (s *Service) listBlockedRuntimeStoredRecords(ctx context.Context, sessionID string) ([]execution.BlockedRuntimeRecord, error) {
	var out []execution.BlockedRuntimeRecord
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.BlockedRuntimes == nil {
			return nil
		}
		items, err := repos.BlockedRuntimes.List(sessionID)
		if err != nil {
			return err
		}
		out = make([]execution.BlockedRuntimeRecord, 0, len(items))
		for _, item := range items {
			out = append(out, cloneBlockedRuntimeRecord(item))
		}
		return nil
	})
	return out, err
}

func (s *Service) listBlockedRuntimeRecords(ctx context.Context) ([]execution.BlockedRuntime, error) {
	var out []execution.BlockedRuntime
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Sessions == nil {
			return nil
		}
		sessions, err := repos.Sessions.List()
		if err != nil {
			return err
		}
		out = make([]execution.BlockedRuntime, 0, len(sessions))
		for _, state := range sessions {
			if state.PendingApprovalID == "" && currentBlockedRuntimeID(state) == "" {
				continue
			}
			item, err := blockedRuntimeFromStateAndRepos(state, repos)
			switch {
			case err == nil:
				out = append(out, item)
			case errors.Is(err, execution.ErrBlockedRuntimeNotFound):
				continue
			default:
				return err
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].RequestedAt != out[j].RequestedAt {
				return out[i].RequestedAt < out[j].RequestedAt
			}
			return out[i].BlockedRuntimeID < out[j].BlockedRuntimeID
		})
		return nil
	})
	return out, err
}

func blockedRuntimeFromStateAndRepos(state session.State, repos persistence.RepositorySet) (execution.BlockedRuntime, error) {
	if state.PendingApprovalID != "" {
		if repos.Approvals == nil {
			return execution.BlockedRuntime{}, approval.ErrApprovalNotFound
		}
		rec, err := repos.Approvals.Get(state.PendingApprovalID)
		if err != nil {
			return execution.BlockedRuntime{}, err
		}
		return blockedRuntimeFromStateAndApproval(state, rec, repos)
	}
	blockedRuntimeID := currentBlockedRuntimeID(state)
	if blockedRuntimeID == "" {
		return execution.BlockedRuntime{}, execution.ErrBlockedRuntimeNotFound
	}
	rec, err := blockedRuntimeRecordOrErr(repos.BlockedRuntimes, blockedRuntimeID)
	if err != nil {
		return execution.BlockedRuntime{}, err
	}
	return blockedRuntimeFromStateAndGenericRecord(state, rec, repos)
}

func blockedRuntimeByApprovalInRepos(approvalID string, repos persistence.RepositorySet) (execution.BlockedRuntime, error) {
	if repos.Approvals == nil {
		return execution.BlockedRuntime{}, approval.ErrApprovalNotFound
	}
	rec, err := repos.Approvals.Get(approvalID)
	if err != nil {
		return execution.BlockedRuntime{}, err
	}
	if repos.Sessions == nil {
		return execution.BlockedRuntime{}, session.ErrSessionNotFound
	}
	state, err := repos.Sessions.Get(rec.SessionID)
	if err != nil {
		return execution.BlockedRuntime{}, err
	}
	if state.PendingApprovalID != approvalID {
		return execution.BlockedRuntime{}, execution.ErrBlockedRuntimeNotFound
	}
	return blockedRuntimeFromStateAndApproval(state, rec, repos)
}

func blockedRuntimeFromStateAndApproval(state session.State, rec approval.Record, repos persistence.RepositorySet) (execution.BlockedRuntime, error) {
	if state.PendingApprovalID == "" || state.PendingApprovalID != rec.ApprovalID || rec.Status != approval.StatusPending || rec.SessionID != "" && rec.SessionID != state.SessionID {
		return execution.BlockedRuntime{}, execution.ErrBlockedRuntimeNotFound
	}
	attempt, ok, err := findBlockedAttemptForApprovalProjectionInStore(repos.Attempts, state.SessionID, rec)
	if err != nil {
		return execution.BlockedRuntime{}, err
	}
	cycleID := executionCycleIDFromStep(rec.Step)
	attemptID := ""
	if ok {
		attemptID = attempt.AttemptID
		if attempt.CycleID != "" {
			cycleID = attempt.CycleID
		}
	}
	target := execution.TargetRef{}
	if ok {
		if ref, hasRef := execution.TargetRefFromMetadata(attempt.Metadata); hasRef {
			target = ref
		} else if ref, hasRef := execution.TargetRefFromMetadata(attempt.Step.Metadata); hasRef {
			target = ref
		}
	}
	if target.TargetID == "" {
		if ref, hasRef := execution.TargetFromStep(rec.Step); hasRef {
			target = ref
		}
	}
	handles, err := blockedRuntimeHandlesForCycle(repos.RuntimeHandles, state.SessionID, cycleID)
	if err != nil {
		return execution.BlockedRuntime{}, err
	}
	return execution.BlockedRuntime{
		BlockedRuntimeID: rec.ApprovalID,
		Kind:             execution.BlockedRuntimeApproval,
		Status:           execution.BlockedRuntimePending,
		WaitingFor:       "approval",
		SessionID:        state.SessionID,
		TaskID:           firstNonEmptyString(state.TaskID, rec.TaskID),
		StepID:           rec.StepID,
		ApprovalID:       rec.ApprovalID,
		AttemptID:        attemptID,
		CycleID:          cycleID,
		Target:           target,
		Condition: execution.BlockedRuntimeCondition{
			Kind:        execution.BlockedRuntimeConditionApproval,
			ReferenceID: rec.ApprovalID,
			WaitingFor:  "approval",
			Metadata:    cloneAnyMap(rec.Metadata),
		},
		Metadata:       cloneAnyMap(rec.Metadata),
		Step:           rec.Step,
		Approval:       rec,
		RuntimeHandles: handles,
		RequestedAt:    rec.RequestedAt,
		UpdatedAt:      rec.UpdatedAt,
	}, nil
}

func blockedRuntimeFromStateAndGenericRecord(state session.State, rec execution.BlockedRuntimeRecord, repos persistence.RepositorySet) (execution.BlockedRuntime, error) {
	if currentBlockedRuntimeID(state) == "" || currentBlockedRuntimeID(state) != rec.BlockedRuntimeID || state.ExecutionState != session.ExecutionBlocked || rec.SessionID != "" && rec.SessionID != state.SessionID {
		return execution.BlockedRuntime{}, execution.ErrBlockedRuntimeNotFound
	}
	return blockedRuntimeFromGenericRecord(state, rec, repos)
}

func blockedRuntimeFromGenericRecord(state session.State, rec execution.BlockedRuntimeRecord, repos persistence.RepositorySet) (execution.BlockedRuntime, error) {
	step, _, err := latestPlanStepByID(repos.Plans, state.SessionID, "", rec.Subject.StepID)
	if err != nil {
		return execution.BlockedRuntime{}, err
	}
	handles, err := blockedRuntimeHandlesForCycle(repos.RuntimeHandles, state.SessionID, rec.Subject.CycleID)
	if err != nil {
		return execution.BlockedRuntime{}, err
	}
	return execution.BlockedRuntime{
		BlockedRuntimeID: rec.BlockedRuntimeID,
		Kind:             rec.Kind,
		Status:           rec.Status,
		WaitingFor:       rec.Condition.WaitingFor,
		SessionID:        state.SessionID,
		TaskID:           firstNonEmptyString(state.TaskID, rec.TaskID),
		StepID:           rec.Subject.StepID,
		ActionID:         rec.Subject.ActionID,
		AttemptID:        rec.Subject.AttemptID,
		CycleID:          rec.Subject.CycleID,
		Target:           rec.Subject.Target,
		Condition:        cloneBlockedRuntimeCondition(rec.Condition),
		Metadata:         cloneAnyMap(rec.Metadata),
		Step:             step,
		RuntimeHandles:   handles,
		RequestedAt:      rec.RequestedAt,
		UpdatedAt:        rec.UpdatedAt,
	}, nil
}

func blockedRuntimeHandlesForCycle(store execution.RuntimeHandleStore, sessionID, cycleID string) ([]execution.RuntimeHandle, error) {
	if store == nil || cycleID == "" {
		return nil, nil
	}
	handles, err := store.List(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]execution.RuntimeHandle, 0, len(handles))
	for _, handle := range handles {
		if handle.CycleID == cycleID {
			out = append(out, handle)
		}
	}
	return out, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneBlockedRuntimeRecord(in execution.BlockedRuntimeRecord) execution.BlockedRuntimeRecord {
	out := in
	out.Subject = cloneBlockedRuntimeSubject(in.Subject)
	out.Condition = cloneBlockedRuntimeCondition(in.Condition)
	out.Metadata = cloneAnyMap(in.Metadata)
	return out
}

func cloneBlockedRuntimeCondition(in execution.BlockedRuntimeCondition) execution.BlockedRuntimeCondition {
	out := in
	out.Metadata = cloneAnyMap(in.Metadata)
	return out
}

func blockedRuntimeRecordFromApproval(rec approval.Record) execution.BlockedRuntimeRecord {
	return execution.BlockedRuntimeRecord{
		BlockedRuntimeID: rec.ApprovalID,
		Kind:             execution.BlockedRuntimeApproval,
		Status:           blockedRuntimeStatusFromApproval(rec.Status),
		SessionID:        rec.SessionID,
		TaskID:           rec.TaskID,
		Subject: execution.BlockedRuntimeSubject{
			StepID: rec.StepID,
		},
		Condition: execution.BlockedRuntimeCondition{
			Kind:        execution.BlockedRuntimeConditionApproval,
			ReferenceID: rec.ApprovalID,
			WaitingFor:  "approval",
			Metadata:    cloneAnyMap(rec.Metadata),
		},
		Metadata:    cloneAnyMap(rec.Metadata),
		RequestedAt: rec.RequestedAt,
		UpdatedAt:   rec.UpdatedAt,
		ResolvedAt:  maxInt64(rec.RespondedAt, rec.ConsumedAt),
	}
}

func blockedRuntimeStatusFromApproval(status approval.Status) execution.BlockedRuntimeStatus {
	switch status {
	case approval.StatusApproved, approval.StatusConsumed:
		return execution.BlockedRuntimeApproved
	case approval.StatusRejected:
		return execution.BlockedRuntimeRejected
	default:
		return execution.BlockedRuntimePending
	}
}

func maxInt64(values ...int64) int64 {
	var out int64
	for _, value := range values {
		if value > out {
			out = value
		}
	}
	return out
}

func (s *Service) getPlanningRecord(ctx context.Context, id string) (planning.Record, error) {
	var out planning.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.PlanningRecords == nil {
			return planning.ErrPlanningRecordNotFound
		}
		var err error
		out, err = repos.PlanningRecords.Get(id)
		return err
	})
	return out, err
}

func (s *Service) listPlanningRecords(ctx context.Context, sessionID string) ([]planning.Record, error) {
	var out []planning.Record
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.PlanningRecords == nil {
			return nil
		}
		var err error
		out, err = repos.PlanningRecords.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) listAuditStoreEvents(ctx context.Context, sessionID string) ([]audit.Event, error) {
	var out []audit.Event
	err := s.readRepositories(ctx, func(repos persistence.RepositorySet) error {
		if repos.Audits == nil {
			return nil
		}
		var err error
		out, err = repos.Audits.List(sessionID)
		return err
	})
	return out, err
}

func (s *Service) latestPlanHasRemainingSteps(ctx context.Context, sessionID string, completedStep plan.StepSpec) (bool, error) {
	target, ok, err := s.planForStep(ctx, sessionID, completedStep)
	if err != nil || !ok {
		return false, err
	}
	for _, st := range target.Steps {
		status := st.Status
		if st.StepID == completedStep.StepID {
			status = plan.StepCompleted
		}
		if status != plan.StepCompleted {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) planForStep(ctx context.Context, sessionID string, step plan.StepSpec) (plan.Spec, bool, error) {
	if step.PlanID != "" {
		item, err := s.getPlanRecord(ctx, step.PlanID)
		if err == nil {
			return annotatePlanIdentity(item), true, nil
		}
		if !errors.Is(err, plan.ErrPlanNotFound) {
			return plan.Spec{}, false, err
		}
	}
	latest, ok, err := s.latestPlanForSession(ctx, sessionID)
	if err != nil || !ok {
		return plan.Spec{}, ok, err
	}
	return annotatePlanIdentity(latest), true, nil
}

func (s *Service) pinnedPlanForSession(ctx context.Context, st session.State) (plan.Spec, bool, error) {
	if st.InFlightStepID == "" && st.CurrentStepID == "" {
		return plan.Spec{}, false, nil
	}
	planID, _, ok := planRefFromSession(st)
	if !ok {
		return plan.Spec{}, false, nil
	}
	item, err := s.getPlanRecord(ctx, planID)
	if err == nil {
		return annotatePlanIdentity(item), true, nil
	}
	if errors.Is(err, plan.ErrPlanNotFound) {
		return plan.Spec{}, false, nil
	}
	return plan.Spec{}, false, err
}
