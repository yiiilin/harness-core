package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/capability"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

const (
	ProgramInteractiveStartToolName  = "interactive.start"
	ProgramInteractiveViewToolName   = "interactive.view"
	ProgramInteractiveWriteToolName  = "interactive.write"
	ProgramInteractiveVerifyToolName = "interactive.verify"
	ProgramInteractiveCloseToolName  = "interactive.close"

	programInteractiveNativeVersion              = "native"
	programInteractivePersistedRuntimeHandlesKey = "_native_runtime_handles_persisted"
)

func (s *Service) invokeNativeProgramAction(ctx context.Context, state session.State, step plan.StepSpec, attempt execution.Attempt, actionRecord *execution.ActionRecord) (*capability.Resolution, action.Result, bool, error) {
	req := capability.Request{
		SessionID: state.SessionID,
		TaskID:    state.TaskID,
		StepID:    step.StepID,
		Action:    step.Action,
	}
	switch step.Action.ToolName {
	case ProgramInteractiveStartToolName:
		if err := s.nativeProgramActionAvailabilityError(step.Action.ToolName); err != nil {
			return nil, capabilityErrorResult(step.Action, err), true, err
		}
		result, err := s.invokeNativeInteractiveStart(ctx, state, step, attempt, actionRecord)
		return s.nativeProgramActionResolution(req), result, true, err
	case ProgramInteractiveViewToolName:
		if err := s.nativeProgramActionAvailabilityError(step.Action.ToolName); err != nil {
			return nil, capabilityErrorResult(step.Action, err), true, err
		}
		result, err := s.invokeNativeInteractiveView(ctx, step)
		return s.nativeProgramActionResolution(req), result, true, err
	case ProgramInteractiveWriteToolName:
		if err := s.nativeProgramActionAvailabilityError(step.Action.ToolName); err != nil {
			return nil, capabilityErrorResult(step.Action, err), true, err
		}
		result, err := s.invokeNativeInteractiveWrite(ctx, step)
		return s.nativeProgramActionResolution(req), result, true, err
	case ProgramInteractiveVerifyToolName:
		if err := s.nativeProgramActionAvailabilityError(step.Action.ToolName); err != nil {
			return nil, capabilityErrorResult(step.Action, err), true, err
		}
		result, err := s.invokeNativeInteractiveVerify(step)
		return s.nativeProgramActionResolution(req), result, true, err
	case ProgramInteractiveCloseToolName:
		if err := s.nativeProgramActionAvailabilityError(step.Action.ToolName); err != nil {
			return nil, capabilityErrorResult(step.Action, err), true, err
		}
		result, err := s.invokeNativeInteractiveClose(ctx, step)
		return s.nativeProgramActionResolution(req), result, true, err
	default:
		return nil, action.Result{}, false, nil
	}
}

func isNativeProgramActionToolName(name string) bool {
	switch name {
	case ProgramInteractiveStartToolName, ProgramInteractiveViewToolName, ProgramInteractiveWriteToolName, ProgramInteractiveVerifyToolName, ProgramInteractiveCloseToolName:
		return true
	default:
		return false
	}
}

func nativeProgramActionRequiresInteractiveController(name string) bool {
	switch name {
	case ProgramInteractiveStartToolName, ProgramInteractiveViewToolName, ProgramInteractiveWriteToolName, ProgramInteractiveCloseToolName:
		return true
	default:
		return false
	}
}

func (s *Service) nativeProgramActionAvailabilityError(name string) error {
	if !isNativeProgramActionToolName(name) {
		return capability.ErrCapabilityNotFound
	}
	if nativeProgramActionRequiresInteractiveController(name) && s.InteractiveController == nil {
		return capability.ErrCapabilityDisabled
	}
	return nil
}

func (s *Service) nativeProgramActionResolution(req capability.Request) *capability.Resolution {
	if !isNativeProgramActionToolName(req.Action.ToolName) {
		return nil
	}
	metadata := map[string]any{"native": true}
	now := s.nowMilli()
	return &capability.Resolution{
		Snapshot: capability.Snapshot{
			SnapshotID:     "cap_" + uuid.NewString(),
			SessionID:      req.SessionID,
			TaskID:         req.TaskID,
			StepID:         req.StepID,
			Scope:          capability.SnapshotScopeAction,
			ToolName:       req.Action.ToolName,
			Version:        programInteractiveNativeVersion,
			CapabilityType: "interactive",
			RiskLevel:      tool.RiskLow,
			Metadata:       metadata,
			ResolvedAt:     now,
		},
		Definition: tool.Definition{
			ToolName:       req.Action.ToolName,
			Version:        programInteractiveNativeVersion,
			CapabilityType: "interactive",
			RiskLevel:      tool.RiskLow,
			Enabled:        true,
			Metadata:       metadata,
		},
	}
}

