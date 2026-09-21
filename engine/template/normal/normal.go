package normal

import (
	"context"
	"fmt"
	"image"
	"math"
	"sort"
	"strconv"
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
	// Copyright is the boilerplate line at the card bottom. Scryfall carries no
	// copyright string, so it is set here rather than read from the card. An
	// empty Copyright uses defaultCopyright
	Copyright string
}

// defaultCopyright is the bottom line used when a Template sets no Copyright.
// Real cards print the set's year, which the card data does not carry, so the
// year is left out
const defaultCopyright = "™ & © Wizards of the Coast"

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

	f := deriveFrame(req.Card)

	layersByName := make(map[string]template.LayerSpec, len(m.Layers))
	for _, l := range m.Layers {
		layersByName[l.Name] = l
	}

	var nodes []canvas.Node
	for _, layer := range m.Layers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !f.conditionMet(layer.Condition) {
			continue
		}
		// An enchantment draws the nyx frame in the background slot, so it sits
		// below the art and follows the same color key as the background
		variants := layer.ColorVariants
		if layer.Name == "background" && f.nyx {
			if nyx, ok := layersByName["nyx"]; ok {
				variants = nyx.ColorVariants
			}
		}
		asset, ok := variants[f.keyForLayer(layer.Name)]
		if !ok {
			asset, ok = variants["any"]
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

		// An enchantment legend gets a hollow crown: erase the crown and the
		// shadow beneath it where the nyx frame shows through, keeping the edge.
		// Off pending a missing layer, the knockout code is kept for that revisit
		if hollowCrownEnabled && (layer.Name == "legendary_crown" || layer.Name == "shadows") && f.nyx && f.legendary {
			if err := knockoutHollowRegion(req.Assets, m, placed); err != nil {
				return nil, err
			}
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
		parts := t.textParts(name, req.Card)
		if len(parts) == 0 {
			continue
		}
		box := m.TextBoxes[name]
		switch {
		case name == "copyright" && f.creature:
			box = copyrightOnArtistRow(box, m)
		case name == "oracle" && f.creature:
			if top, ok := ptBoxTop(req.Assets, layersByName, f); ok && top < box.Y+box.Height {
				if box.Height = top - box.Y; box.Height < 0 {
					box.Height = 0
				}
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
		nodes = append(nodes, &canvas.Layer{Content: buf, Mode: blend.Normal})

		if res.HasDivider {
			div, err := dividerLayer(req.Assets, m, res.DividerY)
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

// dividerLayer builds the floating flavor divider. The divider asset bakes a
// thin graphic into an otherwise transparent full-document image, so this crops
// that strip to its opaque bounds and places its center at centerY, the
// document Y the text layout reported for the divider. It returns nil when the
// manifest carries no divider asset, so a template without one still renders
func dividerLayer(p template.AssetProvider, m *template.Manifest, centerY int) (*canvas.Layer, error) {
	path := layerAssetPath(m, "divider")
	if path == "" {
		return nil, nil
	}
	img, err := template.LoadImage(p, path)
	if err != nil {
		return nil, err
	}
	strip := opaqueBounds(img)
	if strip.Empty() {
		return nil, nil
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("normal: divider asset %q does not support cropping", path)
	}
	buf, err := raster.FromImage(sub.SubImage(strip))
	if err != nil {
		return nil, fmt.Errorf("normal: wrapping divider: %w", err)
	}
	placed := canvas.Place(m.Width, m.Height, buf, strip.Min.X, centerY-strip.Dy()/2)
	return &canvas.Layer{Content: placed, Mode: blend.Normal}, nil
}

// ptBoxTop returns the document Y of the P/T box graphic's top edge, so the
// oracle text can treat it as its floor on creatures and not spill into the box.
// It reports false when the P/T box layer or its asset is missing or transparent
func ptBoxTop(p template.AssetProvider, layers map[string]template.LayerSpec, f frame) (int, bool) {
	spec, ok := layers["pt_box"]
	if !ok {
		return 0, false
	}
	path := spec.ColorVariants[f.ptBox].Path
	if path == "" {
		path = spec.ColorVariants["any"].Path
	}
	if path == "" {
		return 0, false
	}
	img, err := template.LoadImage(p, path)
	if err != nil {
		return 0, false
	}
	b := opaqueBounds(img)
	if b.Empty() {
		return 0, false
	}
	return b.Min.Y, true
}

// knockoutHollowRegion erases a buffer where the nyx frame should show through a
// hollow crown, leaving the rest intact. It applies to the crown and the shadow
// beneath it, so the sky shows cleanly through the crown's opening. The
// hollow_crown_shadow asset is opaque where the layer is kept and transparent
// where the sky shows, so its alpha is the keep factor. It is a no-op when the
// shadow asset is absent
func knockoutHollowRegion(p template.AssetProvider, m *template.Manifest, buf *raster.Buffer) error {
	path := layerAssetPath(m, "hollow_crown_shadow")
	if path == "" {
		return nil
	}
	shadow, err := loadLayer(p, path, m.Width, m.Height)
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

// layerAssetPath returns the color-invariant "any" asset path for a named
// layer, or "" when the layer or that variant is absent
func layerAssetPath(m *template.Manifest, name string) string {
	for _, l := range m.Layers {
		if l.Name == name {
			return l.ColorVariants["any"].Path
		}
	}
	return ""
}

// opaqueBounds is the smallest rectangle covering every pixel of img with any
// alpha, the extent of a graphic baked into an otherwise transparent image. It
// returns the empty rectangle when img is fully transparent
func opaqueBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				found = true
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x >= maxX {
					maxX = x + 1
				}
				if y >= maxY {
					maxY = y + 1
				}
			}
		}
	}
	if !found {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
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

// textParts returns the styled parts for a named box. Most boxes are one part
// in the box's role. The oracle box is rules in the body role then flavor in
// the italic role, which RenderTextBox separates with a divider
func (t *Template) textParts(name string, d *card.Data) []template.TextPart {
	if name == "oracle" {
		var parts []template.TextPart
		if p, ok := t.part(d.OracleText, fonts.Body); ok {
			// Parenthesized reminder text and leading ability or flavor words
			// italicize, while keyword abilities stay roman
			p.Emph = fonts.ResolveFont(fonts.BodyItalic, t.FontDir)
			p.EmphLead = card.EmphasisWords
			parts = append(parts, p)
		}
		if p, ok := t.part(d.FlavorText, fonts.BodyItalic); ok {
			parts = append(parts, p)
		}
		return parts
	}
	text := textFor(name, d)
	if name == "copyright" {
		text = t.copyright()
	}
	if p, ok := t.part(text, roleFor(name)); ok {
		return []template.TextPart{p}
	}
	return nil
}

// copyright returns the configured bottom line, or the default when unset
func (t *Template) copyright() string {
	if t.Copyright != "" {
		return t.Copyright
	}
	return defaultCopyright
}

// part resolves a font for role and pairs it with text, normalizing the text
// when the font falls back to the basic face. It reports false for empty text
func (t *Template) part(text string, role fonts.Role) (template.TextPart, bool) {
	if strings.TrimSpace(text) == "" {
		return template.TextPart{}, false
	}
	sizer := fonts.ResolveFont(role, t.FontDir)
	if sizer.Fallback() {
		text = normalizeForBasicFont(text)
	}
	return template.TextPart{Text: text, Src: sizer}, true
}

// textFor returns the card text for a named box. Unknown names return empty so
// a manifest can define boxes this mapping does not fill. The oracle box is
// built by textParts, so its case here covers only a direct lookup
func textFor(name string, d *card.Data) string {
	switch name {
	case "title":
		return d.Name
	case "mana":
		return d.ManaCost
	case "type":
		return d.TypeLine
	case "oracle":
		// textParts builds the oracle box from rules and flavor as separate
		// styled parts, so this direct lookup returns the rules text alone
		return d.OracleText
	case "pt":
		switch {
		case d.Loyalty != "":
			return d.Loyalty
		case d.Power != "" || d.Toughness != "":
			return d.Power + "/" + d.Toughness
		default:
			return ""
		}
	case "artist":
		return d.Artist
	case "collector":
		return collectorLine(d)
	case "set":
		return setLine(d)
	default:
		return ""
	}
}

// collectorLine formats the rarity and collector number, as "R 0177". Real cards
// print the set total after the number ("0177/302"), which Scryfall's card data
// does not carry, so it is left out
func collectorLine(d *card.Data) string {
	num := padCollectorNumber(d.CollectorNumber)
	if num == "" {
		return ""
	}
	if letter := rarityLetter(d.Rarity); letter != "" {
		return letter + " " + num
	}
	return num
}

// padCollectorNumber left-pads a purely numeric collector number to four digits,
// so 177 reads as 0177. A number carrying a non-digit part is left as is
func padCollectorNumber(n string) string {
	if v, err := strconv.Atoi(n); err == nil {
		return fmt.Sprintf("%04d", v)
	}
	return n
}

// rarityLetter is the single-letter rarity code the collector line prints
func rarityLetter(rarity string) string {
	switch strings.ToLower(rarity) {
	case "common":
		return "C"
	case "uncommon":
		return "U"
	case "rare":
		return "R"
	case "mythic":
		return "M"
	case "special":
		return "S"
	case "bonus":
		return "B"
	case "":
		return ""
	default:
		return strings.ToUpper(rarity[:1])
	}
}

// setLine formats the set code and printing language, as "A25 • EN", defaulting
// to English when the card carries no language
func setLine(d *card.Data) string {
	if d.SetCode == "" {
		return ""
	}
	lang := strings.ToUpper(d.Language)
	if lang == "" {
		lang = "EN"
	}
	return strings.ToUpper(d.SetCode) + " • " + lang
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
