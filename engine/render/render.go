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
		if !f.ConditionMet(layer.Condition) {
			continue
		}
		// An enchantment draws the nyx frame in the background slot, so it sits
		// below the art and follows the same color key as the background
		variants := layer.ColorVariants
		if layer.ColorSlot == "background" && f.Nyx {
			if nyx, ok := layersByName["nyx"]; ok {
				variants = nyx.ColorVariants
			}
		}
		asset, ok := variants[f.Slot(layer.ColorSlot)]
		if !ok {
			asset, ok = variants["any"]
		}
		if !ok {
			// No variant applies to this color. Skipping rather than erroring
			// lets a manifest leave a layer out where it does not apply
			continue
		}
		placed, err := template.LoadLayer(req.Assets, asset.Path, m.Width, m.Height, scale)
		if err != nil {
			return nil, err
		}
		mode, err := template.BlendMode(layer.Blend)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, &canvas.Layer{Content: placed, Mode: mode})

		if req.Art != nil && m.Art.After == layer.Name {
			art := template.FitArt(req.Art, m.Art.Width, m.Art.Height)
			artBuf, err := raster.FromImage(art)
			if err != nil {
				return nil, fmt.Errorf("render: wrapping art: %w", err)
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

	boxNames := sortedKeys(m.TextBoxes)
	for i, name := range boxNames {
		req.Report(stepText, lerp(fracTextFrom, fracTextTo, i, len(boxNames)))
		box := m.TextBoxes[name]
		parts := textParts(name, box, req.Card, syms, req.FontDir, req.Copyright)
		if len(parts) == 0 {
			continue
		}
		if box.DuckLayer != "" && box.DuckRow != "" {
			if _, ok := template.AvoidRect(req.Assets, layersByName, f, box.DuckLayer, scale); ok {
				box = template.MoveToRow(box, m, box.DuckRow)
			}
		}
		if box.ClearOf != "" {
			if otherBox, ok := m.TextBoxes[box.ClearOf]; ok {
				if otherParts := textParts(box.ClearOf, otherBox, req.Card, syms, req.FontDir, req.Copyright); len(otherParts) > 0 {
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
		res, err := template.RenderTextBox(box, m.Width, m.Height, parts...)
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
		nodes = append(nodes, text)

		if res.HasDivider {
			div, err := template.Divider(req.Assets, m, res.DividerY, scale)
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
