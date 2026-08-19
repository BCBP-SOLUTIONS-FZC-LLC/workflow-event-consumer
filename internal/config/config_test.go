package config

import (
	"testing"
	"time"
)

func TestValidate_OK(t *testing.T) {
	cfg := &Config{TopologyPath: "config/topology.yaml"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidate_MissingTopologyPath(t *testing.T) {
	cfg := &Config{TopologyPath: ""}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing TopologyPath")
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppEnv != "dev" {
		t.Errorf("AppEnv = %q, want dev", cfg.AppEnv)
	}
	if cfg.HealthPort != 8082 {
		t.Errorf("HealthPort = %d, want 8082", cfg.HealthPort)
	}
	if cfg.VisibilityTimeout != 30*time.Second {
		t.Errorf("VisibilityTimeout = %v, want 30s", cfg.VisibilityTimeout)
	}
	if cfg.TopologyPath != "config/topology.yaml" {
		t.Errorf("TopologyPath = %q, want config/topology.yaml", cfg.TopologyPath)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("HEALTH_PORT", "9999")
	t.Setenv("VISIBILITY_TIMEOUT", "45s")
	t.Setenv("TOPOLOGY_CONFIG_PATH", "/etc/event-consumer/topology.yaml")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppEnv != "prod" {
		t.Errorf("AppEnv = %q, want prod", cfg.AppEnv)
	}
	if cfg.HealthPort != 9999 {
		t.Errorf("HealthPort = %d, want 9999", cfg.HealthPort)
	}
	if cfg.VisibilityTimeout != 45*time.Second {
		t.Errorf("VisibilityTimeout = %v, want 45s", cfg.VisibilityTimeout)
	}
	if cfg.TopologyPath != "/etc/event-consumer/topology.yaml" {
		t.Errorf("TopologyPath = %q, want /etc/event-consumer/topology.yaml", cfg.TopologyPath)
	}
}

func TestLoad_InvalidIntAndDurationFallBackToDefault(t *testing.T) {
	t.Setenv("HEALTH_PORT", "not-a-number")
	t.Setenv("DRAIN_TIMEOUT", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HealthPort != 8082 {
		t.Errorf("HealthPort = %d, want fallback 8082", cfg.HealthPort)
	}
	if cfg.DrainTimeout != 30*time.Second {
		t.Errorf("DrainTimeout = %v, want fallback 30s", cfg.DrainTimeout)
	}
}
