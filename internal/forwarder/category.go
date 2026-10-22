package forwarder

const ExecutionTarget = "execution"

var category = map[string]string{
	"DelegationStarted":       "delegation",
	"DelegationEnded":         "delegation",
	"UserDeleted":             "user-profile",
	"UserAvailabilityChanged": "user-profile",
	"MembershipRevoked":       "user-profile",
	"TenantStateChanged":      "tenant",
	"TenantMembershipsPurged": "tenant",
	"WorkflowTaskCreated":     "workflow-task",
}

func categoryFor(eventType string) (string, bool) {
	c, ok := category[eventType]
	return c, ok
}

func Categorised(eventType string) bool {
	_, ok := categoryFor(eventType)
	return ok
}