func (s *Service) invokeNativeInteractiveStart(ctx context.Context, state session.State, step plan.StepSpec, attempt execution.Attempt, actionRecord *execution.ActionRecord) (action.Result, error) {
	metadata := programAnyMapArg(step.Action.Args, "metadata")
	metadata = mergeMaps(metadata, executionFactMetadata(step.Metadata))
	if actionRecord != nil && actionRecord.ActionID != "" {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["action_id"] = actionRecord.ActionID
	}
	started, err := s.StartInteractive(ctx, state.SessionID, InteractiveStartRequest{
		HandleID:  programStringArg(step.Action.Args, "handle_id"),
		TaskID:    state.TaskID,
		AttemptID: attempt.AttemptID,
		CycleID:   attempt.CycleID,
		TraceID:   attempt.TraceID,
		Kind:      programStringArg(step.Action.Args, "kind"),
		Spec:      programAnyMapArg(step.Action.Args, "spec"),
		Metadata:  metadata,
	})
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_START_FAILED", err), err
	}
	return nativeInteractiveActionResult(interactiveActionData(started)), nil
}

func (s *Service) invokeNativeInteractiveView(ctx context.Context, step plan.StepSpec) (action.Result, error) {
	handleID, err := programInteractiveHandleID(step.Action.Args)
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_VIEW_FAILED", err), err
	}
	req := InteractiveViewRequest{
		Metadata: programAnyMapArg(step.Action.Args, "metadata"),
	}
	if offset, ok := programInt64Arg(step.Action.Args, "offset"); ok {
		req.Offset = offset
	}
	if maxBytes, ok := programIntArg(step.Action.Args, "max_bytes"); ok {
		req.MaxBytes = maxBytes
	}
	viewed, err := s.ViewInteractive(ctx, handleID, req)
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_VIEW_FAILED", err), err
	}
	data := interactiveActionData(viewed.Runtime)
	data["data"] = viewed.Data
	data["truncated"] = viewed.Truncated
	if viewed.OriginalBytes > 0 {
		data["original_bytes"] = viewed.OriginalBytes
	}
	if viewed.ReturnedBytes > 0 {
		data["returned_bytes"] = viewed.ReturnedBytes
	}
	if viewed.HasMore {
		data["has_more"] = true
	}
	if viewed.NextOffset > 0 {
		data["next_offset"] = viewed.NextOffset
	}
	if viewed.RawRef != "" {
		data["raw_ref"] = viewed.RawRef
	}
	return nativeInteractiveActionResult(data), nil
}

func (s *Service) invokeNativeInteractiveWrite(ctx context.Context, step plan.StepSpec) (action.Result, error) {
	handleID, err := programInteractiveHandleID(step.Action.Args)
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_WRITE_FAILED", err), err
	}
	written, err := s.WriteInteractive(ctx, handleID, InteractiveWriteRequest{
		Input:    programStringArg(step.Action.Args, "input"),
		Metadata: programAnyMapArg(step.Action.Args, "metadata"),
	})
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_WRITE_FAILED", err), err
	}
	data := interactiveActionData(written.Runtime)
	data["bytes"] = written.Bytes
	return nativeInteractiveActionResult(data), nil
}

func (s *Service) invokeNativeInteractiveVerify(step plan.StepSpec) (action.Result, error) {
	handleID, err := programInteractiveHandleID(step.Action.Args)
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_VERIFY_FAILED", err), err
	}
	runtime, err := s.GetInteractiveRuntime(handleID)
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_VERIFY_FAILED", err), err
	}
	return nativeInteractiveActionResult(interactiveActionData(runtime)), nil
}

func (s *Service) invokeNativeInteractiveClose(ctx context.Context, step plan.StepSpec) (action.Result, error) {
	handleID, err := programInteractiveHandleID(step.Action.Args)
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_CLOSE_FAILED", err), err
	}
	closed, err := s.CloseInteractive(ctx, handleID, InteractiveCloseRequest{
		Reason:   programStringArg(step.Action.Args, "reason"),
		Metadata: programAnyMapArg(step.Action.Args, "metadata"),
	})
	if err != nil {
		return nativeInteractiveActionErrorResult("INTERACTIVE_CLOSE_FAILED", err), err
	}
	return nativeInteractiveActionResult(interactiveActionData(closed)), nil
}

func nativeInteractiveActionErrorResult(code string, err error) action.Result {
	return action.Result{
		OK: false,
		Error: &action.Error{
			Code:    code,
			Message: err.Error(),
		},
	}
}

func nativeInteractiveActionResult(data map[string]any) action.Result {
	return action.Result{
		OK:   true,
		Data: data,
		Meta: map[string]any{
			programInteractivePersistedRuntimeHandlesKey: true,
		},
	}
}

