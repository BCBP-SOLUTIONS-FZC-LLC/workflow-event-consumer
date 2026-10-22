package main

import (
	"context"
	"fmt"
	"log"
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

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	wlog, err := logger.NewLogger(cfg.AppEnv)
	if err != nil {
		log.Fatalf("event-consumer: new logger: %v", err)
	}

	shutdownTelemetry, err := observability.InitTelemetry(observability.GinConfig(cfg.OTELServiceName, cfg.BuildVersion, cfg.AppEnv, wlog), wlog)
	if err != nil {
		log.Fatalf("event-consumer: telemetry: %v", err)
	}

	wlog.Info("event-consumer starting", map[string]any{"version": version})

	d, err := buildDeps(cfg, wlog)
	if err != nil {
		fatalf(shutdownTelemetry, "event-consumer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	consumers := make([]consumerLike, len(d.consumers))
	for i, c := range d.consumers {
		consumers[i] = c
	}
	sup := superviseConsumers(ctx, consumers, wlog)

	watch := newReceiveWatch(gincommon.MetricsGatherer(), cfg.ReceiveStallWindow, time.Now)
	healthSrv := newHealthServer(fmt.Sprintf(":%d", cfg.HealthPort), observability.GinConfig(cfg.OTELServiceName, cfg.BuildVersion, cfg.AppEnv, wlog), watch)
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			wlog.Error("health server exited", map[string]any{"error": err.Error()})
			sup.report(fmt.Errorf("health server exited: %w", err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	exitCode := 0
	select {
	case <-sigCh:
	case err := <-sup.failed:
		wlog.Error("event-consumer: a consumer stopped, exiting so the pod restarts", map[string]any{"error": err.Error()})
		exitCode = 1
	}

	cancel()
	if err := sup.stop(shutdownGrace); err != nil {
		wlog.Warn("event-consumer: "+err.Error(), nil)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownGrace)
	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		wlog.Warn("health server shutdown error", map[string]any{"error": err.Error()})
	}
	shutdownCancel()

	shutdownTelemetry()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func fatalf(flush func(), format string, v ...any) {
	log.Printf(format, v...)
	flush()
	os.Exit(1)
}
