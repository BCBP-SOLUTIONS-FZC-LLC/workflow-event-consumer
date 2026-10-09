package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
)

func newHealthServer(addr string, ginCfg gincommon.Config) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	for _, mw := range gincommon.ObservabilityMiddlewares(ginCfg) {
		r.Use(mw)
	}
	r.GET("/healthz", gincommon.HealthHandler())
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
