package config

import (
	"testing"
	"time"
)

func TestValidate_AcceptsEveryBoundaryValue(t *testing.T) {
	valid := func() *Config {
		return &Config{
			AppEnv: "prod", AWSRegion: "us-east-1", HealthPort: 8082, TopologyPath: "t.yaml", ConsumerConcurrency: 10,
			VisibilityTimeout: 30 * time.Second, DrainTimeout: 30 * time.Second, ForwardTimeout: 5 * time.Second,
			HandlerTimeout: 20 * time.Second, RetryBackoffBase: 5 * time.Second, RetryBackoffMax: 5 * time.Minute,
			QueueDepthInterval: 30 * time.Second, MaxReceiveCount: 4, ReceiveStallWindow: 2 * time.Minute,
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"lowest port", func(c *Config) { c.HealthPort = 1 }},
		{"highest port", func(c *Config) { c.HealthPort = 65535 }},
		{"one handler per queue", func(c *Config) { c.ConsumerConcurrency = 1 }},
		{"handler timeout equal to the visibility timeout", func(c *Config) { c.HandlerTimeout = c.VisibilityTimeout }},
		{"forward timeout equal to the handler timeout", func(c *Config) { c.ForwardTimeout = c.HandlerTimeout }},
		{"backoff max equal to the base", func(c *Config) { c.RetryBackoffMax = c.RetryBackoffBase }},
		{"one handling attempt", func(c *Config) { c.MaxReceiveCount = 1 }},
		{"shortest durations", func(c *Config) {
			c.VisibilityTimeout, c.HandlerTimeout, c.ForwardTimeout = time.Nanosecond, time.Nanosecond, time.Nanosecond
			c.DrainTimeout, c.RetryBackoffBase, c.RetryBackoffMax = time.Nanosecond, time.Nanosecond, time.Nanosecond
			c.QueueDepthInterval, c.ReceiveStallWindow = time.Nanosecond, time.Nanosecond
		}},
	} {
		c := valid()
		tc.mutate(c)
		if err := c.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want it accepted", tc.name, err)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"port above the range", func(c *Config) { c.HealthPort = 65536 }},
		{"negative port", func(c *Config) { c.HealthPort = -1 }},
		{"negative concurrency", func(c *Config) { c.ConsumerConcurrency = -1 }},
		{"negative visibility timeout", func(c *Config) { c.VisibilityTimeout = -time.Second }},
		{"negative receive count", func(c *Config) { c.MaxReceiveCount = -1 }},
	} {
		c := valid()
		tc.mutate(c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: Validate() accepted it", tc.name)
		}
	}
}
