package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeConsumer struct {
	startErr error
	stopped  atomic.Bool
	release  chan struct{}
}

func newFakeConsumer(startErr error) *fakeConsumer {
	return &fakeConsumer{startErr: startErr, release: make(chan struct{})}
}

func (f *fakeConsumer) Start(ctx context.Context) error {
	if f.startErr != nil {
		return f.startErr
	}
	select {
	case <-ctx.Done():
	case <-f.release:
	}
	return nil
}

func (f *fakeConsumer) Stop() error {
	f.stopped.Store(true)
	close(f.release)
	return nil
}

type nopLogger struct{}

func (nopLogger) Debug(string, map[string]any) {}
func (nopLogger) Info(string, map[string]any)  {}
func (nopLogger) Warn(string, map[string]any)  {}
func (nopLogger) Error(string, map[string]any) {}

func TestSuperviseConsumers_ReportsAConsumerThatCannotRun(t *testing.T) {
	healthy := newFakeConsumer(nil)
	broken := newFakeConsumer(errors.New("sqs: access denied"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := superviseConsumers(ctx, []consumerLike{healthy, broken}, nopLogger{})

	select {
	case err := <-sup.failed:
		assert.ErrorContains(t, err, "access denied")
	case <-time.After(time.Second):
		t.Fatal("a consumer that exited with an error was not reported")
	}
	cancel()
	require.NoError(t, sup.stop(time.Second))
	assert.True(t, healthy.stopped.Load(), "the surviving consumers are stopped")
}

func TestSuperviseConsumers_ACleanShutdownReportsNothing(t *testing.T) {
	c := newFakeConsumer(nil)
	ctx, cancel := context.WithCancel(context.Background())

	sup := superviseConsumers(ctx, []consumerLike{c}, nopLogger{})
	cancel()

	require.NoError(t, sup.stop(time.Second))
	select {
	case err := <-sup.failed:
		t.Fatalf("unexpected failure: %v", err)
	default:
	}
}

func TestSuperviseConsumers_StopGivesUpAfterTheGracePeriod(t *testing.T) {
	stuck := &stuckConsumer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sup := superviseConsumers(ctx, []consumerLike{stuck}, nopLogger{})
	cancel()

	assert.Error(t, sup.stop(50*time.Millisecond))
}

type stuckConsumer struct{}

func (stuckConsumer) Start(context.Context) error { select {} }
func (stuckConsumer) Stop() error                 { return nil }
