package settings

import "testing"

func TestFeaturesAreWellFormed(t *testing.T) {
	valid := map[string]bool{gateLive: true, gatePlanned: true, gateNeedsEngine: true, gateNeedsTemplate: true}
	seen := map[string]bool{}
	for _, f := range features {
		if seen[f.key] {
			t.Errorf("feature %q listed twice", f.key)
		}
		seen[f.key] = true
		if !valid[f.state] {
			t.Errorf("feature %q has unknown state %q", f.key, f.state)
		}
		// A gated control shows its reason, so a gate without one reads as a bug
		if f.state != gateLive && f.reason == "" {
			t.Errorf("gated feature %q has no reason", f.key)
		}
	}
}

func TestCapabilitiesLiftsSatisfiedEngineGate(t *testing.T) {
	old, oldSat := features, engineSatisfies
	t.Cleanup(func() { features, engineSatisfies = old, oldSat })
	features = []feature{
		{key: "met", state: gateNeedsEngine, reason: "r", minEngine: "0.7.0"},
		{key: "unmet", state: gateNeedsEngine, reason: "r", minEngine: "0.9.0"},
		{key: "unknown", state: gateNeedsEngine, reason: "r"},
	}
	engineSatisfies = func(min string) bool { return min == "0.7.0" }

	caps := capabilities()
	if got := caps["met"].State; got != gateLive {
		t.Errorf("met: state %q, want live", got)
	}
	if got := caps["unmet"]; got.State != gateNeedsEngine || got.Reason == "" {
		t.Errorf("unmet: %+v, want needs-engine with its reason", got)
	}
	if got := caps["unknown"].State; got != gateNeedsEngine {
		t.Errorf("no minEngine: state %q, want needs-engine", got)
	}
}
