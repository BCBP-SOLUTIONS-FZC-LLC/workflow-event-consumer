package consumer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
)

func TestBuild_EmptyQueueURLErrors(t *testing.T) {
	fwd := forwarder.New()
	router := NewRouter(nil, nil)
	_, err := Build(QueueSpec{Name: "membership", URL: ""}, Options{}, fwd, router)
	assert.Error(t, err)
}

func TestBuild_AcceptsTheProductionOptions(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	opts := Options{
		Region:             "us-east-1",
		EndpointURL:        "http://localhost:4566",
		Concurrency:        4,
		VisibilityTimeout:  30 * time.Second,
		DrainTimeout:       10 * time.Second,
		HandlerTimeout:     20 * time.Second,
		RetryBackoffBase:   5 * time.Second,
		RetryBackoffMax:    5 * time.Minute,
		QueueDepthInterval: 30 * time.Second,
	}

	c, err := Build(QueueSpec{Name: "membership", URL: "http://localhost:4566/000000000000/membership-workflow-q"}, opts, forwarder.New(), NewRouter(nil, nil))

	require.NoError(t, err)
	assert.NotNil(t, c)
}
