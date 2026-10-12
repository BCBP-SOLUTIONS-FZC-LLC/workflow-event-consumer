package forwarder

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

type Forwarder struct {
	targets map[string]*Target
}

func New(targets ...*Target) *Forwarder {
	m := make(map[string]*Target, len(targets))
	for _, t := range targets {
		m[t.Name] = t
	}
	return &Forwarder{targets: m}
}

func (f *Forwarder) Names() []string {
	names := make([]string, 0, len(f.targets))
	for name := range f.targets {
		names = append(names, name)
	}
	return names
}

func (f *Forwarder) Forward(ctx context.Context, queue string, targetNames []string, eventType string, body []byte) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	resolved := make([]*Target, 0, len(targetNames))
	for _, name := range targetNames {
		t, ok := f.targets[name]
		if !ok {
			return fmt.Errorf("forwarder: unknown target %q", name)
		}
		resolved = append(resolved, t)
	}

	for _, t := range resolved {
		if !t.accepts(eventType) {
			observability.ForwardTotal.WithLabelValues(queue, t.Name, "skipped").Inc()
			continue
		}
		wg.Add(1)
		go func(t *Target) {
			defer wg.Done()
			start := time.Now()
			err := t.forward(ctx, eventType, body)
			observability.ForwardDurationSeconds.WithLabelValues(queue, t.Name).Observe(time.Since(start).Seconds())
			result := "ok"
			if err != nil {
				result = "error"
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
			observability.ForwardTotal.WithLabelValues(queue, t.Name, result).Inc()
		}(t)
	}
	wg.Wait()

	return errors.Join(errs...)
}
