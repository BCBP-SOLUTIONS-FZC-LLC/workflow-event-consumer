package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/logger"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

type fakeGatherer struct{ successes float64 }

func (g *fakeGatherer) Gather() ([]*dto.MetricFamily, error) {
	name := "platform_dependency_request_seconds"
	count := uint64(g.successes)
	other := uint64(7)
	label := func(k, v string) *dto.LabelPair { return &dto.LabelPair{Name: &k, Value: &v} }
	family := &dto.MetricFamily{Name: &name, Type: dto.MetricType_HISTOGRAM.Enum(), Metric: []*dto.Metric{
		{Label: []*dto.LabelPair{label("dependency", "sqs"), label("operation", "receive_message"), label("outcome", "success")}, Histogram: &dto.Histogram{SampleCount: &count}},
		{Label: []*dto.LabelPair{label("dependency", "sqs"), label("operation", "receive_message"), label("outcome", "error")}, Histogram: &dto.Histogram{SampleCount: &other}},
		{Label: []*dto.LabelPair{label("dependency", "sqs"), label("operation", "delete_message"), label("outcome", "success")}, Histogram: &dto.Histogram{SampleCount: &other}},
	}}
	return []*dto.MetricFamily{family}, nil
}

var _ prometheus.Gatherer = (*fakeGatherer)(nil)

func TestReceiveWatch_HealthyWhileReceivesSucceed(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &fakeGatherer{successes: 10}
	w := newReceiveWatch(g, 2*time.Minute, func() time.Time { return now })

	now = now.Add(90 * time.Second)
	g.successes = 15
	assert.True(t, w.healthy())

	now = now.Add(90 * time.Second)
	g.successes = 20
	assert.True(t, w.healthy(), "progress resets the window")
}

func TestReceiveWatch_UnhealthyWhenNoReceiveSucceedsWithinTheWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &fakeGatherer{successes: 10}
	w := newReceiveWatch(g, 2*time.Minute, func() time.Time { return now })

	now = now.Add(time.Minute)
	assert.True(t, w.healthy(), "inside the window after start")

	now = now.Add(2 * time.Minute)
	assert.False(t, w.healthy(), "errors and deletes do not count as receiving")
}

func TestReceiveWatch_UnhealthyWhenTheSeriesNeverAppears(t *testing.T) {
	now := time.Unix(1000, 0)
	w := newReceiveWatch(prometheus.NewRegistry(), time.Minute, func() time.Time { return now })

	now = now.Add(2 * time.Minute)

	assert.False(t, w.healthy())
}

func TestHealthServer_ReportsAStalledReceive(t *testing.T) {
	now := time.Unix(1000, 0)
	w := newReceiveWatch(&fakeGatherer{successes: 1}, time.Minute, func() time.Time { return now })
	log, err := logger.NewLogger("test")
	require.NoError(t, err)
	srv := newHealthServer(":0", observability.GinConfig(observability.ServiceName, "test", "test", log), w)

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	now = now.Add(2 * time.Minute)
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code, "liveness must not follow an SQS or STS outage that a restart cannot fix")
}

func TestReceiveWatch_UnhealthyUntilTheFirstReceiveSucceeds(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &fakeGatherer{successes: 0}
	w := newReceiveWatch(g, 2*time.Minute, func() time.Time { return now })

	assert.False(t, w.healthy(), "a pod whose first receive has not succeeded is not ready, so a bad rollout never replaces a healthy pod")

	g.successes = 1
	assert.True(t, w.healthy())
}
