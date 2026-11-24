package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/config"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/topology"
)

func depsConfig(topologyPath string) *config.Config {
	return &config.Config{
		AppEnv: "dev", AWSRegion: "us-east-1", AWSEndpointURL: "http://localhost:4566", TopologyPath: topologyPath,
		ConsumerConcurrency: 2, VisibilityTimeout: 30 * time.Second, DrainTimeout: time.Second, ForwardTimeout: 5 * time.Second,
		HandlerTimeout: 20 * time.Second, RetryBackoffBase: 30 * time.Second, RetryBackoffMax: 5 * time.Minute, QueueDepthInterval: 30 * time.Second, MaxReceiveCount: 4,
	}
}

func TestBuildDeps_OneConsumerPerQueueOfTheTopology(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	topo, err := topology.Load("../../config/topology.example.yaml")
	require.NoError(t, err)

	d, err := buildDeps(depsConfig("../../config/topology.example.yaml"), nopLogger{})

	require.NoError(t, err)
	assert.Len(t, d.consumers, len(topo.Queues))
}

func TestBuildDeps_FailsOnAMissingTopology(t *testing.T) {
	_, err := buildDeps(depsConfig("../../config/does-not-exist.yaml"), nopLogger{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "load topology")
}

func TestBuildDeps_NeedsNoTargetSecretOutsideDev(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	cfg := depsConfig("../../config/topology.example.yaml")
	cfg.AppEnv = "prod"

	d, err := buildDeps(cfg, nopLogger{})

	require.NoError(t, err)
	assert.NotEmpty(t, d.consumers, "a production topology builds with no secret in the environment")
}

func writeTopology(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "topology.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestBuildDeps_RefusesATargetAddressThatIsNotSecureOutsideDev(t *testing.T) {
	path := writeTopology(t, "targets:\n  - {name: a, addr: http://plain-host}\nqueues:\n  - {name: q, url: http://q}\n")
	cfg := depsConfig(path)
	cfg.AppEnv = "prod"

	_, err := buildDeps(cfg, nopLogger{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve targets")
}

func TestBuildDeps_NamesTheQueueWhoseConsumerCannotBeBuilt(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	cfg := depsConfig("../../config/topology.example.yaml")
	cfg.MaxReceiveCount = 0

	_, err := buildDeps(cfg, nopLogger{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "build membership consumer")
}

func TestBuildDeps_AFailureToCreateTheDeadLetterPublisherIsNamed(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	cfg := depsConfig("../../config/topology.example.yaml")
	t.Setenv("AWS_CA_BUNDLE", filepath.Join(t.TempDir(), "missing.pem"))

	_, err := buildDeps(cfg, nopLogger{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dead-letter publisher")
}
