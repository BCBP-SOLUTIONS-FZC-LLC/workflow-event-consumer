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
			{Name: "execution", Addr: "http://execution.internal", TokenEnv: "EXECUTION_SERVICE_TOKEN"},
			{Name: "definition", Addr: "http://definition.internal", TokenEnv: "DEFINITION_SERVICE_TOKEN"},
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

func TestResolveTargets_ProdRequiresToken(t *testing.T) {
	_, err := testConfig().ResolveTargets("prod", func(string) string { return "" }, time.Second)
	assert.Error(t, err)
}

func TestTargetNames(t *testing.T) {
	assert.Equal(t, []string{"execution", "definition"}, testConfig().TargetNames())
}
