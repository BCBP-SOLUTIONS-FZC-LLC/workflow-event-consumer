package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
)

// newHealthServer serves /healthz via gincommon.HealthHandler() — this
// service's only other use of platform-gincommon is pkg/logger, so this is
// its first Gin engine — rather than hand-rolling the same {"status":"ok"}
// response gincommon already provides. /metrics stays on raw
// promhttp.Handler(), wrapped through gin.WrapH: platform-gincommon has no
// Gin-native metrics route (MetricsRegisterer returns a prometheus.Registerer,
// not an http.Handler), and every service in this platform serves /metrics
// this way regardless of whether it runs Gin — see iam-user-profile's own
// dedicated metrics port.
func newHealthServer(addr string) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/healthz", gincommon.HealthHandler())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	return &http.Server{Addr: addr, Handler: r}
}
