package topology

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() Config {
	return Config{
		Targets: []Target{
			{Name: "execution", Addr: "https://execution.internal"},
			{Name: "definition", Addr: "https://definition.internal"},
		},
		Queues: []Queue{{Name: "membership", URL: "https://sqs/membership"}},
	}
}

func TestResolveTargets_Success(t *testing.T) {
	targets, err := testConfig().ResolveTargets("dev", time.Second)
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, "execution", targets[0].Name)
	assert.Equal(t, "definition", targets[1].Name)
}

func TestResolveTargets_NeedsNoSecretInAnyEnvironment(t *testing.T) {
	for _, env := range []string{"dev", "local", "test", "staging", "prod"} {
		targets, err := testConfig().ResolveTargets(env, time.Second)
		require.NoError(t, err, env)
		assert.Len(t, targets, 2, env)
	}
}

func TestTargetNames(t *testing.T) {
	assert.Equal(t, []string{"execution", "definition"}, testConfig().TargetNames())
}

func TestResolveTargets_OutsideDevAndLocalTakesHTTPSOrHTTPWithAPort(t *testing.T) {
	cases := []struct {
		addr    string
		wantErr bool
	}{
		{"https://execution.internal", false},
		{"https://execution.internal:8443", false},
		{"http://workflow-execution-service-api:8080", false},
		{"http://execution.internal", true},
		{"ftp://execution.internal:21", true},
		{"execution.internal:8080", true},
		{"", true},
	}
	for _, tc := range cases {
		cfg := testConfig()
		cfg.Targets[0].Addr = tc.addr
		for _, env := range []string{"test", "staging", "prod"} {
			_, err := cfg.ResolveTargets(env, time.Second)
			if tc.wantErr {
				require.Error(t, err, "%q in %s", tc.addr, env)
				assert.Contains(t, err.Error(), "addr", env)
			} else {
				assert.NoError(t, err, "%q in %s", tc.addr, env)
			}
		}
	}
	cfg := testConfig()
	cfg.Targets[0].Addr = "http://execution.internal"
	for _, env := range []string{"dev", "local"} {
		_, err := cfg.ResolveTargets(env, time.Second)
		assert.NoError(t, err, env)
	}
}
