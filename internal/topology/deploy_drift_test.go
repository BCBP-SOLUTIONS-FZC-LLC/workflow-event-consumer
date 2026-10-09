package topology

import (
	"encoding/json"
	"os"
	"path"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestChartTopologyMatchesTheExample(t *testing.T) {
	example, err := Load("../../config/topology.example.yaml")
	require.NoError(t, err)

	raw, err := os.ReadFile("../../deploy/helm/values.yaml")
	require.NoError(t, err)
	var values struct {
		Topology Config `yaml:"topology"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &values))

	assert.Equal(t, example.TargetNames(), values.Topology.TargetNames(), "chart targets")
	assert.Equal(t, queueNames(example), queueNames(values.Topology), "chart queues")
	assert.Equal(t, example.Routes, values.Topology.Routes, "chart routes")
}

func TestIAMPolicyCoversEveryQueue(t *testing.T) {
	example, err := Load("../../config/topology.example.yaml")
	require.NoError(t, err)

	raw, err := os.ReadFile("../../deploy/aws/policy.json")
	require.NoError(t, err)
	var policy struct {
		Statement []struct {
			Resource []string `json:"Resource"`
		} `json:"Statement"`
	}
	require.NoError(t, json.Unmarshal(raw, &policy))
	var granted []string
	for _, st := range policy.Statement {
		for _, arn := range st.Resource {
			granted = append(granted, arn[len("arn:aws:sqs:*:*:"):])
		}
	}
	sort.Strings(granted)

	var wanted []string
	for _, q := range example.Queues {
		wanted = append(wanted, path.Base(q.URL))
	}
	sort.Strings(wanted)
	assert.Equal(t, wanted, granted, "deploy/aws/policy.json must list exactly the queues of the topology")
}

func queueNames(c Config) []string {
	names := make([]string, len(c.Queues))
	for i, q := range c.Queues {
		names[i] = q.Name + "=" + q.URL
	}
	return names
}
