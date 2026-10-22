package observability

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
)

const (
	Domain      = "workflow"
	ServiceName = "workflow-events-consumer"
)

var (
	ForwardTotal           *prometheus.CounterVec
	ForwardDurationSeconds *prometheus.HistogramVec
	HandlerDurationSeconds *prometheus.HistogramVec

	registerOnce sync.Once
)

func Register() {
	registerOnce.Do(registerServiceMetrics)
}

func must[C any](c C, err error) C {
	if err != nil {
		panic("metrics: register service collector: " + err.Error())
	}
	return c
}

func registerServiceMetrics() {
	ForwardTotal = must(gincommon.NewCounterVec(nil,
		"workflow_events_consumer_forward_total",
		"Count of per-target HTTP forward attempts, by queue, target, and result.",
		"queue", "target", "result"))
	ForwardDurationSeconds = must(gincommon.NewHistogramVec(nil,
		"workflow_events_consumer_forward_duration_seconds",
		"Per-target HTTP forward latency, by queue and target.",
		prometheus.DefBuckets, "queue", "target"))
	HandlerDurationSeconds = must(gincommon.NewHistogramVec(nil,
		"workflow_events_consumer_handler_duration_seconds",
		"End-to-end per-message handler latency, by queue and result.",
		prometheus.DefBuckets, "queue", "result"))
}
