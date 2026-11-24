package observability_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

type silentLogger struct{}

func (silentLogger) Debug(string, map[string]interface{}) {}
func (silentLogger) Info(string, map[string]interface{})  {}
func (silentLogger) Warn(string, map[string]interface{})  {}
func (silentLogger) Error(string, map[string]interface{}) {}

func TestGinConfig_CarriesTheSettingsAndTheWorkflowDomain(t *testing.T) {
	log := silentLogger{}

	cfg := observability.GinConfig("svc", "1.2.3", "prod", log)

	assert.Equal(t, "svc", cfg.ServiceName)
	assert.Equal(t, "1.2.3", cfg.BuildVersion)
	assert.Equal(t, "prod", cfg.Environment)
	assert.Equal(t, observability.Domain, cfg.Domain)
	assert.Equal(t, log, cfg.Logger)
}

func TestInitTelemetry_ATracingFailureIsReportedAsOneAndLeavesNoShutdown(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "none")

	shutdown, err := observability.InitTelemetry(gincommon.Config{Logger: silentLogger{}}, silentLogger{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tracing")
	assert.Nil(t, shutdown)
}
