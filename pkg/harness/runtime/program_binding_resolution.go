package runtime

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
)

func (s *Service) resolveProgramBindings(ctx context.Context, sessionID string, step plan.StepSpec) (plan.StepSpec, error) {
	bindings, ok := execution.ProgramInputBindingsFromStep(step)
	if !ok || len(bindings) == 0 {
		return normalizeResolvedProgramActionArgs(step), nil
	}
	step.Action.Args = cloneAnyMap(step.Action.Args)
	for _, binding := range bindings {
		value, err := s.resolveProgramBindingValue(ctx, sessionID, step, binding)
		if err != nil {
			return plan.StepSpec{}, err
		}
		step.Action.Args[binding.Name] = value
	}
	return normalizeResolvedProgramActionArgs(step), nil
}

func normalizeResolvedProgramActionArgs(step plan.StepSpec) plan.StepSpec {
	if !isNativeProgramActionToolName(step.Action.ToolName) || step.Action.ToolName == ProgramInteractiveStartToolName {
		return step
	}
	if step.Action.Args == nil {
		return step
	}
	if handleID := strings.TrimSpace(programStringArg(step.Action.Args, "handle_id")); handleID != "" {
		return step
	}
	handleID, err := programInteractiveHandleID(step.Action.Args)
	if err != nil || strings.TrimSpace(handleID) == "" {
		return step
	}
	step.Action.Args = cloneAnyMap(step.Action.Args)
	step.Action.Args["handle_id"] = handleID
	return step
}

func (s *Service) resolveProgramBindingValue(ctx context.Context, sessionID string, step plan.StepSpec, binding execution.ProgramInputBinding) (any, error) {
	switch binding.Kind {
	case "", execution.ProgramInputBindingLiteral:
		return binding.Value, nil
	case execution.ProgramInputBindingOutputRef:
		if binding.Ref == nil {
			return nil, fmt.Errorf("%w: missing output ref for binding %q", ErrProgramBindingResolveFailed, binding.Name)
		}
		return s.resolveProgramOutputRef(ctx, sessionID, step, *binding.Ref)
	case execution.ProgramInputBindingAttachment:
		if binding.Attachment == nil {
			return nil, fmt.Errorf("%w: missing attachment input for binding %q", ErrProgramBindingResolveFailed, binding.Name)
		}
		if binding.Attachment.Materialize != "" && binding.Attachment.Materialize != execution.AttachmentMaterializeNone {
			return s.materializeProgramAttachment(ctx, sessionID, step, *binding.Attachment)
		}
		switch binding.Attachment.Kind {
		case execution.AttachmentInputText:
			return binding.Attachment.Text, nil
		case execution.AttachmentInputBytes:
			return append([]byte(nil), binding.Attachment.Bytes...), nil
		case execution.AttachmentInputArtifactRef:
			return s.resolveProgramArtifactHandle(ctx, sessionID, step, binding.Attachment.ArtifactID, "", "")
		default:
			return nil, fmt.Errorf("%w: attachment kind %q", ErrProgramAttachmentUnsupported, binding.Attachment.Kind)
		}
	case execution.ProgramInputBindingRuntimeHandleRef:
		if binding.RuntimeHandle == nil {
			return nil, fmt.Errorf("%w: missing runtime handle ref for binding %q", ErrProgramBindingResolveFailed, binding.Name)
		}
		return s.resolveProgramRuntimeHandleRef(ctx, sessionID, step, *binding.RuntimeHandle)
	default:
		return nil, fmt.Errorf("%w: binding kind %q", ErrProgramInputBindingUnsupported, binding.Kind)
	}
}

func (s *Service) materializeProgramAttachment(ctx context.Context, sessionID string, step plan.StepSpec, input execution.AttachmentInput) (any, error) {
	var artifact *execution.Artifact
	if input.Kind == execution.AttachmentInputArtifactRef {
		record, err := s.findProgramAttachmentArtifact(ctx, sessionID, input.ArtifactID)
		if err != nil {
			return nil, err
		}
		artifact = &record
	}
	if s.AttachmentMaterializer == nil {
		return nil, fmt.Errorf("%w: attachment materializer not configured", ErrProgramAttachmentUnsupported)
	}
	return s.AttachmentMaterializer.Materialize(ctx, AttachmentMaterializeRequest{
		SessionID: sessionID,
		Step:      step,
		Input:     input,
		Artifact:  artifact,
	})
}

