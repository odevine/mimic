// Package render draws a card with a template. A template carries its own frame
// art and the places text goes, and the engine fills in what the card says, so
// one renderer serves every WUBRG-colored Magic frame. A template's text boxes
// use the standard metadata vocabulary (title, mana, type, oracle, pt, artist,
// collector, set, copyright), and its look comes entirely from its manifest
package render

import (
	"context"
	"fmt"
	"image"
	"strings"

	"github.com/odevine/impasto/blend"
	"github.com/odevine/impasto/canvas"
	"github.com/odevine/impasto/effects"
	"github.com/odevine/impasto/raster"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/mana"
	"github.com/odevine/mimic/engine/template"
)

// Render step names and the cumulative fraction each phase spans. The fractions
// are for feedback, not timing. Frame layers decode as the compositor reaches
// them, after the text is laid out, so that load owns the widest band
const (
	stepManifest = "Reading template"
	stepFrame    = "Compositing frame"
	stepText     = "Rendering text"
	stepFinalize = "Finalizing"

	fracManifest  = 0.02
	fracFrameFrom = 0.05
	fracFrameTo   = 0.08
	fracTextFrom  = 0.08
	fracTextTo    = 0.35
	fracLoadTo    = 0.9
	fracFinalize  = 0.9
)

// frameLoads counts the frame layers the compositor has loaded so far, so each
// load can report its place in the band between the text and the finish. The
// compositor loads one layer at a time, so the count needs no lock
type frameLoads struct {
	req         template.RenderRequest
	done, total int
}

// started reports that a layer is about to load
func (l *frameLoads) started() {
	l.req.Report(stepFrame, fracTextTo+(fracLoadTo-fracTextTo)*float64(l.done)/float64(max(l.total, 1)))
	l.done++
}

