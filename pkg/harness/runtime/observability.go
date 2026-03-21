package runtime

import (
	"context"
	"strconv"

	"github.com/yiiilin/harness-core/pkg/harness/approval"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/observability"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/planning"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func (s *Service) exportMetricSample(ctx context.Context, name string, labels map[string]string, fields map[string]any) {
	if s.MetricsExporter == nil {
		return
	}
	_ = s.MetricsExporter.ExportMetric(ctx, observability.MetricSample{
		Name:       name,
		Labels:     labels,
		Fields:     fields,
		RecordedAt: s.nowMilli(),
	})
}

func (s *Service) exportTraceSpan(ctx context.Context, span observability.TraceSpan) {
	if s.TraceExporter == nil {
		return
	}
	_ = s.TraceExporter.ExportTrace(ctx, span)
}

func (s *Service) exportStepMetricSample(ctx context.Context, state session.State, step plan.StepSpec, attempt execution.Attempt, actionRecord *execution.ActionRecord, verificationRecord *execution.VerificationRecord, success, policyDenied, verifyFailed, actionFailed bool, durationMS int64) {
	labels := map[string]string{
		"session_id": state.SessionID,
		"task_id":    state.TaskID,
		"step_id":    step.StepID,
		"attempt_id": attempt.AttemptID,
		"cycle_id":   attempt.CycleID,
		"trace_id":   attempt.TraceID,
	}
	if actionRecord != nil && actionRecord.ActionID != "" {
		labels["action_id"] = actionRecord.ActionID
	}
	if verificationRecord != nil && verificationRecord.VerificationID != "" {
		labels["verification_id"] = verificationRecord.VerificationID
	}
	s.exportMetricSample(ctx, "step.run", labels, map[string]any{"success": success, "policy_denied": policyDenied, "verify_failed": verifyFailed, "action_failed": actionFailed, "duration_ms": durationMS})
}

func (s *Service) exportTraceSpans(ctx context.Context, state session.State, step plan.StepSpec, attempt execution.Attempt, actionRecord *execution.ActionRecord, verificationRecord *execution.VerificationRecord) {
	s.exportTraceSpan(ctx, observability.TraceSpan{
		Name:       "step.run",
		TraceID:    attempt.TraceID,
		SpanID:     attempt.AttemptID,
		SessionID:  state.SessionID,
		TaskID:     state.TaskID,
		StepID:     step.StepID,
		AttemptID:  attempt.AttemptID,
		CycleID:    attempt.CycleID,
		StartedAt:  attempt.StartedAt,
		FinishedAt: attempt.FinishedAt,
		Attributes: map[string]any{"step_status": attempt.Status},
	})
	if actionRecord != nil {
		s.exportTraceSpan(ctx, observability.TraceSpan{
			Name:        "tool.invoke",
			TraceID:     actionRecord.TraceID,
			SpanID:      actionRecord.ActionID,
			ParentID:    attempt.AttemptID,
			SessionID:   state.SessionID,
			TaskID:      state.TaskID,
			StepID:      step.StepID,
			AttemptID:   attempt.AttemptID,
			ActionID:    actionRecord.ActionID,
			CycleID:     attempt.CycleID,
			CausationID: actionRecord.CausationID,
			StartedAt:   actionRecord.StartedAt,
			FinishedAt:  actionRecord.FinishedAt,
			Attributes:  map[string]any{"tool_name": actionRecord.ToolName, "status": actionRecord.Status},
		})
	}
	if verificationRecord != nil {
		parentID := attempt.AttemptID
		if actionRecord != nil && actionRecord.ActionID != "" {
			parentID = actionRecord.ActionID
		}
		s.exportTraceSpan(ctx, observability.TraceSpan{
			Name:           "verify.evaluate",
			TraceID:        verificationRecord.TraceID,
			SpanID:         verificationRecord.VerificationID,
			ParentID:       parentID,
			SessionID:      state.SessionID,
			TaskID:         state.TaskID,
			StepID:         step.StepID,
			AttemptID:      attempt.AttemptID,
			ActionID:       verificationRecord.ActionID,
			VerificationID: verificationRecord.VerificationID,
			CycleID:        attempt.CycleID,
			CausationID:    verificationRecord.CausationID,
			StartedAt:      verificationRecord.StartedAt,
			FinishedAt:     verificationRecord.FinishedAt,
			Attributes:     map[string]any{"status": verificationRecord.Status},
		})
	}
}

func (s *Service) exportPlanningObservability(ctx context.Context, record planning.Record) {
	stepCount := any(nil)
	if record.Metadata != nil {
		stepCount = record.Metadata["step_count"]
	}
	fields := map[string]any{
		"success":     record.Status == planning.StatusCompleted,
		"reason":      record.Reason,
		"step_count":  stepCount,
		"duration_ms": record.FinishedAt - record.StartedAt,
	}
	if record.Error != "" {
		fields["error"] = record.Error
	}
	labels := map[string]string{
		"session_id":  record.SessionID,
		"planning_id": record.PlanningID,
	}
	if record.TaskID != "" {
		labels["task_id"] = record.TaskID
	}
	if record.PlanID != "" {
		labels["plan_id"] = record.PlanID
	}
	if record.CapabilityViewID != "" {
		labels["capability_view_id"] = record.CapabilityViewID
	}
	if record.ContextSummaryID != "" {
		labels["context_summary_id"] = record.ContextSummaryID
	}
	s.Metrics.Record("planning.cycle", fields)
	s.exportMetricSample(ctx, "planning.cycle", labels, fields)
	s.exportTraceSpan(ctx, observability.TraceSpan{
		Name:       "planning.cycle",
		TraceID:    record.PlanningID,
		SpanID:     record.PlanningID,
		SessionID:  record.SessionID,
		TaskID:     record.TaskID,
		PlanningID: record.PlanningID,
		StartedAt:  record.StartedAt,
		FinishedAt: record.FinishedAt,
		Attributes: map[string]any{
			"status":             record.Status,
			"reason":             record.Reason,
			"error":              record.Error,
			"plan_id":            record.PlanID,
			"plan_revision":      record.PlanRevision,
			"capability_view_id": record.CapabilityViewID,
			"context_summary_id": record.ContextSummaryID,
			"step_count":         stepCount,
		},
	})
}

func (s *Service) exportApprovalRequestObservability(ctx context.Context, state session.State, attempt execution.Attempt, rec approval.Record) {
	fields := map[string]any{
		"tool_name":    rec.ToolName,
		"reason":       rec.Reason,
		"matched_rule": rec.MatchedRule,
	}
	labels := map[string]string{
		"session_id":  rec.SessionID,
		"approval_id": rec.ApprovalID,
		"step_id":     rec.StepID,
		"cycle_id":    executionCycleIDFromStep(rec.Step),
	}
	if state.TaskID != "" {
		labels["task_id"] = state.TaskID
	}
	if attempt.AttemptID != "" {
		labels["attempt_id"] = attempt.AttemptID
	}
	s.Metrics.Record("approval.request", fields)
	s.exportMetricSample(ctx, "approval.request", labels, fields)
	s.exportTraceSpan(ctx, observability.TraceSpan{
		Name:       "approval.request",
		TraceID:    attempt.TraceID,
		SpanID:     rec.ApprovalID + ":request",
		ParentID:   attempt.AttemptID,
		SessionID:  rec.SessionID,
		TaskID:     state.TaskID,
		StepID:     rec.StepID,
		AttemptID:  attempt.AttemptID,
		ApprovalID: rec.ApprovalID,
		CycleID:    executionCycleIDFromStep(rec.Step),
		StartedAt:  rec.RequestedAt,
		FinishedAt: rec.RequestedAt,
		Attributes: map[string]any{
			"tool_name":    rec.ToolName,
			"reason":       rec.Reason,
			"matched_rule": rec.MatchedRule,
		},
	})
}

func (s *Service) exportApprovalResponseObservability(ctx context.Context, state session.State, rec approval.Record) {
	fields := map[string]any{
		"reply":  string(rec.Reply),
		"status": string(rec.Status),
	}
	labels := map[string]string{
		"session_id":  rec.SessionID,
		"approval_id": rec.ApprovalID,
		"step_id":     rec.StepID,
		"cycle_id":    executionCycleIDFromStep(rec.Step),
	}
	if state.TaskID != "" {
		labels["task_id"] = state.TaskID
	}
	s.Metrics.Record("approval.response", fields)
	s.exportMetricSample(ctx, "approval.response", labels, fields)
	s.exportTraceSpan(ctx, observability.TraceSpan{
		Name:       "approval.response",
		TraceID:    rec.ApprovalID,
		SpanID:     rec.ApprovalID + ":response",
		SessionID:  rec.SessionID,
		TaskID:     state.TaskID,
		StepID:     rec.StepID,
		ApprovalID: rec.ApprovalID,
		CycleID:    executionCycleIDFromStep(rec.Step),
		StartedAt:  rec.RespondedAt,
		FinishedAt: rec.RespondedAt,
		Attributes: map[string]any{
			"reply":  string(rec.Reply),
			"status": string(rec.Status),
		},
	})
}

func (s *Service) exportRecoveryObservability(ctx context.Context, state session.State, success, resumedApproval bool, startedAt, finishedAt int64) {
	fields := map[string]any{
		"success":          success,
		"resumed_approval": resumedApproval,
		"duration_ms":      finishedAt - startedAt,
	}
	labels := map[string]string{"session_id": state.SessionID}
	if state.TaskID != "" {
		labels["task_id"] = state.TaskID
	}
	s.Metrics.Record("session.recover", fields)
	s.exportMetricSample(ctx, "session.recover", labels, fields)
	s.exportTraceSpan(ctx, observability.TraceSpan{
		Name:       "session.recover",
		TraceID:    state.SessionID,
		SpanID:     "recover:" + state.SessionID + ":" + strconv.FormatInt(startedAt, 10),
		SessionID:  state.SessionID,
		TaskID:     state.TaskID,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Attributes: fields,
	})
}

func (s *Service) exportAbortObservability(ctx context.Context, state session.State, request AbortRequest, startedAt, finishedAt int64) {
	fields := map[string]any{
		"code":        request.Code,
		"reason":      request.Reason,
		"duration_ms": finishedAt - startedAt,
	}
	labels := map[string]string{"session_id": state.SessionID}
	if state.TaskID != "" {
		labels["task_id"] = state.TaskID
	}
	s.Metrics.Record("session.abort", fields)
	s.exportMetricSample(ctx, "session.abort", labels, fields)
	s.exportTraceSpan(ctx, observability.TraceSpan{
		Name:       "session.abort",
		TraceID:    state.SessionID,
		SpanID:     "abort:" + state.SessionID + ":" + strconv.FormatInt(startedAt, 10),
		SessionID:  state.SessionID,
		TaskID:     state.TaskID,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Attributes: fields,
	})
}

func (s *Service) exportLeaseObservability(ctx context.Context, name string, state session.State, leaseID string, startedAt, finishedAt int64, fields map[string]any) {
	labels := map[string]string{}
	if state.SessionID != "" {
		labels["session_id"] = state.SessionID
	}
	if leaseID != "" {
		labels["lease_id"] = leaseID
	}
	if mode, _ := fields["claim_mode"].(string); mode != "" {
		labels["claim_mode"] = mode
	}
	s.Metrics.Record(name, fields)
	s.exportMetricSample(ctx, name, labels, fields)
	s.exportTraceSpan(ctx, observability.TraceSpan{
		Name:       name,
		TraceID:    state.SessionID,
		SpanID:     name + ":" + strconv.FormatInt(startedAt, 10),
		SessionID:  state.SessionID,
		LeaseID:    leaseID,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Attributes: fields,
	})
}
