package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func (s *Service) GetBlockedRuntimeProjection(sessionID string) (execution.BlockedRuntimeProjection, error) {
	blocked, err := s.getBlockedRuntimeRecord(context.Background(), sessionID)
	if err != nil {
		return execution.BlockedRuntimeProjection{}, err
	}
	return s.blockedRuntimeProjection(context.Background(), blocked)
}

func (s *Service) GetBlockedRuntimeProjectionByApproval(approvalID string) (execution.BlockedRuntimeProjection, error) {
	blocked, err := s.getBlockedRuntimeByApprovalRecord(context.Background(), approvalID)
	if err != nil {
		return execution.BlockedRuntimeProjection{}, err
	}
	return s.blockedRuntimeProjection(context.Background(), blocked)
}

func (s *Service) ListBlockedRuntimeProjections() ([]execution.BlockedRuntimeProjection, error) {
	items, err := s.listBlockedRuntimeRecords(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]execution.BlockedRuntimeProjection, 0, len(items))
	for _, item := range items {
		projected, err := s.blockedRuntimeProjection(context.Background(), item)
		if err != nil {
			return nil, err
		}
		out = append(out, projected)
	}
	return out, nil
}

func (s *Service) blockedRuntimeProjection(ctx context.Context, blocked execution.BlockedRuntime) (execution.BlockedRuntimeProjection, error) {
	view := execution.BlockedRuntimeProjection{
		Runtime: blocked,
		Wait: execution.BlockedRuntimeWait{
			Scope:      execution.BlockedRuntimeWaitStep,
			StepID:     blocked.StepID,
			WaitingFor: blocked.WaitingFor,
		},
		InteractiveRuntimes: execution.InteractiveRuntimesFromHandles(blocked.RuntimeHandles),
		Metadata: map[string]any{
			"cycle_id":    blocked.CycleID,
			"approval_id": blocked.ApprovalID,
		},
	}
	if target, ok := execution.TargetFromStep(blocked.Step); ok {
		view.Wait.Scope = execution.BlockedRuntimeWaitTarget
		view.Wait.Target = target
	}
	if blocked.CycleID == "" {
		return view, nil
	}

	cycle, err := s.GetExecutionCycle(blocked.SessionID, blocked.CycleID)
	switch {
	case err == nil:
		view.TargetSlices = execution.TargetSlicesFromCycle(cycle)
		if len(view.InteractiveRuntimes) == 0 {
			view.InteractiveRuntimes = execution.InteractiveRuntimesFromHandles(cycle.RuntimeHandles)
		}
		return view, nil
	case errors.Is(err, execution.ErrExecutionCycleNotFound):
		return view, nil
	default:
		return execution.BlockedRuntimeProjection{}, err
	}
}
