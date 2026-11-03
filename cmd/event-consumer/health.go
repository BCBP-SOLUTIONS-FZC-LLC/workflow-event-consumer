package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

func newHealthServer(addr string, ginCfg gincommon.Config, watch *observability.ReceiveWatch) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	for _, mw := range gincommon.ObservabilityMiddlewares(ginCfg) {
		r.Use(mw)
	}
	live := gincommon.HealthHandler()
	r.GET("/healthz", live)
	r.GET("/readyz", func(c *gin.Context) {
		if !watch.Healthy() {
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
