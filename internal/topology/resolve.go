package topology

import (
	"errors"
	"fmt"
	"net/url"
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
			if err := checkTargetAddr(t.Addr); err != nil {
				return nil, fmt.Errorf("topology: target %q: addr %w outside dev and local", t.Name, err)
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

func checkTargetAddr(addr string) error {
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return errors.New("must be an http or https URL")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if u.Port() == "" {
			return errors.New("must name a port when it is http")
		}
		return nil
	}
	return errors.New("must be an http or https URL")
}
