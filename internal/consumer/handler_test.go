package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"
	eventsmock "github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability/obstest"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/mocks"
)

func TestMain(m *testing.M) {
	obstest.Setup()
	os.Exit(m.Run())
}

var testRoutes = map[string][]string{
	"DelegationStarted":           {"execution"},
	"DepartmentMembershipRevoked": {"definition"},
}

func TestMakeHandler_Success(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()

	fwd := forwarder.New(forwarder.NewTarget("execution", exec.URL(), "", "token", time.Second))
	router := NewRouter(testRoutes, []string{"execution"})
	handler := MakeHandler("membership", fwd, router, &eventsmock.DLQPublisher{})

	env := events.Envelope[json.RawMessage]{ID: "evt-1", Type: "DelegationStarted", Payload: json.RawMessage(`{"a":1}`)}
	err := handler(context.Background(), env)
	require.NoError(t, err)

	require.Equal(t, 1, exec.Count())
	got := exec.Received()[0]
	assert.Equal(t, "evt-1", got.ID)
	assert.Equal(t, "DelegationStarted", got.Type)
	assert.JSONEq(t, `{"a":1}`, string(got.Payload))
	assert.Equal(t, "token", exec.LastToken())
}

func TestMakeHandler_RoutesToOnlyOneTarget(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()

	fwd := forwarder.New(
		forwarder.NewTarget("execution", exec.URL(), "", "exec-token", time.Second),
		forwarder.NewTarget("definition", def.URL(), "", "def-token", time.Second),
	)
	router := NewRouter(testRoutes, []string{"execution", "definition"})
	handler := MakeHandler("membership", fwd, router, &eventsmock.DLQPublisher{})

	env := events.Envelope[json.RawMessage]{ID: "evt-3", Type: "DepartmentMembershipRevoked", Payload: json.RawMessage(`{}`)}
	err := handler(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, 0, exec.Count(), "execution should not receive a definition-only event")
	assert.Equal(t, 1, def.Count())
}

func TestMakeHandler_UnknownTypeBroadcastsToEveryTargetThatAcceptsIt(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()

	fwd := forwarder.New(
		forwarder.NewTarget("execution", exec.URL(), "", "exec-token", time.Second),
		forwarder.NewTarget("definition", def.URL(), "", "def-token", time.Second),
	)
	router := NewRouter(testRoutes, []string{"execution", "definition"})
	handler := MakeHandler("membership", fwd, router, &eventsmock.DLQPublisher{})

	env := events.Envelope[json.RawMessage]{ID: "evt-4", Type: "SomeFutureEventType", Payload: json.RawMessage(`{}`)}
	err := handler(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, 0, exec.Count(), "execution has no route for an uncategorised type")
	assert.Equal(t, 1, def.Count())
}

func TestMakeHandler_ForwardFails(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	exec.SetStatus(http.StatusInternalServerError)

	fwd := forwarder.New(forwarder.NewTarget("execution", exec.URL(), "", "token", time.Second))
	router := NewRouter(testRoutes, []string{"execution"})
	handler := MakeHandler("membership", fwd, router, &eventsmock.DLQPublisher{})

	env := events.Envelope[json.RawMessage]{ID: "evt-2", Type: "DelegationStarted", Payload: json.RawMessage(`{}`)}
	err := handler(context.Background(), env)
	assert.Error(t, err)
}

func newDeadLetterHarness(t *testing.T, statuses map[string]int) (*eventsmock.Consumer, *eventsmock.DLQPublisher, map[string]*mocks.Downstream) {
	t.Helper()
	downstreams := map[string]*mocks.Downstream{}
	targets := make([]*forwarder.Target, 0, len(statuses))
	names := make([]string, 0, len(statuses))
	for name, status := range statuses {
		d := mocks.NewDownstream()
		d.SetStatus(status)
		t.Cleanup(d.Close)
		downstreams[name] = d
		targets = append(targets, forwarder.NewTarget(name, d.URL(), "", "token", time.Second))
		names = append(names, name)
	}
	dlq := &eventsmock.DLQPublisher{}
	router := NewRouter(map[string][]string{"Evt": names}, names)
	c := &eventsmock.Consumer{}
	c.SetHandler(MakeHandler("membership", forwarder.New(targets...), router, dlq))
	return c, dlq, downstreams
}

func deadLetterEnvelope() events.Envelope[json.RawMessage] {
	return events.Envelope[json.RawMessage]{ID: "evt-9", Type: "Evt", Source: "iam", Payload: json.RawMessage(`{}`)}
}

func TestMakeHandler_PermanentFailureIsDeadLettered(t *testing.T) {
	c, dlq, _ := newDeadLetterHarness(t, map[string]int{"definition": http.StatusBadRequest})

	require.NoError(t, c.Inject(deadLetterEnvelope()))

	sent := dlq.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, eventsmock.MockQueueURL, sent[0].SourceQueueURL)
	assert.Contains(t, sent[0].Reason, "definition")
	assert.Contains(t, sent[0].Reason, "400")
	var original events.Envelope[json.RawMessage]
	require.NoError(t, json.Unmarshal(sent[0].Body, &original))
	assert.Equal(t, "evt-9", original.ID, "the original message is forwarded")
}

func TestMakeHandler_TransientFailureIsRetried(t *testing.T) {
	c, dlq, _ := newDeadLetterHarness(t, map[string]int{"definition": http.StatusServiceUnavailable})

	require.Error(t, c.Inject(deadLetterEnvelope()))
	assert.Empty(t, dlq.Sent())
}

func TestMakeHandler_MixedFailureIsRetried(t *testing.T) {
	c, dlq, _ := newDeadLetterHarness(t, map[string]int{
		"definition": http.StatusBadRequest,
		"gateway":    http.StatusServiceUnavailable,
	})

	require.Error(t, c.Inject(deadLetterEnvelope()))
	assert.Empty(t, dlq.Sent())
}

func TestMakeHandler_FailedDeadLetterKeepsTheMessage(t *testing.T) {
	c, dlq, _ := newDeadLetterHarness(t, map[string]int{"definition": http.StatusBadRequest})
	dlq.SetError(errors.New("sqs unavailable"))

	err := c.Inject(deadLetterEnvelope())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sqs unavailable")
	assert.Contains(t, err.Error(), "definition")
}

func TestMakeHandler_ForwardsTheOriginalBodyOfASchemaEncodedEvent(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	fwd := forwarder.New(forwarder.NewTarget("execution", exec.URL(), "", "token", time.Second))
	c := &eventsmock.Consumer{Codec: events.NoopCodec{}}
	c.SetHandler(MakeHandler("membership", fwd, NewRouter(testRoutes, []string{"execution"}), &eventsmock.DLQPublisher{}))

	env := events.Envelope[json.RawMessage]{ID: "evt-7", Type: "DelegationStarted", Source: "iam", SchemaID: "schema-1", Payload: json.RawMessage(`"AAEC"`)}
	require.NoError(t, c.Inject(env))

	require.Equal(t, 1, exec.Count())
	got := exec.Received()[0]
	assert.Equal(t, "schema-1", got.SchemaID)
	assert.JSONEq(t, `"AAEC"`, string(got.Payload), "the encoded payload reaches the target for its own decoder")
}