func (s *Service) findProgramAttachmentArtifact(ctx context.Context, sessionID, artifactID string) (execution.Artifact, error) {
	if strings.TrimSpace(artifactID) == "" {
		return execution.Artifact{}, fmt.Errorf("%w: missing attachment artifact id", ErrProgramBindingResolveFailed)
	}
	artifacts, err := s.listArtifactRecords(ctx, sessionID)
	if err != nil {
		return execution.Artifact{}, err
	}
	for _, record := range artifacts {
		if record.ArtifactID == artifactID {
			return record, nil
		}
	}
	return execution.Artifact{}, fmt.Errorf("%w: artifact ref %q", ErrProgramBindingResolveFailed, artifactID)
}

func (s *Service) resolveProgramOutputRef(ctx context.Context, sessionID string, step plan.StepSpec, ref execution.OutputRef) (any, error) {
	switch ref.Kind {
	case execution.OutputRefArtifact:
		return s.resolveProgramArtifactHandle(ctx, sessionID, step, ref.ArtifactID, ref.StepID, ref.ActionID)
	case execution.OutputRefAttachment:
		if ref.AttachmentID == "" {
			return nil, fmt.Errorf("%w: missing attachment id", ErrProgramBindingResolveFailed)
		}
		return execution.AttachmentRef{AttachmentID: ref.AttachmentID}, nil
	}
	record, err := s.findProgramOutputAction(ctx, sessionID, step, ref)
	if err != nil {
		return nil, err
	}
	result := rawPreferredActionResult(record.Result)
	switch ref.Kind {
	case "", execution.OutputRefStructured:
		if strings.TrimSpace(ref.Path) == "" {
			return cloneAnyMap(result.Data), nil
		}
		value, ok := resolveProgramResultPath(result, ref.Path)
		if !ok {
			return nil, fmt.Errorf("%w: output path %q not found", ErrProgramBindingResolveFailed, ref.Path)
		}
		return value, nil
	case execution.OutputRefText:
		if strings.TrimSpace(ref.Path) != "" {
			value, ok := resolveProgramResultPath(result, ref.Path)
			if !ok {
				return nil, fmt.Errorf("%w: text path %q not found", ErrProgramBindingResolveFailed, ref.Path)
			}
			text, ok := stringFromProgramValue(value)
			if !ok {
				return nil, fmt.Errorf("%w: text path %q is not a string", ErrProgramBindingResolveFailed, ref.Path)
			}
			return text, nil
		}
		if text, ok := stringFromProgramValue(result.Data["stdout"]); ok {
			return text, nil
		}
		if text, ok := stringFromProgramValue(result.Data["text"]); ok {
			return text, nil
		}
		return nil, fmt.Errorf("%w: text output missing", ErrProgramBindingResolveFailed)
	case execution.OutputRefBytes:
		if strings.TrimSpace(ref.Path) != "" {
			value, ok := resolveProgramResultPath(result, ref.Path)
			if !ok {
				return nil, fmt.Errorf("%w: bytes path %q not found", ErrProgramBindingResolveFailed, ref.Path)
			}
			bytes, ok := bytesFromProgramValue(value)
			if !ok {
				return nil, fmt.Errorf("%w: bytes path %q is not bytes", ErrProgramBindingResolveFailed, ref.Path)
			}
			return bytes, nil
		}
		if bytes, ok := bytesFromProgramValue(result.Data["bytes"]); ok {
			return bytes, nil
		}
		if text, ok := stringFromProgramValue(result.Data["stdout"]); ok {
			return []byte(text), nil
		}
		return nil, fmt.Errorf("%w: bytes output missing", ErrProgramBindingResolveFailed)
	default:
		return nil, fmt.Errorf("%w: output ref kind %q", ErrProgramOutputRefUnsupported, ref.Kind)
	}
}

func (s *Service) findProgramOutputAction(ctx context.Context, sessionID string, step plan.StepSpec, ref execution.OutputRef) (execution.ActionRecord, error) {
	actions, err := s.listActionRecords(ctx, sessionID)
	if err != nil {
		return execution.ActionRecord{}, err
	}
	currentTarget, hasCurrentTarget := execution.TargetFromStep(step)
	type candidate struct {
		record execution.ActionRecord
		score  int
	}
	candidates := make([]candidate, 0, len(actions))
	for _, record := range actions {
		if ref.ActionID != "" && record.ActionID != ref.ActionID {
			continue
		}
		if ref.StepID != "" && !programOutputRefMatchesStep(record, ref.StepID) {
			continue
		}
		score := 0
		if hasCurrentTarget {
			if target, ok := execution.TargetRefFromMetadata(record.Metadata); ok {
				if target.TargetID == currentTarget.TargetID {
					score += 10
				} else {
					score -= 5
				}
			}
		}
		if record.FinishedAt > 0 {
			score++
		}
		candidates = append(candidates, candidate{record: record, score: score})
	}
	if len(candidates) == 0 {
		return execution.ActionRecord{}, fmt.Errorf("%w: output ref %+v", ErrProgramBindingResolveFailed, ref)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].record.FinishedAt != candidates[j].record.FinishedAt {
			return candidates[i].record.FinishedAt > candidates[j].record.FinishedAt
		}
		return candidates[i].record.ActionID > candidates[j].record.ActionID
	})
	return candidates[0].record, nil
}

