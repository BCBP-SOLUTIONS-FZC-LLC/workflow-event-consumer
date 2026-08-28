package forwarder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/mocks"
)

func TestMain(m *testing.M) {
	observability.Register()
	m.Run()
}

func TestForward_BothOK(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	b := mocks.NewDownstream()
	defer b.Close()

	fwd := New(NewTarget("a", a.URL(), "token-a", time.Second), NewTarget("b", b.URL(), "token-b", time.Second))

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

	fwd := New(NewTarget("a", a.URL(), "token-a", time.Second), NewTarget("b", b.URL(), "token-b", time.Second))

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

	fwd := New(NewTarget("a", a.URL(), "token-a", time.Second), NewTarget("b", b.URL(), "token-b", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{"id":"1"}`))
	assert.Error(t, err)
}

func TestForward_OneBadRequest(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()
	b := mocks.NewDownstream()
	defer b.Close()
	b.SetStatus(http.StatusBadRequest)

	fwd := New(NewTarget("a", a.URL(), "token-a", time.Second), NewTarget("b", b.URL(), "token-b", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{"id":"1"}`))
	assert.Error(t, err)
}

func TestForward_UnknownTarget(t *testing.T) {
	fwd := New(NewTarget("a", "http://unused", "token", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"nonexistent"}, "test.event", []byte(`{}`))
	assert.Error(t, err)
}

func TestForward_MalformedTargetURL(t *testing.T) {
	fwd := New(NewTarget("bad", "http://example.com\x7f", "token", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"bad"}, "test.event", []byte(`{}`))
	assert.Error(t, err)
}

func TestForward_TimeoutEnforced(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	fwd := New(NewTarget("slow", slow.URL, "token", 20*time.Millisecond))

	err := fwd.Forward(context.Background(), "membership", []string{"slow"}, "test.event", []byte(`{}`))
	assert.Error(t, err)
}

func TestForward_Concurrent(t *testing.T) {
	var inFlight int32
	var maxInFlight int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			cur := atomic.LoadInt32(&maxInFlight)
			if n <= cur || atomic.CompareAndSwapInt32(&maxInFlight, cur, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}
	a := httptest.NewServer(http.HandlerFunc(handler))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(handler))
	defer b.Close()

	fwd := New(NewTarget("a", a.URL, "t", time.Second), NewTarget("b", b.URL, "t", time.Second))

	var wg sync.WaitGroup
	wg.Add(1)
	start := time.Now()
	go func() {
		defer wg.Done()
		_ = fwd.Forward(context.Background(), "membership", []string{"a", "b"}, "test.event", []byte(`{}`))
	}()
	wg.Wait()
	elapsed := time.Since(start)

	assert.Equal(t, int32(2), maxInFlight, "both targets should be called concurrently")
	assert.Less(t, elapsed, 100*time.Millisecond)
}

func TestForward_MappedTypeUsesCategoryPath(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()

	fwd := New(NewTarget("execution", a.URL(), "token", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"execution"}, "delegation.started", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "/events/delegation", a.LastPath())
}

func TestForward_UnmappedTypeFallsBackToLegacyPath(t *testing.T) {
	a := mocks.NewDownstream()
	defer a.Close()

	fwd := New(NewTarget("definition", a.URL(), "token", time.Second))

	err := fwd.Forward(context.Background(), "membership", []string{"definition"}, "department.membership.revoked", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "/internal/events", a.LastPath())
}

func TestNames(t *testing.T) {
	fwd := New(NewTarget("a", "http://unused-a", "t", time.Second), NewTarget("b", "http://unused-b", "t", time.Second))
	names := fwd.Names()
	sort.Strings(names)
	assert.Equal(t, []string{"a", "b"}, names)
}
