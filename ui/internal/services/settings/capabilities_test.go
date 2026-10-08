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

func TestForceLiveOpensListedGatesOnly(t *testing.T) {
	old := features
	t.Cleanup(func() { features = old; forced = map[string]bool{} })
	features = []feature{
		{key: "a", state: gatePlanned, reason: "r #1"},
		{key: "b", state: gatePlanned, reason: "r #1"},
	}
	if err := ForceLive("a"); err != nil {
		t.Fatal(err)
	}
	got := capabilities()
	if got["a"].State != gateLive || got["a"].Reason != "" {
		t.Errorf("a = %+v, want live with no reason", got["a"])
	}
	if got["b"].State != gatePlanned {
		t.Errorf("b = %+v, want planned", got["b"])
	}
}

func TestForceLiveRejectsAnUnknownKeyAndChangesNothing(t *testing.T) {
	old := features
	t.Cleanup(func() { features = old; forced = map[string]bool{} })
	features = []feature{{key: "a", state: gatePlanned, reason: "r #1"}}
	if err := ForceLive("a", "typo"); err == nil || !strings.Contains(err.Error(), "typo") {
		t.Fatalf("ForceLive = %v, want an error naming the key", err)
	}
	if capabilities()["a"].State != gatePlanned {
		t.Error("a failed call still opened a gate")
	}
}

func TestForceLiveReplacesTheEarlierList(t *testing.T) {
	old := features
	t.Cleanup(func() { features = old; forced = map[string]bool{} })
	features = []feature{{key: "a", state: gatePlanned, reason: "r #1"}}
	ForceLive("a")
	ForceLive()
	if capabilities()["a"].State != gatePlanned {
		t.Error("an empty list left a gate open")
	}
}
