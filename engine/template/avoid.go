package template

import (
	"image"

	"github.com/odevine/mimic/engine/frame"
)

// EntryBounds reports where a stack entry's art draws for a card, so a text box
// that names it as its AvoidLayer can keep its ink clear of it, in the layout's
// own document coordinates. It resolves the entry the way the main compositing
// pass would, so it reports false when the entry's condition does not hold for the
// card, when no image of its part applies, or when the image is fully
// transparent. That means a box can name an entry unconditionally: it only ever
// narrows for a card whose render actually draws that entry. k is the keys of the
// face the entry reads. The rectangle comes back in the scaled document's
// coordinates, so it lines up with the text boxes that consult it. docW and docH
// are the authored document's size, which an image cut to its bounds is placed
// within, and zero reads as the image's own size
func EntryBounds(p AssetProvider, e *Entry, k frame.Keys, features frame.Features, s Scale, docW, docH int) (image.Rectangle, bool) {
	if !e.Condition.Met(k, features) {
		return image.Rectangle{}, false
	}
	path := e.Part.Variants[k.Material(e.Part.Slot).Key()]
	if path == "" {
		path = e.Part.Variants["any"]
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
	at := e.Origin()
	scaled, origin := s.ImageAt(img, at.X, at.Y, docW, docH)
	b := OpaqueBounds(scaled)
	if b.Empty() {
		return image.Rectangle{}, false
	}
	return b.Add(origin), true
}
