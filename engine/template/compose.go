package template

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/odevine/impasto/blend"
	"github.com/odevine/impasto/canvas"
	"github.com/odevine/impasto/raster"
	xdraw "golang.org/x/image/draw"
)

// LoadLayer decodes a layer PNG, resamples it to the render scale, and places
// it at the document origin. Frame layers are authored document-sized, so the
// origin placement leaves them where the artwork put them
func LoadLayer(p AssetProvider, path string, w, h int, s Scale) (*raster.Buffer, error) {
	img, err := LoadImage(p, path)
	if err != nil {
		return nil, err
	}
	buf, err := raster.FromImage(s.Image(img))
	if err != nil {
		return nil, fmt.Errorf("template: wrapping layer %q: %w", path, err)
	}
	return canvas.Place(w, h, buf, 0, 0), nil
}

// Divider builds a floating divider graphic, such as the rule between a
// creature's rules and flavor text. The divider asset bakes a thin graphic into
// an otherwise transparent full-document image, so this crops that strip to its
// opaque bounds and places its center at centerY, the document Y a text layout
// reported for the divider. It returns nil when the manifest carries no
// "divider" layer, so a template without one still renders
func Divider(p AssetProvider, m *Manifest, centerY int, s Scale) (*canvas.Layer, error) {
	path := LayerAssetPath(m, "divider")
	if path == "" {
		return nil, nil
	}
	img, err := LoadImage(p, path)
	if err != nil {
		return nil, err
	}
	// Resampling before the crop measures the strip in the scaled document's own
	// coordinates, which is where centerY already is
	img = s.Image(img)
	strip := OpaqueBounds(img)
	if strip.Empty() {
		return nil, nil
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("template: divider asset %q does not support cropping", path)
	}
	buf, err := raster.FromImage(sub.SubImage(strip))
	if err != nil {
		return nil, fmt.Errorf("template: wrapping divider: %w", err)
	}
	placed := canvas.Place(m.Width, m.Height, buf, strip.Min.X, centerY-strip.Dy()/2)
	return &canvas.Layer{Content: placed, Mode: blend.Normal}, nil
}

// LayerAssetPath returns the color-invariant "any" asset path for a named
// layer, or "" when the layer or that variant is absent
func LayerAssetPath(m *Manifest, name string) string {
	for _, l := range m.Layers {
		if l.Name == name {
			return l.ColorVariants["any"].Path
		}
	}
	return ""
}

// OpaqueBounds is the smallest rectangle covering every pixel of img with any
// alpha, the extent of a graphic baked into an otherwise transparent image. It
// returns the empty rectangle when img is fully transparent
func OpaqueBounds(img image.Image) image.Rectangle {
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

// FitArt scales art to cover a w by h window, preserving aspect ratio and
// cropping the overflow so any source art fills the window. A non-positive
// window returns the art unchanged, leaving it at native size
func FitArt(src image.Image, w, h int) image.Image {
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

// BlendMode maps a manifest blend name to an impasto mode. An empty name is
// Normal, an unrecognized one is an error so a manifest typo is not silently
// composited the wrong way
func BlendMode(name string) (blend.Mode, error) {
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
		return blend.Normal, fmt.Errorf("template: unknown blend mode %q", name)
	}
}
