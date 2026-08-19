package topology

import (
	"fmt"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
)

// ResolveTargets builds one forwarder.Target per declared target, resolving
// each TokenEnv via tokenLookup (os.Getenv in production, a fake map in
// tests). A target whose token resolves empty is only an error when
// appEnv == "prod" — local/dev runs may point at services with auth
// disabled entirely.
func (c Config) ResolveTargets(appEnv string, tokenLookup func(string) string, timeout time.Duration) ([]*forwarder.Target, error) {
	targets := make([]*forwarder.Target, 0, len(c.Targets))
	for _, t := range c.Targets {
		token := tokenLookup(t.TokenEnv)
		if appEnv == "prod" && token == "" {
			return nil, fmt.Errorf("topology: target %q: env var %s is required when APP_ENV=prod", t.Name, t.TokenEnv)
		}
		targets = append(targets, forwarder.NewTarget(t.Name, t.Addr, token, timeout))
	}
	return targets, nil
}

// TargetNames returns every declared target name, in declaration order —
// the fallback broadcast set for an event type absent from Routes.
func (c Config) TargetNames() []string {
	names := make([]string, len(c.Targets))
	for i, t := range c.Targets {
		names[i] = t.Name
	}
	return names
}
