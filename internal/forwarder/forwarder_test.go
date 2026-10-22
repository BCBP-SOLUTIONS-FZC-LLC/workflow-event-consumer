package forwarder

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability/obstest"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/mocks"
)

func TestMain(m *testing.M) {
	obstest.Setup()
	os.Exit(m.Run())
}

func TestForward_BothOK(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	b := mocks.NewDownstream()
	defer b.Close()

	fwd := New(NewTarget("a", a.URL(), "", "token-a", time.Second), NewTarget("b", b.URL(), "", "token-b", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{"id":"1"}`))
	require.NoError(t, err)
	require.Equal(t, 1, a.Count())
	require.Equal(t, 1, b.Count())
	assert.Equal(t, "1", a.Received()[0].ID)
	assert.Equal(t, "1", b.Received()[0].ID)
	assert.Equal(t, "token-a", a.LastToken())
	assert.Equal(t, "token-b", b.LastToken())
}

func TestForward_OnlyNamedTargetCalled(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	b := mocks.NewDownstream()
	defer b.Close()

	fwd := New(NewTarget("a", a.URL(), "", "token-a", time.Second), NewTarget("b", b.URL(), "", "token-b", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a"}, "test.event", []byte(`{"id":"1"}`))
	require.NoError(t, err)
	assert.Equal(t, 1, a.Count())
	assert.Equal(t, 0, b.Count(), "target b should never have been called")
}

func TestForward_OneFails(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	b := mocks.NewDownstream()
	defer b.Close()
	b.SetStatus(http.StatusInternalServerError)

	fwd := New(NewTarget("a", a.URL(), "", "token-a", time.Second), NewTarget("b", b.URL(), "", "token-b", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{"id":"1"}`))
	assert.Error(t, err)
}

func TestForward_OneBadRequest(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	b := mocks.NewDownstream()
	defer b.Close()
	b.SetStatus(http.StatusBadRequest)

	fwd := New(NewTarget("a", a.URL(), "", "token-a", time.Second), NewTarget("b", b.URL(), "", "token-b", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{"id":"1"}`))
	assert.Error(t, err)
}

func TestForward_UnknownTarget(t *testing.T) {
	fwd := New(NewTarget("a", "http://unused", "", "token", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"nonexistent"}, "test.event", []byte(`{}`))
	assert.Error(t, err)
}

func TestForward_MalformedTargetURL(t *testing.T) {
	fwd := New(NewTarget("bad", "http://example.com\x7f", "", "token", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"bad"}, "test.event", []byte(`{}`))
	assert.Error(t, err)
}

func TestForward_TimeoutEnforced(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	fwd := New(NewTarget("slow", slow.URL, "", "token", 20*time.Millisecond))

	err := fwd.Forward(context.Background(), "membership", []string{"slow"}, "test.event", []byte(`{}`))
	assert.Error(t, err)
}

func TestForward_Concurrent(t *testing.T) {
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	handler := func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		<-release
		w.WriteHeader(http.StatusOK)
	}
	a := httptest.NewServer(http.HandlerFunc(handler))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(handler))
	defer b.Close()
	fwd := New(NewTarget("a", a.URL, "", "t", 5*time.Second), NewTarget("b", b.URL, "", "t", 5*time.Second))

	done := make(chan error, 1)
	go func() {
		done <- fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{}`))
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-arrived:
		case <-time.After(3 * time.Second):
			close(release)
			t.Fatal("both targets must be in flight at the same time")
		}
	}
	close(release)
	require.NoError(t, <-done)
}

func TestPermanent_WrappedAndTransportErrors(t *testing.T) {
	permanent := &StatusError{Target: "a", Status: http.StatusBadRequest}
	transient := &StatusError{Target: "b", Status: http.StatusServiceUnavailable}

	assert.True(t, Permanent(fmt.Errorf("wrapped: %w", permanent)))
	assert.True(t, Permanent(errors.Join(errors.Join(permanent, permanent), permanent)))
	assert.False(t, Permanent(errors.Join(errors.Join(permanent, transient), permanent)))
	assert.False(t, Permanent(errors.New("a: connection refused")), "a transport error is transient")
}

