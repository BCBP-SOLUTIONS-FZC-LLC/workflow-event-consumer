package observability

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	receiveSeries = "platform_dependency_request_seconds"
)

type ReceiveWatch struct {
	gatherer prometheus.Gatherer
	window   time.Duration
	now      func() time.Time

	mu         sync.Mutex
	last       uint64
	seen       bool
	lastChange time.Time
}

func NewReceiveWatch(g prometheus.Gatherer, window time.Duration, now func() time.Time) *ReceiveWatch {
	w := &ReceiveWatch{gatherer: g, window: window, now: now, lastChange: now()}
	w.last = w.successfulReceives()
	w.seen = w.last > 0
	return w
}

func (w *ReceiveWatch) Healthy() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if count := w.successfulReceives(); count > w.last {
		w.last = count
		w.seen = true
		w.lastChange = w.now()
	}
	return w.seen && w.now().Sub(w.lastChange) <= w.window
}

func (w *ReceiveWatch) successfulReceives() uint64 {
	families, err := w.gatherer.Gather()
	if err != nil {
		return w.last
	}
	var total uint64
	for _, f := range families {
		if f.GetName() != receiveSeries {
			continue
		}
		for _, m := range f.GetMetric() {
			if hasLabels(m, map[string]string{"dependency": "sqs", "operation": "receive_message", "outcome": "success"}) {
				total += m.GetHistogram().GetSampleCount()
			}
		}
	}
	return total
}

func hasLabels(m *dto.Metric, want map[string]string) bool {
	matched := 0
	for _, lp := range m.GetLabel() {
		if v, ok := want[lp.GetName()]; ok && v == lp.GetValue() {
			matched++
		}
	}
	return matched == len(want)
}
