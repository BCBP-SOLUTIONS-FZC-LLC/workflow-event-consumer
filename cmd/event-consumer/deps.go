package main

import (
	"fmt"
	"os"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/config"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/consumer"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/topology"
)

type deps struct {
	consumers []events.Consumer
}

func buildDeps(cfg *config.Config, logger consumer.Logger) (*deps, error) {
	topo, err := topology.Load(cfg.TopologyPath)
	if err != nil {
		return nil, fmt.Errorf("load topology: %w", err)
	}

	targets, err := topo.ResolveTargets(cfg.AppEnv, os.Getenv, cfg.ForwardTimeout)
	if err != nil {
		return nil, fmt.Errorf("resolve targets: %w", err)
	}
	fwd := forwarder.New(targets...)
	router := consumer.NewRouter(topo.Routes, topo.TargetNames())

	deadLetters, err := events.NewSQSDLQPublisher(events.DLQConfig{
		Region:       cfg.AWSRegion,
		EndpointURL:  cfg.AWSEndpointURL,
		ConsumerName: observability.ServiceName,
		Logger:       logger,
	})
	if err != nil {
		return nil, fmt.Errorf("new dead-letter publisher: %w", err)
	}

	opts := consumer.Options{
		DeadLetters:        deadLetters,
		Region:             cfg.AWSRegion,
		EndpointURL:        cfg.AWSEndpointURL,
		Concurrency:        cfg.ConsumerConcurrency,
		VisibilityTimeout:  cfg.VisibilityTimeout,
		DrainTimeout:       cfg.DrainTimeout,
		HandlerTimeout:     cfg.HandlerTimeout,
		RetryBackoffBase:   cfg.RetryBackoffBase,
		RetryBackoffMax:    cfg.RetryBackoffMax,
		QueueDepthInterval: cfg.QueueDepthInterval,
		Logger:             logger,
	}

	consumers := make([]events.Consumer, 0, len(topo.Queues))
	for _, q := range topo.Queues {
		spec := consumer.QueueSpec{Name: q.Name, URL: q.URL}
		c, err := consumer.Build(spec, opts, fwd, router)
		if err != nil {
			return nil, fmt.Errorf("build %s consumer: %w", q.Name, err)
		}
		consumers = append(consumers, c)
	}

	return &deps{consumers: consumers}, nil
}