func TestForward_MappedTypeUsesCategoryPath(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()

	fwd := New(NewTarget("execution", a.URL(), "", "token", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"execution"}, "DelegationStarted", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/internal/events/delegation", a.LastPath())
}

func TestForward_UnmappedTypeFallsBackToLegacyPath(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()

	fwd := New(NewTarget("definition", a.URL(), "", "token", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"definition"}, "DepartmentMembershipRevoked", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/internal/events", a.LastPath())
}

// TestForward_MappedType_OnlyExecutionGetsCategoryPath is a regression test:
// WorkflowTaskCreated routes to both execution and workflow-stream-gateway
// (config/topology.example.yaml), but only execution registers a
// /events/workflow-task subpath — workflow-stream-gateway only ever serves
// its one bare endpoint. categoryFor must not be applied target-agnostically.
func TestForward_MappedType_OnlyExecutionGetsCategoryPath(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	gateway := mocks.NewDownstream()
	defer gateway.Close()

	fwd := New(
		NewTarget("execution", exec.URL(), "", "exec-token", time.Second),
		NewTarget("workflow-stream-gateway", gateway.URL(), "", "gw-token", time.Second),
	)

	err := fwd.Forward(context.Background(), "dashboard", []string{"workflow-stream-gateway", "execution"}, "WorkflowTaskCreated", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/internal/events/workflow-task", exec.LastPath())
	assert.Equal(t, "/api/v1/internal/events", gateway.LastPath())
}

func TestNames(t *testing.T) {
	fwd := New(NewTarget("a", "http://unused-a", "", "t", time.Second), NewTarget("b", "http://unused-b", "", "t", time.Second))
	names := fwd.Names()
	sort.Strings(names)
	assert.Equal(t, []string{"a", "b"}, names)
}

func TestForward_RecordsTheForwardDuration(t *testing.T) {
	const name = "workflow_events_consumer_forward_duration_seconds"
	a := mocks.NewDownstream()
	defer a.Close()
	fwd := New(NewTarget("a", a.URL(), "", "token-a", time.Second))
	before := obstest.Sum(t, name, map[string]string{"queue": "membership", "target": "a"})

	require.NoError(t, fwd.Forward(context.Background(), "membership", []string{"a"}, "test.event", []byte(`{"id":"1"}`)))

	assert.Equal(t, before+1, obstest.Sum(t, name, map[string]string{"queue": "membership", "target": "a"}))
}

func TestForward_PropagatesTraceContext(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "none")
	shutdown, err := gincommon.InitTracingWithConfig(gincommon.TracingConfig{
		ServiceName: "workflow-events-consumer", BuildVersion: "test", Domain: "workflow", Environment: "test",
	})
	require.NoError(t, err)
	t.Cleanup(shutdown)
	ctx, span := gincommon.NewTracer("test").Start(context.Background(), "caller")
	defer span.End()

	var traceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("Traceparent")
	}))
	defer srv.Close()
	fwd := New(NewTarget("a", srv.URL, "", "token-a", time.Second))

	require.NoError(t, fwd.Forward(ctx, "membership", []string{"a"}, "test.event", []byte(`{"id":"1"}`)))

	assert.NotEmpty(t, traceparent, "the forwarded request carries the trace context")
}

func TestForward_FailureClassification(t *testing.T) {
	tests := []struct {
		status    int
		permanent bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusRequestEntityTooLarge, true},
		{http.StatusUnprocessableEntity, true},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
		{http.StatusMethodNotAllowed, false},
		{http.StatusRequestTimeout, false},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
		{http.StatusServiceUnavailable, false},
		{http.StatusMovedPermanently, false},
	}
	for _, tc := range tests {
		d := mocks.NewDownstream()
		d.SetStatus(tc.status)
		fwd := New(NewTarget("a", d.URL(), "", "t", time.Second))

		err := fwd.Forward(context.Background(), "q", []string{"a"}, "Evt", []byte(`{"id":"1"}`))

		require.Error(t, err, "status %d", tc.status)
		assert.Equal(t, tc.permanent, Permanent(err), "status %d", tc.status)
		d.Close()
	}
}

