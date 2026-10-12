package topology

import (
	"fmt"
	"strings"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
)

func (c Config) ResolveTargets(appEnv string, tokenLookup func(string) string, timeout time.Duration) ([]*forwarder.Target, error) {
	targets := make([]*forwarder.Target, 0, len(c.Targets))
	for _, t := range c.Targets {
		token := tokenLookup(t.TokenEnv)
		if appEnv != "dev" && appEnv != "local" {
			if token == "" {
				return nil, fmt.Errorf("topology: target %q: env var %s is required outside dev and local", t.Name, t.TokenEnv)
			}
			if !strings.HasPrefix(t.Addr, "https://") {
				return nil, fmt.Errorf("topology: target %q: addr must be https outside dev and local", t.Name)
			}
		}
		targets = append(targets, forwarder.NewTarget(t.Name, t.Addr, t.PathPrefix, token, timeout))
	}
	return targets, nil
}

func (c Config) TargetNames() []string {
	names := make([]string, len(c.Targets))
	for i, t := range c.Targets {
		names[i] = t.Name
	}
	return names
}
