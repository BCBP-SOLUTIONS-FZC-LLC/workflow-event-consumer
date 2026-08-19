package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/pkg/events"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-gincommon/pkg/logger"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/config"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "event-consumer: %v\n", err)
		os.Exit(1)
	}

	observability.Register()

	wlog, err := logger.NewLogger(cfg.AppEnv)
	if err != nil {
		log.Fatalf("event-consumer: new logger: %v", err)
	}

	d, err := buildDeps(cfg, wlog)
	if err != nil {
		log.Fatalf("event-consumer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, c := range d.consumers {
		wg.Add(1)
		go func(c events.Consumer) {
			defer wg.Done()
			if err := c.Start(ctx); err != nil {
				wlog.Error("consumer exited", map[string]any{"error": err.Error()})
			}
		}(c)
	}

	healthSrv := newHealthServer(fmt.Sprintf(":%d", cfg.HealthPort))
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			wlog.Error("health server exited", map[string]any{"error": err.Error()})
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cancel()
	for _, c := range d.consumers {
		if err := c.Stop(); err != nil {
			wlog.Warn("consumer stop error", map[string]any{"error": err.Error()})
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		wlog.Warn("health server shutdown error", map[string]any{"error": err.Error()})
	}

	drained := make(chan struct{})
	go func() {
		wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(30 * time.Second):
		wlog.Warn("event-consumer: shutdown grace period elapsed with consumers still running", nil)
	}
}
