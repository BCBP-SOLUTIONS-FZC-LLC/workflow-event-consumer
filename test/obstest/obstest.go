package obstest

import (
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

func Init(tb testing.TB) {
	tb.Helper()
	Setup()
}

func Setup() {
	_ = gincommon.ObservabilityMiddlewares(gincommon.Config{
		ServiceName:  "workflow-events-consumer",
		BuildVersion: "test",
		Domain:       observability.Domain,
		Environment:  "test",
	})
	observability.Register()
	_, _ = events.InitMetrics(events.MetricsIdentityFromLabels(gincommon.MetricsConstLabels()), gincommon.MetricsRegisterer())
}

func Sum(tb testing.TB, name string, labels map[string]string) float64 {
	tb.Helper()
	families, err := gincommon.MetricsGatherer().Gather()
	if err != nil {
		tb.Fatalf("gather: %v", err)
	}
	var total float64
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			if matches(m, labels) {
				total += value(f.GetType(), m)
			}
		}
	}
	return total
}

func matches(m *dto.Metric, labels map[string]string) bool {
	for k, v := range labels {
		found := false
		for _, lp := range m.GetLabel() {
			if lp.GetName() == k && lp.GetValue() == v {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func value(t dto.MetricType, m *dto.Metric) float64 {
	if t == dto.MetricType_COUNTER {
		return m.GetCounter().GetValue()
	}
	if t == dto.MetricType_GAUGE {
		return m.GetGauge().GetValue()
	}
	if t == dto.MetricType_HISTOGRAM {
		return float64(m.GetHistogram().GetSampleCount())
	}
	return 0
}

type ReceiveGatherer struct{ Successes float64 }

func (g *ReceiveGatherer) Gather() ([]*dto.MetricFamily, error) {
	name := "platform_dependency_request_seconds"
	count := uint64(g.Successes)
	other := uint64(7)
	label := func(k, v string) *dto.LabelPair { return &dto.LabelPair{Name: &k, Value: &v} }
	family := &dto.MetricFamily{Name: &name, Type: dto.MetricType_HISTOGRAM.Enum(), Metric: []*dto.Metric{
		{Label: []*dto.LabelPair{label("dependency", "sqs"), label("operation", "receive_message"), label("outcome", "success")}, Histogram: &dto.Histogram{SampleCount: &count}},
		{Label: []*dto.LabelPair{label("dependency", "sqs"), label("operation", "receive_message"), label("outcome", "error")}, Histogram: &dto.Histogram{SampleCount: &other}},
		{Label: []*dto.LabelPair{label("dependency", "sqs"), label("operation", "delete_message"), label("outcome", "success")}, Histogram: &dto.Histogram{SampleCount: &other}},
	}}
	return []*dto.MetricFamily{family}, nil
}