// Render composites the frame layers, the card art, and the text boxes into a
// single buffer, drawing with the layout the template has for the face's shape. It
// returns an error rather than panicking on a missing layer image or an invalid
// document, so one bad card does not take down a batch render, and an
// *template.UnsupportedError for a face the template has no layout for
//
// A split card is drawn from one layout that scopes its entries and text boxes to
// a face. Each half reads its own face, so the halves frame in their own colors,
// and the card's one art image is cut in two for the art windows. The frame is laid
// out in the layout's authored canvas and turned by its rotation, after which a
// box in the output space is drawn upright in the delivered one
func Render(ctx context.Context, t *template.Template, req template.RenderRequest) (*raster.Buffer, error) {
	if t == nil {
		return nil, fmt.Errorf("render: no template")
	}
	if req.Card == nil {
		return nil, fmt.Errorf("render: request has no card")
	}
	// Conditions and opacities read feature values the template offers, held to
	// their ranges, so a caller's stale or out of range choice cannot reach them
	req.Features = t.ResolveFeatures(req.Features)
	shapes := template.Classify(req.Card)
	if req.Face < 0 || req.Face >= len(shapes) {
		return nil, fmt.Errorf("render: face %d out of range, %q renders %d", req.Face, req.Card.Name, len(shapes))
	}
	layout, ok := t.Layout(shapes[req.Face])
	if !ok {
		return nil, &template.UnsupportedError{Template: t.ID(), Shape: shapes[req.Face]}
	}
	// Everything below works in the scaled layout's coordinates, so a preview and a
	// print-ready export run the same layout over smaller numbers rather than down
	// one path each
	geo := layout.Geometry(req.DPI)
	scale := geo.Scale
	authored := image.Pt(layout.Canvas.Width, layout.Canvas.Height)
	req.Report(stepManifest, fracManifest)

	d := req.FaceCard()
	f := frame.DeriveFace(d, req.FaceSide())
	symSpec, drawSymbol := setSymbolSpec(req, layout, geo, f)
	f.SetSymbol = drawSymbol
	fc := newFaces(req, shapes, d, f)

	// Of the specs a text box offers, the override whose condition holds draws, and
	// every lookup by box key below sees it. Each box resolves against its own face
	boxes := resolveBoxes(layout, geo, fc, req.Features)

	var nodes []canvas.Node
	loads := &frameLoads{req: req}
	for i, e := range layout.Stack {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req.Report(stepFrame, lerp(fracFrameFrom, fracFrameTo, i, len(layout.Stack)))
		_, k := fc.get(e.Face)
		node, err := layerNode(ctx, loads, t, req, geo, authored, e, k, scale)
		if err != nil {
			return nil, err
		}
		if node != nil {
			nodes = append(nodes, node)
			loads.total++
		}

		// The art takes its place in the stack whether or not the entry it follows
		// draws, so a template with a background per face can name the last of them
		if req.Art == nil {
			continue
		}
		for slotIdx, slot := range geo.Art {
			if slot.After != e.ID {
				continue
			}
			src := req.Art
			if len(geo.Art) > 1 {
				src = template.HalfOfArt(req.Art, slotIdx, len(geo.Art))
			}
			fw, fh := slot.Width, slot.Height
			if quarter(slot.Rotate) {
				fw, fh = fh, fw
			}
			art := template.FitArt(src, fw, fh)
			artBuf, err := raster.FromImage(art)
			if err != nil {
				return nil, fmt.Errorf("render: wrapping art: %w", err)
			}
			artBuf = template.Rotate(artBuf, slot.Rotate)
			nodes = append(nodes, &canvas.Layer{Content: artBuf, Origin: image.Pt(slot.X, slot.Y), Mode: blend.Normal})
		}
	}

	// One renderer serves every box, so its faces and rasterized pips are built
	// once for the card rather than once per box. The artist credit draws from
	// that same font but in its own box color, so it takes a renderer beside it
	var syms symbols
	if ms := mana.NewSymbols(req.FontDir); ms != nil {
		syms.mana = ms
		if box, ok := boxes.byDraws("artist"); ok {
			syms.artist = mana.ArtistNib{Sym: ms, Ink: template.ParseHexColor(box.spec.Color)}
		}
	}

	if drawSymbol {
		node, err := symbolNode(req.SetSymbol, symSpec)
		if err != nil {
			return nil, err
		}
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	var outNodes []canvas.Node
	for i, job := range boxes.jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req.Report(stepText, lerp(fracTextFrom, fracTextTo, i, len(boxes.jobs)))
		docW, docH := geo.Width, geo.Height
		if job.space == template.SpaceOutput {
			docW, docH = geo.DeliveredWidth, geo.DeliveredHeight
		}
		got, err := drawBox(req, t, layout, authored, job, boxes, fc, syms, scale, docW, docH)
		if err != nil {
			return nil, err
		}
		if job.space == template.SpaceOutput {
			outNodes = append(outNodes, got...)
		} else {
			nodes = append(nodes, got...)
		}
	}

	// A pass-through root is the correct default. Nothing sits beneath the
	// document root, so pass-through and isolated behave the same here
	doc := &canvas.Document{
		Width:  geo.Width,
		Height: geo.Height,
		Root:   canvas.Group{PassThrough: true, Layers: nodes},
	}
	out, err := canvas.Render(doc)
	if err != nil {
		return nil, err
	}
	req.Report(stepFinalize, fracFinalize)
	if layout.Canvas.Rotate == 0 {
		return out, nil
	}
	out = template.Rotate(out, layout.Canvas.Rotate)
	if len(outNodes) == 0 {
		return out, nil
	}
	turned := &canvas.Document{
		Width:  out.Width,
		Height: out.Height,
		Root: canvas.Group{PassThrough: true, Layers: append(
			[]canvas.Node{&canvas.Layer{Content: out, Mode: blend.Normal}}, outNodes...)},
	}
	return canvas.Render(turned)
}

// faces is what each face a layout can scope something to prints and frames from.
// The face being drawn is always known, and the others are read the first time
// something asks for them
type faces struct {
	req    template.RenderRequest
	shapes []frame.Shape
	self   faceData
	half   [2]faceData
	other  faceData
}

type faceData struct {
	data *card.Data
	keys frame.Keys
	read bool
}

func newFaces(req template.RenderRequest, shapes []frame.Shape, d *card.Data, f frame.Keys) *faces {
	return &faces{req: req, shapes: shapes, self: faceData{d, f, true}}
}

// get returns what a face prints and its frame keys. The names are the manifest's:
// "self" or empty is the face being drawn, "first" and "second" are the halves of a
// split card, and "other" is the face of a double-faced card that is not being
// drawn. A face the card does not have is the face being drawn
func (fc *faces) get(face string) (*card.Data, frame.Keys) {
	switch face {
	case "first":
		return fc.readHalf(0)
	case "second":
		return fc.readHalf(1)
	case "other":
		return fc.readOther()
	}
	return fc.self.data, fc.self.keys
}

func (fc *faces) readHalf(i int) (*card.Data, frame.Keys) {
	h := &fc.half[i]
	if !h.read {
		h.data = fc.self.data.Half(i)
		h.keys = frame.DeriveFace(h.data, frame.Single)
		h.keys.SetSymbol = fc.self.keys.SetSymbol
		h.read = true
	}
	return h.data, h.keys
}

