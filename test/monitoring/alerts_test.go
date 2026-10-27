package monitoring_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ruleFile struct {
	Groups []struct {
		Rules []struct {
			Alert       string            `yaml:"alert"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

func runbookAnchors(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile("../../docs/runbook.md")
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[string]bool{}
	notAnchor := regexp.MustCompile(`[^a-z0-9 -]`)
	for _, line := range strings.Split(string(body), "\n") {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			anchors[strings.ReplaceAll(notAnchor.ReplaceAllString(strings.ToLower(heading), ""), " ", "-")] = true
		}
	}
	return anchors
}

func TestEveryAlertCarriesARunbookThatIsDocumented(t *testing.T) {
	body, err := os.ReadFile("../../deploy/monitoring/app-alerts.yml")
	if err != nil {
		t.Fatal(err)
	}
	var rules ruleFile
	if err := yaml.Unmarshal(body, &rules); err != nil {
		t.Fatal(err)
	}
	anchors := runbookAnchors(t)
	seen := 0
	for _, g := range rules.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" {
				continue
			}
			seen++
			link := r.Annotations["runbook"]
			anchor, ok := strings.CutPrefix(link, "docs/runbook.md#")
			if !ok || !anchors[anchor] {
				t.Errorf("alert %s: runbook %q does not name a section of docs/runbook.md", r.Alert, link)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no alerts read")
	}
}