func (s *Service) resolveProgramArtifactHandle(ctx context.Context, sessionID string, step plan.StepSpec, artifactID, stepID, actionID string) (execution.ArtifactRef, error) {
	artifacts, err := s.listArtifactRecords(ctx, sessionID)
	if err != nil {
		return execution.ArtifactRef{}, err
	}
	currentTarget, hasCurrentTarget := execution.TargetFromStep(step)
	type candidate struct {
		record execution.Artifact
		score  int
	}
	candidates := make([]candidate, 0, len(artifacts))
	for _, record := range artifacts {
		if artifactID != "" && record.ArtifactID != artifactID {
			continue
		}
		if actionID != "" && record.ActionID != actionID {
			continue
		}
		if stepID != "" && !programOutputRefMatchesArtifact(record, stepID) {
			continue
		}
		score := 0
		if hasCurrentTarget {
			if target, ok := execution.TargetRefFromMetadata(record.Metadata); ok {
				if target.TargetID == currentTarget.TargetID {
					score += 10
				} else {
					score -= 5
				}
			}
		}
		candidates = append(candidates, candidate{record: record, score: score})
	}
	if len(candidates) == 0 {
		return execution.ArtifactRef{}, fmt.Errorf("%w: artifact ref %q", ErrProgramBindingResolveFailed, artifactID)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].record.CreatedAt != candidates[j].record.CreatedAt {
			return candidates[i].record.CreatedAt > candidates[j].record.CreatedAt
		}
		return candidates[i].record.ArtifactID > candidates[j].record.ArtifactID
	})
	record := candidates[0].record
	return execution.ArtifactRef{
		ArtifactID: record.ArtifactID,
		Name:       record.Name,
		Kind:       record.Kind,
	}, nil
}

func (s *Service) resolveProgramRuntimeHandleRef(ctx context.Context, sessionID string, step plan.StepSpec, ref execution.RuntimeHandleRef) (execution.RuntimeHandleRef, error) {
	if strings.TrimSpace(ref.HandleID) == "" && strings.TrimSpace(ref.StepID) == "" && strings.TrimSpace(ref.ActionID) == "" {
		return execution.RuntimeHandleRef{}, fmt.Errorf("%w: runtime handle ref %+v", ErrProgramBindingResolveFailed, ref)
	}

	handles, err := s.listRuntimeHandleRecords(ctx, sessionID)
	if err != nil {
		return execution.RuntimeHandleRef{}, err
	}

	currentTarget, hasCurrentTarget := execution.TargetFromStep(step)
	attemptsByID := map[string]execution.Attempt{}
	if ref.StepID != "" || hasCurrentTarget {
		attempts, err := s.listAttemptRecords(ctx, sessionID)
		if err != nil {
			return execution.RuntimeHandleRef{}, err
		}
		for _, attempt := range attempts {
			attemptsByID[attempt.AttemptID] = attempt
		}
	}

	type candidate struct {
		handle     execution.RuntimeHandle
		attempt    execution.Attempt
		hasAttempt bool
		actionID   string
		score      int
	}

	candidates := make([]candidate, 0, len(handles))
	for _, handle := range handles {
		if ref.HandleID != "" && handle.HandleID != ref.HandleID {
			continue
		}
		if ref.Kind != "" && handle.Kind != ref.Kind {
			continue
		}
		if ref.Status != "" && handle.Status != ref.Status {
			continue
		}
		if ref.Version > 0 && handle.Version != ref.Version {
			continue
		}
		actionID := runtimeHandleActionID(handle)
		if ref.ActionID != "" && actionID != ref.ActionID {
			continue
		}
		attempt, hasAttempt := attemptsByID[handle.AttemptID]
		if ref.StepID != "" && !programRuntimeHandleMatchesStep(attempt, hasAttempt, ref.StepID) {
			continue
		}

		score := 0
		if ref.HandleID != "" && handle.HandleID == ref.HandleID {
			score += 100
		}
		if ref.ActionID != "" && actionID == ref.ActionID {
			score += 20
		}
		if ref.StepID != "" {
			score += 5
		}
		if hasCurrentTarget && hasAttempt {
			if target, ok := execution.TargetFromStep(attempt.Step); ok {
				if target.TargetID != currentTarget.TargetID {
					continue
				}
				score += 10
			}
		}
		if isRuntimeHandleActive(handle) {
			score++
		}

		candidates = append(candidates, candidate{
			handle:     handle,
			attempt:    attempt,
			hasAttempt: hasAttempt,
			actionID:   actionID,
			score:      score,
		})
	}
	if len(candidates) == 0 {
		return execution.RuntimeHandleRef{}, fmt.Errorf("%w: runtime handle ref %+v", ErrProgramBindingResolveFailed, ref)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].handle.UpdatedAt != candidates[j].handle.UpdatedAt {
			return candidates[i].handle.UpdatedAt > candidates[j].handle.UpdatedAt
		}
		if candidates[i].handle.Version != candidates[j].handle.Version {
			return candidates[i].handle.Version > candidates[j].handle.Version
		}
		return candidates[i].handle.HandleID > candidates[j].handle.HandleID
	})

	selected := candidates[0]
	resolved := execution.RuntimeHandleRefFromHandle(selected.handle)
	if selected.actionID != "" {
		resolved.ActionID = selected.actionID
	}
	if selected.hasAttempt {
		resolved.StepID = firstNonEmptyString(selected.attempt.StepID, selected.attempt.Step.StepID)
	}
	return resolved, nil
}

