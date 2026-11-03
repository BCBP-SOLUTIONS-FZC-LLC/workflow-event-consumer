package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/logger"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability/obstest"
)

func TestHealthServer_ReportsAStalledReceive(t *testing.T) {
	now := time.Unix(1000, 0)
	w := observability.NewReceiveWatch(&obstest.ReceiveGatherer{Successes: 1}, time.Minute, func() time.Time { return now })
	log, err := logger.NewLogger("test")
	require.NoError(t, err)
	srv := newHealthServer(":0", observability.GinConfig(observability.ServiceName, "test", "test", log), w)

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	now = now.Add(2 * time.Minute)
	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	rec = httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code, "liveness must not follow an SQS or STS outage that a restart cannot fix")
}
