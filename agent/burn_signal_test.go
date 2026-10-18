package agent

import "testing"

func TestBurnSignalEnabledDefaultsFalseWhenUnset(t *testing.T) {
	old := BurnSignalEnabled
	t.Cleanup(func() { BurnSignalEnabled = old })
	BurnSignalEnabled = nil
	if burnSignalEnabled() {
		t.Fatal("burnSignalEnabled() with nil var = true, want false")
	}
}

func TestBurnSignalEnabledReflectsVar(t *testing.T) {
	old := BurnSignalEnabled
	t.Cleanup(func() { BurnSignalEnabled = old })
	on := false
	BurnSignalEnabled = func() bool { return on }
	if burnSignalEnabled() {
		t.Fatal("burnSignalEnabled() = true before flipping on")
	}
	on = true
	if !burnSignalEnabled() {
		t.Fatal("burnSignalEnabled() = false after flipping on")
	}
}
