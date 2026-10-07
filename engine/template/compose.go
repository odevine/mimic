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

// LoadLayer decodes a layer PNG that fills the document and resamples it to the
// render scale. It returns the buffer at the PNG's own size and the document
// origin it belongs at
func LoadLayer(p AssetProvider, path string, s Scale) (*raster.Buffer, image.Point, error) {
	return LoadLayerAt(p, path, s, 0, 0, 0, 0)
}

// LoadLayerAt is LoadLayer for a PNG belonging at (x, y) in an authored document
// of docW by docH, such as a layer cut to its bounds or one half of a split
// card. A docW or docH of zero is the PNG's own size. The buffer covers just the
// part of the scaled document the PNG touches, and its pixels are the ones the
// document-sized layer would have there. It returns a nil buffer for a PNG that
// lands on no pixel
func LoadLayerAt(p AssetProvider, path string, s Scale, x, y, docW, docH int) (*raster.Buffer, image.Point, error) {
	img, err := LoadImage(p, path)
	if err != nil {
		return nil, image.Point{}, err
	}
	if b := img.Bounds(); docW <= 0 || docH <= 0 {
		docW, docH = b.Dx(), b.Dy()
	}
	scaled, at := s.ImageAt(img, x, y, docW, docH)
	if scaled.Bounds().Empty() {
		return nil, at, nil
	}
	buf, err := raster.FromImage(scaled)
	if err != nil {
		return nil, image.Point{}, fmt.Errorf("template: wrapping layer %q: %w", path, err)
	}
	return buf, at, nil
}

// Mirror flips buf left to right about its vertical center within region, a
// rectangle on the left half whose pixels each trade places with their mirror
// image. The empty rectangle flips the whole buffer. A region reaching past
// the center stops there, so no pixel is swapped twice
func Mirror(buf *raster.Buffer, region image.Rectangle) {
	half := image.Rect(0, 0, buf.Width/2, buf.Height)
	if region.Empty() {
		region = half
	}
	region = region.Intersect(half)
	for y := region.Min.Y; y < region.Max.Y; y++ {
		row := y * buf.Width
		for x := region.Min.X; x < region.Max.X; x++ {
			a := (row + x) * 4
			b := (row + buf.Width - 1 - x) * 4
			for c := 0; c < 4; c++ {
				buf.Pix[a+c], buf.Pix[b+c] = buf.Pix[b+c], buf.Pix[a+c]
			}
		}
	}
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
	return &canvas.Layer{Content: buf, Origin: image.Pt(strip.Min.X, centerY-strip.Dy()/2), Mode: blend.Normal}, nil
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
	switch src := img.(type) {
	case *image.NRGBA:
		return alphaBounds(src.Pix, src.Stride, src.Rect, 4, 3, 3)
	case *image.RGBA:
		return alphaBounds(src.Pix, src.Stride, src.Rect, 4, 3, 3)
	case *image.NRGBA64:
		return alphaBounds(src.Pix, src.Stride, src.Rect, 8, 6, 7)
	}
	return opaqueBoundsAt(img)
}

// alphaBounds finds the rectangle of pixels with any alpha by reading the alpha
// bytes of pix directly. Each pixel is bpp bytes, and its alpha lives at the
// byte offsets a0 through a1. Every row is read from both ends and stops at the
// first hit, so only the transparent margins are scanned in full
func alphaBounds(pix []uint8, stride int, r image.Rectangle, bpp, a0, a1 int) image.Rectangle {
	minX, minY := r.Max.X, r.Max.Y
	maxX, maxY := r.Min.X, r.Min.Y
	w := r.Dx()
	found := false
	for y := 0; y < r.Dy(); y++ {
		row := pix[y*stride : y*stride+w*bpp]
		first := 0
		for first < w && !visible(row, first*bpp, a0, a1) {
			first++
		}
		if first == w {
			continue
		}
		last := w - 1
		for !visible(row, last*bpp, a0, a1) {
			last--
		}
		found = true
		minX = min(minX, r.Min.X+first)
		maxX = max(maxX, r.Min.X+last+1)
		minY = min(minY, r.Min.Y+y)
		maxY = r.Min.Y + y + 1
	}
	if !found {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

// visible reports whether the pixel starting at off has any alpha
func visible(row []uint8, off, a0, a1 int) bool {
	return row[off+a0] != 0 || row[off+a1] != 0
}

// opaqueBoundsAt is OpaqueBounds for any image type, reading through At
func opaqueBoundsAt(img image.Image) image.Rectangle {
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

// Rotate returns buf turned clockwise by degrees, a multiple of 90. A quarter
// turn swaps the width and height. Zero returns buf itself
func Rotate(buf *raster.Buffer, degrees int) *raster.Buffer {
	turns := ((degrees/90)%4 + 4) % 4
	if turns == 0 {
		return buf
	}
	w, h := buf.Width, buf.Height
	ow, oh := w, h
	if turns%2 == 1 {
		ow, oh = h, w
	}
	out := raster.MustNewBuffer(ow, oh)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch turns {
			case 1:
				dx, dy = h-1-y, x
			case 2:
				dx, dy = w-1-x, h-1-y
			default:
				dx, dy = y, w-1-x
			}
			si := (y*w + x) * 4
			di := (dy*ow + dx) * 4
			copy(out.Pix[di:di+4], buf.Pix[si:si+4])
		}
	}
	return out
}

// HalfOfArt is the i-th of n equal vertical strips of an art image, counted from
// the left. A split card's art crop holds both halves side by side, and each
// half's art window takes its own strip
func HalfOfArt(img image.Image, i, n int) image.Image {
	b := img.Bounds()
	if n <= 1 {
		return img
	}
	x0 := b.Min.X + b.Dx()*i/n
	x1 := b.Min.X + b.Dx()*(i+1)/n
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		out := image.NewRGBA(image.Rect(0, 0, x1-x0, b.Dy()))
		xdraw.Draw(out, out.Bounds(), img, image.Pt(x0, b.Min.Y), xdraw.Src)
		return out
	}
	return sub.SubImage(image.Rect(x0, b.Min.Y, x1, b.Max.Y))
}

