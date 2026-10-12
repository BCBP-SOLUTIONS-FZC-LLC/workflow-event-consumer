package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type consumerLike interface {
	Start(ctx context.Context) error
	Stop() error
}

type superviseLogger interface {
	Warn(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
}

type supervisor struct {
	consumers []consumerLike
	log       superviseLogger
	wg        sync.WaitGroup
	failed    chan error
	stopping  atomic.Bool
}

func superviseConsumers(ctx context.Context, consumers []consumerLike, log superviseLogger) *supervisor {
	s := &supervisor{consumers: consumers, log: log, failed: make(chan error, len(consumers)+1)}
	for _, c := range consumers {
		s.wg.Add(1)
		go func(c consumerLike) {
			defer s.wg.Done()
			err := c.Start(ctx)
			switch {
			case err != nil:
				log.Error("consumer exited", map[string]any{"error": err.Error()})
				s.report(fmt.Errorf("consumer exited: %w", err))
			case ctx.Err() == nil && !s.stopping.Load():
				log.Error("consumer returned without being stopped", nil)
				s.report(errors.New("consumer returned without being stopped"))
			}
		}(c)
	}
	return s
}

func (s *supervisor) report(err error) {
	select {
	case s.failed <- err:
	default:
	}
}

func (s *supervisor) stop(grace time.Duration) error {
	s.stopping.Store(true)
	for _, c := range s.consumers {
		if err := c.Stop(); err != nil {
			s.log.Warn("consumer stop error", map[string]any{"error": err.Error()})
		}
	}
	drained := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		return nil
	case <-time.After(grace):
		return errors.New("consumers still running after the grace period")
	}
}
