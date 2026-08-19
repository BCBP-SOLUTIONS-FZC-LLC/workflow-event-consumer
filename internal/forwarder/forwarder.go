package forwarder

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

func (f *Forwarder) Forward(ctx context.Context, queue string, targetNames []string, body []byte) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for _, name := range targetNames {
		t, ok := f.targets[name]
		if !ok {
			return fmt.Errorf("forwarder: unknown target %q", name)
		}
		wg.Add(1)
		go func(t *Target) {
			defer wg.Done()
			err := t.forward(ctx, body)
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