// BlendStops are the seams of a layer that blends n colors left to right, as
// fractions of the document's width, each color's hold then the blend into the
// next, such as a fuse bar whose halves run in different colors. The seams for
// two and three colors are the printed card's, and four add the middle one
func BlendStops(n int) []float64 {
	switch n {
	case 2:
		return []float64{.50, .54}
	case 3:
		return []float64{.28, .33, .71, .76}
	case 4:
		return []float64{.28, .33, .50, .54, .71, .76}
	}
	return nil
}

// blendWeights is how much of each of n colors shows at a document fraction t,
// where each color holds until its seam starts and fades into the next across it
func blendWeights(stops []float64, n int, t float64, w []float64) {
	for i := range w {
		w[i] = 0
	}
	k := 0
	for k < n-1 && t >= stops[2*k+1] {
		k++
	}
	if k == n-1 || t < stops[2*k] {
		w[k] = 1
		return
	}
	mix := (t - stops[2*k]) / (stops[2*k+1] - stops[2*k])
	w[k], w[k+1] = 1-mix, mix
}

// LoadBlendedLayer decodes several color variants of one layer and blends them
// left to right at the seams BlendStops gives, for a layer that belongs at
// (x, y) in an authored document of docW by docH. It returns the blended buffer
// at the variants' own size with its origin in the scaled document, so a bar
// costs the size of the bar rather than of the document. They must be the same size
func LoadBlendedLayer(p AssetProvider, paths []string, s Scale, x, y, docW, docH int) (*raster.Buffer, image.Point, error) {
	stops := BlendStops(len(paths))
	if stops == nil {
		return nil, image.Point{}, fmt.Errorf("template: cannot blend %d colors", len(paths))
	}
	var bufs []*raster.Buffer
	var origin image.Point
	for _, path := range paths {
		buf, at, err := LoadLayerAt(p, path, s, x, y, docW, docH)
		if err != nil {
			return nil, image.Point{}, err
		}
		if buf == nil {
			return nil, at, nil
		}
		origin = at
		if len(bufs) > 0 && !buf.SameSize(bufs[0]) {
			return nil, image.Point{}, fmt.Errorf("template: blended layers %q and %q differ in size", paths[0], path)
		}
		bufs = append(bufs, buf)
	}
	w := max(s.Px(docW), 1)
	if s.Native() {
		w = docW
	}
	out := raster.MustNewBuffer(bufs[0].Width, bufs[0].Height)
	weights := make([]float64, len(bufs))
	for sx := 0; sx < out.Width; sx++ {
		blendWeights(stops, len(bufs), (float64(origin.X+sx)+0.5)/float64(w), weights)
		for sy := 0; sy < out.Height; sy++ {
			i := (sy*out.Width + sx) * 4
			for c := 0; c < 4; c++ {
				var v float64
				for k, buf := range bufs {
					v += weights[k] * float64(buf.Pix[i+c])
				}
				out.Pix[i+c] = float32(v)
			}
		}
	}
	return out, origin, nil
}

// BlendedPaths lists the variants a blended color key draws, one per letter, or
// nil when the key is not several letters each with a variant of its own
func BlendedPaths(variants map[string]LayerAsset, key string) []string {
	if len(key) < 2 {
		return nil
	}
	var paths []string
	for _, r := range key {
		v, ok := variants[string(r)]
		if !ok {
			return nil
		}
		paths = append(paths, v.Path)
	}
	return paths
}
