package agent

// RootCauseGroupingEnabled reports whether report_finding's
// "shares_root_cause_with" parameter (and the resulting root_cause_clusters
// data in graph_overview) is active. Assigned once at startup
// (server/assembly.go's wireTools); nil until then, which
// rootCauseGroupingEnabled treats as off.
var RootCauseGroupingEnabled func() bool

func rootCauseGroupingEnabled() bool {
	return RootCauseGroupingEnabled != nil && RootCauseGroupingEnabled()
}
