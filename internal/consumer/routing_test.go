package consumer

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouter_KnownTypesResolveToTheirOwnRoute(t *testing.T) {
	all := []string{"execution", "definition"}
	routes := map[string][]string{
		"DelegationStarted":           {"execution"},
		"DepartmentMembershipRevoked": {"definition"},
	}
	router := NewRouter(routes, all)

	assert.Equal(t, []string{"execution"}, router.Resolve("DelegationStarted"))
	assert.Equal(t, []string{"definition"}, router.Resolve("DepartmentMembershipRevoked"))
}

// TestRouter_ExplicitlyEmptyRouteDrops covers the third routing state, which
// is distinct from an unknown type: a type declared with no targets is one
// the platform knows about and deliberately does not forward. Omitting it
// instead would hit the unknown-type fallback below and broadcast it to
// every service on every delivery, for each to acknowledge and discard.
func TestRouter_ExplicitlyEmptyRouteDrops(t *testing.T) {
	all := []string{"execution", "definition"}
	router := NewRouter(map[string][]string{"TenderAssigneeOverridden": {}}, all)

	assert.Empty(t, router.Resolve("TenderAssigneeOverridden"),
		"a deliberately-unconsumed event must not fall back to the broadcast set")
}

func TestRouter_UnknownTypeFallsBackToAll(t *testing.T) {
	all := []string{"execution", "definition"}
	router := NewRouter(map[string][]string{"DelegationStarted": {"execution"}}, all)

	got := router.Resolve("SomeFutureEventType")
	sort.Strings(got)
	assert.Equal(t, []string{"definition", "execution"}, got)
}
