package observability_test

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/obstest"
)

func TestReceiveWatch_HealthyWhileReceivesSucceed(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &obstest.ReceiveGatherer{Successes: 10}
	w := observability.NewReceiveWatch(g, 2*time.Minute, func() time.Time { return now })

	now = now.Add(90 * time.Second)
	g.Successes = 15
	assert.True(t, w.Healthy())

	now = now.Add(90 * time.Second)
	g.Successes = 20
	assert.True(t, w.Healthy(), "progress resets the window")
}

func TestReceiveWatch_UnhealthyWhenNoReceiveSucceedsWithinTheWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &obstest.ReceiveGatherer{Successes: 10}
	w := observability.NewReceiveWatch(g, 2*time.Minute, func() time.Time { return now })

	now = now.Add(time.Minute)
	assert.True(t, w.Healthy(), "inside the window after start")

	now = now.Add(2 * time.Minute)
	assert.False(t, w.Healthy(), "errors and deletes do not count as receiving")
}

func TestReceiveWatch_UnhealthyWhenTheSeriesNeverAppears(t *testing.T) {
	now := time.Unix(1000, 0)
	w := observability.NewReceiveWatch(prometheus.NewRegistry(), time.Minute, func() time.Time { return now })

	now = now.Add(2 * time.Minute)

	assert.False(t, w.Healthy())
}

func TestReceiveWatch_UnhealthyUntilTheFirstReceiveSucceeds(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &obstest.ReceiveGatherer{Successes: 0}
	w := observability.NewReceiveWatch(g, 2*time.Minute, func() time.Time { return now })

	assert.False(t, w.Healthy(), "a pod whose first receive has not succeeded is not ready, so a bad rollout never replaces a healthy pod")

	g.Successes = 1
	assert.True(t, w.Healthy())
}

type switchableGatherer struct {
	inner *obstest.ReceiveGatherer
	fail  bool
}

func (g *switchableGatherer) Gather() ([]*dto.MetricFamily, error) {
	if g.fail {
		return nil, errors.New("scrape failed")
	}
	return g.inner.Gather()
}

func TestReceiveWatch_AFailedScrapeNeitherInventsNorErasesASuccess(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &switchableGatherer{inner: &obstest.ReceiveGatherer{Successes: 3}}
	w := observability.NewReceiveWatch(g, 2*time.Minute, func() time.Time { return now })

	g.fail = true
	now = now.Add(time.Minute)
	assert.True(t, w.Healthy(), "a scrape failure inside the window keeps the earlier success")

	now = now.Add(2 * time.Minute)
	assert.False(t, w.Healthy(), "a scrape failure never counts as a receive")
}

func TestReceiveWatch_AScrapeThatFailsAtStartIsNotHealthy(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &switchableGatherer{inner: &obstest.ReceiveGatherer{Successes: 3}, fail: true}
	w := observability.NewReceiveWatch(g, time.Minute, func() time.Time { return now })

	assert.False(t, w.Healthy())
}

type otherSeriesGatherer struct{}

func (otherSeriesGatherer) Gather() ([]*dto.MetricFamily, error) {
	name := "platform_something_else"
	count := uint64(9)
	k, v := "outcome", "success"
	return []*dto.MetricFamily{{Name: &name, Type: dto.MetricType_HISTOGRAM.Enum(), Metric: []*dto.Metric{
		{Label: []*dto.LabelPair{{Name: &k, Value: &v}}, Histogram: &dto.Histogram{SampleCount: &count}},
	}}}, nil
}

func TestReceiveWatch_OnlyTheSqsReceiveSeriesCounts(t *testing.T) {
	now := time.Unix(1000, 0)
	w := observability.NewReceiveWatch(otherSeriesGatherer{}, time.Minute, func() time.Time { return now })

	assert.False(t, w.Healthy())
}
