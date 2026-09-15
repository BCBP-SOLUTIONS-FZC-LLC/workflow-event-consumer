package topology

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads and validates the topology file at path, failing fast on any
// structural problem — a duplicate name, a route pointing at an undeclared
// target, a missing required field. This runs once at startup, so a mistake
// here should stop the process rather than surface later as a silently
// dropped or misrouted event.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("topology: reading %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("topology: parsing %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("topology: %s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	if len(c.Targets) == 0 {
		return fmt.Errorf("at least one target is required")
	}
	if len(c.Queues) == 0 {
		return fmt.Errorf("at least one queue is required")
	}

	targetNames := make(map[string]bool, len(c.Targets))
	for _, t := range c.Targets {
		if t.Name == "" {
			return fmt.Errorf("target entry missing name")
		}
		if targetNames[t.Name] {
			return fmt.Errorf("target: duplicate name %q", t.Name)
		}
		targetNames[t.Name] = true
		if t.Addr == "" {
			return fmt.Errorf("target %q: addr is required", t.Name)
		}
		if t.TokenEnv == "" {
			return fmt.Errorf("target %q: tokenEnv is required", t.Name)
		}
	}

	queueNames := make(map[string]bool, len(c.Queues))
	for _, q := range c.Queues {
		if q.Name == "" {
			return fmt.Errorf("queue entry missing name")
		}
		if queueNames[q.Name] {
			return fmt.Errorf("queue: duplicate name %q", q.Name)
		}
		queueNames[q.Name] = true
		if q.URL == "" {
			return fmt.Errorf("queue %q: url is required", q.Name)
		}
	}

	for eventType, targets := range c.Routes {
		for _, name := range targets {
			if !targetNames[name] {
				return fmt.Errorf("route %q: references undeclared target %q", eventType, name)
			}
		}
	}

	return nil
}
