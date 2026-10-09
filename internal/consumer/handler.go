package consumer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

func MakeHandler(queue string, fwd *forwarder.Forwarder, router Router) events.Handler {
	return func(ctx context.Context, env events.Envelope[json.RawMessage]) error {
		start := time.Now()

		body, _ := json.Marshal(env)

		targets := router.Resolve(env.Type)

		result := "ok"
		if err := fwd.Forward(ctx, queue, targets, env.Type, body); err != nil {
			result = "error"
			observability.HandlerDurationSeconds.WithLabelValues(queue, result).Observe(time.Since(start).Seconds())
			return err
		}

		observability.HandlerDurationSeconds.WithLabelValues(queue, result).Observe(time.Since(start).Seconds())
		return nil
	}
}
