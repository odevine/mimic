package main

import (
	"github.com/odevine/mimic/engine/template"
)

// defaultPreviewDPI is what the preview renders at until someone chooses
// otherwise. A card at 300 dpi is around 800 pixels wide, which fills the
// preview pane on a normal display and renders in a fraction of the time a
// print-ready export takes
const defaultPreviewDPI = 300

// The render targets a client can ask for. An output render is the full-size
// one behind Save; anything else is a preview
const (
	targetPreview = "preview"
	targetOutput  = "output"
)

// resolutionSettings is what the resolution endpoint returns: the two chosen
// resolutions, the presets a picker offers, and the bounds a custom dpi has to
// stay inside. Every field is resolved against the active template, so the
// frontend renders the numbers it is handed without knowing anything about the
// template's authored size
type resolutionSettings struct {
	Preview template.Resolution   `json:"preview"`
	Output  template.Resolution   `json:"output"`
	Presets []template.Resolution `json:"presets"`
	MinDPI  int                   `json:"minDpi"`
	MaxDPI  int                   `json:"maxDpi"`
}

// resolutions resolves the stored preview and output dpi against the active
// template. A template authored below a stored resolution clamps it rather than
// upscaling, so switching templates never leaves a preference that renders
// something the assets cannot support
func (s *server) resolutions() (resolutionSettings, error) {
	m, err := s.pipe.manifest()
	if err != nil {
		return resolutionSettings{}, err
	}
	preview, output := s.prefs.resolution()
	return resolutionSettings{
		Preview: m.Resolution(previewOrDefault(preview)),
		Output:  m.Resolution(output),
		Presets: m.Presets(),
		MinDPI:  min(template.MinDPI, m.NativeDPI()),
		MaxDPI:  m.NativeDPI(),
	}, nil
}

// renderDPI is the resolution to render at for a target. The engine clamps what
// it is given against the template that is actually loaded, so this passes the
// stored preference straight through: a stored output of zero means the
// template's own resolution, which is what a save wants
func (s *server) renderDPI(target string) int {
	preview, output := s.prefs.resolution()
	if target == targetOutput {
		return output
	}
	return previewOrDefault(preview)
}

// previewOrDefault fills in the preview resolution nobody has chosen yet. Zero
// cannot stand in for it the way it does for the output resolution, since there
// zero means the template's own, which is the whole thing a preview avoids
func previewOrDefault(dpi int) int {
	if dpi <= 0 {
		return defaultPreviewDPI
	}
	return dpi
}