func programOutputRefMatchesStep(record execution.ActionRecord, stepID string) bool {
	if record.StepID == stepID {
		return true
	}
	nodeID, _ := record.Metadata[execution.ProgramMetadataKeyNodeID].(string)
	return nodeID == stepID
}

func programOutputRefMatchesArtifact(record execution.Artifact, stepID string) bool {
	if record.StepID == stepID {
		return true
	}
	nodeID, _ := record.Metadata[execution.ProgramMetadataKeyNodeID].(string)
	return nodeID == stepID
}

func programRuntimeHandleMatchesStep(attempt execution.Attempt, hasAttempt bool, stepID string) bool {
	if !hasAttempt {
		return false
	}
	if attempt.StepID == stepID || attempt.Step.StepID == stepID {
		return true
	}
	nodeID, _ := attempt.Step.Metadata[execution.ProgramMetadataKeyNodeID].(string)
	return nodeID == stepID
}

func runtimeHandleActionID(handle execution.RuntimeHandle) string {
	if handle.Metadata == nil {
		return ""
	}
	actionID, _ := handle.Metadata["action_id"].(string)
	return actionID
}

func resolveProgramResultPath(result action.Result, path string) (any, bool) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return result.Data, true
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > 0 && parts[0] == "result" {
		return descendProgramResultPath(result, parts)
	}
	return descendProgramValue(result.Data, parts)
}

func descendProgramResultPath(result action.Result, parts []string) (any, bool) {
	if len(parts) == 0 || parts[0] != "result" {
		return nil, false
	}
	if len(parts) == 1 {
		return result, true
	}
	var current any
	switch parts[1] {
	case "ok":
		current = result.OK
	case "data":
		current = result.Data
	case "meta":
		current = result.Meta
	case "error":
		if result.Error == nil {
			return nil, false
		}
		current = map[string]any{
			"code":    result.Error.Code,
			"message": result.Error.Message,
		}
	default:
		return nil, false
	}
	if len(parts) == 2 {
		return current, true
	}
	return descendProgramValue(current, parts[2:])
}

func descendProgramValue(current any, parts []string) (any, bool) {
	value := current
	for _, part := range parts {
		switch typed := value.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			value = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			value = typed[index]
		default:
			reflected := reflect.ValueOf(value)
			if !reflected.IsValid() {
				return nil, false
			}
			if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
				return nil, false
			}
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= reflected.Len() {
				return nil, false
			}
			value = reflected.Index(index).Interface()
		}
	}
	return value, true
}

func stringFromProgramValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	default:
		return "", false
	}
}

func bytesFromProgramValue(value any) ([]byte, bool) {
	switch typed := value.(type) {
	case []byte:
		return append([]byte(nil), typed...), true
	case string:
		return []byte(typed), true
	default:
		return nil, false
	}
}
