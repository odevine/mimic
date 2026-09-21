package normal

import (
	"context"
	"fmt"
	"image"
	"math"
	"sort"
	"strings"

	"github.com/odevine/impasto/blend"
	"github.com/odevine/impasto/canvas"
	"github.com/odevine/impasto/raster"
	xdraw "golang.org/x/image/draw"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/template"
)

const templateName = "normal"

func init() {
	template.Register(templateName, func() template.Template { return &Template{} })
}

// Template renders a card with the Normal frame
type Template struct {
	// FontDir is an optional directory of user-supplied font overrides. A file
	// named for its role (Beleren, Plantin) is used ahead of the embedded
	// default. An empty FontDir uses the embedded defaults
	FontDir string
}

// boxRoles maps each named text box to the font role it draws in. Names not
// listed draw in the body role
var boxRoles = map[string]fonts.Role{
	"title":  fonts.Title,
	"type":   fonts.Title,
	"pt":     fonts.Title,
	"mana":   fonts.Body,
	"oracle": fonts.Body,
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

	f := deriveFrame(req.Card)

	var nodes []canvas.Node
	for _, layer := range m.Layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !f.conditionMet(layer.Condition) {
			continue
		}
		asset, ok := layer.ColorVariants[f.keyForLayer(layer.Name)]
		if !ok {
			asset, ok = layer.ColorVariants["any"]
		}
		if !ok {
			// No variant applies to this color. Skipping rather than erroring
			// lets a manifest leave a layer out where it does not apply
			continue
		}
		placed, err := loadLayer(req.Assets, asset.Path, m.Width, m.Height)
		if err != nil {
			return nil, err
		}
		mode, err := blendMode(layer.Blend)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, &canvas.Layer{Content: placed, Mode: mode})

		if req.Art != nil && m.Art.After == layer.Name {
			art := fitArt(req.Art, m.Art.Width, m.Art.Height)
			artBuf, err := raster.FromImage(art)
			if err != nil {
				return nil, fmt.Errorf("normal: wrapping art: %w", err)
			}
			placedArt := canvas.Place(m.Width, m.Height, artBuf, m.Art.X, m.Art.Y)
			nodes = append(nodes, &canvas.Layer{Content: placedArt, Mode: blend.Normal})
		}
	}

	for _, name := range sortedKeys(m.TextBoxes) {
		text := textFor(name, req.Card)
		if strings.TrimSpace(text) == "" {
			continue
		}
		box := m.TextBoxes[name]
		sizer := fonts.ResolveFont(roleFor(name), t.FontDir)
		face, err := sizer.Face(box.FontSize)
		if err != nil {
			return nil, fmt.Errorf("normal: resolving font for text box %q: %w", name, err)
		}
		if sizer.Fallback() {
			text = normalizeForBasicFont(text)
		}
		img := template.RenderTextBox(box, text, m.Width, m.Height, face)
		buf, err := raster.FromImage(img)
		if err != nil {
			return nil, fmt.Errorf("normal: wrapping text box %q: %w", name, err)
		}
		nodes = append(nodes, &canvas.Layer{Content: buf, Mode: blend.Normal})
	}

	// A pass-through root is the correct default. Nothing sits beneath the
	// document root, so pass-through and isolated behave the same here
	doc := &canvas.Document{
		Width:  m.Width,
		Height: m.Height,
		Root:   canvas.Group{PassThrough: true, Layers: nodes},
	}
	return canvas.Render(doc)
}

// loadLayer decodes a layer PNG and places it at the document origin. Frame
// layers are authored document-sized, so the origin placement leaves them where
// the artwork put them
func loadLayer(p template.AssetProvider, path string, w, h int) (*raster.Buffer, error) {
	img, err := template.LoadImage(p, path)
	if err != nil {
		return nil, err
	}
	buf, err := raster.FromImage(img)
	if err != nil {
		return nil, fmt.Errorf("normal: wrapping layer %q: %w", path, err)
	}
	return canvas.Place(w, h, buf, 0, 0), nil
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

// fitArt scales art to cover a w by h window, preserving aspect ratio and
// cropping the overflow so any source art fills the window. A non-positive
// window returns the art unchanged, leaving it at native size
func fitArt(src image.Image, w, h int) image.Image {
	if w <= 0 || h <= 0 {
		return src
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if sw <= 0 || sh <= 0 {
		return dst
	}
	scale := math.Max(float64(w)/float64(sw), float64(h)/float64(sh))
	dw := int(math.Round(float64(sw) * scale))
	dh := int(math.Round(float64(sh) * scale))
	offset := image.Rect((w-dw)/2, (h-dh)/2, (w-dw)/2+dw, (h-dh)/2+dh)
	xdraw.CatmullRom.Scale(dst, offset, src, sb, xdraw.Over, nil)
	return dst
}

// textFor returns the card text for a named box. Unknown names return empty so
// a manifest can define boxes this mapping does not fill
func textFor(name string, d *card.Data) string {
	switch name {
	case "title":
		return d.Name
	case "mana":
		return d.ManaCost
	case "type":
		return d.TypeLine
	case "oracle":
		switch {
		case d.FlavorText == "":
			return d.OracleText
		case d.OracleText == "":
			return d.FlavorText
		default:
			return d.OracleText + "\n" + d.FlavorText
		}
	case "pt":
		switch {
		case d.Loyalty != "":
			return d.Loyalty
		case d.Power != "" || d.Toughness != "":
			return d.Power + "/" + d.Toughness
		default:
			return ""
		}
	default:
		return ""
	}
}

// blendMode maps a manifest blend name to an impasto mode. An empty name is
// Normal, an unrecognized one is an error so a manifest typo is not silently
// composited the wrong way
func blendMode(name string) (blend.Mode, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "normal":
		return blend.Normal, nil
	case "multiply":
		return blend.Multiply, nil
	case "screen":
		return blend.Screen, nil
	case "overlay":
		return blend.Overlay, nil
	case "softlight":
		return blend.SoftLight, nil
	case "hardlight":
		return blend.HardLight, nil
	case "colordodge":
		return blend.ColorDodge, nil
	case "colorburn":
		return blend.ColorBurn, nil
	case "darken":
		return blend.Darken, nil
	case "lighten":
		return blend.Lighten, nil
	case "difference":
		return blend.Difference, nil
	case "exclusion":
		return blend.Exclusion, nil
	case "hue":
		return blend.Hue, nil
	case "saturation":
		return blend.Saturation, nil
	case "color":
		return blend.Color, nil
	case "luminosity":
		return blend.Luminosity, nil
	default:
		return blend.Normal, fmt.Errorf("normal: unknown blend mode %q", name)
	}
}

func sortedKeys(m map[string]template.TextBoxSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
