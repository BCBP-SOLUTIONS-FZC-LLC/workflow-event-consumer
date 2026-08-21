package consumer

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/mocks"
)

func TestMain(m *testing.M) {
	observability.Register()
	m.Run()
}

var testRoutes = map[string][]string{
	"delegation.started":            {"execution"},
	"department.membership.revoked": {"definition"},
}

func TestMakeHandler_Success(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()

	fwd := forwarder.New(forwarder.NewTarget("execution", exec.URL(), "token", time.Second))
	router := NewRouter(testRoutes, []string{"execution"})
	handler := MakeHandler("membership", fwd, router)

	env := events.Envelope[json.RawMessage]{ID: "evt-1", Type: "delegation.started", Payload: json.RawMessage(`{"a":1}`)}
	err := handler(context.Background(), env)
	require.NoError(t, err)

	require.Equal(t, 1, exec.Count())
	got := exec.Received()[0]
	assert.Equal(t, "evt-1", got.ID)
	assert.Equal(t, "delegation.started", got.Type)
	assert.JSONEq(t, `{"a":1}`, string(got.Payload))
	assert.Equal(t, "token", exec.LastToken())
}

func TestMakeHandler_RoutesToOnlyOneTarget(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()

	fwd := forwarder.New(
		forwarder.NewTarget("execution", exec.URL(), "exec-token", time.Second),
		forwarder.NewTarget("definition", def.URL(), "def-token", time.Second),
	)
	router := NewRouter(testRoutes, []string{"execution", "definition"})
	handler := MakeHandler("membership", fwd, router)

	env := events.Envelope[json.RawMessage]{ID: "evt-3", Type: "department.membership.revoked", Payload: json.RawMessage(`{}`)}
	err := handler(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, 0, exec.Count(), "execution should not receive a definition-only event")
	assert.Equal(t, 1, def.Count())
}

func TestMakeHandler_UnknownTypeBroadcastsToAll(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()

	fwd := forwarder.New(
		forwarder.NewTarget("execution", exec.URL(), "exec-token", time.Second),
		forwarder.NewTarget("definition", def.URL(), "def-token", time.Second),
	)
	router := NewRouter(testRoutes, []string{"execution", "definition"})
	handler := MakeHandler("membership", fwd, router)

	env := events.Envelope[json.RawMessage]{ID: "evt-4", Type: "SomeFutureEventType", Payload: json.RawMessage(`{}`)}
	err := handler(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, 1, exec.Count())
	assert.Equal(t, 1, def.Count())
}

func TestMakeHandler_ForwardFails(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	exec.SetStatus(http.StatusInternalServerError)

	fwd := forwarder.New(forwarder.NewTarget("execution", exec.URL(), "token", time.Second))
	router := NewRouter(testRoutes, []string{"execution"})
	handler := MakeHandler("membership", fwd, router)

	env := events.Envelope[json.RawMessage]{ID: "evt-2", Type: "delegation.started", Payload: json.RawMessage(`{}`)}
	err := handler(context.Background(), env)
	assert.Error(t, err)
}
