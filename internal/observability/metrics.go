package observability

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
)

var (
	ForwardTotal           *prometheus.CounterVec
	ForwardDurationSeconds *prometheus.HistogramVec
	HandlerDurationSeconds *prometheus.HistogramVec

	registerOnce sync.Once
)

func Register() {
	registerOnce.Do(register)
}

func register() {
	ForwardTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workflow_events_consumer_forward_total",
		Help: "Count of per-target HTTP forward attempts, by queue, target, and result.",
	}, []string{"queue", "target", "result"})

	ForwardDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_events_consumer_forward_duration_seconds",
		Help:    "Per-target HTTP forward latency, by queue and target.",
		Buckets: prometheus.DefBuckets,
	}, []string{"queue", "target"})

	HandlerDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "workflow_events_consumer_handler_duration_seconds",
		Help:    "End-to-end per-message handler latency, by queue and result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"queue", "result"})

	// Registered against gincommon.MetricsRegisterer(), not
	// prometheus.MustRegister on the default registry directly — the same
	// convention definition_service and execution_service already follow, so
	// a test that swaps in an isolated registry sees these collectors too.
	gincommon.MetricsRegisterer().MustRegister(ForwardTotal, ForwardDurationSeconds, HandlerDurationSeconds)
}
