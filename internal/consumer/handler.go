package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
)

func MakeHandler(queue string, fwd *forwarder.Forwarder, router Router, dlq events.DLQPublisher) events.Handler {
	return func(ctx context.Context, env events.Envelope[json.RawMessage]) error {
		start := time.Now()

		body, _ := json.Marshal(env)

		targets := router.Resolve(env.Type)

		err := fwd.Forward(ctx, queue, targets, env.Type, body)
		if err != nil && forwarder.Permanent(err) {
			err = deadLetter(ctx, dlq, err)
		}

		result := "ok"
		if err != nil {
			result = "error"
		}
		observability.HandlerDurationSeconds.WithLabelValues(queue, result).Observe(time.Since(start).Seconds())
		return err
	}
}

func deadLetter(ctx context.Context, dlq events.DLQPublisher, cause error) error {
	src, ok := events.SourceMessageFromContext(ctx)
	if !ok {
		return cause
	}
	reason := fmt.Sprintf("permanent forward failure: %v", cause)
	if err := dlq.SendToDLQ(ctx, src.QueueURL, src.Body, src.Attributes, reason); err != nil {
		return errors.Join(cause, fmt.Errorf("dead-letter: %w", err))
	}
	return nil
}
