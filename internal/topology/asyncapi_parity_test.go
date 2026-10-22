package topology

import (
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type asyncAPI struct {
	Channels map[string]struct {
		Address  string                       `yaml:"address"`
		Messages map[string]map[string]string `yaml:"messages"`
	} `yaml:"channels"`
	Operations map[string]struct {
		Action string `yaml:"action"`
	} `yaml:"operations"`
	Components struct {
		Messages map[string]struct {
			Targets []string `yaml:"x-targets"`
		} `yaml:"messages"`
	} `yaml:"components"`
}

func loadAsyncAPI(t *testing.T) asyncAPI {
	t.Helper()
	raw, err := os.ReadFile("../../api/asyncapi.yaml")
	require.NoError(t, err)
	var spec asyncAPI
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	return spec
}

func TestAsyncAPIChannelsAreTheTopologyQueues(t *testing.T) {
	example, err := Load("../../config/topology.example.yaml")
	require.NoError(t, err)
	spec := loadAsyncAPI(t)

	var wanted, got []string
	for _, q := range example.Queues {
		wanted = append(wanted, path.Base(q.URL))
	}
	for _, ch := range spec.Channels {
		got = append(got, ch.Address)
	}
	sort.Strings(wanted)
	sort.Strings(got)
	assert.Equal(t, wanted, got, "api/asyncapi.yaml must declare one channel per queue of the topology")

	for name, op := range spec.Operations {
		assert.Equal(t, "receive", op.Action, "operation %s: the consumer only receives", name)
	}
}

func TestAsyncAPIMessagesAreTheTopologyRoutes(t *testing.T) {
	example, err := Load("../../config/topology.example.yaml")
	require.NoError(t, err)
	spec := loadAsyncAPI(t)

	channelMessages := map[string]bool{}
	for _, ch := range spec.Channels {
		for name := range ch.Messages {
			channelMessages[name] = true
		}
	}
	var wantedTypes, gotTypes []string
	for eventType := range example.Routes {
		wantedTypes = append(wantedTypes, eventType)
	}
	for name := range channelMessages {
		gotTypes = append(gotTypes, name)
	}
	sort.Strings(wantedTypes)
	sort.Strings(gotTypes)
	assert.Equal(t, wantedTypes, gotTypes, "every event type the channels carry has a route, and every route is carried by a channel")

	for eventType, targets := range example.Routes {
		msg, ok := spec.Components.Messages[eventType]
		require.True(t, ok, "components.messages.%s", eventType)
		want := append([]string{}, targets...)
		got := append([]string{}, msg.Targets...)
		sort.Strings(want)
		sort.Strings(got)
		assert.Equal(t, want, got, "x-targets of %s must equal its route", eventType)
	}
	assert.False(t, strings.Contains(strings.Join(gotTypes, ","), "."), "event types are PascalCase")
}
