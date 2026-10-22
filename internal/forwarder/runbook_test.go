package forwarder

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunbookCategoryTableMatchesTheCategoryMap(t *testing.T) {
	raw, err := os.ReadFile("../../docs/runbook.md")
	require.NoError(t, err)
	doc := string(raw)

	heading := "## Type-scoped forwarding subpaths"
	start := strings.Index(doc, heading)
	require.GreaterOrEqual(t, start, 0)
	section := doc[start+len(heading):]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}

	got := map[string]string{}
	for _, m := range regexp.MustCompile("(?m)^\\| `(\\w+)` \\| `([a-z-]+)` \\|").FindAllStringSubmatch(section, -1) {
		got[m[1]] = m[2]
	}
	assert.Equal(t, category, got, "docs/runbook.md category table must list exactly the category map")
}
