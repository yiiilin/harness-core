package runtime_test

import (
	"context"
	"testing"

	"github.com/yiiilin/harness-core/pkg/harness/observability"
)

type recordingMetricsExporter struct {
	samples []observability.MetricSample
}

func (e *recordingMetricsExporter) ExportMetric(_ context.Context, sample observability.MetricSample) error {
	e.samples = append(e.samples, sample)
	return nil
}

type recordingTraceExporter struct {
	spans []observability.TraceSpan
}

func (e *recordingTraceExporter) ExportTrace(_ context.Context, span observability.TraceSpan) error {
	e.spans = append(e.spans, span)
	return nil
}

func TestRunStepExportsVendorNeutralObservabilitySamples(t *testing.T) {
	metricsExporter := &recordingMetricsExporter{}
	traceExporter := &recordingTraceExporter{}
	rt, sess, step := newHappyRuntime(t)
	rt.MetricsExporter = metricsExporter
	rt.TraceExporter = traceExporter

	if _, err := rt.RunStep(context.Background(), sess.SessionID, step); err != nil {
		t.Fatalf("run step: %v", err)
	}
	if len(metricsExporter.samples) == 0 {
		t.Fatalf("expected exported metric samples")
	}

	sample := metricsExporter.samples[0]
	if sample.Name != "step.run" {
		t.Fatalf("expected step.run metric sample, got %#v", sample)
	}
	if sample.Labels["session_id"] != sess.SessionID || sample.Labels["attempt_id"] == "" || sample.Labels["trace_id"] == "" {
		t.Fatalf("expected correlation labels on metric sample, got %#v", sample)
	}

	foundVerifySpan := false
	for _, span := range traceExporter.spans {
		if span.Name != "verify.evaluate" {
			continue
		}
		foundVerifySpan = true
		if span.TraceID == "" || span.AttemptID == "" || span.VerificationID == "" || span.CausationID == "" {
			t.Fatalf("expected correlation ids on verify trace span, got %#v", span)
		}
	}
	if !foundVerifySpan {
		t.Fatalf("expected verify trace span, got %#v", traceExporter.spans)
	}
}
