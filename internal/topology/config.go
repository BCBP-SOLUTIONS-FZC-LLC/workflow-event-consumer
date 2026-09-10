// Package topology defines and loads the queue/target/routing registry this
// service reads at startup, mirroring workflow-connectors' aliasconfig
// package: a single YAML file is the source of truth for what queues exist,
// what downstream services exist, and which event types route to which
// service — adding any of the three is a config edit, not a Go change.
package topology

// Config is the full topology: every SQS queue this bridge consumes from,
// every downstream HTTP target it can forward to, and the event-type-to-
// target routing table.
type Config struct {
	Version int                 `yaml:"version"`
	Targets []Target            `yaml:"targets"`
	Queues  []Queue             `yaml:"queues"`
	Routes  map[string][]string `yaml:"routes"`
}

// Target is one downstream service's internal-events endpoint. Addr is not a
// secret and lives directly in the file; TokenEnv only names the environment
// variable holding that service's own INTERNAL_API_TOKEN — the token value
// itself is never stored here. PathPrefix is that service's real registered
// base path (e.g. "/api/v1/internal/events") — real, per-target config
// rather than a value assumed identical for every target; it defaults to
// "/api/v1/internal/events" when empty, since every target that exists today
// serves that same versioned path.
type Target struct {
	Name       string `yaml:"name"`
	Addr       string `yaml:"addr"`
	TokenEnv   string `yaml:"tokenEnv"`
	PathPrefix string `yaml:"pathPrefix,omitempty"`
}

// Queue is one SQS queue this bridge polls.
type Queue struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}
