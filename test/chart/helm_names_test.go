package chart_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	chartDir        = "../../deploy/helm"
	callerAccount   = "workflow-events-consumer"
	callerNamespace = "workflow-app"
)

type chartObject struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				ServiceAccountName string `yaml:"serviceAccountName"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func helmRender(t *testing.T, release string, extra ...string) string {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed")
	}
	args := append([]string{"template", release, chartDir, "--namespace", callerNamespace}, extra...)
	out, err := exec.Command(helm, args...).CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func decodeAll(t *testing.T, out string) []chartObject {
	t.Helper()
	dec := yaml.NewDecoder(strings.NewReader(out))
	var docs []chartObject
	for {
		var doc chartObject
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs
		}
		require.NoError(t, err, out)
		docs = append(docs, doc)
	}
}

func TestHelmNames_ReleaseWfRendersTheFixedDeploymentServiceAndServiceAccountNames(t *testing.T) {
	for _, release := range []string{"wf", "events", "a-much-longer-release-name"} {
		t.Run(release, func(t *testing.T) {
			byKind := map[string][]string{}
			var podAccounts []string
			for _, o := range decodeAll(t, helmRender(t, release)) {
				byKind[o.Kind] = append(byKind[o.Kind], o.Metadata.Name)
				if o.Kind == "Deployment" {
					podAccounts = append(podAccounts, o.Spec.Template.Spec.ServiceAccountName)
				}
			}

			assert.Equal(t, []string{callerAccount}, byKind["Deployment"])
			assert.Equal(t, []string{callerAccount}, byKind["Service"])
			assert.Equal(t, []string{callerAccount}, byKind["ServiceAccount"], "the Definition, Execution and Stream Gateway policies name this account")
			assert.Equal(t, []string{callerAccount}, podAccounts, "the pods run as the account the other charts' policies name")
			assert.Equal(t, []string{callerAccount + "-topology"}, byKind["ConfigMap"])
		})
	}
}

func helmRenderErr(t *testing.T, release string, extra ...string) (string, error) {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed")
	}
	args := append([]string{"template", release, chartDir, "--namespace", callerNamespace}, extra...)
	out, err := exec.Command(helm, args...).CombinedOutput()
	return string(out), err
}

func podAccounts(t *testing.T, out string) []string {
	t.Helper()
	var accounts []string
	for _, o := range decodeAll(t, out) {
		if o.Kind == "Deployment" {
			accounts = append(accounts, o.Spec.Template.Spec.ServiceAccountName)
		}
	}
	return accounts
}

func TestHelmServiceAccount_IsTheCallerNameForEveryRelease(t *testing.T) {
	for _, release := range []string{"wf", "x"} {
		t.Run(release, func(t *testing.T) {
			out := helmRender(t, release)
			var accounts []string
			for _, o := range decodeAll(t, out) {
				if o.Kind == "ServiceAccount" {
					accounts = append(accounts, o.Metadata.Name)
				}
			}

			assert.Equal(t, []string{callerAccount}, accounts)
			assert.Equal(t, []string{callerAccount}, podAccounts(t, out))
		})
	}
}

// The other charts' policies admit this one caller by its ServiceAccount; another name would be refused by all three.
func TestHelmServiceAccount_AnotherNameIsRefusedWithAClearMessage(t *testing.T) {
	out, err := helmRenderErr(t, "wf", "--set", "serviceAccount.name=other")

	require.Error(t, err, "a ServiceAccount other than the one the callers' policies name would be refused by the mesh")
	assert.Contains(t, out, "serviceAccount.name")
	assert.Contains(t, out, callerAccount)
	assert.Contains(t, out, "other")

	out, err = helmRenderErr(t, "wf", "--set", "serviceAccount.name="+callerAccount)
	require.NoError(t, err, out, "the fixed name itself is accepted")
}

func TestHelmServiceAccount_WithoutCreationThePodsStillRunAsTheCallerName(t *testing.T) {
	out := helmRender(t, "wf", "--set", "serviceAccount.create=false")

	assert.Equal(t, []string{callerAccount}, podAccounts(t, out), "the default account would not match the callers' policies")
	for _, o := range decodeAll(t, out) {
		assert.NotEqual(t, "ServiceAccount", o.Kind, "create=false renders none; the account is managed elsewhere")
	}
}

func TestHelmNames_NoObjectNameDependsOnTheReleaseName(t *testing.T) {
	for _, o := range decodeAll(t, helmRender(t, "wf", "--set", "autoscaling.enabled=true")) {
		assert.True(t, strings.HasPrefix(o.Metadata.Name, callerAccount), "%s %s", o.Kind, o.Metadata.Name)
	}
}

func TestHelmNames_TheValuesDefaultTheFullnameOverride(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)
	var values struct {
		FullnameOverride string `yaml:"fullnameOverride"`
	}
	require.NoError(t, yaml.Unmarshal(body, &values))

	assert.Equal(t, callerAccount, values.FullnameOverride)
}

// The shared tokens are gone: no template reads one and the values do not carry one.
func TestHelmChart_CarriesNoSharedToken(t *testing.T) {
	var files []string
	var matches []string
	removed := []string{"EXECUTION_SERVICE_TOKEN", "DEFINITION_SERVICE_TOKEN", "WORKFLOW_STREAM_GATEWAY_TOKEN", "tokenEnv", "x-internal-token", "secretValues", "existingSecret"}

	err := filepath.WalkDir(chartDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		files = append(files, path)
		for _, name := range removed {
			if bytes.Contains(body, []byte(name)) {
				matches = append(matches, path+" names "+name)
			}
		}
		return nil
	})

	require.NoError(t, err)
	require.NotEmpty(t, files, "the chart directory was not found, so the check proved nothing")
	assert.Contains(t, files, filepath.Join(chartDir, "values.yaml"))
	assert.Contains(t, files, filepath.Join(chartDir, "templates", "deployment.yaml"))
	assert.Empty(t, matches)
}

func TestHelmValues_NoCommentPointsAtAFileTheChartDoesNotHave(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)

	assert.NotContains(t, string(body), ".env", "the values are rendered into the Deployment's environment; no .env file is merged")
}
