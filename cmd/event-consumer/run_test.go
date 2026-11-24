package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/config"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/consumer"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

type recordingLogger struct {
	mu       sync.Mutex
	warnings []string
	errors   []string
}

func (*recordingLogger) Debug(string, map[string]any) {}
func (*recordingLogger) Info(string, map[string]any)  {}

func (l *recordingLogger) Warn(msg string, _ map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, msg)
}

func (l *recordingLogger) Error(msg string, _ map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, msg)
}

func (l *recordingLogger) hasWarning(fragment string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, w := range l.warnings {
		if bytes.Contains([]byte(w), []byte(fragment)) {
			return true
		}
	}
	return false
}

func loopback(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func listenLoopback(t *testing.T) listenFunc {
	return func(string) (net.Listener, error) { return loopback(t), nil }
}

func serveConfig(port int) *config.Config {
	return &config.Config{
		AppEnv: "test", OTELServiceName: "workflow-events-consumer", BuildVersion: "test",
		HealthPort: port, ReceiveStallWindow: time.Minute,
	}
}

type flushCounter struct{ n atomic.Int32 }

func (f *flushCounter) flush() { f.n.Add(1) }

func signals(sig ...os.Signal) chan os.Signal {
	ch := make(chan os.Signal, len(sig)+1)
	for _, s := range sig {
		ch <- s
	}
	return ch
}

func TestServe_ASignalStopsTheConsumersAndExitsCleanly(t *testing.T) {
	c := newFakeConsumer(nil)
	flush := &flushCounter{}
	log := &recordingLogger{}

	code := serve(serveConfig(0), log, []consumerLike{c}, loopback(t), signals(syscall.SIGTERM), time.Minute, flush.flush)

	assert.Equal(t, 0, code)
	assert.True(t, c.stopped.Load(), "the consumer is asked to stop")
	assert.Equal(t, int32(1), flush.n.Load(), "telemetry is flushed once")
	assert.Empty(t, log.errors)
}

func TestServe_AFailedConsumerExitsOneAfterTheSameDrain(t *testing.T) {
	failing := newFakeConsumer(errors.New("receive denied"))
	flush := &flushCounter{}
	log := &recordingLogger{}

	code := serve(serveConfig(0), log, []consumerLike{failing}, loopback(t), signals(), time.Minute, flush.flush)

	assert.Equal(t, 1, code, "a stopped consumer restarts the pod")
	assert.Equal(t, int32(1), flush.n.Load())
	assert.Contains(t, log.errors, "event-consumer: a consumer stopped, exiting so the pod restarts")
}

func TestServe_AHealthServerThatStopsServingExitsOne(t *testing.T) {
	closed := loopback(t)
	require.NoError(t, closed.Close())
	flush := &flushCounter{}
	log := &recordingLogger{}

	code := serve(serveConfig(0), log, []consumerLike{newFakeConsumer(nil)}, closed, signals(), time.Minute, flush.flush)

	assert.Equal(t, 1, code)
	assert.Contains(t, log.errors, "health server exited")
	assert.Equal(t, int32(1), flush.n.Load())
}

func TestServe_AConsumerThatNeverStopsIsWarnedAboutAndTelemetryIsStillFlushed(t *testing.T) {
	flush := &flushCounter{}
	log := &recordingLogger{}

	code := serve(serveConfig(0), log, []consumerLike{stuckConsumer{}}, loopback(t), signals(syscall.SIGINT), 50*time.Millisecond, flush.flush)

	assert.Equal(t, 0, code)
	assert.True(t, log.hasWarning("consumers still running after the grace period"))
	assert.Equal(t, int32(1), flush.n.Load(), "a stuck consumer cannot keep telemetry from being flushed")
}

func noTelemetry(flush *flushCounter) telemetryStarter {
	return func(gincommon.Config, observability.Logger) (func(), error) { return flush.flush, nil }
}

func TestRun_ABadConfigurationExitsOneAndSaysSo(t *testing.T) {
	t.Setenv("APP_ENV", "")
	var stderr bytes.Buffer

	code := run(&stderr, signals(), noTelemetry(&flushCounter{}), buildDeps, listenLoopback(t))

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "config error")
	assert.Contains(t, stderr.String(), "APP_ENV")
}

func validEnv(t *testing.T, topologyPath string) {
	t.Helper()
	t.Setenv("APP_ENV", "test")
	t.Setenv("BUILD_VERSION", "test")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ENDPOINT_URL", "http://127.0.0.1:1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("HEALTH_PORT", "8082")
	t.Setenv("TOPOLOGY_CONFIG_PATH", topologyPath)
}

func TestRun_ATelemetryFailureExitsOne(t *testing.T) {
	validEnv(t, "../../config/topology.example.yaml")
	failing := func(gincommon.Config, observability.Logger) (func(), error) { return nil, errors.New("tracing down") }

	code := run(&bytes.Buffer{}, signals(), failing, buildDeps, listenLoopback(t))

	assert.Equal(t, 1, code)
}

func TestRun_AMissingTopologyFlushesTelemetryAndExitsOne(t *testing.T) {
	validEnv(t, "../../config/does-not-exist.yaml")
	flush := &flushCounter{}

	code := run(&bytes.Buffer{}, signals(), noTelemetry(flush), buildDeps, listenLoopback(t))

	assert.Equal(t, 1, code)
	assert.Equal(t, int32(1), flush.n.Load(), "telemetry is flushed before a start-up failure exits")
}

func TestRun_AValidStartServesTheBuiltConsumersUntilASignal(t *testing.T) {
	validEnv(t, "../../config/topology.example.yaml")
	flush := &flushCounter{}
	c := newFakeConsumer(nil)
	build := func(*config.Config, consumer.Logger) (*deps, error) {
		return &deps{consumers: []events.Consumer{c}}, nil
	}

	code := run(&bytes.Buffer{}, signals(syscall.SIGTERM), noTelemetry(flush), build, listenLoopback(t))

	assert.Equal(t, 0, code)
	assert.True(t, c.stopped.Load(), "the built consumer is served and then stopped")
	assert.Equal(t, int32(1), flush.n.Load(), "telemetry is flushed once on the way out")
}

func TestListenTCP_BindsOnceAndNamesTheAddressWhenTaken(t *testing.T) {
	first, err := listenTCP("127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	addr := first.Addr().String()

	_, err = listenTCP(addr)

	require.Error(t, err)
	assert.Contains(t, err.Error(), addr)
}

func TestRun_AHealthPortThatCannotBeBoundFlushesTelemetryAndExitsOne(t *testing.T) {
	validEnv(t, "../../config/topology.example.yaml")
	flush := &flushCounter{}
	c := newFakeConsumer(nil)
	build := func(*config.Config, consumer.Logger) (*deps, error) {
		return &deps{consumers: []events.Consumer{c}}, nil
	}
	taken := func(string) (net.Listener, error) { return nil, errors.New("address already in use") }

	code := run(&bytes.Buffer{}, signals(), noTelemetry(flush), build, taken)

	assert.Equal(t, 1, code)
	assert.False(t, c.stopped.Load(), "no consumer was started, so none is stopped")
	assert.Equal(t, int32(1), flush.n.Load(), "telemetry is flushed before the exit")
}
