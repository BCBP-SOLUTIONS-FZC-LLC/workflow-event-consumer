package topology

import (
	"encoding/json"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestChartTopologyMatchesTheExample(t *testing.T) {
	example, err := Load("../../config/topology.example.yaml")
	require.NoError(t, err)

	raw, err := os.ReadFile("../../deploy/helm/values.yaml")
	require.NoError(t, err)
	var values struct {
		Topology Config `yaml:"topology"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &values))

	assert.Equal(t, example.Targets, values.Topology.Targets, "chart targets: name, addr, tokenEnv and pathPrefix")
	assert.Equal(t, queueNames(example), queueNames(values.Topology), "chart queues")
	assert.Equal(t, example.Routes, values.Topology.Routes, "chart routes")
}

func TestIAMPolicyCoversEveryQueueAndItsDeadLetterQueue(t *testing.T) {
	example, err := Load("../../config/topology.example.yaml")
	require.NoError(t, err)

	raw, err := os.ReadFile("../../deploy/aws/policy.json")
	require.NoError(t, err)
	var policy struct {
		Statement []struct {
			Sid      string   `json:"Sid"`
			Action   []string `json:"Action"`
			Resource []string `json:"Resource"`
		} `json:"Statement"`
	}
	require.NoError(t, json.Unmarshal(raw, &policy))

	resources := map[string][]string{}
	actions := map[string][]string{}
	for _, st := range policy.Statement {
		for _, arn := range st.Resource {
			resources[st.Sid] = append(resources[st.Sid], arn[len("arn:aws:sqs:*:*:"):])
		}
		actions[st.Sid] = st.Action
	}

	var queues, deadLetters []string
	for _, q := range example.Queues {
		queues = append(queues, path.Base(q.URL))
		deadLetters = append(deadLetters, path.Base(q.URL)+"-dlq")
	}
	sort.Strings(queues)
	sort.Strings(deadLetters)
	sort.Strings(resources["WorkflowEventsConsumerSQSConsume"])
	sort.Strings(resources["WorkflowEventsConsumerSQSDeadLetter"])

	assert.Equal(t, queues, resources["WorkflowEventsConsumerSQSConsume"], "the consume statement lists exactly the queues of the topology")
	assert.Equal(t, deadLetters, resources["WorkflowEventsConsumerSQSDeadLetter"], "the dead-letter statement lists exactly the dead-letter queues of the topology")
	assert.ElementsMatch(t, []string{"sqs:GetQueueUrl", "sqs:SendMessage", "sqs:GetQueueAttributes"}, actions["WorkflowEventsConsumerSQSDeadLetter"])
}

func TestIAMKMSPolicyCoversTheSourceAndDeadLetterKeys(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/aws/policy-kms.json")
	require.NoError(t, err, "a role whose queues use a customer-managed key needs a KMS statement")
	var policy struct {
		Statement []struct {
			Sid       string                       `json:"Sid"`
			Action    []string                     `json:"Action"`
			Condition map[string]map[string]string `json:"Condition"`
		} `json:"Statement"`
	}
	require.NoError(t, json.Unmarshal(raw, &policy))

	actions := map[string][]string{}
	for _, st := range policy.Statement {
		actions[st.Sid] = st.Action
		assert.Contains(t, st.Condition["StringLike"]["kms:ViaService"], "sqs.", "%s is limited to use through SQS", st.Sid)
	}
	assert.ElementsMatch(t, []string{"kms:Decrypt"}, actions["WorkflowEventsConsumerKMSSourceQueues"], "receiving from an encrypted queue needs decrypt")
	assert.ElementsMatch(t, []string{"kms:GenerateDataKey", "kms:Decrypt"}, actions["WorkflowEventsConsumerKMSDeadLetterQueues"], "sending to an encrypted dead-letter queue needs both")
}

func queueNames(c Config) []string {
	names := make([]string, len(c.Queues))
	for i, q := range c.Queues {
		names[i] = q.Name + "=" + q.URL
	}
	return names
}

func TestRunbookRouteTableMatchesTheExample(t *testing.T) {
	example, err := Load("../../config/topology.example.yaml")
	require.NoError(t, err)

	raw, err := os.ReadFile("../../docs/runbook.md")
	require.NoError(t, err)
	section := runbookSection(t, string(raw), "## Event routing catalogue")

	got := map[string][]string{}
	for _, m := range regexp.MustCompile("(?m)^\\| `(\\w+)` \\| (.+) \\|$").FindAllStringSubmatch(section, -1) {
		targets := []string{}
		if m[2] != "dropped" {
			targets = strings.Split(m[2], ", ")
		}
		got[m[1]] = targets
	}

	want := map[string][]string{}
	for eventType, targets := range example.Routes {
		if targets == nil {
			targets = []string{}
		}
		want[eventType] = targets
	}
	assert.Equal(t, want, got, "docs/runbook.md route table must list exactly the routes of the topology")
}

func runbookSection(t *testing.T, doc, heading string) string {
	t.Helper()
	start := strings.Index(doc, heading)
	require.GreaterOrEqual(t, start, 0, "runbook heading %q", heading)
	rest := doc[start+len(heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func TestDockerfileLeavesNoCredentialInALayer(t *testing.T) {
	raw, err := os.ReadFile("../../Dockerfile")
	require.NoError(t, err)

	var instructions []string
	var current strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimRight(line, " ")
		current.WriteString(strings.TrimSuffix(trimmed, "\\"))
		current.WriteString(" ")
		if !strings.HasSuffix(trimmed, "\\") {
			instructions = append(instructions, current.String())
			current.Reset()
		}
	}

	for _, ins := range instructions {
		if !strings.Contains(ins, "git-credentials") {
			continue
		}
		assert.True(t, strings.HasPrefix(strings.TrimSpace(ins), "RUN --mount=type=secret"), "credentials are written only inside a secret mount: %s", ins)
		assert.Regexp(t, `rm -f [^;&]*git-credentials`, ins, "the same RUN removes the credential file")
		assert.Contains(t, ins, "go mod download", "the same RUN uses the credential")
	}
}

func TestReleaseBuilderCacheExportsOnlyTheFinalStageLayers(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/release.yml")
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "mode=max", "mode=max exports every builder layer to the registry")
	assert.NotContains(t, string(raw), "cache-from", "a release build must not read a cache that anyone with package write can poison")
}

func TestChartGivesPodsTimeToDrain(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/helm/values.yaml")
	require.NoError(t, err)
	var values struct {
		TerminationGracePeriodSeconds int `yaml:"terminationGracePeriodSeconds"`
		ServiceMonitor                struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"serviceMonitor"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &values))

	assert.GreaterOrEqual(t, values.TerminationGracePeriodSeconds, 70, "drain timeout 30s plus the handler deadline 20s, with margin")
	assert.True(t, values.ServiceMonitor.Enabled, "alerts need the scrape")

	deployment, err := os.ReadFile("../../deploy/helm/templates/deployment.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(deployment), ".Values.terminationGracePeriodSeconds")
}

func TestAlertsSelectTheChartsJobName(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/monitoring/app-alerts.yml")
	require.NoError(t, err)
	alerts := string(raw)

	assert.NotRegexp(t, `job="`, alerts, "the ServiceMonitor's job is <release>-workflow-events-consumer, which an exact match never selects")
	assert.Contains(t, alerts, "WorkflowEventsConsumerDeadLetterBacklog")
	assert.NotContains(t, alerts, "by (queue) (increase(platform_dlq_messages_total", "that series carries no queue label")
}