func interactiveActionData(runtime execution.InteractiveRuntime) map[string]any {
	ref := execution.RuntimeHandleRefFromHandle(runtime.Handle)
	data := map[string]any{
		"handle_id":      ref.HandleID,
		"kind":           ref.Kind,
		"status":         firstNonEmptyString(runtime.Observation.Status, string(ref.Status)),
		"version":        ref.Version,
		"handle":         runtimeHandleRefValue(ref),
		"runtime_handle": runtime.Handle,
		"runtime":        interactiveRuntimeValue(runtime),
	}
	if reason := firstNonEmptyString(runtime.Observation.StatusReason, runtime.Handle.StatusReason); reason != "" {
		data["status_reason"] = reason
	}
	if runtime.Observation.NextOffset > 0 {
		data["next_offset"] = runtime.Observation.NextOffset
	}
	if runtime.Observation.Closed {
		data["closed"] = true
	}
	return data
}

func runtimeHandleRefValue(ref execution.RuntimeHandleRef) map[string]any {
	out := map[string]any{
		"handle_id": ref.HandleID,
	}
	if ref.StepID != "" {
		out["step_id"] = ref.StepID
	}
	if ref.ActionID != "" {
		out["action_id"] = ref.ActionID
	}
	if ref.Kind != "" {
		out["kind"] = ref.Kind
	}
	if ref.Status != "" {
		out["status"] = string(ref.Status)
	}
	if ref.Version > 0 {
		out["version"] = ref.Version
	}
	return out
}

func interactiveRuntimeValue(runtime execution.InteractiveRuntime) map[string]any {
	out := map[string]any{
		"handle": runtimeHandleRefValue(execution.RuntimeHandleRefFromHandle(runtime.Handle)),
		"observation": map[string]any{
			"next_offset": runtime.Observation.NextOffset,
			"closed":      runtime.Observation.Closed,
			"status":      runtime.Observation.Status,
		},
	}
	if runtime.Observation.StatusReason != "" {
		out["observation"].(map[string]any)["status_reason"] = runtime.Observation.StatusReason
	}
	if runtime.Observation.ExitCode != nil {
		out["observation"].(map[string]any)["exit_code"] = *runtime.Observation.ExitCode
	}
	if runtime.Observation.Snapshot.ArtifactID != "" {
		out["observation"].(map[string]any)["snapshot_artifact_id"] = runtime.Observation.Snapshot.ArtifactID
	}
	capabilities := map[string]any{}
	if runtime.Capabilities.Reopen {
		capabilities["reopen"] = true
	}
	if runtime.Capabilities.View {
		capabilities["view"] = true
	}
	if runtime.Capabilities.Write {
		capabilities["write"] = true
	}
	if runtime.Capabilities.Close {
		capabilities["close"] = true
	}
	if len(capabilities) > 0 {
		out["capabilities"] = capabilities
	}
	if runtime.LastOperation.Kind != "" {
		out["last_operation"] = map[string]any{
			"kind":   string(runtime.LastOperation.Kind),
			"at":     runtime.LastOperation.At,
			"offset": runtime.LastOperation.Offset,
			"bytes":  runtime.LastOperation.Bytes,
		}
	}
	if len(runtime.Metadata) > 0 {
		out["metadata"] = cloneAnyMap(runtime.Metadata)
	}
	return out
}

func programInteractiveHandleID(args map[string]any) (string, error) {
	if handleID := strings.TrimSpace(programStringArg(args, "handle_id")); handleID != "" {
		return handleID, nil
	}
	raw, ok := args["handle"]
	if !ok || raw == nil {
		return "", fmt.Errorf("%w: missing interactive handle argument", ErrProgramBindingResolveFailed)
	}
	switch typed := raw.(type) {
	case execution.RuntimeHandleRef:
		if strings.TrimSpace(typed.HandleID) != "" {
			return typed.HandleID, nil
		}
	case *execution.RuntimeHandleRef:
		if typed != nil && strings.TrimSpace(typed.HandleID) != "" {
			return typed.HandleID, nil
		}
	case execution.RuntimeHandle:
		if strings.TrimSpace(typed.HandleID) != "" {
			return typed.HandleID, nil
		}
	case *execution.RuntimeHandle:
		if typed != nil && strings.TrimSpace(typed.HandleID) != "" {
			return typed.HandleID, nil
		}
	case map[string]any:
		if handleID, _ := typed["handle_id"].(string); strings.TrimSpace(handleID) != "" {
			return handleID, nil
		}
	}
	return "", fmt.Errorf("%w: missing interactive handle id", ErrProgramBindingResolveFailed)
}

func programStringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, _ := args[key].(string)
	return value
}

func programAnyMapArg(args map[string]any, key string) map[string]any {
	if args == nil {
		return nil
	}
	value, _ := args[key].(map[string]any)
	return cloneAnyMap(value)
}

func programInt64Arg(args map[string]any, key string) (int64, bool) {
	if args == nil {
		return 0, false
	}
	return asInt64(args[key])
}

func programIntArg(args map[string]any, key string) (int, bool) {
	value, ok := programInt64Arg(args, key)
	if !ok {
		return 0, false
	}
	return int(value), true
}
