package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/logger"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/obstest"
)

func newTestHealthServer(t *testing.T, w *observability.ReceiveWatch) *http.Server {
	t.Helper()
	log, err := logger.NewLogger("test")
	require.NoError(t, err)
	return newHealthServer(":0", observability.GinConfig(observability.ServiceName, "test", "test", log), w)
}

func TestHealthServer_AnswersOnlyItsThreeRoutes(t *testing.T) {
	srv := newTestHealthServer(t, observability.NewReceiveWatch(&obstest.ReceiveGatherer{Successes: 1}, time.Minute, time.Now))
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/healthz"},
		{http.MethodDelete, "/readyz"},
		{http.MethodGet, "/"},
		{http.MethodGet, "/api/v1/internal/events"},
	} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s %s", tc.method, tc.path)
	}
}

func TestHealthServer_ANotReadyAnswerSaysTheReceiveStalled(t *testing.T) {
	srv := newTestHealthServer(t, observability.NewReceiveWatch(&obstest.ReceiveGatherer{}, time.Minute, time.Now))

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "not ready before the first successful receive")
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "receive_stalled", body["status"])
}

func TestHealthServer_BoundsEveryPhaseOfARequest(t *testing.T) {
	srv := newTestHealthServer(t, observability.NewReceiveWatch(&obstest.ReceiveGatherer{}, time.Minute, time.Now))

	assert.Equal(t, 5*time.Second, srv.ReadHeaderTimeout)
	assert.Equal(t, 10*time.Second, srv.ReadTimeout)
	assert.Equal(t, 30*time.Second, srv.WriteTimeout)
	assert.Equal(t, 60*time.Second, srv.IdleTimeout)
}
