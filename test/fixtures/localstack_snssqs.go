//go:build integration

package fixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TopicQueue struct {
	Name     string
	QueueURL string
	DLQURL   string
}

type LocalStackSNSSQS struct {
	Container   testcontainers.Container
	EndpointURL string
	SNSClient   *sns.Client
	SQSClient   *sqs.Client
	Queues      map[string]TopicQueue
}

// SNS topic names may only contain ASCII letters, numbers, underscores, and
// hyphens — the dotted event-family names used elsewhere (iam.membership.events)
// are logical names, not valid SNS resource names, so the actual topics use
// hyphens instead.
var Topologies = []struct {
	Topic string
	Queue string
	DLQ   string
}{
	{Topic: "iam-membership-events", Queue: "membership-workflow-q", DLQ: "membership-workflow-q-dlq"},
	{Topic: "iam-user-events", Queue: "user-wf-q", DLQ: "user-wf-q-dlq"},
	{Topic: "wf-template-events", Queue: "wf-execution-sync-q", DLQ: "wf-execution-sync-q-dlq"},
}

func NewLocalStackSNSSQS(ctx context.Context, t *testing.T) *LocalStackSNSSQS {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test — requires Docker (-short skips)")
	}

	req := testcontainers.ContainerRequest{
		Image:        "localstack/localstack:3",
		ExposedPorts: []string{"4566/tcp"},
		Env:          map[string]string{"SERVICES": "sns,sqs"},
		WaitingFor: wait.ForLog("Ready.").
			WithOccurrence(1).
			WithStartupTimeout(90 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("NewLocalStackSNSSQS: start container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("NewLocalStackSNSSQS: host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, "4566")
	if err != nil {
		t.Fatalf("NewLocalStackSNSSQS: port: %v", err)
	}
	endpointURL := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		t.Fatalf("NewLocalStackSNSSQS: aws config: %v", err)
	}

	snsClient := sns.NewFromConfig(awsCfg, func(o *sns.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
	})
	sqsClient := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
	})

	ls := &LocalStackSNSSQS{
		Container:   container,
		EndpointURL: endpointURL,
		SNSClient:   snsClient,
		SQSClient:   sqsClient,
		Queues:      make(map[string]TopicQueue),
	}

	for _, topo := range Topologies {
		topicOut, err := snsClient.CreateTopic(ctx, &sns.CreateTopicInput{Name: aws.String(topo.Topic)})
		if err != nil {
			t.Fatalf("NewLocalStackSNSSQS: CreateTopic %s: %v", topo.Topic, err)
		}
		queueURL, dlqURL := createQueueWithDLQAndSubscribe(ctx, t, snsClient, sqsClient, aws.ToString(topicOut.TopicArn), topo.Queue, topo.DLQ)
		ls.Queues[topo.Topic] = TopicQueue{Name: topo.Queue, QueueURL: queueURL, DLQURL: dlqURL}
	}

	return ls
}

func createQueueWithDLQAndSubscribe(
	ctx context.Context, t *testing.T,
	snsClient *sns.Client, sqsClient *sqs.Client,
	topicARN, queueName, dlqName string,
) (string, string) {
	t.Helper()

	dlqOut, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(dlqName)})
	if err != nil {
		t.Fatalf("createQueueWithDLQAndSubscribe: CreateQueue %s: %v", dlqName, err)
	}
	dlqURL := aws.ToString(dlqOut.QueueUrl)
	dlqARN := queueARN(ctx, t, sqsClient, dlqURL)

	redrivePolicy, _ := json.Marshal(map[string]string{
		"deadLetterTargetArn": dlqARN,
		"maxReceiveCount":     "5",
	})

	queueOut, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
		Attributes: map[string]string{
			"RedrivePolicy": string(redrivePolicy),
		},
	})
	if err != nil {
		t.Fatalf("createQueueWithDLQAndSubscribe: CreateQueue %s: %v", queueName, err)
	}
	queueURL := aws.ToString(queueOut.QueueUrl)
	queueArn := queueARN(ctx, t, sqsClient, queueURL)

	if _, err := snsClient.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: aws.String(topicARN),
		Protocol: aws.String("sqs"),
		Endpoint: aws.String(queueArn),
		Attributes: map[string]string{
			"RawMessageDelivery": "true",
		},
	}); err != nil {
		t.Fatalf("createQueueWithDLQAndSubscribe: Subscribe %s -> %s: %v", queueName, topicARN, err)
	}

	return queueURL, dlqURL
}

func queueARN(ctx context.Context, t *testing.T, sqsClient *sqs.Client, queueURL string) string {
	t.Helper()
	attr, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		t.Fatalf("queueARN: GetQueueAttributes %s: %v", queueURL, err)
	}
	return attr.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
}

func (ls *LocalStackSNSSQS) Publish(ctx context.Context, t *testing.T, topic string, body []byte) {
	t.Helper()
	tq, ok := ls.Queues[topic]
	if !ok {
		t.Fatalf("Publish: unknown topic %s", topic)
	}
	topicOut, err := ls.SNSClient.CreateTopic(ctx, &sns.CreateTopicInput{Name: aws.String(topic)})
	if err != nil {
		t.Fatalf("Publish: CreateTopic %s: %v", topic, err)
	}
	if _, err := ls.SNSClient.Publish(ctx, &sns.PublishInput{
		TopicArn: topicOut.TopicArn,
		Message:  aws.String(string(body)),
	}); err != nil {
		t.Fatalf("Publish: %s (queue %s): %v", topic, tq.Name, err)
	}
}

func (ls *LocalStackSNSSQS) DrainQueue(ctx context.Context, t *testing.T, queueURL string) []string {
	t.Helper()
	var bodies []string
	for {
		out, err := ls.SQSClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     0,
		})
		if err != nil {
			t.Fatalf("DrainQueue: ReceiveMessage: %v", err)
		}
		if len(out.Messages) == 0 {
			break
		}
		for _, m := range out.Messages {
			bodies = append(bodies, aws.ToString(m.Body))
			if _, delErr := ls.SQSClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: m.ReceiptHandle,
			}); delErr != nil {
				t.Logf("DrainQueue: DeleteMessage warning: %v", delErr)
			}
		}
	}
	return bodies
}

func (ls *LocalStackSNSSQS) ApproximateReceiveCount(ctx context.Context, t *testing.T, queueURL string) int {
	t.Helper()
	out, err := ls.SQSClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:                    aws.String(queueURL),
		MaxNumberOfMessages:         1,
		WaitTimeSeconds:             0,
		MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{sqstypes.MessageSystemAttributeNameApproximateReceiveCount},
	})
	if err != nil {
		t.Fatalf("ApproximateReceiveCount: %v", err)
	}
	if len(out.Messages) == 0 {
		return 0
	}
	n := 0
	fmt.Sscanf(out.Messages[0].Attributes[string(sqstypes.MessageSystemAttributeNameApproximateReceiveCount)], "%d", &n)
	return n
}
