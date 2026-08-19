package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Queues, downstream targets, and event routing all live in the file at
// TopologyPath (see internal/topology) — not here. This struct only holds
// settings that are the same regardless of which queues/targets exist.
type Config struct {
	AppEnv       string
	BuildVersion string

	HealthPort int

	AWSRegion      string
	AWSEndpointURL string

	ConsumerConcurrency int
	VisibilityTimeout   time.Duration
	DrainTimeout        time.Duration
	ForwardTimeout      time.Duration

	TopologyPath string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppEnv:       getEnvOrDefault("APP_ENV", "dev"),
		BuildVersion: getEnvOrDefault("BUILD_VERSION", "dev"),

		HealthPort: getEnvIntOrDefault("HEALTH_PORT", 8082),

		AWSRegion:      getEnvOrDefault("AWS_REGION", "us-east-1"),
		AWSEndpointURL: getEnvOrDefault("AWS_ENDPOINT_URL", ""),

		ConsumerConcurrency: getEnvIntOrDefault("CONSUMER_CONCURRENCY", 10),
		VisibilityTimeout:   getEnvDurationOrDefault("VISIBILITY_TIMEOUT", 30*time.Second),
		DrainTimeout:        getEnvDurationOrDefault("DRAIN_TIMEOUT", 30*time.Second),
		ForwardTimeout:      getEnvDurationOrDefault("FORWARD_TIMEOUT", 5*time.Second),

		TopologyPath: getEnvOrDefault("TOPOLOGY_CONFIG_PATH", "config/topology.yaml"),
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.TopologyPath == "" {
		return fmt.Errorf("TOPOLOGY_CONFIG_PATH is required")
	}
	return nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvIntOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvDurationOrDefault(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