func (fc *faces) readOther() (*card.Data, frame.Keys) {
	if !fc.other.read {
		fc.other.read = true
		if len(fc.shapes) == 2 && (fc.req.Face == 0 || fc.req.Face == 1) {
			o := 1 - fc.req.Face
			fc.other.data = fc.req.Card.Face(o)
			fc.other.keys = frame.DeriveFace(fc.other.data, fc.shapes[o].Side())
			fc.other.keys.SetSymbol = fc.self.keys.SetSymbol
		} else {
			fc.other.data, fc.other.keys = fc.self.data, fc.self.keys
		}
	}
	return fc.other.data, fc.other.keys
}

// resolvedBox is one text box as it draws for this card
type resolvedBox struct {
	key     string
	draws   string
	face    string
	space   string
	spec    template.TextBoxSpec
	divider string
}

// boxSet is a layout's text boxes after resolution for a card
type boxSet struct {
	byKey map[string]resolvedBox
	// jobs lists every box in draw order, the face being drawn first and then the
	// first and second halves, each in sorted key order
	jobs []resolvedBox
}

func (b boxSet) byDraws(name string) (resolvedBox, bool) {
	for _, j := range b.jobs {
		if j.draws == name {
			return j, true
		}
	}
	return resolvedBox{}, false
}

// faceOrder is the order boxes are drawn in by the face they read
var faceOrder = []string{"self", "first", "second", "other"}

// resolveBoxes picks each text box's override for the card, and drops the boxes
// whose condition does not hold
func resolveBoxes(layout *template.Layout, geo *template.Geometry, fc *faces, features frame.Features) boxSet {
	out := boxSet{byKey: map[string]resolvedBox{}}
	for _, face := range faceOrder {
		for _, b := range layout.Boxes {
			bf := b.Face
			if bf == "" {
				bf = "self"
			}
			if bf != face {
				continue
			}
			_, k := fc.get(b.Face)
			if !b.Condition.Met(k, features) {
				continue
			}
			sb := geo.Boxes[b.Key]
			spec, divider := sb.Base, b.Base.Flow.Divider
			if i, ok := b.Pick(func(c frame.Condition) bool { return c.Met(k, features) }); ok {
				spec, divider = sb.Overrides[i], b.Overrides[i].Box.Flow.Divider
			}
			rb := resolvedBox{key: b.Key, draws: b.Draws, face: b.Face, space: b.Space, spec: spec, divider: divider}
			out.byKey[b.Key] = rb
			out.jobs = append(out.jobs, rb)
		}
	}
	return out
}

// drawBox lays one text box out and returns the nodes it draws, the text and a
// flavor divider when the box carries one. docW and docH are the canvas the box
// is laid out in, which is the delivered one for a box in the output space
func drawBox(req template.RenderRequest, t *template.Template, layout *template.Layout, authored image.Point, job resolvedBox, boxes boxSet, fc *faces, syms symbols, scale template.Scale, docW, docH int) ([]canvas.Node, error) {
	box := job.spec
	d, _ := fc.get(job.face)
	parts := textParts(job.draws, box, d, syms, req.FontDir, req.Copyright)
	if len(parts) == 0 {
		return nil, nil
	}
	bounds := func(id string) (image.Rectangle, bool) {
		e, ok := layout.Entry(id)
		if !ok {
			return image.Rectangle{}, false
		}
		_, k := fc.get(e.Face)
		return template.EntryBounds(t.Assets(), e, k, req.Features, scale, authored.X, authored.Y)
	}
	if box.DuckLayer != "" && box.DuckRow != "" {
		if _, ok := bounds(box.DuckLayer); ok {
			rows := map[string]template.TextBoxSpec{}
			for k, b := range boxes.byKey {
				rows[k] = b.spec
			}
			box = template.MoveToRow(box, rows, box.DuckRow)
		}
	}
	if box.ClearOf != "" {
		if other, ok := boxes.byKey[box.ClearOf]; ok {
			od, _ := fc.get(other.face)
			if otherParts := textParts(other.draws, other.spec, od, syms, req.FontDir, req.Copyright); len(otherParts) > 0 {
				span, err := template.MeasureTextSpan(other.spec, otherParts...)
				if err != nil {
					return nil, fmt.Errorf("render: measuring %q to clear: %w", box.ClearOf, err)
				}
				box = template.ClearOf(box, span)
			}
		}
	}
	if box.AvoidLayer != "" {
		if r, ok := bounds(box.AvoidLayer); ok {
			box.Avoid = r
		}
	}
	// A turned box lays its text out in the rect turned a quarter about its middle,
	// and the finished text is turned back about that same middle
	cx2, cy2 := 2*box.X+box.Width, 2*box.Y+box.Height
	if quarter(box.Rotate) {
		box.Width, box.Height = box.Height, box.Width
		box.X, box.Y = (cx2-box.Width)/2, (cy2-box.Height)/2
	}
	res, err := template.RenderTextBox(box, docW, docH, parts...)
	if err != nil {
		return nil, fmt.Errorf("render: rendering text box %q: %w", job.key, err)
	}
	buf, err := raster.FromImage(res.Image)
	if err != nil {
		return nil, fmt.Errorf("render: wrapping text box %q: %w", job.key, err)
	}
	origin := res.Image.Bounds().Min
	if box.Rotate != 0 {
		buf = template.Rotate(buf, box.Rotate)
		origin = template.TurnRect(res.Image.Bounds(), cx2, cy2, box.Rotate).Min
	}
	text := &canvas.Layer{Content: buf, Origin: origin, Mode: blend.Normal}
	if box.Shadow != nil {
		text.Effects = []effects.Effect{box.Shadow.Effect(box.FontSize)}
	}
	out := []canvas.Node{text}

	if res.HasDivider && job.divider != "" {
		part, ok := layout.Part(job.divider)
		if !ok {
			return nil, fmt.Errorf("render: text box %q names a divider part %q the layout does not have", job.key, job.divider)
		}
		div, err := template.Divider(t.Assets(), part.Variants["any"], res.DividerY, scale)
		if err != nil {
			return nil, err
		}
		if div != nil {
			out = append(out, div)
		}
	}
	return out, nil
}

