package runtime

import (
	"context"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	"github.com/yiiilin/harness-core/pkg/harness/observability"
	"github.com/yiiilin/harness-core/pkg/harness/plan"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func (s *Service) exportStepMetricSample(ctx context.Context, state session.State, step plan.StepSpec, attempt execution.Attempt, actionRecord *execution.ActionRecord, verificationRecord *execution.VerificationRecord, success, policyDenied, verifyFailed, actionFailed bool, durationMS int64) {
	if s.MetricsExporter == nil {
		return
	}
	labels := map[string]string{
		"session_id": state.SessionID,
		"task_id":    state.TaskID,
		"step_id":    step.StepID,
		"attempt_id": attempt.AttemptID,
		"trace_id":   attempt.TraceID,
	}
	if actionRecord != nil && actionRecord.ActionID != "" {
		labels["action_id"] = actionRecord.ActionID
	}
	if verificationRecord != nil && verificationRecord.VerificationID != "" {
		labels["verification_id"] = verificationRecord.VerificationID
	}
	_ = s.MetricsExporter.ExportMetric(ctx, observability.MetricSample{
		Name:       "step.run",
		Labels:     labels,
		Fields:     map[string]any{"success": success, "policy_denied": policyDenied, "verify_failed": verifyFailed, "action_failed": actionFailed, "duration_ms": durationMS},
		RecordedAt: time.Now().UnixMilli(),
	})
}

func (s *Service) exportTraceSpans(ctx context.Context, state session.State, step plan.StepSpec, attempt execution.Attempt, actionRecord *execution.ActionRecord, verificationRecord *execution.VerificationRecord) {
	if s.TraceExporter == nil {
		return
	}
	_ = s.TraceExporter.ExportTrace(ctx, observability.TraceSpan{
		Name:       "step.run",
		TraceID:    attempt.TraceID,
		SpanID:     attempt.AttemptID,
		SessionID:  state.SessionID,
		TaskID:     state.TaskID,
		StepID:     step.StepID,
		AttemptID:  attempt.AttemptID,
		StartedAt:  attempt.StartedAt,
		FinishedAt: attempt.FinishedAt,
		Attributes: map[string]any{"step_status": attempt.Status},
	})
	if actionRecord != nil {
		_ = s.TraceExporter.ExportTrace(ctx, observability.TraceSpan{
			Name:        "tool.invoke",
			TraceID:     actionRecord.TraceID,
			SpanID:      actionRecord.ActionID,
			ParentID:    attempt.AttemptID,
			SessionID:   state.SessionID,
			TaskID:      state.TaskID,
			StepID:      step.StepID,
			AttemptID:   attempt.AttemptID,
			ActionID:    actionRecord.ActionID,
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
		_ = s.TraceExporter.ExportTrace(ctx, observability.TraceSpan{
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
			CausationID:    verificationRecord.CausationID,
			StartedAt:      verificationRecord.StartedAt,
			FinishedAt:     verificationRecord.FinishedAt,
			Attributes:     map[string]any{"status": verificationRecord.Status},
		})
	}
}
