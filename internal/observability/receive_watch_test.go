package observability_test

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability/obstest"
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
