package settings

import (
	"regexp"
	"strings"
	"testing"
)

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
		// A gate waits on something, and the reason says which issue tracks it, so
		// it never promises a version
		if f.state != gateLive && !regexp.MustCompile(`#\d+`).MatchString(f.reason) {
			t.Errorf("gated feature %q does not name its tracking issue: %q", f.key, f.reason)
		}
		if strings.Contains(f.reason, "v1.0") {
			t.Errorf("gated feature %q promises a version: %q", f.key, f.reason)
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
