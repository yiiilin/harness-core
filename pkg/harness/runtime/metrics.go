package runtime

import (
	"github.com/yiiilin/harness-core/pkg/harness/observability"
)

type Metrics interface {
	Record(name string, fields map[string]any)
}

type noopMetrics struct{}

func (noopMetrics) Record(_ string, _ map[string]any) {}

func metricsOrNoop(m Metrics) Metrics {
	if m == nil {
		return noopMetrics{}
	}
	return m
}

func SnapshotMetrics(recorder interface{ Snapshot() observability.Snapshot }) observability.Snapshot {
	if recorder == nil {
		return observability.Snapshot{}
	}
	return recorder.Snapshot()
}
