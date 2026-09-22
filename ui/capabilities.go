package main

import "github.com/odevine/mimic/engine/version"

// The four gate levels a feature can sit at. Only live is usable. The other
// three render the control dimmed with a reason, and differ in what resolves
// them: waiting, upgrading the engine, or switching template
const (
	gateLive          = "live"
	gatePlanned       = "planned"
	gateNeedsEngine   = "needs-engine"
	gateNeedsTemplate = "needs-template"
)

// gate is one feature's state as the frontend reads it
type gate struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// feature is one row of the build-time table. minEngine, when set on a
// needs-engine feature, names the engine release that lifts the gate, so a
// build stamped with that engine or newer reports the feature live without
// anyone editing this table
type feature struct {
	key       string
	state     string
	reason    string
	minEngine string
}

// features is what this release implements. Removing a gate means changing a
// state here, never the markup, since the frontend renders what it is told
var features = []feature{
	{key: "flow.single", state: gateLive},
	{key: "flow.list", state: gatePlanned, reason: "Rendering from a list is coming in v1.0"},
	{key: "flow.art", state: gateNeedsEngine, reason: "Needs engine support for art override"},
	{key: "flow.run", state: gatePlanned, reason: "The Run Console arrives with batch rendering in v1.0"},

	{key: "single.printings", state: gateLive},
	{key: "single.symbols", state: gateLive},
	{key: "single.zoom", state: gateLive},
	{key: "single.compare", state: gatePlanned, reason: "Comparing against the Scryfall scan is coming in v1.0"},
	{key: "single.artDrop", state: gateNeedsEngine, reason: "Needs engine support for art override"},
	{key: "single.dfc", state: gateNeedsEngine, reason: "Engine renders the front face only"},

	{key: "overrides", state: gatePlanned, reason: "Overrides are coming in v1.0"},
	{key: "overrides.global", state: gatePlanned, reason: "Global overrides are coming in v1.0"},
	{key: "overrides.layers", state: gateNeedsEngine, reason: "Needs manifest introspection and layer forcing"},
	{key: "overrides.textboxes", state: gateNeedsEngine, reason: "Needs per-box overrides in RenderRequest"},
	{key: "overrides.assets", state: gatePlanned, reason: "Asset substitution is planned after v1.0"},
	{key: "presets", state: gatePlanned, reason: "Presets are coming in v1.0"},

	{key: "run.retryFailed", state: gatePlanned, reason: "Coming in v1.0"},
	{key: "palette", state: gatePlanned, reason: "The command palette is coming in v1.0"},

	{key: "settings.resolution", state: gateLive},
	{key: "settings.bitDepth", state: gateNeedsEngine, reason: "Needs engine support for 16-bit output"},
	{key: "settings.bleed", state: gateNeedsEngine, reason: "Needs engine support for trimming the bleed"},
	{key: "settings.concurrency", state: gatePlanned, reason: "Arrives with batch rendering in v1.0"},
	{key: "settings.outputDir", state: gatePlanned, reason: "Arrives with batch rendering in v1.0"},
	{key: "settings.filenameTemplate", state: gatePlanned, reason: "Arrives with batch rendering in v1.0"},
	{key: "settings.runReport", state: gatePlanned, reason: "Arrives with batch rendering in v1.0"},
	{key: "settings.scryfallRate", state: gatePlanned, reason: "Arrives with batch rendering in v1.0"},
	{key: "settings.preferNonPromo", state: gatePlanned, reason: "Coming in v1.0"},
	{key: "settings.language", state: gatePlanned, reason: "Coming in v1.0"},
	{key: "settings.filenameRegex", state: gatePlanned, reason: "Arrives with rendering from your art in v1.0"},
	{key: "settings.density", state: gatePlanned, reason: "Coming in v1.0"},
	{key: "settings.restoreSession", state: gatePlanned, reason: "Coming in v1.0"},
	{key: "output.pdf", state: gatePlanned, reason: "Print sheets are planned after v1.0"},
}

// engineSatisfies is a seam over version.Satisfies so a test can lift a
// needs-engine gate without stamping a binary
var engineSatisfies = version.Satisfies

// capabilities resolves the feature table into the map the frontend applies.
// A needs-engine feature whose minEngine the running engine satisfies is
// reported live. The active template will feed needs-template gates once a
// manifest can declare what it supports
func capabilities() map[string]gate {
	out := make(map[string]gate, len(features))
	for _, f := range features {
		g := gate{State: f.state, Reason: f.reason}
		if f.state == gateNeedsEngine && f.minEngine != "" && engineSatisfies(f.minEngine) {
			g = gate{State: gateLive}
		}
		out[f.key] = g
	}
	return out
}