// quarter reports whether a turn of degrees swaps width and height
func quarter(degrees int) bool { return (degrees/90)%2 != 0 }

// lerp reports the cumulative fraction at item i of n across a phase spanning
// [from, to], stepping to the end as items complete. n <= 0 yields from
func lerp(from, to float64, i, n int) float64 {
	if n <= 0 {
		return from
	}
	return from + (to-from)*float64(i+1)/float64(n)
}

// layerNode builds one stack entry's node for this card, or returns nil when its
// condition does not hold or no variant applies to the card's color. The node
// decodes its PNG when the compositor reaches it and lets go of the pixels once
// blended, so a render holds one frame layer at a time rather than all of them
func layerNode(ctx context.Context, loads *frameLoads, t *template.Template, req template.RenderRequest, geo *template.Geometry, authored image.Point, e *template.Entry, k frame.Keys, scale template.Scale) (canvas.Node, error) {
	if !e.Condition.Met(k, req.Features) {
		return nil, nil
	}
	opacity := e.Opacity
	if e.OpacityFeature != "" {
		opacity, _ = req.Features[e.OpacityFeature].(float64)
	}
	// The compositor reads an opacity of zero as opaque, so an entry that is fully
	// transparent is left out
	if opacity <= 0 {
		return nil, nil
	}
	material := k.Material(e.Part.Slot)
	variants := e.Part.Variants
	var blended []string
	var blender template.Blender
	if _, whole := variants[material.Key()]; !whole {
		if paths := template.BlendedPaths(variants, material); paths != nil {
			if bl, ok := template.NewBlender(len(paths), e.Part.Blend, authored.X); ok {
				blended, blender = paths, bl.Shifted(e.Origin().X-e.Part.Origin.X, authored.X)
			}
		}
	}
	var asset string
	if blended == nil {
		var ok bool
		if asset, ok = variants[material.Key()]; !ok {
			if asset, ok = variants["any"]; !ok {
				// No variant applies to this color. Skipping rather than erroring
				// lets a manifest leave a layer out where it does not apply
				return nil, nil
			}
		}
	}
	at := e.Origin()
	mirror := e.Mirror != nil && e.Mirror.Condition.Met(k, req.Features)
	assets := t.Assets()

	// An entry that is not mirrored is only composited, so its image goes to the
	// compositor as it is, shared and with its index of visible pixels, and no float
	// buffer is built for it. A blend is built once as an image and shared the same way
	if !mirror {
		return &canvas.Layer{Mode: e.Mode, Opacity: float32(opacity), LoadImage: func() (image.Image, image.Point, error) {
			if err := ctx.Err(); err != nil {
				return nil, image.Point{}, err
			}
			loads.started()
			if blended != nil {
				return template.LoadBlendedImage(assets, blended, blender, scale, at.X, at.Y, authored.X, authored.Y, true)
			}
			return template.LoadLayerImage(assets, asset, scale, at.X, at.Y, authored.X, authored.Y)
		}}, nil
	}

	// A mirror rewrites pixels, so it works on a buffer of its own
	return &canvas.Layer{Mode: e.Mode, Opacity: float32(opacity), Load: func() (*raster.Buffer, image.Point, error) {
		if err := ctx.Err(); err != nil {
			return nil, image.Point{}, err
		}
		loads.started()
		if blended != nil {
			img, origin, err := template.LoadBlendedImage(assets, blended, blender, scale, at.X, at.Y, authored.X, authored.Y, false)
			if err != nil || img == nil {
				return nil, origin, err
			}
			buf, err := raster.FromImage(img)
			return mirrored(buf, origin, err, true, e, geo, scale)
		}
		buf, origin, err := template.LoadLayerAt(assets, asset, scale, at.X, at.Y, authored.X, authored.Y)
		return mirrored(buf, origin, err, true, e, geo, scale)
	}}, nil
}

