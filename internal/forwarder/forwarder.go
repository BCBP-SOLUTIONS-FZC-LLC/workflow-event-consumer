package forwarder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

const (
	classOK        = "2xx"
	classRedirect  = "3xx"
	classClient    = "4xx"
	classServer    = "5xx"
	classTransport = "transport"
	classNone      = "none"
)

type Logger interface {
	Error(msg string, fields map[string]interface{})
}

type Forwarder struct {
	targets map[string]*Target
	log     Logger
}

func (f *Forwarder) WithLogger(log Logger) *Forwarder {
	f.log = log
	return f
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
			observability.ForwardTotal.WithLabelValues(queue, t.Name, "skipped", classNone).Inc()
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
			observability.ForwardTotal.WithLabelValues(queue, t.Name, result, statusClass(err)).Inc()
			f.logRefusal(queue, eventType, err)
		}(t)
	}
	wg.Wait()

	return errors.Join(errs...)
}

func statusClass(err error) string {
	if err == nil {
		return classOK
	}
	var se *StatusError
	if !errors.As(err, &se) {
		return classTransport
	}
	switch {
	case se.Status >= 500:
		return classServer
	case se.Status >= 400:
		return classClient
	}
	return classRedirect
}

func (f *Forwarder) logRefusal(queue, eventType string, err error) {
	var se *StatusError
	if f.log == nil || !errors.As(err, &se) || (se.Status != http.StatusUnauthorized && se.Status != http.StatusForbidden) {
		return
	}
	f.log.Error("forward refused by target: the caller identity or the target's authorization policy is wrong", map[string]interface{}{
		"queue":      queue,
		"target":     se.Target,
		"status":     se.Status,
		"event_type": eventType,
	})
}
