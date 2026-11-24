package forwarder

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/obstest"
)

func TestForward_ACallCancelledWhileTheTargetHoldsItIsATransientTransportFailure(t *testing.T) {
	const metric = "workflow_events_consumer_forward_total"
	arrived := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(arrived)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	labels := map[string]string{"queue": "cancel-q", "target": "a", "result": "error", "status_class": "transport"}
	before := obstest.Sum(t, metric, labels)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-arrived
		cancel()
	}()

	err := New(NewTarget("a", srv.URL, "", time.Minute)).Forward(ctx, "cancel-q", []string{"a"}, "Evt", []byte(`{"id":"1"}`))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, Permanent(err), "a cancelled call is retried, never dead-lettered")
	assert.Equal(t, before+1, obstest.Sum(t, metric, labels))
}

func TestForward_AnAnswerBodyIsDrainedOnlyUpToTheLimit(t *testing.T) {
	var newConns atomic.Int32
	size := atomic.Int64{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", int(size.Load()))))
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	fwd := New(NewTarget("a", srv.URL, "", time.Minute))
	send := func() {
		t.Helper()
		require.NoError(t, fwd.Forward(context.Background(), "drain-q", []string{"a"}, "Evt", []byte(`{}`)))
	}

	size.Store(1 << 10)
	send()
	send()
	assert.Equal(t, int32(1), newConns.Load(), "a small answer is read to the end and the connection is reused")

	size.Store(4 * maxDrainedResponseBytes)
	send()
	send()
	assert.Equal(t, int32(2), newConns.Load(), "an answer beyond the drain limit is cut off, so its connection is closed and the next call opens a new one")
}

func TestForward_EveryFailureOfConcurrentTargetsIsReturned(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	t.Cleanup(bad.Close)
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	t.Cleanup(down.Close)
	fwd := New(NewTarget("definition", bad.URL, "", time.Minute), NewTarget("gateway", down.URL, "", time.Minute))

	err := fwd.Forward(context.Background(), "join-q", []string{"definition", "gateway"}, "Evt", []byte(`{}`))

	var statuses []int
	for _, e := range unwrapAll(err) {
		var se *StatusError
		if errors.As(e, &se) {
			statuses = append(statuses, se.Status)
		}
	}
	assert.ElementsMatch(t, []int{http.StatusBadRequest, http.StatusServiceUnavailable}, statuses)
}

func unwrapAll(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		return j.Unwrap()
	}
	return []error{err}
}
