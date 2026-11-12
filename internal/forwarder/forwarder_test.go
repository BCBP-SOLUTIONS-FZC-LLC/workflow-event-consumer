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
	"sync"
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

	fwd := New(NewTarget("a", a.URL(), "", time.Second), NewTarget("b", b.URL(), "", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{"id":"1"}`))
	require.NoError(t, err)
	require.Equal(t, 1, a.Count())
	require.Equal(t, 1, b.Count())
	assert.Equal(t, "1", a.Received()[0].ID)
	assert.Equal(t, "1", b.Received()[0].ID)
	assert.Equal(t, "iam-system", a.LastHeaders().Get("x-user-id"))
	assert.Equal(t, "iam-system", b.LastHeaders().Get("x-user-id"))
}

func TestForward_EveryTargetGetsTheSystemCallerIdentityAndNoToken(t *testing.T) {
	for _, name := range []string{"definition", ExecutionTarget, "workflow-stream-gateway"} {
		t.Run(name, func(t *testing.T) {
			d := mocks.NewDownstream()
			defer d.Close()
			fwd := New(NewTarget(name, d.URL(), "", time.Second))

			err := fwd.Forward(context.Background(), "membership", []string{name}, "WorkflowTaskCreated", []byte(`{"id":"1","tenant_id":"t-1"}`))

			require.NoError(t, err)
			require.Equal(t, 1, d.Count())
			h := d.LastHeaders()
			assert.Equal(t, []string{"iam-system"}, h.Values("x-user-id"))
			assert.Equal(t, []string{"iam-system"}, h.Values("x-tenant-roles"))
			assert.Equal(t, "application/json", h.Get("Content-Type"))
			for _, header := range []string{"x-internal-token", "x-tenant-id", "Authorization"} {
				assert.Empty(t, h.Values(header), "%s is not sent: the mesh names the caller, and the body is not parsed for a tenant", header)
			}
			assert.JSONEq(t, `{"id":"1","tenant_id":"t-1"}`, string(d.Bodies()[0]))
		})
	}
}

func TestForward_OnlyNamedTargetCalled(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	b := mocks.NewDownstream()
	defer b.Close()

	fwd := New(NewTarget("a", a.URL(), "", time.Second), NewTarget("b", b.URL(), "", time.Second))

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

	fwd := New(NewTarget("a", a.URL(), "", time.Second), NewTarget("b", b.URL(), "", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{"id":"1"}`))
	assert.Error(t, err)
}

