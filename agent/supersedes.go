package agent

// FactSupersedesEnabled reports whether record_fact's "supersedes" parameter
// (and the resulting fact_supersedes data in graph_overview) is active.
// Assigned once at startup (server/assembly.go's wireTools); nil until then,
// which factSupersedesEnabled treats as off.
var FactSupersedesEnabled func() bool

func factSupersedesEnabled() bool {
	return FactSupersedesEnabled != nil && FactSupersedesEnabled()
}
