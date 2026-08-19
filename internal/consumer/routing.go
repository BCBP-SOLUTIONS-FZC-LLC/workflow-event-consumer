package consumer

// Router resolves which targets a given event type forwards to. It's built
// from topology.Config.Routes at startup — the catalogue itself lives in
// config/topology.yaml, not in Go source.
type Router struct {
	routes  map[string][]string
	targets []string
}

// NewRouter builds a Router from a routing table and the full set of
// declared target names (the fallback broadcast set).
func NewRouter(routes map[string][]string, allTargets []string) Router {
	return Router{routes: routes, targets: allTargets}
}

// Resolve returns the targets eventType should be forwarded to. A type
// absent from the routing table is unrecognized — fall back to every
// declared target so a new event type is never silently dropped before
// config/topology.yaml is updated to know about it.
func (r Router) Resolve(eventType string) []string {
	if targets, ok := r.routes[eventType]; ok {
		return targets
	}
	return r.targets
}
