// Package render implements a manifest-driven renderer for any WUBRG-colored
// Magic frame: one whose layers key off engine/frame's five color slots and
// whose text boxes use the standard metadata vocabulary (title, mana, type,
// oracle, pt, artist, collector, set, copyright). A template built on this
// vocabulary needs no Go of its own beyond registering its name; its look
// comes entirely from its manifest
package render

import (
	"context"
	"fmt"
	"sort"
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
// are for feedback, not timing: the frame loop does the PNG decodes and is the
// heaviest, so it owns the widest band
const (
	stepManifest = "Reading template"
	stepFrame    = "Compositing frame"
	stepText     = "Rendering text"
	stepFinalize = "Finalizing"

	fracManifest  = 0.02
	fracFrameFrom = 0.05
	fracFrameTo   = 0.55
	fracTextFrom  = 0.55
	fracTextTo    = 0.85
	fracFinalize  = 0.9
)

// Template renders a card against any WUBRG-frame manifest. name is what
// Name() reports, which is also the name it was registered under
type Template struct {
	name string
}

// New builds a Template that reports name from Name(). It carries no other
// state: FontDir and Copyright travel on the RenderRequest, since they are
// caller configuration rather than anything the frame itself carries
func New(name string) *Template { return &Template{name: name} }

// Name reports the registry name this template was constructed with
func (t *Template) Name() string { return t.name }

// Render composites the frame layers, the card art, and the text boxes into a
// single buffer. It returns an error rather than panicking on a missing
// manifest, a missing layer PNG, or an invalid document, so one bad card does
// not take down a batch render
//
// A split card is drawn from one manifest that scopes its layers and text boxes
// to a half. Each half reads its own face, so the halves frame in their own
// colors, and the card's one art image is cut in two for the art windows. The
// frame is laid out in the manifest's authored canvas and turned by its Rotate,
// after which a box in the output space is drawn upright in the delivered one
func (t *Template) Render(ctx context.Context, req template.RenderRequest) (*raster.Buffer, error) {
	if req.Card == nil {
		return nil, fmt.Errorf("render: request has no card")
	}
	if req.Assets == nil {
		return nil, fmt.Errorf("render: request has no asset provider")
	}
	m, err := req.Assets.Manifest()
	if err != nil {
		return nil, err
	}
	// Everything below works in the scaled manifest's coordinates, so a preview
	// and a print-ready export run the same layout over smaller numbers rather
	// than down one path each
	scale := m.ScaleForDPI(req.DPI)
	m = m.Scaled(scale)
	req.Report(stepManifest, fracManifest)

	d := req.FaceCard()
	f := frame.DeriveFace(d, req.FaceSide())
	symSpec, drawSymbol := setSymbolSpec(req, m, f)
	f.SetSymbol = drawSymbol
	halves := newHalves(d, f, m)

	// Of the specs a manifest offers for one box, only the one whose condition
	// holds for the face it is scoped to draws, and every lookup by box name
	// below sees it. Each half resolves its boxes against its own face
	boxes := resolveBoxes(m.TextBoxes, halves)

	layersByName := make(map[string]template.LayerSpec, len(m.Layers))
	for _, l := range m.Layers {
		layersByName[l.Name] = l
	}

	var nodes []canvas.Node
	artSlots := m.ArtSlots()
	for i, layer := range m.Layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req.Report(stepFrame, lerp(fracFrameFrom, fracFrameTo, i, len(m.Layers)))
		node, err := layerNode(req, m, layer, layersByName, halves.keys(layer.Half), scale)
		if err != nil {
			return nil, err
		}
		if node != nil {
			nodes = append(nodes, node)
		}

		// The art takes its place in the stack whether or not the layer it
		// follows draws for this card, so a template with a background per face
		// can name the last of them
		if req.Art == nil {
			continue
		}
		for slotIdx, slot := range artSlots {
			if slot.After != layer.Name {
				continue
			}
			src := req.Art
			if len(m.Arts) > 0 {
				src = template.HalfOfArt(req.Art, slotIdx, len(m.Arts))
			}
			art := template.FitArt(src, slot.Width, slot.Height)
			artBuf, err := raster.FromImage(art)
			if err != nil {
				return nil, fmt.Errorf("render: wrapping art: %w", err)
			}
			placedArt := canvas.Place(m.Width, m.Height, artBuf, slot.X, slot.Y)
			nodes = append(nodes, &canvas.Layer{Content: placedArt, Mode: blend.Normal})
		}
	}

	// One renderer serves every box, so its faces and rasterized pips are built
	// once for the card rather than once per box. The artist credit draws from
	// that same font but in its own box color, so it takes a renderer beside it
	var syms symbols
	if ms := mana.NewSymbols(req.FontDir); ms != nil {
		syms.mana = ms
		if box, ok := boxes.whole()["artist"]; ok {
			syms.artist = mana.ArtistNib{Sym: ms, Ink: template.ParseHexColor(box.Color)}
		}
	}

	if drawSymbol {
		node, err := symbolNode(req.SetSymbol, symSpec, d.Rarity, m.Width, m.Height)
		if err != nil {
			return nil, err
		}
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	jobs := boxes.jobs()
	var outNodes []canvas.Node
	for i, job := range jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req.Report(stepText, lerp(fracTextFrom, fracTextTo, i, len(jobs)))
		docW, docH := m.Width, m.Height
		if job.box.Space == template.SpaceOutput {
			docW, docH = m.DeliveredWidth(), m.DeliveredHeight()
		}
		got, err := t.drawBox(req, m, job, boxes, layersByName, halves, syms, scale, docW, docH)
		if err != nil {
			return nil, err
		}
		if job.box.Space == template.SpaceOutput {
			outNodes = append(outNodes, got...)
		} else {
			nodes = append(nodes, got...)
		}
	}

	// A pass-through root is the correct default. Nothing sits beneath the
	// document root, so pass-through and isolated behave the same here
	req.Report(stepFinalize, fracFinalize)
	doc := &canvas.Document{
		Width:  m.Width,
		Height: m.Height,
		Root:   canvas.Group{PassThrough: true, Layers: nodes},
	}
	out, err := canvas.Render(doc)
	if err != nil {
		return nil, err
	}
	if m.Rotate == 0 {
		return out, nil
	}
	out = template.Rotate(out, m.Rotate)
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

// drawBox lays one text box out and returns the nodes it draws, the text and a
// flavor divider when the box carries one. docW and docH are the canvas the box
// is laid out in, which is the delivered one for a box in the output space
func (t *Template) drawBox(req template.RenderRequest, m *template.Manifest, job boxJob, boxes boxSet, layersByName map[string]template.LayerSpec, halves halfSet, syms symbols, scale template.Scale, docW, docH int) ([]canvas.Node, error) {
	name, box := job.name, job.box
	d, f := halves.data(job.half), halves.keys(job.half)
	group := boxes[job.half]
	parts := textParts(name, box, d, syms, req.FontDir, req.Copyright)
	if len(parts) == 0 {
		return nil, nil
	}
	if box.DuckLayer != "" && box.DuckRow != "" {
		if _, ok := template.AvoidRect(req.Assets, layersByName, f, box.DuckLayer, scale); ok {
			box = template.MoveToRow(box, &template.Manifest{TextBoxes: group}, box.DuckRow)
		}
	}
	if box.ClearOf != "" {
		if otherBox, ok := group[box.ClearOf]; ok {
			if otherParts := textParts(box.ClearOf, otherBox, d, syms, req.FontDir, req.Copyright); len(otherParts) > 0 {
				span, err := template.MeasureTextSpan(otherBox, otherParts...)
				if err != nil {
					return nil, fmt.Errorf("render: measuring %q to clear: %w", box.ClearOf, err)
				}
				box = template.ClearOf(box, span)
			}
		}
	}
	if box.AvoidLayer != "" {
		if r, ok := template.AvoidRect(req.Assets, layersByName, f, box.AvoidLayer, scale); ok {
			box.Avoid = r
		}
	}
	res, err := template.RenderTextBox(box, docW, docH, parts...)
	if err != nil {
		return nil, fmt.Errorf("render: rendering text box %q: %w", name, err)
	}
	buf, err := raster.FromImage(res.Image)
	if err != nil {
		return nil, fmt.Errorf("render: wrapping text box %q: %w", name, err)
	}
	text := &canvas.Layer{Content: buf, Mode: blend.Normal}
	if box.Shadow != nil {
		text.Effects = []effects.Effect{box.Shadow.Effect(box.FontSize)}
	}
	out := []canvas.Node{text}

	if res.HasDivider {
		div, err := template.Divider(req.Assets, m, res.DividerY, scale)
		if err != nil {
			return nil, err
		}
		if div != nil {
			out = append(out, div)
		}
	}
	return out, nil
}

// halfSet is what each half of a split card prints and frames from, beside the
// whole card's own. A manifest with no half scoping never reads the halves
type halfSet struct {
	whole     *card.Data
	wholeKeys frame.Keys
	halfData  [2]*card.Data
	halfKeys  [2]frame.Keys
}

// newHalves reads each half's face and keys, only when the manifest scopes
// something to a half
func newHalves(d *card.Data, f frame.Keys, m *template.Manifest) halfSet {
	h := halfSet{whole: d, wholeKeys: f}
	if !usesHalves(m) {
		return h
	}
	for i := range h.halfData {
		h.halfData[i] = d.Half(i)
		h.halfKeys[i] = frame.DeriveFace(h.halfData[i], frame.Single)
		h.halfKeys[i].SetSymbol = f.SetSymbol
	}
	return h
}

func usesHalves(m *template.Manifest) bool {
	for _, l := range m.Layers {
		if l.Half != 0 {
			return true
		}
	}
	for _, b := range m.TextBoxes {
		if b.Half != 0 {
			return true
		}
	}
	return false
}

// data is the face half h prints, where 0 is the whole card
func (h halfSet) data(half int) *card.Data {
	if half <= 0 || half > len(h.halfData) || h.halfData[half-1] == nil {
		return h.whole
	}
	return h.halfData[half-1]
}

// keys are the frame keys of half h, where 0 is the whole card
func (h halfSet) keys(half int) frame.Keys {
	if half <= 0 || half > len(h.halfKeys) || h.halfData[half-1] == nil {
		return h.wholeKeys
	}
	return h.halfKeys[half-1]
}

// boxSet is a manifest's text boxes after resolution, one map per half, with
// index 0 holding the boxes of the whole card. Each is keyed by logical box name
type boxSet [3]map[string]template.TextBoxSpec

// resolveBoxes picks each logical box's spec for every half it is scoped to
func resolveBoxes(specs map[string]template.TextBoxSpec, halves halfSet) boxSet {
	var groups [3]map[string]template.TextBoxSpec
	for key, spec := range specs {
		if groups[spec.Half] == nil {
			groups[spec.Half] = map[string]template.TextBoxSpec{}
		}
		groups[spec.Half][key] = spec
	}
	var out boxSet
	for h, group := range groups {
		if len(group) > 0 {
			out[h] = template.ResolveTextBoxes(group, halves.keys(h))
		}
	}
	return out
}

// whole is the boxes of the whole card
func (b boxSet) whole() map[string]template.TextBoxSpec { return b[0] }

// boxJob is one resolved text box to draw
type boxJob struct {
	half int
	name string
	box  template.TextBoxSpec
}

// jobs lists every box in draw order, the whole card's first and then each
// half's, each in sorted name order
func (b boxSet) jobs() []boxJob {
	var out []boxJob
	for h, group := range b {
		for _, name := range sortedKeys(group) {
			out = append(out, boxJob{half: h, name: name, box: group[name]})
		}
	}
	return out
}

// lerp reports the cumulative fraction at item i of n across a phase spanning
// [from, to], stepping to the end as items complete. n <= 0 yields from
func lerp(from, to float64, i, n int) float64 {
	if n <= 0 {
		return from
	}
	return from + (to-from)*float64(i+1)/float64(n)
}

// layerNode loads one frame layer's variant for this card, or returns nil when
// its condition does not hold or no variant applies to the card's color
func layerNode(req template.RenderRequest, m *template.Manifest, layer template.LayerSpec, layersByName map[string]template.LayerSpec, f frame.Keys, scale template.Scale) (canvas.Node, error) {
	if !f.ConditionMet(layer.Condition) {
		return nil, nil
	}
	// An enchantment draws the nyx frame in the background slot, so it sits
	// below the art and follows the same color key as the background
	variants := layer.ColorVariants
	if layer.ColorSlot == "background" && f.Nyx {
		if nyx, ok := layersByName["nyx"]; ok {
			variants = nyx.ColorVariants
		}
	}
	key := f.Slot(layer.ColorSlot)
	x, y := scale.Px(layer.X), scale.Px(layer.Y)
	var placed *raster.Buffer
	var err error
	if blend := blendedPaths(layer, variants, key); blend != nil {
		placed, err = template.LoadBlendedLayer(req.Assets, blend, m.Width, m.Height, scale, x, y)
	} else {
		asset, ok := variants[key]
		if !ok {
			asset, ok = variants["any"]
		}
		if !ok {
			// No variant applies to this color. Skipping rather than erroring
			// lets a manifest leave a layer out where it does not apply
			return nil, nil
		}
		placed, err = template.LoadLayerAt(req.Assets, asset.Path, m.Width, m.Height, scale, x, y)
	}
	if err != nil {
		return nil, err
	}
	if layer.Mirror != nil && f.ConditionMet(layer.Mirror.Condition) {
		template.Mirror(placed, scale.Rect(layer.Mirror.Region()))
	}
	mode, err := template.BlendMode(layer.Blend)
	if err != nil {
		return nil, err
	}
	return &canvas.Layer{Content: placed, Mode: mode}, nil
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

func sortedKeys(m map[string]template.TextBoxSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// blendedPaths is the variants a layer blends for a color key of several
// letters that has no variant of its own, or nil for a layer that does not blend
// or a key that is not such a blend
func blendedPaths(layer template.LayerSpec, variants map[string]template.LayerAsset, key string) []string {
	if !layer.ColorBlend {
		return nil
	}
	if _, whole := variants[key]; whole {
		return nil
	}
	return template.BlendedPaths(variants, key)
}
