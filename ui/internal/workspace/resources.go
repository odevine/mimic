package workspace

import (
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/resource"
)

// The render targets a client can ask for. An output render is the full-size
// one behind Save, and anything else is a preview
const (
	TargetPreview = "preview"
	TargetOutput  = "output"
)

// DefaultPreviewDPI is what the preview renders at until someone chooses
// otherwise. A card at 300 dpi is around 800 pixels wide, which fills the
// preview pane on a normal display and renders in a fraction of the time a
// print-ready export takes
const DefaultPreviewDPI = 300

// ReadMemory is how the workspace reads this computer's memory, a variable so a
// test can stand in a computer of any size
var ReadMemory = resource.ReadMemory

// CPUCount is how many processors a run is sized for
var CPUCount = resource.CPUs

// fallbackWidth and fallbackHeight size a render when the template's manifest
// cannot be read, which is the authored size of the frames the app ships
const (
	fallbackWidth  = 3264
	fallbackHeight = 4440
)

// RenderDPI is the resolution to render at for a target. The engine clamps what
// it is given against the template that is actually loaded, so this passes the
// stored preference straight through: a stored output of zero means the
// template's own resolution, which is what a save wants
func (w *Workspace) RenderDPI(target string) int {
	preview, output := w.Prefs.Resolution()
	if target == TargetOutput {
		return output
	}
	return PreviewOrDefault(preview)
}

// ClampedDPI is RenderDPI limited to what the active template can render, read
// once so the number reported back is the one actually rendered even if the
// setting changes while the render is in flight
func (w *Workspace) ClampedDPI(target string) int {
	dpi := w.RenderDPI(target)
	if m, err := w.Pipe.Manifest(); err == nil {
		dpi = m.ClampDPI(dpi)
	}
	return dpi
}

// PreviewOrDefault fills in the preview resolution nobody has chosen yet. Zero
// cannot stand in for it the way it does for the output resolution, since there
// zero means the template's own, which is the whole thing a preview avoids
func PreviewOrDefault(dpi int) int {
	if dpi <= 0 {
		return DefaultPreviewDPI
	}
	return dpi
}

// RenderBytes estimates the memory one render adds at the output resolution, the
// size a run renders at
func (w *Workspace) RenderBytes() uint64 {
	m, err := w.Pipe.Manifest()
	if err != nil {
		return resource.RenderBytes(fallbackWidth, fallbackHeight)
	}
	scaled := m.Scaled(m.ScaleForDPI(m.ClampDPI(w.RenderDPI(TargetOutput))))
	return resource.RenderBytes(scaled.Width, scaled.Height)
}

// LayerCacheName is the layer cache size the settings choose, with the default
// standing in for an empty or unknown choice
func (w *Workspace) LayerCacheName() string {
	if name := w.Prefs.Settings().LayerCache; resource.ValidCacheSize(name) {
		return name
	}
	return resource.DefaultCacheSize
}

// AutoConcurrency is how many cards a run renders at once when the setting is
// automatic, sized to this computer's memory and processors
func (w *Workspace) AutoConcurrency() int {
	return resource.Workers(ReadMemory(), w.RenderBytes(), CPUCount(), resource.CacheBytes(w.LayerCacheName()))
}

// ClampOutputDPI keeps a stored output resolution inside what the template can
// render, preserving zero as "the template's own" so the preference tracks a
// later template rather than pinning this one's number
func ClampOutputDPI(m *template.Manifest, dpi int) int {
	if dpi <= 0 || dpi >= m.NativeDPI() {
		return 0
	}
	return m.ClampDPI(dpi)
}
