package render

import (
	"fmt"
	"image"
	"math"

	"github.com/odevine/impasto/blend"
	"github.com/odevine/impasto/canvas"
	"github.com/odevine/impasto/raster"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/card/svgpath"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/template"
)

// setSymbolSpec is the manifest's place for the card's set symbol, and whether
// the symbol draws at all: the request must carry one, the manifest must give it
// a box, and the spec's own condition must hold for the card
func setSymbolSpec(req template.RenderRequest, m *template.Manifest, f frame.Keys) (template.SymbolSpec, bool) {
	spec, ok := m.Symbols[template.SymbolSet]
	if !ok || req.SetSymbol == nil || req.SetSymbol.Icon == nil || spec.Width <= 0 || spec.Height <= 0 {
		return spec, false
	}
	return spec, f.ConditionMet(spec.Condition)
}

// symbolNode draws a set symbol into its box. The symbol is scaled to fit the
// box without changing its shape, placed by the spec's alignment, and drawn in
// the colors its own file carries. It returns nil for a symbol with nothing to
// draw
func symbolNode(sym *card.SetSymbol, spec template.SymbolSpec) (canvas.Node, error) {
	icon := sym.Icon
	ink := icon.Bounds()
	if len(icon.Shapes) == 0 || ink.W <= 0 || ink.H <= 0 {
		return nil, nil
	}
	boxW, boxH := float64(spec.Width), float64(spec.Height)
	fit := spec.Scale
	if fit <= 0 {
		fit = 1
	}
	k := math.Min(boxW/float64(ink.W), boxH/float64(ink.H)) * fit
	w, h := float64(ink.W)*k, float64(ink.H)*k

	x0, y0 := float64(spec.X), float64(spec.Y)
	switch spec.Align {
	case "left":
	case "center":
		x0 += (boxW - w) / 2
	default:
		x0 += boxW - w
	}
	switch spec.VAlign {
	case "top":
	case "bottom":
		y0 += boxH - h
	default:
		y0 += (boxH - h) / 2
	}

	// The symbol is drawn into a buffer just big enough for it, which the layer
	// places in the document at its origin. The margin leaves room for the edge's
	// anti-aliasing
	const pad = 2
	bufX, bufY := int(math.Floor(x0))-pad, int(math.Floor(y0))-pad
	bufW, bufH := int(math.Ceil(w))+2*pad+1, int(math.Ceil(h))+2*pad+1
	buf, err := raster.NewBuffer(bufW, bufH)
	if err != nil {
		return nil, fmt.Errorf("render: set symbol buffer: %w", err)
	}

	// An icon point (x, y) lands at ((x-ink.X)*k + x0 - bufX, (y-ink.Y)*k + y0 - bufY)
	m := svgpath.Translate(x0-float64(bufX)-float64(ink.X)*k, y0-float64(bufY)-float64(ink.Y)*k).Mul(svgpath.ScaleBy(k, k))
	icon.Draw(buf, m)
	return &canvas.Layer{Content: buf, Origin: image.Pt(bufX, bufY), Mode: blend.Normal}, nil
}
