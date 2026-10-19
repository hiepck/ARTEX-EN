package agent

import "testing"

func TestReachabilityEdgesEnabledDefaultsFalseWhenUnset(t *testing.T) {
	old := ReachabilityEdgesEnabled
	t.Cleanup(func() { ReachabilityEdgesEnabled = old })
	ReachabilityEdgesEnabled = nil
	if reachabilityEdgesEnabled() {
		t.Fatal("reachabilityEdgesEnabled() with nil var = true, want false")
	}
}

func TestReachabilityEdgesEnabledReflectsVar(t *testing.T) {
	old := ReachabilityEdgesEnabled
	t.Cleanup(func() { ReachabilityEdgesEnabled = old })
	on := false
	ReachabilityEdgesEnabled = func() bool { return on }
	if reachabilityEdgesEnabled() {
		t.Fatal("reachabilityEdgesEnabled() = true before flipping on")
	}
	on = true
	if !reachabilityEdgesEnabled() {
		t.Fatal("reachabilityEdgesEnabled() = false after flipping on")
	}
}
