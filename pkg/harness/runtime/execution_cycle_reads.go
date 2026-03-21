package runtime

import (
	"sort"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

func (s *Service) GetExecutionCycle(sessionID, cycleID string) (execution.ExecutionCycle, error) {
	if cycleID == "" {
		return execution.ExecutionCycle{}, execution.ErrExecutionCycleNotFound
	}
	cycles, err := s.ListExecutionCycles(sessionID)
	if err != nil {
		return execution.ExecutionCycle{}, err
	}
	for _, cycle := range cycles {
		if cycle.CycleID == cycleID {
			return cycle, nil
		}
	}
	return execution.ExecutionCycle{}, execution.ErrExecutionCycleNotFound
}

func (s *Service) ListExecutionCycles(sessionID string) ([]execution.ExecutionCycle, error) {
	attempts, err := s.ListAttempts(sessionID)
	if err != nil {
		return nil, err
	}
	actions, err := s.ListActions(sessionID)
	if err != nil {
		return nil, err
	}
	verifications, err := s.ListVerifications(sessionID)
	if err != nil {
		return nil, err
	}
	artifacts, err := s.ListArtifacts(sessionID)
	if err != nil {
		return nil, err
	}
	runtimeHandles, err := s.ListRuntimeHandles(sessionID)
	if err != nil {
		return nil, err
	}

	type cycleEnvelope struct {
		cycle     execution.ExecutionCycle
		firstSeen int64
	}

	byID := map[string]*cycleEnvelope{}
	ensure := func(cycleID, recordSessionID, taskID, stepID, approvalID, traceID string, startedAt int64) *cycleEnvelope {
		env, ok := byID[cycleID]
		if !ok {
			env = &cycleEnvelope{
				cycle: execution.ExecutionCycle{
					CycleID:   cycleID,
					SessionID: recordSessionID,
				},
				firstSeen: startedAt,
			}
			byID[cycleID] = env
		}
		if env.cycle.SessionID == "" {
			env.cycle.SessionID = recordSessionID
		}
		if env.cycle.TaskID == "" {
			env.cycle.TaskID = taskID
		}
		if env.cycle.StepID == "" {
			env.cycle.StepID = stepID
		}
		if env.cycle.ApprovalID == "" {
			env.cycle.ApprovalID = approvalID
		}
		if env.cycle.TraceID == "" {
			env.cycle.TraceID = traceID
		}
		if startedAt > 0 && (env.firstSeen == 0 || startedAt < env.firstSeen) {
			env.firstSeen = startedAt
		}
		if startedAt > 0 && (env.cycle.StartedAt == 0 || startedAt < env.cycle.StartedAt) {
			env.cycle.StartedAt = startedAt
		}
		return env
	}
	updateFinishedAt := func(env *cycleEnvelope, finishedAt int64) {
		if env == nil || finishedAt <= 0 {
			return
		}
		if finishedAt > env.cycle.FinishedAt {
			env.cycle.FinishedAt = finishedAt
		}
	}

	for _, attempt := range attempts {
		if attempt.CycleID == "" {
			continue
		}
		env := ensure(attempt.CycleID, attempt.SessionID, attempt.TaskID, attempt.StepID, attempt.ApprovalID, attempt.TraceID, attempt.StartedAt)
		env.cycle.Attempts = append(env.cycle.Attempts, attempt)
		updateFinishedAt(env, attempt.FinishedAt)
	}
	for _, actionRecord := range actions {
		if actionRecord.CycleID == "" {
			continue
		}
		env := ensure(actionRecord.CycleID, actionRecord.SessionID, actionRecord.TaskID, actionRecord.StepID, "", actionRecord.TraceID, actionRecord.StartedAt)
		env.cycle.Actions = append(env.cycle.Actions, actionRecord)
		updateFinishedAt(env, actionRecord.FinishedAt)
	}
	for _, verificationRecord := range verifications {
		if verificationRecord.CycleID == "" {
			continue
		}
		env := ensure(verificationRecord.CycleID, verificationRecord.SessionID, verificationRecord.TaskID, verificationRecord.StepID, "", verificationRecord.TraceID, verificationRecord.StartedAt)
		env.cycle.Verifications = append(env.cycle.Verifications, verificationRecord)
		updateFinishedAt(env, verificationRecord.FinishedAt)
	}
	for _, artifact := range artifacts {
		if artifact.CycleID == "" {
			continue
		}
		env := ensure(artifact.CycleID, artifact.SessionID, artifact.TaskID, artifact.StepID, "", artifact.TraceID, artifact.CreatedAt)
		env.cycle.Artifacts = append(env.cycle.Artifacts, artifact)
		updateFinishedAt(env, artifact.CreatedAt)
	}
	for _, handle := range runtimeHandles {
		if handle.CycleID == "" {
			continue
		}
		env := ensure(handle.CycleID, handle.SessionID, handle.TaskID, "", "", handle.TraceID, handle.CreatedAt)
		env.cycle.RuntimeHandles = append(env.cycle.RuntimeHandles, handle)
		updateFinishedAt(env, maxRuntimeHandleTimestamp(handle))
	}

	out := make([]execution.ExecutionCycle, 0, len(byID))
	for _, env := range byID {
		out = append(out, env.cycle)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt == out[j].StartedAt {
			return out[i].CycleID < out[j].CycleID
		}
		return out[i].StartedAt < out[j].StartedAt
	})
	return out, nil
}

func maxRuntimeHandleTimestamp(handle execution.RuntimeHandle) int64 {
	maxTS := handle.UpdatedAt
	if handle.CreatedAt > maxTS {
		maxTS = handle.CreatedAt
	}
	if handle.ClosedAt > maxTS {
		maxTS = handle.ClosedAt
	}
	if handle.InvalidatedAt > maxTS {
		maxTS = handle.InvalidatedAt
	}
	return maxTS
}
