package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidate_MissingTopologyPath(t *testing.T) {
	requiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.TopologyPath = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing TopologyPath")
	}
}

func TestLoad_Defaults(t *testing.T) {
	requiredEnv(t)
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
	requiredEnv(t)
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

func TestLoad_MalformedValuesFailFast(t *testing.T) {
	requiredEnv(t)
	for name, env := range map[string][2]string{
		"int":      {"CONSUMER_CONCURRENCY", "many"},
		"duration": {"VISIBILITY_TIMEOUT", "soon"},
		"port":     {"HEALTH_PORT", "http"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(env[0], env[1])
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), env[0]) {
				t.Fatalf("Load() error = %v, want one naming %s", err, env[0])
			}
		})
	}
}

func TestLoad_RejectsUnknownAppEnv(t *testing.T) {
	requiredEnv(t)
	for _, env := range []string{"production", "development", "qa"} {
		t.Setenv("APP_ENV", env)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "APP_ENV") {
			t.Errorf("APP_ENV=%s: error = %v, want one naming APP_ENV", env, err)
		}
	}
}

func TestLoad_AcceptsEveryPlatformAppEnv(t *testing.T) {
	requiredEnv(t)
	for _, env := range []string{"local", "dev", "test", "staging", "prod"} {
		t.Setenv("APP_ENV", env)
		if _, err := Load(); err != nil {
			t.Errorf("APP_ENV=%s: %v", env, err)
		}
	}
}

func TestIsDev(t *testing.T) {
	requiredEnv(t)
	for env, want := range map[string]bool{"local": true, "dev": true, "test": false, "staging": false, "prod": false} {
		if got := (&Config{AppEnv: env}).IsDev(); got != want {
			t.Errorf("IsDev(%s) = %v, want %v", env, got, want)
		}
	}
}

func TestLoad_ConsumerOptionDefaults(t *testing.T) {
	requiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OTELServiceName != "workflow-events-consumer" {
		t.Errorf("OTELServiceName = %q", cfg.OTELServiceName)
	}
	if cfg.HandlerTimeout != 20*time.Second || cfg.RetryBackoffBase != 5*time.Second || cfg.RetryBackoffMax != 5*time.Minute || cfg.QueueDepthInterval != 30*time.Second {
		t.Errorf("defaults = %v %v %v %v", cfg.HandlerTimeout, cfg.RetryBackoffBase, cfg.RetryBackoffMax, cfg.QueueDepthInterval)
	}
}

func TestValidate_RejectsInconsistentSettings(t *testing.T) {
	requiredEnv(t)
	valid := func() *Config {
		return &Config{
			AppEnv: "dev", AWSRegion: "us-east-1", HealthPort: 8082, TopologyPath: "t.yaml", ConsumerConcurrency: 10,
			VisibilityTimeout: 30 * time.Second, DrainTimeout: 30 * time.Second, ForwardTimeout: 5 * time.Second,
			HandlerTimeout: 20 * time.Second, RetryBackoffBase: 5 * time.Second, RetryBackoffMax: 5 * time.Minute,
			QueueDepthInterval: 30 * time.Second,
		}
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"port", func(c *Config) { c.HealthPort = 0 }, "HEALTH_PORT"},
		{"concurrency", func(c *Config) { c.ConsumerConcurrency = 0 }, "CONSUMER_CONCURRENCY"},
		{"visibility", func(c *Config) { c.VisibilityTimeout = 0 }, "VISIBILITY_TIMEOUT must be > 0"},
		{"drain", func(c *Config) { c.DrainTimeout = 0 }, "DRAIN_TIMEOUT"},
		{"forward", func(c *Config) { c.ForwardTimeout = 0 }, "FORWARD_TIMEOUT must be > 0"},
		{"handler unset", func(c *Config) { c.HandlerTimeout = 0 }, "HANDLER_TIMEOUT must be > 0"},
		{"handler beyond visibility", func(c *Config) { c.HandlerTimeout = 31 * time.Second }, "must not exceed VISIBILITY_TIMEOUT"},
		{"forward beyond handler", func(c *Config) { c.ForwardTimeout = 21 * time.Second }, "FORWARD_TIMEOUT must not exceed"},
		{"backoff base unset", func(c *Config) { c.RetryBackoffBase = 0 }, "RETRY_BACKOFF_BASE must be > 0"},
		{"backoff max below base", func(c *Config) { c.RetryBackoffMax = time.Second }, "RETRY_BACKOFF_MAX"},
		{"queue depth interval", func(c *Config) { c.QueueDepthInterval = 0 }, "QUEUE_DEPTH_INTERVAL"},
		{"region", func(c *Config) { c.AWSRegion = "" }, "AWS_REGION is required"},
	} {
		c := valid()
		tc.mutate(c)
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate() = %v, want an error containing %q", tc.name, err, tc.want)
		}
	}
}

func requiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "dev")
	t.Setenv("AWS_REGION", "us-east-1")
}

func TestLoad_RequiresAppEnvAndRegion(t *testing.T) {
	tests := []struct {
		name   string
		unset  string
		wantIn string
	}{
		{"app env", "APP_ENV", "APP_ENV is required"},
		{"region", "AWS_REGION", "AWS_REGION is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requiredEnv(t)
			t.Setenv(tc.unset, "")

			_, err := Load()

			if err == nil || !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("error = %v, want one containing %q", err, tc.wantIn)
			}
		})
	}
}
