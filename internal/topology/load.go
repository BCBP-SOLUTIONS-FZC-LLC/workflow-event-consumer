package topology

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
)

const formatVersion = 1

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Config{}, fmt.Errorf("topology: reading %s: %w", path, err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("topology: parsing %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("topology: %s: %w", path, err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.Version != formatVersion {
		return fmt.Errorf("version must be %d, got %d", formatVersion, c.Version)
	}
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
			if name == forwarder.ExecutionTarget && !forwarder.Categorised(eventType) {
				return fmt.Errorf("route %q: the %s target has no route for this event type", eventType, name)
			}
		}
	}

	return nil
}
