package forwarder

// category maps an event type to the URL subpath execution_service registers
// for it (RegisterInternalEventsRoutes). Kept in sync by hand with that
// switch — an event type absent here falls back to the legacy
// /internal/events catch-all, which every target (including
// definition/workflow-stream-gateway, neither of which has a category
// mapping) still serves.
var category = map[string]string{
	"delegation.started":        "delegation",
	"delegation.ended":          "delegation",
	"user.deleted":              "user-profile",
	"user.availability.changed": "user-profile",
	"tenant.state.changed":      "tenant",
}

func categoryFor(eventType string) (string, bool) {
	c, ok := category[eventType]
	return c, ok
}
