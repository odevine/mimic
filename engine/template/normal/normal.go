package normal

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
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

const templateName = "normal"

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

func init() {
	template.Register(templateName, func() template.Template { return &Template{} })
}

// Template renders a card with the Normal frame. It carries no state of its
// own: FontDir and Copyright travel on the RenderRequest, since they are
// caller configuration rather than anything specific to this frame
type Template struct{}

// hollowCrownEnabled turns on the nyx hollow-crown knockout. Off for now while a
// missing layer is tracked down, the knockout code stays in place for the revisit
const hollowCrownEnabled = false

// boxRoles maps each named text box to the font role it draws in. Names not
// listed draw in the body role
var boxRoles = map[string]fonts.Role{
	"title":     fonts.Title,
	"type":      fonts.Title,
	"pt":        fonts.Title,
	"mana":      fonts.Body,
	"oracle":    fonts.Body,
	"artist":    fonts.SmallCaps,
	"collector": fonts.Info,
	"set":       fonts.Info,
	"copyright": fonts.Body,
}

// Name reports the registry name this template is registered under
func (t *Template) Name() string { return templateName }

// Render composites the frame layers, the card art, and the text boxes into a
// single buffer. It returns an error rather than panicking on a missing
// manifest, a missing layer PNG, or an invalid document, so one bad card does
// not take down a batch render
func (t *Template) Render(ctx context.Context, req template.RenderRequest) (*raster.Buffer, error) {
	if req.Card == nil {
		return nil, fmt.Errorf("normal: render request has no card")
	}
	if req.Assets == nil {
		return nil, fmt.Errorf("normal: render request has no asset provider")
	}
	m, err := req.Assets.Manifest()
	if err != nil {
		return nil, err
	}
	req.Report(stepManifest, fracManifest)

	f := frame.Derive(req.Card)

	layersByName := make(map[string]template.LayerSpec, len(m.Layers))
	for _, l := range m.Layers {
		layersByName[l.Name] = l
	}

	var nodes []canvas.Node
	for i, layer := range m.Layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req.Report(stepFrame, lerp(fracFrameFrom, fracFrameTo, i, len(m.Layers)))
		if !conditionMet(f, layer.Condition) {
			continue
		}
		// An enchantment draws the nyx frame in the background slot, so it sits
		// below the art and follows the same color key as the background
		variants := layer.ColorVariants
		if layer.Name == "background" && f.Nyx {
			if nyx, ok := layersByName["nyx"]; ok {
				variants = nyx.ColorVariants
			}
		}
		asset, ok := variants[keyForLayer(f, layer.Name)]
		if !ok {
			asset, ok = variants["any"]
		}
		if !ok {
			// No variant applies to this color. Skipping rather than erroring
			// lets a manifest leave a layer out where it does not apply
			continue
		}
		placed, err := template.LoadLayer(req.Assets, asset.Path, m.Width, m.Height)
		if err != nil {
			return nil, err
		}
		mode, err := template.BlendMode(layer.Blend)
		if err != nil {
			return nil, err
		}

		// An enchantment legend gets a hollow crown: erase the crown and the
		// shadow beneath it where the nyx frame shows through, keeping the edge.
		// Off pending a missing layer, the knockout code is kept for that revisit
		if hollowCrownEnabled && (layer.Name == "legendary_crown" || layer.Name == "shadows") && f.Nyx && f.Legendary {
			if err := knockoutHollowRegion(req.Assets, m, placed); err != nil {
				return nil, err
			}
		}

		nodes = append(nodes, &canvas.Layer{Content: placed, Mode: mode})

		if req.Art != nil && m.Art.After == layer.Name {
			art := template.FitArt(req.Art, m.Art.Width, m.Art.Height)
			artBuf, err := raster.FromImage(art)
			if err != nil {
				return nil, fmt.Errorf("normal: wrapping art: %w", err)
			}
			placedArt := canvas.Place(m.Width, m.Height, artBuf, m.Art.X, m.Art.Y)
			nodes = append(nodes, &canvas.Layer{Content: placedArt, Mode: blend.Normal})
		}
	}

	// One renderer serves every box, so its faces and rasterized pips are built
	// once for the card rather than once per box. The artist credit draws from
	// that same font but in its own box color, so it takes a renderer beside it
	var syms symbols
	if ms := mana.NewSymbols(req.FontDir); ms != nil {
		syms.mana = ms
		if box, ok := m.TextBoxes["artist"]; ok {
			syms.artist = mana.ArtistNib{Sym: ms, Ink: template.ParseHexColor(box.Color)}
		}
	}

	cost, err := costSpan(m, req.Card, syms, req.FontDir)
	if err != nil {
		return nil, fmt.Errorf("normal: measuring the mana cost: %w", err)
	}

	boxNames := sortedKeys(m.TextBoxes)
	for i, name := range boxNames {
		req.Report(stepText, lerp(fracTextFrom, fracTextTo, i, len(boxNames)))
		parts := textParts(name, req.Card, syms, req.FontDir, req.Copyright)
		if len(parts) == 0 {
			continue
		}
		box := m.TextBoxes[name]
		switch {
		case name == "mana":
			box = manaBox(box)
		case name == "title":
			box = titleClearOf(box, cost)
		case name == "copyright" && f.Creature:
			box = copyrightOnArtistRow(box, m)
		case name == "oracle" && f.Creature:
			if pt, ok := ptBoxRect(req.Assets, layersByName, f); ok {
				box.Avoid = pt
			}
		}
		res, err := template.RenderTextBox(box, m.Width, m.Height, parts...)
		if err != nil {
			return nil, fmt.Errorf("normal: rendering text box %q: %w", name, err)
		}
		buf, err := raster.FromImage(res.Image)
		if err != nil {
			return nil, fmt.Errorf("normal: wrapping text box %q: %w", name, err)
		}
		text := &canvas.Layer{Content: buf, Mode: blend.Normal}
		if name == "mana" {
			text.Effects = []effects.Effect{costShadow(box.FontSize)}
		}
		nodes = append(nodes, text)

		if res.HasDivider {
			div, err := template.Divider(req.Assets, m, res.DividerY)
			if err != nil {
				return nil, err
			}
			if div != nil {
				nodes = append(nodes, div)
			}
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
	return canvas.Render(doc)
}

// lerp reports the cumulative fraction at item i of n across a phase spanning
// [from, to], stepping to the end as items complete. n <= 0 yields from
func lerp(from, to float64, i, n int) float64 {
	if n <= 0 {
		return from
	}
	return from + (to-from)*float64(i+1)/float64(n)
}

// ptBoxRect returns where the P/T box graphic draws, so a creature's rules text
// can keep out of it while still using the whole height of its own box. It
// reports false when the P/T box layer or its asset is missing or transparent
func ptBoxRect(p template.AssetProvider, layers map[string]template.LayerSpec, f frame.Keys) (image.Rectangle, bool) {
	spec, ok := layers["pt_box"]
	if !ok {
		return image.Rectangle{}, false
	}
	path := spec.ColorVariants[f.PTBox].Path
	if path == "" {
		path = spec.ColorVariants["any"].Path
	}
	if path == "" {
		return image.Rectangle{}, false
	}
	img, err := template.LoadImage(p, path)
	if err != nil {
		return image.Rectangle{}, false
	}
	b := template.OpaqueBounds(img)
	if b.Empty() {
		return image.Rectangle{}, false
	}
	return b, true
}

// knockoutHollowRegion erases a buffer where the nyx frame should show through a
// hollow crown, leaving the rest intact. It applies to the crown and the shadow
// beneath it, so the sky shows cleanly through the crown's opening. The
// hollow_crown_shadow asset is opaque where the layer is kept and transparent
// where the sky shows, so its alpha is the keep factor. It is a no-op when the
// shadow asset is absent
func knockoutHollowRegion(p template.AssetProvider, m *template.Manifest, buf *raster.Buffer) error {
	path := template.LayerAssetPath(m, "hollow_crown_shadow")
	if path == "" {
		return nil
	}
	shadow, err := template.LoadLayer(p, path, m.Width, m.Height)
	if err != nil {
		return err
	}
	n := len(buf.Pix)
	if len(shadow.Pix) < n {
		n = len(shadow.Pix)
	}
	for j := 0; j+3 < n; j += 4 {
		keep := shadow.Pix[j+3]
		buf.Pix[j] *= keep
		buf.Pix[j+1] *= keep
		buf.Pix[j+2] *= keep
		buf.Pix[j+3] *= keep
	}
	return nil
}

// roleFor returns the font role a named box draws in, defaulting to the body
// role for names boxRoles does not list
func roleFor(name string) fonts.Role {
	if role, ok := boxRoles[name]; ok {
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
// in the box's role. The oracle box is rules in the body role then flavor in
// the italic role, which RenderTextBox separates with a divider. The mana cost
// and the rules text are where a card's own symbols appear, and the artist
// credit opens with the nib the engine prepends, so those are the parts that
// carry a renderer
func textParts(name string, d *card.Data, syms symbols, fontDir, copyrightOverride string) []template.TextPart {
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
	p, ok := part(text, roleFor(name), fontDir)
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

// titleCostGap is the space kept between the card name and the mana cost, as a
// fraction of the name's font size, so the two never read as one run of ink
const titleCostGap = 0.5

// Where the black copy behind each symbol sits, both measured off a Scryfall
// scan of a printed card. The distance is a fraction of the pip's diameter, and
// the lean turns it off straight down, in degrees clockwise, which carries it a
// little to the left the way a printed cost does
const (
	costShadowDrop = 0.116
	costShadowLean = 25
)

// costShadow is the black disc sitting behind and below each symbol of the mana
// cost. A printed cost draws a solid offset copy rather than a soft shadow, so
// this casts with no blur at full opacity, and the symbol on top leaves only a
// crescent showing. A drop shadow off the layer's alpha is that same offset
// copy, one per symbol, without the symbol renderer knowing.
//
// It goes on the cost's layer alone, since the symbols in rules text lie flat.
// It covers whatever that box draws, which is the symbols and, for a code with
// no artwork, the literal text they fell back to
func costShadow(size float64) *effects.DropShadow {
	// impasto measures the angle clockwise from the positive x axis, so half pi
	// is straight down and the lean turns it further clockwise
	angle := math.Pi/2 + costShadowLean*math.Pi/180
	return &effects.DropShadow{
		Color:    color.Black,
		Opacity:  1,
		Angle:    float32(angle),
		Distance: float32(size * mana.PipDiameter * costShadowDrop),
	}
}

// manaBox is the mana cost's box with shrink-to-fit switched off, its floor
// raised to its own size. A symbol is a fixed size on a printed card, so a heavy
// cost grows leftward out of its box rather than shrinking into it
func manaBox(box template.TextBoxSpec) template.TextBoxSpec {
	box.MinFontSize = box.FontSize
	return box
}

// costSpan reports where the mana cost lands in the title bar, which is what
// the name has to stop short of. A card with no cost, or a template with no
// mana box, measures to an empty span
func costSpan(m *template.Manifest, d *card.Data, syms symbols, fontDir string) (template.TextSpan, error) {
	box, ok := m.TextBoxes["mana"]
	if !ok {
		return template.TextSpan{}, nil
	}
	parts := textParts("mana", d, syms, fontDir, "")
	if len(parts) == 0 {
		return template.TextSpan{}, nil
	}
	return template.MeasureTextSpan(manaBox(box), parts...)
}

// titleClearOf narrows the name box so it stops a gap short of the mana cost,
// leaving the name to shrink rather than run under the symbols. It only ever
// narrows, so a card whose cost leaves room keeps the box the manifest gave it
func titleClearOf(box template.TextBoxSpec, cost template.TextSpan) template.TextBoxSpec {
	if cost.Empty() {
		return box
	}
	room := cost.Left - int(math.Round(box.FontSize*titleCostGap)) - box.X
	if room < box.Width {
		if box.Width = room; box.Width < 0 {
			box.Width = 0
		}
	}
	return box
}

// copyrightOnArtistRow moves the copyright down from the collector row to the
// artist row on creatures, so it clears the P/T box that sits on the collector
// row. It falls back to the copyright's own row when no artist box is defined
func copyrightOnArtistRow(box template.TextBoxSpec, m *template.Manifest) template.TextBoxSpec {
	if artist, ok := m.TextBoxes["artist"]; ok {
		box.Y = artist.Y
	}
	return box
}

func sortedKeys(m map[string]template.TextBoxSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