// mirrored flips a loaded layer left to right when its mirror holds, and
// otherwise returns it as it is. A mirror flips about the document's centre, so
// the layer takes the whole document to flip in
func mirrored(buf *raster.Buffer, origin image.Point, err error, mirror bool, e *template.Entry, geo *template.Geometry, scale template.Scale) (*raster.Buffer, image.Point, error) {
	if err != nil || !mirror || buf == nil {
		return buf, origin, err
	}
	placed := canvas.Place(geo.Width, geo.Height, buf, origin.X, origin.Y)
	template.Mirror(placed, scale.Rect(e.Mirror.Region))
	return placed, image.Point{}, nil
}

// roleFor returns the font role a box's manifest Font names, defaulting to the
// body role for a box that names none or an unrecognized one
func roleFor(font string) fonts.Role {
	if role, ok := fonts.ParseRole(font); ok {
		return role
	}
	return fonts.Body
}

// basicFontReplacer maps glyphs basicfont.Face7x13 lacks to ones it has
var basicFontReplacer = strings.NewReplacer("—", "-", "–", "-")

// normalizeForBasicFont rewrites text so it renders on the basicfont fallback,
// which lacks glyphs the embedded fonts carry. All text display applies it when
// a box falls back to that face, and skips it otherwise so a real font keeps
// its own glyphs
func normalizeForBasicFont(s string) string {
	return basicFontReplacer.Replace(s)
}

// symbols are the renderers a card's boxes draw braced codes through: the pips
// for the cost and the rules text, the nib for the artist credit. Either may be
// nil, which leaves a code as its literal characters
type symbols struct {
	mana   template.SymbolRenderer
	artist template.SymbolRenderer
}

// textParts returns the styled parts for a named box. Most boxes are one part
// in the box's manifest-declared font role. The oracle box is rules in the
// body role then flavor in the italic role, which RenderTextBox separates with
// a divider. The mana cost and the rules text are where a card's own symbols
// appear, and the artist credit opens with the nib the engine prepends, so
// those are the parts that carry a renderer
func textParts(name string, box template.TextBoxSpec, d *card.Data, syms symbols, fontDir, copyrightOverride string) []template.TextPart {
	if name == "oracle" {
		var parts []template.TextPart
		if p, ok := part(d.OracleText, fonts.Body, fontDir); ok {
			// Parenthesized reminder text and leading ability or flavor words
			// italicize, while keyword abilities stay roman
			p.Emph = fonts.ResolveFont(fonts.BodyItalic, fontDir)
			p.EmphLead = card.EmphasisWords
			p.Sym = syms.mana
			parts = append(parts, p)
		}
		if p, ok := part(d.FlavorText, fonts.BodyItalic, fontDir); ok {
			parts = append(parts, p)
		}
		return parts
	}
	text := card.TextFor(name, d)
	switch {
	case name == "copyright":
		text = card.CopyrightLine(copyrightOverride, d)
	case name == "artist" && syms.artist != nil && text != "":
		// The nib sits flush against the name, so the code carries no space and
		// the symbol's own advance opens the gap
		text = "{" + mana.NibCode + "}" + text
	}
	p, ok := part(text, roleFor(box.Font), fontDir)
	if !ok {
		return nil
	}
	switch name {
	case "mana":
		p.Sym = syms.mana
	case "artist":
		p.Sym = syms.artist
	}
	return []template.TextPart{p}
}

// part resolves a font for role and pairs it with text, normalizing the text
// when the font falls back to the basic face. It reports false for empty text
func part(text string, role fonts.Role, fontDir string) (template.TextPart, bool) {
	if strings.TrimSpace(text) == "" {
		return template.TextPart{}, false
	}
	sizer := fonts.ResolveFont(role, fontDir)
	if sizer.Fallback() {
		text = normalizeForBasicFont(text)
	}
	return template.TextPart{Text: text, Src: sizer}, true
}
