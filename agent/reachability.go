package agent

// ReachabilityEdgesEnabled reports whether report_finding's "reaches"
// parameter (and the resulting asset_edges data in graph_overview) is
// active. Assigned once at startup (server/assembly.go's wireTools); nil
// until then, which reachabilityEdgesEnabled treats as off.
var ReachabilityEdgesEnabled func() bool

func reachabilityEdgesEnabled() bool {
	return ReachabilityEdgesEnabled != nil && ReachabilityEdgesEnabled()
}
