package topology

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "topology.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

const validYAML = `
version: 1
targets:
  - name: execution
    addr: http://execution.internal
    tokenEnv: EXECUTION_SERVICE_TOKEN
  - name: definition
    addr: http://definition.internal
    tokenEnv: DEFINITION_SERVICE_TOKEN
queues:
  - name: membership
    url: https://sqs/membership
  - name: user
    url: https://sqs/user
routes:
  DelegationStarted: [execution]
  DepartmentMembershipRevoked: [definition]
`

func TestLoad_Valid(t *testing.T) {
	path := writeFile(t, validYAML)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Len(t, cfg.Targets, 2)
	assert.Len(t, cfg.Queues, 2)
	assert.Equal(t, []string{"execution"}, cfg.Routes["DelegationStarted"])
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	assert.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeFile(t, "not: [valid: yaml")
	_, err := Load(path)
	assert.Error(t, err)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "no targets",
			yaml:    "version: 1\nqueues: [{name: q, url: u}]\n",
			wantErr: "at least one target",
		},
		{
			name:    "no queues",
			yaml:    "version: 1\ntargets: [{name: t, addr: a, tokenEnv: E}]\n",
			wantErr: "at least one queue",
		},
		{
			name:    "target missing name",
			yaml:    "version: 1\ntargets: [{addr: a, tokenEnv: E}]\nqueues: [{name: q, url: u}]\n",
			wantErr: "target entry missing name",
		},
		{
			name:    "duplicate target name",
			yaml:    "version: 1\ntargets: [{name: t, addr: a, tokenEnv: E}, {name: t, addr: b, tokenEnv: F}]\nqueues: [{name: q, url: u}]\n",
			wantErr: "duplicate name",
		},
		{
			name:    "target missing addr",
			yaml:    "version: 1\ntargets: [{name: t, tokenEnv: E}]\nqueues: [{name: q, url: u}]\n",
			wantErr: "addr is required",
		},
		{
			name:    "target missing tokenEnv",
			yaml:    "version: 1\ntargets: [{name: t, addr: a}]\nqueues: [{name: q, url: u}]\n",
			wantErr: "tokenEnv is required",
		},
		{
			name:    "queue missing name",
			yaml:    "version: 1\ntargets: [{name: t, addr: a, tokenEnv: E}]\nqueues: [{url: u}]\n",
			wantErr: "queue entry missing name",
		},
		{
			name:    "duplicate queue name",
			yaml:    "version: 1\ntargets: [{name: t, addr: a, tokenEnv: E}]\nqueues: [{name: q, url: u}, {name: q, url: v}]\n",
			wantErr: "duplicate name",
		},
		{
			name:    "queue missing url",
			yaml:    "version: 1\ntargets: [{name: t, addr: a, tokenEnv: E}]\nqueues: [{name: q}]\n",
			wantErr: "url is required",
		},
		{
			name:    "route with no targets",
			yaml:    "version: 1\ntargets: [{name: t, addr: a, tokenEnv: E}]\nqueues: [{name: q, url: u}]\nroutes: {SomeType: []}\n",
			wantErr: "at least one target is required",
		},
		{
			name:    "route references undeclared target",
			yaml:    "version: 1\ntargets: [{name: t, addr: a, tokenEnv: E}]\nqueues: [{name: q, url: u}]\nroutes: {SomeType: [nonexistent]}\n",
			wantErr: "undeclared target",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, tt.yaml)
			_, err := Load(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
