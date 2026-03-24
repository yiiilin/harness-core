package runtime

import (
	"context"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

type InteractiveRuntimeUpdate struct {
	Capabilities  *execution.InteractiveCapabilities `json:"capabilities,omitempty"`
	Observation   *execution.InteractiveObservation  `json:"observation,omitempty"`
	LastOperation *execution.InteractiveOperation    `json:"last_operation,omitempty"`
	Metadata      map[string]any                     `json:"metadata,omitempty"`
}

func (s *Service) GetInteractiveRuntime(handleID string) (execution.InteractiveRuntime, error) {
	handle, err := s.GetRuntimeHandle(handleID)
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	out, ok := execution.InteractiveRuntimeFromHandle(handle)
	if !ok {
		return execution.InteractiveRuntime{}, execution.ErrRecordNotFound
	}
	return out, nil
}

func (s *Service) ListInteractiveRuntimes(sessionID string) ([]execution.InteractiveRuntime, error) {
	handles, err := s.ListRuntimeHandles(sessionID)
	if err != nil {
		return nil, err
	}
	return execution.InteractiveRuntimesFromHandles(handles), nil
}

func (s *Service) UpdateInteractiveRuntime(ctx context.Context, handleID string, update InteractiveRuntimeUpdate) (execution.InteractiveRuntime, error) {
	handle, err := s.UpdateRuntimeHandle(ctx, handleID, runtimeHandleUpdateFromInteractive(update))
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	out, ok := execution.InteractiveRuntimeFromHandle(handle)
	if !ok {
		return execution.InteractiveRuntime{}, execution.ErrRecordNotFound
	}
	return out, nil
}

func (s *Service) UpdateClaimedInteractiveRuntime(ctx context.Context, handleID, leaseID string, update InteractiveRuntimeUpdate) (execution.InteractiveRuntime, error) {
	handle, err := s.UpdateClaimedRuntimeHandle(ctx, handleID, leaseID, runtimeHandleUpdateFromInteractive(update))
	if err != nil {
		return execution.InteractiveRuntime{}, err
	}
	out, ok := execution.InteractiveRuntimeFromHandle(handle)
	if !ok {
		return execution.InteractiveRuntime{}, execution.ErrRecordNotFound
	}
	return out, nil
}

func runtimeHandleUpdateFromInteractive(update InteractiveRuntimeUpdate) RuntimeHandleUpdate {
	metadata := execution.ApplyInteractiveRuntimeMetadata(update.Metadata, update.Capabilities, update.Observation, update.LastOperation)
	return RuntimeHandleUpdate{Metadata: metadata}
}
