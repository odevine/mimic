package render

import (
	"fmt"
	"image"
	"sort"
	"strconv"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/template"
)

// ItemKind is what a described item is
type ItemKind string

const (
	ItemLayer  ItemKind = "layer"
	ItemArt    ItemKind = "art"
	ItemSymbol ItemKind = "symbol"
	ItemText   ItemKind = "text"
)

// Item is one thing a layout draws, or would draw for another card
type Item struct {
	Kind ItemKind
	// Key names the item within its layout: a stack entry's id, an art window's
	// index, a symbol's name, or a text box's key
	Key string
	// Part is the art set a layer draws from, and After is the stack entry an art
	// window sits directly above
	Part  string
	After string
	// Images are the paths of the images a layer draws, one unless it blends a pair
	Images []string
	// Drawn says whether the item draws for this card. Why says what stops it when
	// it does not
	Drawn bool
	Why   string
	// Ink says the rectangles are where the item's text is drawn and not its own
	// rectangle, which a text box set on its baseline has no use for: its top is
	// the first baseline, and the text sits above it. Such a box can be moved by
	// them and not resized
	Ink bool
	// BaselineAuthored and BaselineDelivered are, for an Ink item, the line its first
	// baseline lies on, from one end of the box's width to the other, in the same two
	// canvases as Authored and Delivered. Each is a rectangle with no thickness across
	// the line, since a quarter turn of the layout turns the line to a vertical one.
	// The box's height has no part in where a baseline box draws, so this is all of the
	// box's own rectangle that matters, and it is what Rect.Y, X and Width say. Both are
	// empty for an item that is not Ink
	BaselineAuthored  image.Rectangle
	BaselineDelivered image.Rectangle
	// PlacedAuthored and PlacedDelivered are, for a text box that draws, the rectangle it
	// ends up in once the card has moved it to the row of another box or narrowed it to
	// stay clear of one, in the same two canvases. The rectangles above are the box as the
	// layout gives it, or where its text is. Moved says the box's place changed and
	// Narrowed that its width did. Narrowing to keep out of a layer is not a change to the
	// box, since it only shortens lines, and is not reported
	PlacedAuthored  image.Rectangle
	PlacedDelivered image.Rectangle
	Moved           bool
	Narrowed        bool
	// Override is, for a text box, the override of it that is in force for this card,
	// which is why its rectangle and style are not the ones the box is written with, or
	// nil when the box stands as authored
	Override *Override
	// Fit is, for a text box that draws, how its text fitted
	Fit *template.TextFit
	// Authored is where the item is in the layout's own canvas and Delivered is where
	// it is in the finished image, after the layout's rotation. Both are in the
	// canvas's own pixels, not scaled for a preview. A layer's rectangle is the
	// visible pixels of its image. A layer that does not draw has neither
	Authored  image.Rectangle
	Delivered image.Rectangle
}

// Override names the override of a text box that is in force: its condition as the layout
// writes it, and its place among the box's overrides in the order the layout lists them
type Override struct {
	When  string
	Index int
}

// Description is what a template's layout does for one card
type Description struct {
	Layout string
	// Authored and Delivered are the sizes of the layout's canvas and of the
	// finished image
	Authored  image.Point
	Delivered image.Point
	// Items lists the stack in draw order with each art window right after the entry
	// it sits above, then the symbols, then the text boxes by key
	Items []Item
}

