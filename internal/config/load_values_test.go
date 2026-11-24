package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad_ReportsTheFirstMalformedValue(t *testing.T) {
	requiredEnv(t)
	t.Setenv("HEALTH_PORT", "not-a-port")
	t.Setenv("VISIBILITY_TIMEOUT", "not-a-duration")

	_, err := Load()

	if err == nil || !strings.Contains(err.Error(), "HEALTH_PORT") {
		t.Fatalf("Load() = %v, want the HEALTH_PORT error, which is read first", err)
	}
	if strings.Contains(err.Error(), "VISIBILITY_TIMEOUT") {
		t.Errorf("Load() = %v, want only the first malformed value reported", err)
	}
}

func TestLoad_AnEmptySettingTakesItsDefault(t *testing.T) {
	requiredEnv(t)
	for _, key := range []string{"BUILD_VERSION", "OTEL_SERVICE_NAME", "AWS_ENDPOINT_URL", "TOPOLOGY_CONFIG_PATH", "HEALTH_PORT", "VISIBILITY_TIMEOUT"} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TopologyPath != "config/topology.yaml" {
		t.Errorf("TopologyPath = %q, want the default", cfg.TopologyPath)
	}
	if cfg.OTELServiceName == "" || cfg.BuildVersion == "" {
		t.Errorf("OTELServiceName = %q, BuildVersion = %q, want their defaults", cfg.OTELServiceName, cfg.BuildVersion)
	}
	if cfg.AWSEndpointURL != "" {
		t.Errorf("AWSEndpointURL = %q, want empty (no emulator)", cfg.AWSEndpointURL)
	}
	if cfg.HealthPort != 8082 || cfg.VisibilityTimeout != 30*time.Second {
		t.Errorf("HealthPort = %d, VisibilityTimeout = %v, want 8082 and 30s", cfg.HealthPort, cfg.VisibilityTimeout)
	}
}
