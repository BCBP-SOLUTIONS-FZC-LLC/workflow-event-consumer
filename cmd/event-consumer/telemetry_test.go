package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/logger"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/obsregistry"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

var ruleMetricPattern = regexp.MustCompile(`(?:workflow_events_consumer|platform)_[a-z_]+`)

func TestStandard_LibrariesShareRegistryCollectors(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "none")
	log, err := logger.NewLogger("test")
	require.NoError(t, err)

	ginCfg := observability.GinConfig(observability.ServiceName, "test", "test", log)
	shutdown, err := observability.InitTelemetry(ginCfg, log)
	require.NoError(t, err)
	t.Cleanup(shutdown)

	observability.ForwardTotal.WithLabelValues("membership", "execution", "ok").Inc()
	observability.ForwardDurationSeconds.WithLabelValues("membership", "execution").Observe(0.1)
	observability.HandlerDurationSeconds.WithLabelValues("membership", "ok").Observe(0.1)

	srv := newHealthServer(":0", ginCfg)
	for _, path := range []string{"/healthz", "/metrics"} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code, path)
	}

	gathered, err := gincommon.MetricsGatherer().Gather()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, mf := range gathered {
		names[mf.GetName()] = true
	}
	for _, want := range []string{
		"platform_build_info",
		"platform_library_info",
		"platform_http_requests_total",
		"workflow_events_consumer_forward_total",
		"workflow_events_consumer_forward_duration_seconds",
		"workflow_events_consumer_handler_duration_seconds",
	} {
		assert.True(t, names[want], "%s must be exported", want)
	}

	registry := obsregistryNames(t)
	body, readErr := os.ReadFile("../../deploy/monitoring/app-alerts.yml")
	require.NoError(t, readErr)
	for _, line := range strings.Split(string(body), "\n") {
		for _, token := range ruleMetricPattern.FindAllString(line, -1) {
			name := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(token, "_bucket"), "_count"), "_sum")
			if strings.HasPrefix(name, "workflow_events_consumer_") {
				assert.True(t, names[name], "alerts reference %s, which the service does not export", token)
			} else {
				assert.True(t, registry[name], "alerts reference %s, which the platform registry does not define", token)
			}
		}
	}

	rep, err := obsregistry.Validate(gincommon.MetricsGatherer(), obsregistry.Options{})
	require.NoError(t, err)
	assert.Empty(t, rep.Errors(), "scrape must conform to the Platform Observability Registry")

	if out := os.Getenv("METRICS_SCRAPE_OUT"); out != "" {
		scrape := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil)
		req.Header.Set("Accept", "text/plain")
		gincommon.MetricsHandler().ServeHTTP(scrape, req)
		require.NoError(t, os.WriteFile(out, scrape.Body.Bytes(), 0o600))
	}
}

func obsregistryNames(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, m := range obsregistry.Default().Metrics {
		names[m.Name] = true
	}
	return names
}
