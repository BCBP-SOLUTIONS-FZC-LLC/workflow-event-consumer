package consumer

type Router struct {
	routes  map[string][]string
	targets []string
}

func NewRouter(routes map[string][]string, allTargets []string) Router {
	return Router{routes: routes, targets: allTargets}
}

func (r Router) Resolve(eventType string) []string {
	if targets, ok := r.routes[eventType]; ok {
		return targets
	}
	return r.targets
}