func TestForward_OneBadRequest(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	b := mocks.NewDownstream()
	defer b.Close()
	b.SetStatus(http.StatusBadRequest)

	fwd := New(NewTarget("a", a.URL(), "", time.Second), NewTarget("b", b.URL(), "", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{"id":"1"}`))
	assert.Error(t, err)
}

func TestForward_UnknownTarget(t *testing.T) {
	fwd := New(NewTarget("a", "http://unused", "", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"nonexistent"}, "test.event", []byte(`{}`))
	assert.Error(t, err)
}

func TestForward_MalformedTargetURL(t *testing.T) {
	fwd := New(NewTarget("bad", "http://example.com\x7f", "", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"bad"}, "test.event", []byte(`{}`))
	assert.Error(t, err)
}

func TestForward_TimeoutEnforced(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	fwd := New(NewTarget("slow", slow.URL, "", 20*time.Millisecond))

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
	fwd := New(NewTarget("a", a.URL, "", 5*time.Second), NewTarget("b", b.URL, "", 5*time.Second))

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

	fwd := New(NewTarget("execution", a.URL(), "", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"execution"}, "DelegationStarted", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/internal/events/delegation", a.LastPath())
}

func TestForward_UnmappedTypeFallsBackToLegacyPath(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()

	fwd := New(NewTarget("definition", a.URL(), "", time.Second))

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
		NewTarget("execution", exec.URL(), "", time.Second),
		NewTarget("workflow-stream-gateway", gateway.URL(), "", time.Second),
	)

	err := fwd.Forward(context.Background(), "dashboard", []string{"workflow-stream-gateway", "execution"}, "WorkflowTaskCreated", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/internal/events/workflow-task", exec.LastPath())
	assert.Equal(t, "/api/v1/internal/events", gateway.LastPath())
}

func TestNames(t *testing.T) {
	fwd := New(NewTarget("a", "http://unused-a", "", time.Second), NewTarget("b", "http://unused-b", "", time.Second))
	names := fwd.Names()
	sort.Strings(names)
	assert.Equal(t, []string{"a", "b"}, names)
}

func TestForward_RecordsTheForwardDuration(t *testing.T) {
	const name = "workflow_events_consumer_forward_duration_seconds"
	a := mocks.NewDownstream()
	defer a.Close()
	fwd := New(NewTarget("a", a.URL(), "", time.Second))
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
	fwd := New(NewTarget("a", srv.URL, "", time.Second))

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
		fwd := New(NewTarget("a", d.URL(), "", time.Second))

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
		NewTarget("bad", bad.URL(), "", time.Second),
		NewTarget("down", down.URL(), "", time.Second),
		NewTarget("ok", ok.URL(), "", time.Second),
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
	fwd := New(NewTarget("execution", exec.URL(), "", time.Second), NewTarget("definition", def.URL(), "", time.Second))

	err := fwd.Forward(context.Background(), "q", []string{"execution", "definition"}, "SomeFutureEventType", []byte(`{"id":"1"}`))

	require.NoError(t, err)
	assert.Equal(t, 0, exec.Count(), "execution registers a route per event category only")
	assert.Equal(t, 1, def.Count())
}

func TestForward_ExecutionReceivesACategorisedType(t *testing.T) {
	exec := mocks.NewDownstream()
	defer exec.Close()
	fwd := New(NewTarget("execution", exec.URL(), "", time.Second))

	require.NoError(t, fwd.Forward(context.Background(), "q", []string{"execution"}, "UserDeleted", []byte(`{"id":"1"}`)))

	assert.Equal(t, 1, exec.Count())
	assert.Equal(t, "/api/v1/internal/events/user-profile", exec.LastPath())
}

func TestForward_DoesNotFollowARedirect(t *testing.T) {
	elsewhere := mocks.NewDownstream()
	defer elsewhere.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL()+"/stolen", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	fwd := New(NewTarget("a", redirector.URL, "", time.Second))

	err := fwd.Forward(context.Background(), "q", []string{"a"}, "Evt", []byte(`{"id":"1"}`))

	require.Error(t, err)
	assert.False(t, Permanent(err), "a redirect is a deployment fault that a later attempt can fix")
	assert.Equal(t, 0, elsewhere.Count(), "the body and identity headers are not re-sent to the redirect target")
}

func TestForward_AnUnknownTargetFailsBeforeAnyTargetIsCalled(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	fwd := New(NewTarget("a", a.URL(), "", time.Second))

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
	fwd := New(NewTarget("a", srv.URL, "", 5*time.Second))

	for i := 0; i < 3; i++ {
		require.NoError(t, fwd.Forward(context.Background(), "membership", []string{"a"}, "test.event", []byte(`{}`)))
	}

	assert.EqualValues(t, 1, connections.Load(), "an unread body costs a new connection, and a TLS handshake, for every forward")
}

func TestForward_CountsEachAttemptByStatusClass(t *testing.T) {
	const metric = "workflow_events_consumer_forward_total"
	tests := []struct {
		name   string
		status int
		result string
		class  string
	}{
		{"success", http.StatusOK, "ok", "2xx"},
		{"redirect that is not followed", http.StatusTemporaryRedirect, "error", "3xx"},
		{"malformed request", http.StatusBadRequest, "error", "4xx"},
		{"wrong identity", http.StatusUnauthorized, "error", "4xx"},
		{"policy denial", http.StatusForbidden, "error", "4xx"},
		{"server error", http.StatusServiceUnavailable, "error", "5xx"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := mocks.NewDownstream()
			defer d.Close()
			d.SetStatus(tc.status)
			labels := map[string]string{"queue": "class-" + tc.name, "target": "a", "result": tc.result, "status_class": tc.class}
			before := obstest.Sum(t, metric, labels)
			queueBefore := obstest.Sum(t, metric, map[string]string{"queue": "class-" + tc.name})
			fwd := New(NewTarget("a", d.URL(), "", time.Second))

			_ = fwd.Forward(context.Background(), "class-"+tc.name, []string{"a"}, "Evt", []byte(`{"id":"1"}`))

			assert.Equal(t, before+1, obstest.Sum(t, metric, labels))
			assert.Equal(t, queueBefore+1, obstest.Sum(t, metric, map[string]string{"queue": "class-" + tc.name}), "one attempt is one count, under one class only")
		})
	}
}

func TestForward_ACallThatGetsNoAnswerIsCountedAsTransport(t *testing.T) {
	const metric = "workflow_events_consumer_forward_total"
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := dead.URL
	dead.Close()
	labels := map[string]string{"queue": "transport-q", "target": "a", "result": "error", "status_class": "transport"}
	before := obstest.Sum(t, metric, labels)

	err := New(NewTarget("a", addr, "", time.Second)).Forward(context.Background(), "transport-q", []string{"a"}, "Evt", []byte(`{}`))

	require.Error(t, err)
	assert.Equal(t, before+1, obstest.Sum(t, metric, labels))
}

func TestForward_ASkippedTargetIsCountedWithNoClass(t *testing.T) {
	const metric = "workflow_events_consumer_forward_total"
	labels := map[string]string{"queue": "skipped-q", "target": "execution", "result": "skipped", "status_class": "none"}
	before := obstest.Sum(t, metric, labels)

	err := New(NewTarget("execution", "http://unused", "", time.Second)).Forward(context.Background(), "skipped-q", []string{"execution"}, "SomeFutureEventType", []byte(`{}`))

	require.NoError(t, err)
	assert.Equal(t, before+1, obstest.Sum(t, metric, labels))
}

type loggedError struct {
	msg    string
	fields map[string]interface{}
}

type recordingLogger struct {
	mu     sync.Mutex
	errors []loggedError
}

func (l *recordingLogger) Error(msg string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, loggedError{msg, fields})
}

func (l *recordingLogger) logged() []loggedError {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]loggedError(nil), l.errors...)
}

func TestForward_ALogsAnAuthenticationOrPolicyRefusalAtErrorWithTheTargetAndStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			d := mocks.NewDownstream()
			defer d.Close()
			d.SetStatus(status)
			log := &recordingLogger{}
			fwd := New(NewTarget("workflow-stream-gateway", d.URL(), "", time.Second)).WithLogger(log)

			err := fwd.Forward(context.Background(), "dashboard", []string{"workflow-stream-gateway"}, "WorkflowTaskCreated", []byte(`{"id":"1"}`))

			require.Error(t, err)
			require.Len(t, log.logged(), 1)
			entry := log.logged()[0]
			assert.Contains(t, entry.msg, "refused")
			assert.Equal(t, "workflow-stream-gateway", entry.fields["target"])
			assert.Equal(t, status, entry.fields["status"])
			assert.Equal(t, "dashboard", entry.fields["queue"])
			assert.Equal(t, "WorkflowTaskCreated", entry.fields["event_type"])
		})
	}
}

func TestForward_AnotherFailureIsNotLoggedAsARefusal(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError, http.StatusTemporaryRedirect} {
		d := mocks.NewDownstream()
		d.SetStatus(status)
		log := &recordingLogger{}
		fwd := New(NewTarget("a", d.URL(), "", time.Second)).WithLogger(log)

		require.Error(t, fwd.Forward(context.Background(), "q", []string{"a"}, "Evt", []byte(`{}`)))

		assert.Empty(t, log.logged(), "status %d", status)
		d.Close()
	}
}

func TestForward_AForwarderWithNoLoggerStillReportsARefusal(t *testing.T) {
	d := mocks.NewDownstream()
	defer d.Close()
	d.SetStatus(http.StatusForbidden)

	err := New(NewTarget("a", d.URL(), "", time.Second)).Forward(context.Background(), "q", []string{"a"}, "Evt", []byte(`{}`))

	var se *StatusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusForbidden, se.Status)
}
