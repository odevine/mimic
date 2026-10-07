package template

import (
	"image"

	"github.com/odevine/mimic/engine/frame"
)

// AvoidRect reports where a named layer's art draws, so a text box that names
// it as AvoidLayer can keep its ink clear of it, in the manifest's own document
// coordinates. It resolves the layer the same way the main compositing pass
// would, so it reports false when the layer's Condition does not hold for this
// card, when no color variant of it resolves, or when the resolved asset is
// fully transparent. That means a box can name an AvoidLayer unconditionally: it
// only ever narrows for a card whose render actually draws that layer. The
// rectangle comes back in the scaled document's coordinates, so it lines up
// with the text boxes that consult it. docW and docH are the authored document's
// size, which a layer cut to its bounds is placed within, and zero reads as the
// asset's own size
func AvoidRect(p AssetProvider, layers map[string]LayerSpec, f frame.Keys, name string, s Scale, docW, docH int) (image.Rectangle, bool) {
	spec, ok := layers[name]
	if !ok || !f.ConditionMet(spec.Condition) {
		return image.Rectangle{}, false
	}
	path := spec.ColorVariants[f.Slot(spec.ColorSlot)].Path
	if path == "" {
		path = spec.ColorVariants["any"].Path
	}
	if path == "" {
		return image.Rectangle{}, false
	}
	img, err := LoadImage(p, path)
	if err != nil {
		return image.Rectangle{}, false
	}
	if b := img.Bounds(); docW <= 0 || docH <= 0 {
		docW, docH = b.Dx(), b.Dy()
	}
	scaled, at := s.ImageAt(img, spec.X, spec.Y, docW, docH)
	b := OpaqueBounds(scaled)
	if b.Empty() {
		return image.Rectangle{}, false
	}
	return b.Add(at), true
}