func TestForward_PermanentOnlyWhenEveryFailureIsPermanent(t *testing.T) {
	bad := mocks.NewDownstream()
	bad.SetStatus(http.StatusBadRequest)
	defer bad.Close()
	down := mocks.NewDownstream()
	down.SetStatus(http.StatusServiceUnavailable)
	defer down.Close()
	ok := mocks.NewDownstream()
	defer ok.Close()

	fwd := New(
		NewTarget("bad", bad.URL(), "", "t", time.Second),
		NewTarget("down", down.URL(), "", "t", time.Second),
		NewTarget("ok", ok.URL(), "", "t", time.Second),
	)

	both := fwd.Forward(context.Background(), "q", []string{"bad", "down"}, "Evt", []byte(`{}`))
	assert.False(t, Permanent(both), "a transient failure keeps the message retryable")

	permanentAndOK := fwd.Forward(context.Background(), "q", []string{"bad", "ok"}, "Evt", []byte(`{}`))
	assert.True(t, Permanent(permanentAndOK))

	assert.False(t, Permanent(nil))
}

func TestForward_ExecutionIsSkippedForATypeItHasNoRouteFor(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()
	fwd := New(NewTarget("execution", exec.URL(), "", "t", time.Second), NewTarget("definition", def.URL(), "", "t", time.Second))

	err := fwd.Forward(context.Background(), "q", []string{"execution", "definition"}, "SomeFutureEventType", []byte(`{"id":"1"}`))

	require.NoError(t, err)
	assert.Equal(t, 0, exec.Count(), "execution registers a route per event category only")
	assert.Equal(t, 1, def.Count())
}

func TestForward_ExecutionReceivesACategorisedType(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	fwd := New(NewTarget("execution", exec.URL(), "", "t", time.Second))

	require.NoError(t, fwd.Forward(context.Background(), "q", []string{"execution"}, "UserDeleted", []byte(`{"id":"1"}`)))

	assert.Equal(t, 1, exec.Count())
	assert.Equal(t, "/api/v1/internal/events/user-profile", exec.LastPath())
}

func TestForward_DoesNotFollowARedirectWithTheToken(t *testing.T) {
	elsewhere := mocks.NewDownstream()
	defer elsewhere.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL()+"/stolen", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	fwd := New(NewTarget("a", redirector.URL, "", "secret-token", time.Second))

	err := fwd.Forward(context.Background(), "q", []string{"a"}, "Evt", []byte(`{"id":"1"}`))

	require.Error(t, err)
	assert.False(t, Permanent(err), "a redirect is a deployment fault that a later attempt can fix")
	assert.Equal(t, 0, elsewhere.Count(), "the body and token are not re-sent to the redirect target")
}

func TestForward_AnUnknownTargetFailsBeforeAnyTargetIsCalled(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	fwd := New(NewTarget("a", a.URL(), "", "t", time.Second))

	err := fwd.Forward(context.Background(), "q", []string{"a", "ghost"}, "Evt", []byte(`{"id":"1"}`))

	require.Error(t, err)
	assert.Equal(t, 0, a.Count(), "a delivered event would be reported failed and redelivered")
}

func TestCategorised(t *testing.T) {
	assert.True(t, Categorised("UserDeleted"))
	assert.False(t, Categorised("SomeFutureEventType"))
}

func TestForward_ReusesTheConnectionAfterAnAnswerWithABody(t *testing.T) {
	var connections atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","detail":"` + strings.Repeat("x", 512) + `"}`))
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()
	fwd := New(NewTarget("a", srv.URL, "", "token", 5*time.Second))

	for i := 0; i < 3; i++ {
		require.NoError(t, fwd.Forward(context.Background(), "membership", []string{"a"}, "test.event", []byte(`{}`)))
	}

	assert.EqualValues(t, 1, connections.Load(), "an unread body costs a new connection, and a TLS handshake, for every forward")
}
