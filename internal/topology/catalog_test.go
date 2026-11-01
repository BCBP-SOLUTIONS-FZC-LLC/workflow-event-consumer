package topology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionEventCatalogIsRoutedInFull(t *testing.T) {
	example, err := Load("../../config/topology.example.yaml")
	require.NoError(t, err)

	gateway := []string{"workflow-stream-gateway"}
	want := map[string][]string{
		"WorkflowTaskCreated":         {"workflow-stream-gateway", "execution"},
		"WorkflowTaskClaimed":         gateway,
		"WorkflowTaskCompleted":       gateway,
		"WorkflowTaskDeferred":        gateway,
		"WorkflowTaskReassigned":      gateway,
		"WorkflowTaskSuperseded":      gateway,
		"WorkflowTaskFailed":          gateway,
		"WorkflowTaskSlaWarning":      gateway,
		"WorkflowTaskSlaBreached":     gateway,
		"WorkflowInstancePaused":      gateway,
		"WorkflowInstanceResumed":     gateway,
		"WorkflowInstanceCancelled":   gateway,
		"WorkflowInstanceTerminated":  gateway,
		"WorkflowInstanceDegraded":    gateway,
		"WorkflowInstanceFailed":      gateway,
		"WorkflowInstanceFinished":    gateway,
		"WorkflowInstanceStarted":     {},
		"WorkflowInstanceForceRouted": {},
	}
	for eventType, targets := range want {
		got, ok := example.Routes[eventType]
		assert.True(t, ok, "%s has an explicit route", eventType)
		if targets == nil {
			targets = []string{}
		}
		if got == nil {
			got = []string{}
		}
		assert.Equal(t, targets, got, "targets of %s", eventType)
	}
}
