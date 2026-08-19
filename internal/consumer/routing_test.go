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

func TestRouter_UnknownTypeFallsBackToAll(t *testing.T) {
	all := []string{"execution", "definition"}
	router := NewRouter(map[string][]string{"DelegationStarted": {"execution"}}, all)

	got := router.Resolve("SomeFutureEventType")
	sort.Strings(got)
	assert.Equal(t, []string{"definition", "execution"}, got)
}
