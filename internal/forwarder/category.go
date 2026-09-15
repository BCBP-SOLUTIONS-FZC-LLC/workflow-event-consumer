package forwarder

// executionTargetName is the only target whose topology-config name the
// category subpaths below apply to (target.go's forward). It must match
// whatever name a deployment gives the execution_service target in its own
// topology config's targets: list.
const executionTargetName = "execution"

// category maps an event type to the URL subpath execution_service registers
// for it (router.go). execution_service has no generic catch-all — every
// event type routed to it must have an entry here, or forward() sends it to
// a path execution_service doesn't serve. definition_service and
// workflow-stream-gateway have one bare endpoint each and never receive a
// category-mapped type.
var category = map[string]string{
	"DelegationStarted":       "delegation",
	"DelegationEnded":         "delegation",
	"UserDeleted":             "user-profile",
	"UserAvailabilityChanged": "user-profile",
	// Removing a user from a tenant has the same effect on execution as
	// deleting them — every active assignment they hold is vacated — so it
	// shares UserDeleted's own subpath rather than getting a new one.
	"MembershipRevoked":  "user-profile",
	"TenantStateChanged": "tenant",
	// IAM Core's tenant-offboarding cascade signal, handled on the same
	// endpoint as TenantStateChanged's own offboard branch.
	"TenantMembershipsPurged": "tenant",
	"WorkflowTaskCreated":     "workflow-task",
}

func categoryFor(eventType string) (string, bool) {
	c, ok := category[eventType]
	return c, ok
}
