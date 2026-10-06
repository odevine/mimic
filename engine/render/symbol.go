package render

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/odevine/impasto/blend"
	"github.com/odevine/impasto/canvas"
	"github.com/odevine/impasto/gradient"
	"github.com/odevine/impasto/path"
	"github.com/odevine/impasto/raster"

	"github.com/odevine/mimic/engine/card"
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
// box without changing its shape, placed by the spec's alignment, filled with
// the paint for the card's rarity, and outlined when the spec asks for it. It
// returns nil for a symbol with nothing to draw
func symbolNode(sym *card.SetSymbol, spec template.SymbolSpec, rarity string, docW, docH int) (canvas.Node, error) {
	ink := sym.Icon.Bounds()
	if len(sym.Icon.Shapes) == 0 || ink.W <= 0 || ink.H <= 0 {
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

	// The symbol is drawn into a buffer just big enough for it and its outline,
	// then placed in the document, since a document-sized raster of a small shape
	// would cost far more than the shape does
	var outlineW float64
	if spec.Outline != nil && spec.Outline.Width > 0 {
		outlineW = spec.Outline.Width
	}
	pad := int(math.Ceil(outlineW)) + 2
	bufX, bufY := int(math.Floor(x0))-pad, int(math.Floor(y0))-pad
	bufW, bufH := int(math.Ceil(w))+2*pad+1, int(math.Ceil(h))+2*pad+1
	buf, err := raster.NewBuffer(bufW, bufH)
	if err != nil {
		return nil, fmt.Errorf("render: set symbol buffer: %w", err)
	}

	// A point (x, y) of the icon lands at ((x-ink.X)*k + x0 - bufX, ...)
	sx := float32(k)
	dx := float32(x0 - float64(bufX) - float64(ink.X)*k)
	dy := float32(y0 - float64(bufY) - float64(ink.Y)*k)
	paths := make([]*path.Path, len(sym.Icon.Shapes))
	for i, s := range sym.Icon.Shapes {
		paths[i] = s.Transformed(sx, sx, dx, dy)
	}

	// The outline sits outside the edge, so it is a stroke twice as wide drawn
	// first, with the fill covering the half that falls inside
	if outlineW > 0 {
		edge := path.FlatColorSRGB(template.ParseHexColor(spec.Outline.Color))
		style := path.StrokeStyle{Width: float32(2 * outlineW), Join: path.JoinRound, Cap: path.CapRound}
		for _, p := range paths {
			path.StrokePath(buf, p, style, edge)
		}
	}
	paint := symbolPaint(spec, rarity, float64(bufW), float64(bufH), float64(pad), w, h)
	for i, p := range paths {
		path.Fill(buf, p, sym.Icon.Shapes[i].Rule, paint)
	}
	return &canvas.Layer{Content: canvas.Place(docW, docH, buf, bufX, bufY), Mode: blend.Normal}, nil
}

// symbolPaint is the fill for the card's rarity. Several stops make a linear
// gradient laid across the symbol's own extent at the paint's angle, and a
// single stop is a flat color. A rarity the spec does not list takes common's
// paint, and with no common entry the symbol is black
func symbolPaint(spec template.SymbolSpec, rarity string, bufW, bufH, pad, w, h float64) path.Paint {
	p, ok := spec.Rarity[rarityKey(rarity)]
	if !ok {
		p, ok = spec.Rarity["common"]
	}
	if !ok || len(p.Stops) == 0 {
		return path.FlatColorSRGB(color.Black)
	}
	if len(p.Stops) == 1 {
		return path.FlatColorSRGB(template.ParseHexColor(p.Stops[0].Color))
	}
	stops := make([]gradient.Stop, len(p.Stops))
	for i, s := range p.Stops {
		stops[i] = gradient.Stop{Pos: float32(s.At), Color: template.ParseHexColor(s.Color), Opacity: 1}
	}
	// The gradient runs between the two points where a line through the symbol's
	// center at the angle meets its bounding box's extreme corners, so the first
	// stop sits at one corner and the last at the opposite one
	a := p.Angle * math.Pi / 180
	ux, uy := math.Cos(a), math.Sin(a)
	half := (math.Abs(w*ux) + math.Abs(h*uy)) / 2
	cx, cy := pad+w/2, pad+h/2
	p0 := path.Point{X: float32(cx - half*ux), Y: float32(cy - half*uy)}
	p1 := path.Point{X: float32(cx + half*ux), Y: float32(cy + half*uy)}
	return gradient.New(gradient.Linear, gradient.Pad, p0, p1, stops)
}

// rarityKey maps a card's rarity to the manifest key for its paint. A bonus
// card prints in the mythic color, and any other rarity a manifest does not
// know is left to fall back to common
func rarityKey(rarity string) string {
	r := strings.ToLower(strings.TrimSpace(rarity))
	if r == "bonus" {
		return "mythic"
	}
	return r
}
