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
			{Name: "execution", Addr: "https://execution.internal", TokenEnv: "EXECUTION_SERVICE_TOKEN"},
			{Name: "definition", Addr: "https://definition.internal", TokenEnv: "DEFINITION_SERVICE_TOKEN"},
		},
		Queues: []Queue{{Name: "membership", URL: "https://sqs/membership"}},
	}
}

func TestResolveTargets_Success(t *testing.T) {
	tokens := map[string]string{"EXECUTION_SERVICE_TOKEN": "exec-tok", "DEFINITION_SERVICE_TOKEN": "def-tok"}
	targets, err := testConfig().ResolveTargets("dev", func(k string) string { return tokens[k] }, time.Second)
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, "execution", targets[0].Name)
	assert.Equal(t, "definition", targets[1].Name)
}

func TestResolveTargets_DevAllowsMissingToken(t *testing.T) {
	targets, err := testConfig().ResolveTargets("dev", func(string) string { return "" }, time.Second)
	require.NoError(t, err)
	assert.Len(t, targets, 2)
}

func TestResolveTargets_RequiresATokenOutsideDevAndLocal(t *testing.T) {
	for _, env := range []string{"test", "staging", "prod"} {
		_, err := testConfig().ResolveTargets(env, func(string) string { return "" }, time.Second)
		assert.Error(t, err, env)
	}
	for _, env := range []string{"dev", "local"} {
		_, err := testConfig().ResolveTargets(env, func(string) string { return "" }, time.Second)
		assert.NoError(t, err, env)
	}
}

func TestTargetNames(t *testing.T) {
	assert.Equal(t, []string{"execution", "definition"}, testConfig().TargetNames())
}

func TestResolveTargets_OutsideDevAndLocalTakesHTTPSOrHTTPWithAPort(t *testing.T) {
	token := func(string) string { return "tok" }
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
			_, err := cfg.ResolveTargets(env, token, time.Second)
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
		_, err := cfg.ResolveTargets(env, token, time.Second)
		assert.NoError(t, err, env)
	}
}
