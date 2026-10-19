package consumer

import (
	"fmt"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
)

type Logger interface {
	Debug(msg string, fields map[string]interface{})
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
}

type QueueSpec struct {
	Name string
	URL  string
}

const DefaultMaxReceiveCount = 4

type Options struct {
	DeadLetters        events.DLQPublisher
	Region             string
	EndpointURL        string
	Concurrency        int
	VisibilityTimeout  time.Duration
	DrainTimeout       time.Duration
	HandlerTimeout     time.Duration
	RetryBackoffBase   time.Duration
	RetryBackoffMax    time.Duration
	QueueDepthInterval time.Duration
	MaxReceiveCount    int
	Logger             Logger
}

func Build(spec QueueSpec, opts Options, fwd *forwarder.Forwarder, router Router) (events.Consumer, error) {
	if opts.DeadLetters == nil {
		return nil, fmt.Errorf("new sqs consumer for queue %s: dead-letter publisher is required", spec.Name)
	}
	if opts.MaxReceiveCount < 1 {
		return nil, fmt.Errorf("new sqs consumer for queue %s: max receive count must be at least 1, got %d", spec.Name, opts.MaxReceiveCount)
	}
	c, err := events.NewSQSConsumer(
		events.SQSConfig{
			QueueURL:    spec.URL,
			Region:      opts.Region,
			EndpointURL: opts.EndpointURL,
			Logger:      opts.Logger,
		},
		MakeHandler(spec.Name, fwd, router, opts.DeadLetters),
		events.WithConcurrency(opts.Concurrency),
		events.WithVisibilityTimeout(opts.VisibilityTimeout),
		events.WithDrainTimeout(opts.DrainTimeout),
		events.WithHandlerTimeout(opts.HandlerTimeout),
		events.WithRetryBackoff(opts.RetryBackoffBase, opts.RetryBackoffMax),
		events.WithQueueDepthMetrics(opts.QueueDepthInterval),
		events.WithConsumerCodec(events.NoopCodec{}),
		events.WithDLQForwarding(opts.DeadLetters),
		events.WithMaxReceiveCount(opts.MaxReceiveCount),
	)
	if err != nil {
		return nil, fmt.Errorf("new sqs consumer for queue %s: %w", spec.Name, err)
	}
	return c, nil
}
