package consumer

import (
	"fmt"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/pkg/events"

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

type Options struct {
	Region            string
	EndpointURL       string
	Concurrency       int
	VisibilityTimeout time.Duration
	DrainTimeout      time.Duration
	Logger            Logger
}

func Build(spec QueueSpec, opts Options, fwd *forwarder.Forwarder, router Router) (events.Consumer, error) {
	c, err := events.NewSQSConsumer(
		events.SQSConfig{
			QueueURL:    spec.URL,
			Region:      opts.Region,
			EndpointURL: opts.EndpointURL,
			Logger:      opts.Logger,
		},
		MakeHandler(spec.Name, fwd, router),
		events.WithConcurrency(opts.Concurrency),
		events.WithVisibilityTimeout(opts.VisibilityTimeout),
		events.WithDrainTimeout(opts.DrainTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("new sqs consumer for queue %s: %w", spec.Name, err)
	}
	return c, nil
}
