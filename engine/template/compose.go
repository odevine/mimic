package template

import (
	"fmt"
	"image"
	"math"

	"github.com/odevine/impasto/blend"
	"github.com/odevine/impasto/canvas"
	"github.com/odevine/impasto/raster"
	"github.com/odevine/mimic/engine/manifest"
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
	scaled, at, err := loadLayerImage(p, path, s, x, y, docW, docH, false)
	if err != nil || scaled == nil {
		return nil, at, err
	}
	buf, err := raster.FromImage(scaled)
	if err != nil {
		return nil, image.Point{}, fmt.Errorf("template: wrapping layer %q: %w", path, err)
	}
	return buf, at, nil
}

// LoadLayerImage is LoadLayerAt for a layer that is only to be composited, which
// returns the image and not a buffer made from it. At native scale that is the
// decoded image itself, shared and with its index when the provider keeps one, and
// below it the box-scaled part of the document the image touches. Either way it is
// what a canvas layer's Image takes, so the buffer is never built. It returns a nil
// image for a PNG that lands on no pixel
func LoadLayerImage(p AssetProvider, path string, s Scale, x, y, docW, docH int) (image.Image, image.Point, error) {
	return loadLayerImage(p, path, s, x, y, docW, docH, true)
}

// loadLayerImage decodes a layer and resamples it to the render scale, with the
// provider's index of it when indexed is set and one exists
func loadLayerImage(p AssetProvider, path string, s Scale, x, y, docW, docH int, indexed bool) (image.Image, image.Point, error) {
	var img image.Image
	var err error
	if indexed && s.Native() {
		img, err = LoadIndexedImage(p, path)
	} else {
		img, err = LoadImage(p, path)
	}
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
	return scaled, at, nil
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
// reported for the divider. It returns nil when path is empty, so a template
// without a divider still renders
func Divider(p AssetProvider, path string, centerY int, s Scale) (*canvas.Layer, error) {
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
	m, err := manifest.ParseMode(name)
	if err != nil {
		return blend.Normal, fmt.Errorf("template: %w", err)
	}
	return m, nil
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

// TurnRect returns r turned clockwise by degrees, a multiple of 90, about the point
// at half of (cx2, cy2). The center is given doubled so a box with an odd side can
// turn about its middle
func TurnRect(r image.Rectangle, cx2, cy2, degrees int) image.Rectangle {
	for turns := ((degrees/90)%4 + 4) % 4; turns > 0; turns-- {
		// A quarter turn sends (x, y) to (cx - (y - cy), cy + (x - cx))
		r = image.Rect(
			(cx2-(2*r.Max.Y-cy2))/2, (cy2+(2*r.Min.X-cx2))/2,
			(cx2-(2*r.Min.Y-cy2))/2, (cy2+(2*r.Max.X-cx2))/2,
		)
	}
	return r
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
