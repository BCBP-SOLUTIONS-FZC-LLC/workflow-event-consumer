//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/platform-events/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/consumer"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/forwarder"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/internal/observability"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/fixtures"
	"github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-events-consumer/test/mocks"
)

// events.NewSQSConsumer resolves credentials via the AWS SDK's default chain
// — it has no static-credentials injection point of its own — so LocalStack
// runs need these set for the SDK to sign requests at all.
func setLocalStackCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")
}

// testRoutes mirrors config/topology.example.yaml's routing table for the
// event types these tests actually publish.
var testRoutes = map[string][]string{
	"delegation.started":            {"execution"},
	"user.deleted":                  {"execution"},
	"department.membership.revoked": {"definition"},
}

func buildConsumer(t *testing.T, ls *fixtures.LocalStackSNSSQS, queueName, queueURL string, fwd *forwarder.Forwarder, visibilityTimeout time.Duration) events.Consumer {
	t.Helper()
	router := consumer.NewRouter(testRoutes, []string{"execution", "definition"})
	c, err := consumer.Build(
		consumer.QueueSpec{Name: queueName, URL: queueURL},
		consumer.Options{
			Region:            "us-east-1",
			EndpointURL:       ls.EndpointURL,
			Concurrency:       2,
			VisibilityTimeout: visibilityTimeout,
			DrainTimeout:      5 * time.Second,
		},
		fwd,
		router,
	)
	require.NoError(t, err)
	return c
}

func startConsumer(t *testing.T, c events.Consumer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = c.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = c.Stop()
	})
}

func TestLocalStackFlow_RoutesToOnlyExecution(t *testing.T) {
	observability.Register()
	setLocalStackCredentials(t)
	ctx := context.Background()
	ls := fixtures.NewLocalStackSNSSQS(ctx, t)

	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()

	fwd := forwarder.New(
		forwarder.NewTarget("execution", exec.URL(), "exec-token", 5*time.Second),
		forwarder.NewTarget("definition", def.URL(), "def-token", 5*time.Second),
	)

	membershipQueue := ls.Queues["iam-membership-events"]
	startConsumer(t, buildConsumer(t, ls, "membership", membershipQueue.QueueURL, fwd, 5*time.Second))

	// delegation.started is execution-only per config/topology.example.yaml/testRoutes —
	// definition_service should never see it.
	env := events.NewEnvelope("delegation.started", "iam", json.RawMessage(`{"delegate_id":"u-1"}`))
	body, err := json.Marshal(env)
	require.NoError(t, err)
	ls.Publish(ctx, t, "iam-membership-events", body)

	require.Eventually(t, func() bool {
		return exec.Count() == 1
	}, 15*time.Second, 200*time.Millisecond, "execution should receive the event")

	require.Eventually(t, func() bool {
		return len(ls.DrainQueue(ctx, t, membershipQueue.QueueURL)) == 0
	}, 15*time.Second, 200*time.Millisecond, "message should be acked once execution succeeds")

	assert.Equal(t, 0, def.Count(), "definition should never receive an execution-only event")
	assert.Equal(t, "exec-token", exec.LastToken())
}

func TestLocalStackFlow_RoutesToOnlyDefinition(t *testing.T) {
	observability.Register()
	setLocalStackCredentials(t)
	ctx := context.Background()
	ls := fixtures.NewLocalStackSNSSQS(ctx, t)

	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()

	fwd := forwarder.New(
		forwarder.NewTarget("execution", exec.URL(), "exec-token", 5*time.Second),
		forwarder.NewTarget("definition", def.URL(), "def-token", 5*time.Second),
	)

	membershipQueue := ls.Queues["iam-membership-events"]
	startConsumer(t, buildConsumer(t, ls, "membership", membershipQueue.QueueURL, fwd, 5*time.Second))

	// department.membership.revoked is definition-only per config/topology.example.yaml/testRoutes.
	env := events.NewEnvelope("department.membership.revoked", "iam", json.RawMessage(`{"department_id":"d-1"}`))
	body, err := json.Marshal(env)
	require.NoError(t, err)
	ls.Publish(ctx, t, "iam-membership-events", body)

	require.Eventually(t, func() bool {
		return def.Count() == 1
	}, 15*time.Second, 200*time.Millisecond, "definition should receive the event")

	require.Eventually(t, func() bool {
		return len(ls.DrainQueue(ctx, t, membershipQueue.QueueURL)) == 0
	}, 15*time.Second, 200*time.Millisecond, "message should be acked once definition succeeds")

	assert.Equal(t, 0, exec.Count(), "execution should never receive a definition-only event")
	assert.Equal(t, "def-token", def.LastToken())
}

func TestLocalStackFlow_UnknownTypeBroadcastsToAll(t *testing.T) {
	observability.Register()
	setLocalStackCredentials(t)
	ctx := context.Background()
	ls := fixtures.NewLocalStackSNSSQS(ctx, t)

	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()

	fwd := forwarder.New(
		forwarder.NewTarget("execution", exec.URL(), "exec-token", 5*time.Second),
		forwarder.NewTarget("definition", def.URL(), "def-token", 5*time.Second),
	)

	membershipQueue := ls.Queues["iam-membership-events"]
	startConsumer(t, buildConsumer(t, ls, "membership", membershipQueue.QueueURL, fwd, 5*time.Second))

	env := events.NewEnvelope("SomeFutureEventType", "iam", json.RawMessage(`{}`))
	body, err := json.Marshal(env)
	require.NoError(t, err)
	ls.Publish(ctx, t, "iam-membership-events", body)

	require.Eventually(t, func() bool {
		return exec.Count() == 1 && def.Count() == 1
	}, 15*time.Second, 200*time.Millisecond, "an unrecognized type should broadcast to every target")
}

func TestLocalStackFlow_OneTargetFailsBlocksAck(t *testing.T) {
	observability.Register()
	setLocalStackCredentials(t)
	ctx := context.Background()
	ls := fixtures.NewLocalStackSNSSQS(ctx, t)

	exec := mocks.NewDownstream()
	defer exec.Close()
	def := mocks.NewDownstream()
	defer def.Close()
	// user.deleted is execution-only (testRoutes) — failing execution itself
	// is the only way to exercise "redeliver until the resolved target succeeds".
	exec.SetStatus(http.StatusInternalServerError)

	fwd := forwarder.New(
		forwarder.NewTarget("execution", exec.URL(), "exec-token", 5*time.Second),
		forwarder.NewTarget("definition", def.URL(), "def-token", 5*time.Second),
	)

	userQueue := ls.Queues["iam-user-events"]
	startConsumer(t, buildConsumer(t, ls, "user", userQueue.QueueURL, fwd, 2*time.Second))

	env := events.NewEnvelope("user.deleted", "iam", json.RawMessage(`{"user_id":"u-2"}`))
	body, err := json.Marshal(env)
	require.NoError(t, err)
	ls.Publish(ctx, t, "iam-user-events", body)

	require.Eventually(t, func() bool {
		return exec.Count() >= 2
	}, 15*time.Second, 200*time.Millisecond, "message should redeliver while execution keeps failing")

	exec.SetStatus(http.StatusOK)

	require.Eventually(t, func() bool {
		return len(ls.DrainQueue(ctx, t, userQueue.QueueURL)) == 0
	}, 15*time.Second, 200*time.Millisecond, "message should clear once execution succeeds")
}
