package main

import (
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
