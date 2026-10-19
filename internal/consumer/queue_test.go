package consumer

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	eventsmock "github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/v2/pkg/events/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
)

func TestBuild_EmptyQueueURLErrors(t *testing.T) {
	fwd := forwarder.New()
	router := NewRouter(nil, nil)
	_, err := Build(QueueSpec{Name: "membership", URL: ""}, Options{DeadLetters: &eventsmock.DLQPublisher{}, Region: "us-east-1"}, fwd, router)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "dead-letter publisher")
}

func TestBuild_AcceptsTheProductionOptions(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	opts := Options{
		DeadLetters:        &eventsmock.DLQPublisher{},
		Region:             "us-east-1",
		EndpointURL:        "http://localhost:4566",
		Concurrency:        4,
		VisibilityTimeout:  30 * time.Second,
		DrainTimeout:       10 * time.Second,
		HandlerTimeout:     20 * time.Second,
		RetryBackoffBase:   5 * time.Second,
		RetryBackoffMax:    5 * time.Minute,
		QueueDepthInterval: 30 * time.Second,
		MaxReceiveCount:    DefaultMaxReceiveCount,
	}

	c, err := Build(QueueSpec{Name: "membership", URL: "http://localhost:4566/000000000000/membership-workflow-q"}, opts, forwarder.New(), NewRouter(nil, nil))

	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestBuild_RequiresADeadLetterPublisher(t *testing.T) {
	_, err := Build(QueueSpec{Name: "membership", URL: "http://localhost:4566/000000000000/membership-workflow-q"}, Options{}, forwarder.New(), NewRouter(nil, nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dead-letter publisher")
}

func TestBuildRefusesAThresholdThatWouldTurnDeadLetterRoutingOff(t *testing.T) {
	for _, n := range []int{0, -3} {
		_, err := Build(QueueSpec{Name: "membership", URL: "http://localhost:4566/000000000000/membership-workflow-q"},
			Options{DeadLetters: &eventsmock.DLQPublisher{}, Region: "us-east-1", MaxReceiveCount: n}, forwarder.New(), NewRouter(nil, nil))
		require.Error(t, err, n)
		assert.Contains(t, err.Error(), "max receive count")
	}
}

func TestReceiveThresholdIsBelowTheRedriveCountOfTheLocalQueues(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/init-localstack.sh")
	require.NoError(t, err)

	m := regexp.MustCompile(`maxReceiveCount\\?":\\?"(\d+)`).FindStringSubmatch(string(raw))
	require.Len(t, m, 2, "the local stack sets a redrive count")
	redrive, err := strconv.Atoi(m[1])
	require.NoError(t, err)
	assert.Less(t, DefaultMaxReceiveCount, redrive, "SQS would move the message before the consumer dead-letters it")
}
