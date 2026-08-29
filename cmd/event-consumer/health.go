package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
)

func newHealthServer(addr string) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/healthz", gincommon.HealthHandler())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	return &http.Server{Addr: addr, Handler: r}
}
