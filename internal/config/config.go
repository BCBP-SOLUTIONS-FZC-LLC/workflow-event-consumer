package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv          string
	BuildVersion    string
	OTELServiceName string

	HealthPort int

	AWSRegion      string
	AWSEndpointURL string

	ConsumerConcurrency int
	VisibilityTimeout   time.Duration
	DrainTimeout        time.Duration
	ForwardTimeout      time.Duration
	HandlerTimeout      time.Duration
	RetryBackoffBase    time.Duration
	RetryBackoffMax     time.Duration
	QueueDepthInterval  time.Duration

	TopologyPath string
}

var appEnvs = []string{"local", "dev", "test", "staging", "prod"}

func (c *Config) IsDev() bool {
	return c.AppEnv == "dev" || c.AppEnv == "local"
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	var parseErr error
	record := func(err error) {
		if err != nil && parseErr == nil {
			parseErr = err
		}
	}
	envInt := func(key string, fallback int) int {
		v, err := getEnvInt(key, fallback)
		record(err)
		return v
	}
	envDuration := func(key string, fallback time.Duration) time.Duration {
		v, err := getEnvDuration(key, fallback)
		record(err)
		return v
	}

	cfg := &Config{
		AppEnv:          os.Getenv("APP_ENV"),
		BuildVersion:    getEnvOrDefault("BUILD_VERSION", "dev"),
		OTELServiceName: getEnvOrDefault("OTEL_SERVICE_NAME", "workflow-events-consumer"),

		HealthPort: envInt("HEALTH_PORT", 8082),

		AWSRegion:      os.Getenv("AWS_REGION"),
		AWSEndpointURL: getEnvOrDefault("AWS_ENDPOINT_URL", ""),

		ConsumerConcurrency: envInt("CONSUMER_CONCURRENCY", 10),
		VisibilityTimeout:   envDuration("VISIBILITY_TIMEOUT", 30*time.Second),
		DrainTimeout:        envDuration("DRAIN_TIMEOUT", 30*time.Second),
		ForwardTimeout:      envDuration("FORWARD_TIMEOUT", 5*time.Second),
		HandlerTimeout:      envDuration("HANDLER_TIMEOUT", 20*time.Second),
		RetryBackoffBase:    envDuration("RETRY_BACKOFF_BASE", 5*time.Second),
		RetryBackoffMax:     envDuration("RETRY_BACKOFF_MAX", 5*time.Minute),
		QueueDepthInterval:  envDuration("QUEUE_DEPTH_INTERVAL", 30*time.Second),

		TopologyPath: getEnvOrDefault("TOPOLOGY_CONFIG_PATH", "config/topology.yaml"),
	}

	if parseErr != nil {
		return nil, parseErr
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.AppEnv == "" {
		return fmt.Errorf("APP_ENV is required, one of %s", strings.Join(appEnvs, ", "))
	}
	if !slices.Contains(appEnvs, c.AppEnv) {
		return fmt.Errorf("APP_ENV must be one of %s", strings.Join(appEnvs, ", "))
	}
	if c.AWSRegion == "" {
		return fmt.Errorf("AWS_REGION is required")
	}
	if c.TopologyPath == "" {
		return fmt.Errorf("TOPOLOGY_CONFIG_PATH is required")
	}
	if c.HealthPort < 1 || c.HealthPort > 65535 {
		return fmt.Errorf("HEALTH_PORT must be in [1, 65535]")
	}
	if c.ConsumerConcurrency <= 0 {
		return fmt.Errorf("CONSUMER_CONCURRENCY must be > 0")
	}
	if c.VisibilityTimeout <= 0 {
		return fmt.Errorf("VISIBILITY_TIMEOUT must be > 0")
	}
	if c.DrainTimeout <= 0 {
		return fmt.Errorf("DRAIN_TIMEOUT must be > 0")
	}
	if c.ForwardTimeout <= 0 {
		return fmt.Errorf("FORWARD_TIMEOUT must be > 0")
	}
	if c.HandlerTimeout <= 0 {
		return fmt.Errorf("HANDLER_TIMEOUT must be > 0")
	}
	if c.RetryBackoffBase <= 0 {
		return fmt.Errorf("RETRY_BACKOFF_BASE must be > 0")
	}
	if c.HandlerTimeout > c.VisibilityTimeout {
		return fmt.Errorf("HANDLER_TIMEOUT must not exceed VISIBILITY_TIMEOUT")
	}
	if c.ForwardTimeout > c.HandlerTimeout {
		return fmt.Errorf("FORWARD_TIMEOUT must not exceed HANDLER_TIMEOUT")
	}
	if c.RetryBackoffMax < c.RetryBackoffBase {
		return fmt.Errorf("RETRY_BACKOFF_MAX must not be below RETRY_BACKOFF_BASE")
	}
	if c.QueueDepthInterval <= 0 {
		return fmt.Errorf("QUEUE_DEPTH_INTERVAL must be > 0")
	}
	return nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	return d, nil
}
