package agent

// BurnSignalEnabled reports whether per-intent resource-burn visibility
// should be computed and exposed in graph_overview. Assigned once at
// startup (server/assembly.go's wireTools); nil until then, which
// burnSignalEnabled treats as off.
var BurnSignalEnabled func() bool

func burnSignalEnabled() bool {
	return BurnSignalEnabled != nil && BurnSignalEnabled()
}