// Describe reports what the template's layout for the card draws, where, and why
// anything does not, the way Render would decide each, without drawing. The card's
// face, art, set symbol and features are read from req as Render reads them. An
// editor uses it to say which layers a card shows and to find them on the image
func Describe(t *template.Template, req template.RenderRequest) (*Description, error) {
	if t == nil {
		return nil, fmt.Errorf("render: no template")
	}
	if req.Card == nil {
		return nil, fmt.Errorf("render: request has no card")
	}
	req.Features = t.ResolveFeatures(req.Features)
	shapes := template.Classify(req.Card)
	if req.Face < 0 || req.Face >= len(shapes) {
		return nil, fmt.Errorf("render: face %d out of range, %q renders %d", req.Face, req.Card.Name, len(shapes))
	}
	layout, ok := t.Layout(shapes[req.Face])
	if !ok {
		return nil, &template.UnsupportedError{Template: t.ID(), Shape: shapes[req.Face]}
	}
	// Geometry at the native resolution is the layout's own numbers
	geo := layout.Geometry(layout.NativeDPI())
	authored := image.Pt(layout.Canvas.Width, layout.Canvas.Height)
	d := &describer{t: t, req: req, layout: layout, authored: authored}
	d.out = Description{Layout: layout.Name, Authored: authored, Delivered: image.Pt(layout.DeliveredWidth(), layout.DeliveredHeight())}

	f := frame.DeriveFace(req.FaceCard(), req.FaceSide())
	_, drawSymbol := setSymbolSpec(req, layout, geo, f)
	f.SetSymbol = drawSymbol
	d.fc = newFaces(req, shapes, req.FaceCard(), f)

	for _, e := range layout.Stack {
		d.layer(e)
		d.art(geo, e.ID)
	}
	d.symbols(geo, drawSymbol)
	d.text(geo)
	return &d.out, nil
}

type describer struct {
	t        *template.Template
	req      template.RenderRequest
	layout   *template.Layout
	authored image.Point
	fc       *faces
	out      Description
}

// add places an item's rectangles, given in the canvas it was laid out in
func (d *describer) add(it Item, r image.Rectangle, output bool) {
	if !r.Empty() {
		it.Authored, it.Delivered = d.both(r, output)
	}
	d.out.Items = append(d.out.Items, it)
}

// both gives a rectangle in the canvas it was laid out in as it is in the layout's own
// canvas and in the finished image. A rectangle with no thickness turns the same way
func (d *describer) both(r image.Rectangle, output bool) (authored, delivered image.Rectangle) {
	deg := d.layout.Canvas.Rotate
	if output {
		return template.RotateRect(r, d.out.Delivered.X, d.out.Delivered.Y, 360-deg), r
	}
	return r, template.RotateRect(r, d.authored.X, d.authored.Y, deg)
}

func (d *describer) layer(e *template.Entry) {
	e = forced(d.req, e)
	it := Item{Kind: ItemLayer, Key: e.ID, Part: e.Part.Name}
	_, k := d.fc.get(e.Face)
	switch {
	case !e.Condition.Met(k, d.req.Features):
		it.Why = fmt.Sprintf("its condition %q does not hold for this card", e.Condition.String())
		d.add(it, image.Rectangle{}, false)
		return
	case entryOpacity(e, d.req) <= 0:
		it.Why = "its opacity is 0"
		d.add(it, image.Rectangle{}, false)
		return
	}
	material := k.Material(e.Part.Slot)
	variants := e.Part.Variants
	var bounds image.Rectangle
	if _, whole := variants[material.Key()]; !whole {
		if paths := template.BlendedPaths(variants, material); paths != nil {
			it.Images = paths
			at := e.Origin()
			for _, p := range paths {
				img, err := template.LoadImage(d.t.Assets(), p)
				if err != nil {
					it.Why = fmt.Sprintf("the image %s cannot be read: %v", p, err)
					d.add(it, image.Rectangle{}, false)
					return
				}
				bounds = bounds.Union(template.OpaqueBounds(img).Add(at))
			}
			d.finishLayer(it, bounds)
			return
		}
	}
	path, ok := variants[material.Key()]
	if !ok {
		if path, ok = variants["any"]; !ok {
			it.Why = fmt.Sprintf("its part has no image for %q and no \"any\" image", material.Key())
			d.add(it, image.Rectangle{}, false)
			return
		}
	}
	it.Images = []string{path}
	r, visible := template.EntryBounds(d.t.Assets(), e, k, d.req.Features, 1, d.authored.X, d.authored.Y)
	if !visible {
		it.Why = "its image has no visible pixel"
		d.add(it, image.Rectangle{}, false)
		return
	}
	d.finishLayer(it, r)
}

func (d *describer) finishLayer(it Item, r image.Rectangle) {
	if r.Empty() {
		it.Why = "its image has no visible pixel"
		d.add(it, image.Rectangle{}, false)
		return
	}
	it.Drawn = true
	d.add(it, r, false)
}

// entryOpacity is the opacity a layer draws at for the request
func entryOpacity(e *template.Entry, req template.RenderRequest) float64 {
	if e.OpacityFeature != "" {
		v, _ := req.Features[e.OpacityFeature].(float64)
		return v
	}
	return e.Opacity
}

