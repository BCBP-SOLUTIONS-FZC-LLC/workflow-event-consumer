package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/gincommon"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/logger"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/config"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

const shutdownGrace = 30 * time.Second

var version = "dev"

type telemetryStarter func(gincommon.Config, observability.Logger) (func(), error)

type listenFunc func(addr string) (net.Listener, error)

type serveLogger interface {
	observability.Logger
	superviseLogger
}

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	os.Exit(run(os.Stderr, sigCh, observability.InitTelemetry, buildDeps, listenTCP))
}

func listenTCP(addr string) (net.Listener, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return ln, nil
}

func run(stderr io.Writer, sigCh <-chan os.Signal, startTelemetry telemetryStarter, build depsBuilder, listen listenFunc) int {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	wlog, err := logger.NewLogger(cfg.AppEnv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "event-consumer: new logger: %v\n", err)
		return 1
	}

	shutdownTelemetry, err := startTelemetry(observability.GinConfig(cfg.OTELServiceName, cfg.BuildVersion, cfg.AppEnv, wlog), wlog)
	if err != nil {
		wlog.Error("telemetry init failed", map[string]any{"error": err.Error()})
		return 1
	}

	wlog.Info("event-consumer starting", map[string]any{"version": version})

	d, err := build(cfg, wlog)
	if err != nil {
		wlog.Error("startup failed", map[string]any{"error": err.Error()})
		shutdownTelemetry()
		return 1
	}

	consumers := make([]consumerLike, len(d.consumers))
	for i, c := range d.consumers {
		consumers[i] = c
	}
	ln, err := listen(fmt.Sprintf(":%d", cfg.HealthPort))
	if err != nil {
		wlog.Error("health server cannot listen", map[string]any{"error": err.Error()})
		shutdownTelemetry()
		return 1
	}
	return serve(cfg, wlog, consumers, ln, sigCh, shutdownGrace, shutdownTelemetry)
}

func serve(cfg *config.Config, wlog serveLogger, consumers []consumerLike, ln net.Listener, sigCh <-chan os.Signal, grace time.Duration, shutdownTelemetry func()) int {
	ctx, cancel := context.WithCancel(context.Background())
	sup := superviseConsumers(ctx, consumers, wlog)

	watch := observability.NewReceiveWatch(gincommon.MetricsGatherer(), cfg.ReceiveStallWindow, time.Now)
	healthSrv := newHealthServer(fmt.Sprintf(":%d", cfg.HealthPort), observability.GinConfig(cfg.OTELServiceName, cfg.BuildVersion, cfg.AppEnv, wlog), watch)
	go func() {
		if err := healthSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			wlog.Error("health server exited", map[string]any{"error": err.Error()})
			sup.report(fmt.Errorf("health server exited: %w", err))
		}
	}()

	exitCode := 0
	select {
	case <-sigCh:
	case err := <-sup.failed:
		wlog.Error("event-consumer: a consumer stopped, exiting so the pod restarts", map[string]any{"error": err.Error()})
		exitCode = 1
	}

	cancel()
	if err := sup.stop(grace); err != nil {
		wlog.Warn("event-consumer: "+err.Error(), nil)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), grace)
	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		wlog.Warn("health server shutdown error", map[string]any{"error": err.Error()})
	}
	shutdownCancel()

	shutdownTelemetry()
	return exitCode
}
