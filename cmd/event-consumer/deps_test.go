package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/config"
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

	d, err := buildDeps(depsConfig("../../config/topology.example.yaml"), nopLogger{})

	require.NoError(t, err)
	assert.Len(t, d.consumers, 4)
}

func TestBuildDeps_FailsOnAMissingTopology(t *testing.T) {
	_, err := buildDeps(depsConfig("../../config/does-not-exist.yaml"), nopLogger{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "load topology")
}

func TestBuildDeps_RefusesAnEmptyTokenOutsideDev(t *testing.T) {
	cfg := depsConfig("../../config/topology.example.yaml")
	cfg.AppEnv = "prod"
	for _, name := range []string{"EXECUTION_SERVICE_TOKEN", "DEFINITION_SERVICE_TOKEN", "WORKFLOW_STREAM_GATEWAY_TOKEN"} {
		t.Setenv(name, "")
	}

	_, err := buildDeps(cfg, nopLogger{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve targets")
}