func (d *describer) art(geo *template.Geometry, after string) {
	for i, slot := range geo.Art {
		if slot.After != after {
			continue
		}
		it := Item{Kind: ItemArt, Key: strconv.Itoa(i), After: after, Drawn: d.req.Art != nil}
		if !it.Drawn {
			it.Why = "no art was given"
		}
		d.add(it, image.Rect(slot.X, slot.Y, slot.X+slot.Width, slot.Y+slot.Height), false)
	}
}

func (d *describer) symbols(geo *template.Geometry, drawSymbol bool) {
	for _, name := range sortedKeys(d.layout.Symbols) {
		spec := geo.Symbols[name]
		it := Item{Kind: ItemSymbol, Key: name}
		f := d.fc.self.keys
		switch {
		case name != template.SymbolSet:
			it.Drawn = d.layout.Symbols[name].Condition.Met(f, d.req.Features)
		default:
			it.Drawn = drawSymbol
			if !it.Drawn {
				it.Why = "the card has no set symbol"
			}
		}
		d.add(it, image.Rect(spec.X, spec.Y, spec.X+spec.Width, spec.Y+spec.Height), false)
	}
}

func (d *describer) text(geo *template.Geometry) {
	boxes := resolveBoxes(d.layout, geo, d.fc, d.req.Features)
	fr := fonts.NewResolver(d.t.Fonts(), d.req.FontPool, d.req.FontOverrides)
	syms := newSymbols(fr, boxes)
	for _, b := range d.layout.Boxes {
		it := Item{Kind: ItemText, Key: b.Key}
		job, drawn := boxes.byKey[b.Key]
		if !drawn {
			it.Why = fmt.Sprintf("its condition %q does not hold for this card", b.Condition.String())
			base := geo.Boxes[b.Key].Base
			d.add(it, image.Rect(base.X, base.Y, base.X+base.Width, base.Y+base.Height), b.Space == template.SpaceOutput)
			continue
		}
		box, parts, err := placeBox(d.req, d.t, d.layout, d.authored, job, boxes, d.fc, syms, fr, 1)
		it.Drawn = err == nil && len(parts) > 0
		if !it.Drawn {
			it.Why = fmt.Sprintf("the card has no %s to draw", job.draws)
		}
		r := image.Rect(job.spec.X, job.spec.Y, job.spec.X+job.spec.Width, job.spec.Y+job.spec.Height)
		// A box set on its baseline is shown where its text is. A box turned a quarter
		// lays its text out in a rectangle of its own, so it keeps the box's
		if it.Drawn && job.spec.VAlign == "baseline" && job.spec.Rotate == 0 {
			if ink, err := template.MeasureTextBox(box, parts...); err == nil && !ink.Empty() {
				r, it.Ink = ink, true
				// The box's own place is the baseline, its Y, across its width
				it.BaselineAuthored, it.BaselineDelivered = d.both(image.Rect(job.spec.X, job.spec.Y, job.spec.X+job.spec.Width, job.spec.Y), job.space == template.SpaceOutput)
			}
		}
		it.Override = d.override(b, job)
		if it.Drawn {
			placed := image.Rect(box.X, box.Y, box.X+box.Width, box.Y+box.Height)
			it.PlacedAuthored, it.PlacedDelivered = d.both(placed, job.space == template.SpaceOutput)
			it.Moved = box.X != job.spec.X || box.Y != job.spec.Y
			it.Narrowed = box.Width < job.spec.Width
			if fit, err := template.FitTextBox(box, parts...); err == nil {
				it.Fit = &fit
			}
		}
		d.add(it, r, job.space == template.SpaceOutput)
	}
}

// override says which override of a box is in force for the card, or nil for none. The
// layout tries overrides in an order of its own, so the index is where the override is in
// the layout's list, found by its condition
func (d *describer) override(b *template.Box, job resolvedBox) *Override {
	if job.pick < 0 || job.pick >= len(b.Overrides) {
		return nil
	}
	when := b.Overrides[job.pick].Condition.String()
	index := job.pick
	for i, o := range b.Base.When {
		if o.Condition == when {
			index = i
			break
		}
	}
	return &Override{When: when, Index: index}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
