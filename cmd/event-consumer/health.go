package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
)

const (
	receiveStallWindow = 2 * time.Minute

	receiveSeries = "platform_dependency_request_seconds"
)

type receiveWatch struct {
	gatherer prometheus.Gatherer
	window   time.Duration
	now      func() time.Time

	mu         sync.Mutex
	last       uint64
	seen       bool
	lastChange time.Time
}

func newReceiveWatch(g prometheus.Gatherer, window time.Duration, now func() time.Time) *receiveWatch {
	w := &receiveWatch{gatherer: g, window: window, now: now, lastChange: now()}
	w.last = w.successfulReceives()
	w.seen = w.last > 0
	return w
}

func (w *receiveWatch) healthy() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if count := w.successfulReceives(); count > w.last {
		w.last = count
		w.seen = true
		w.lastChange = w.now()
	}
	return w.seen && w.now().Sub(w.lastChange) <= w.window
}

func (w *receiveWatch) successfulReceives() uint64 {
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

func newHealthServer(addr string, ginCfg gincommon.Config, watch *receiveWatch) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	for _, mw := range gincommon.ObservabilityMiddlewares(ginCfg) {
		r.Use(mw)
	}
	live := gincommon.HealthHandler()
	r.GET("/healthz", func(c *gin.Context) {
		if !watch.healthy() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "receive_stalled"})
			return
		}
		live(c)
	})
	r.GET("/metrics", gin.WrapH(gincommon.MetricsHandler()))
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
