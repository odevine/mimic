package template

import (
	"image"
	"math"

	xdraw "golang.org/x/image/draw"
)

// CardWidthInches is the physical width of a card canvas including the bleed a
// print service trims off, which is what turns a manifest's pixel width into a
// dpi. A manifest authored at some other physical size states its own DPI
// rather than relying on this
const CardWidthInches = 2.72

// MinDPI is the floor a requested resolution clamps to. Type stops resolving
// into readable shapes below it, so a smaller render has nothing to show
const MinDPI = 72

// PresetDPIs are the round resolutions a ui offers alongside a template's own.
// One at or above the template's native dpi is dropped, since rendering past
// the authored size resamples assets up without adding any detail
var PresetDPIs = []int{150, 300, 600, 1200}

// Scale multiplies a template's authored geometry and its assets, so a preview
// can render at a fraction of the size a print-ready export needs. The zero
// value and 1 both render at the authored size
type Scale float64

// Factor is the multiplier to apply, reading the zero value as native
func (s Scale) Factor() float64 {
	if s <= 0 {
		return 1
	}
	return float64(s)
}

// Native reports whether s leaves geometry and assets untouched
func (s Scale) Native() bool { return s.Factor() == 1 }

// Px scales a pixel measurement to the nearest whole pixel
func (s Scale) Px(v int) int { return int(math.Round(float64(v) * s.Factor())) }

// F scales a fractional measurement such as a font size. It does not round, so
// a small preview keeps the sub-pixel type sizes its layout needs
func (s Scale) F(v float64) float64 { return v * s.Factor() }

// Rect scales a rectangle, which is how a measurement taken from an asset at
// its authored size lands in a scaled document's coordinates
func (s Scale) Rect(r image.Rectangle) image.Rectangle {
	if s.Native() || r.Empty() {
		return r
	}
	return image.Rect(s.Px(r.Min.X), s.Px(r.Min.Y), s.Px(r.Max.X), s.Px(r.Max.Y))
}

// Image resamples img by the scale factor, anchored at the origin. It returns
// img as it is at native scale, so a full-size render pays nothing for the
// resolution setting existing
func (s Scale) Image(img image.Image) image.Image {
	if s.Native() {
		return img
	}
	b := img.Bounds()
	w, h := max(s.Px(b.Dx()), 1), max(s.Px(b.Dy()), 1)
	if s.Factor() < 1 {
		return boxScale(img, w, h)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}

// boxScale resamples src down to w by h by averaging every source pixel each
// destination pixel covers. Averaging the whole covered area rather than
// sampling a few points within it is what keeps a frame's fine pinlines from
// aliasing into moire at preview sizes, and it costs one pass over the source
// where a filter kernel costs several
func boxScale(src image.Image, w, h int) *image.RGBA {
	rgba := premultiplied(src)
	out, _ := boxScaleRegion(rgba, 0, 0, rgba.Bounds().Dx(), rgba.Bounds().Dy(), w, h)
	return out
}

// premultiplied is src as an RGBA image with its origin at zero. Averaging
// happens in premultiplied alpha, or the color under a transparent pixel bleeds
// into its neighbors as a halo
func premultiplied(src image.Image) *image.RGBA {
	if rgba, ok := src.(*image.RGBA); ok && rgba.Rect.Min == (image.Point{}) {
		return rgba
	}
	rgba := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	xdraw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, xdraw.Src)
	return rgba
}

// boxSpans gives the source column each destination column starts at when n
// source pixels shrink to scaled, with one more entry for where the last ends
func boxSpans(n, scaled int) []int {
	cols := make([]int, scaled+1)
	for i := range cols {
		cols[i] = i * n / scaled
	}
	return cols
}

// boxSpan is the source range destination pixel i covers, at least one pixel wide
func boxSpan(cols []int, i int) (lo, hi int) {
	lo, hi = cols[i], cols[i+1]
	if hi <= lo {
		hi = lo + 1
	}
	return lo, hi
}

// boxScaleRegion box-scales a document of docW by docH down to w by h when the
// document is transparent except for src, which sits at (ox, oy) in it. It
// returns only the destination pixels src touches and where they start, and
// each is exactly the pixel scaling the whole document would give, so a layer cut
// to its bounds shrinks to the same pixels as the document-sized layer it was cut from
func boxScaleRegion(src *image.RGBA, ox, oy, docW, docH, w, h int) (*image.RGBA, image.Point) {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	cols, rows := boxSpans(docW, w), boxSpans(docH, h)
	x0, x1 := touched(cols, w, ox, ox+sw)
	y0, y1 := touched(rows, h, oy, oy+sh)
	dst := image.NewRGBA(image.Rect(0, 0, max(x1-x0, 0), max(y1-y0, 0)))
	for y := y0; y < y1; y++ {
		slo, shi := boxSpan(rows, y)
		row := dst.Pix[(y-y0)*dst.Stride:]
		for x := x0; x < x1; x++ {
			xlo, xhi := boxSpan(cols, x)
			// Only the part of the span inside src has color. The rest is
			// transparent, so it adds nothing but its share of the area
			cx0, cx1 := max(xlo, ox), min(xhi, ox+sw)
			var r, g, b, a uint32
			for sy := max(slo, oy); sy < min(shi, oy+sh); sy++ {
				line := src.Pix[(sy-oy)*src.Stride+(cx0-ox)*4 : (sy-oy)*src.Stride+(cx1-ox)*4]
				for i := 0; i < len(line); i += 4 {
					r += uint32(line[i])
					g += uint32(line[i+1])
					b += uint32(line[i+2])
					a += uint32(line[i+3])
				}
			}
			n := uint32((xhi - xlo) * (shi - slo))
			o := (x - x0) * 4
			row[o] = uint8(r / n)
			row[o+1] = uint8(g / n)
			row[o+2] = uint8(b / n)
			row[o+3] = uint8(a / n)
		}
	}
	return dst, image.Pt(x0, y0)
}

// touched is the destination range [lo, hi) whose source spans overlap [from, to)
func touched(cols []int, n, from, to int) (lo, hi int) {
	lo, hi = n, 0
	for i := 0; i < n; i++ {
		a, b := boxSpan(cols, i)
		if b > from && a < to {
			lo, hi = min(lo, i), i+1
		}
	}
	if hi <= lo {
		return 0, 0
	}
	return lo, hi
}

// ImageAt resamples img, which sits at (x, y) in an authored document of docW by
// docH, and returns the part of the scaled document it covers with the point
// that part starts at. Every pixel is the one Image gives for a document-sized
// layer holding img at that position, so cutting a layer to its bounds changes
// no pixel at any scale. An empty result means img lands on no pixel
func (s Scale) ImageAt(img image.Image, x, y, docW, docH int) (image.Image, image.Point) {
	if s.Native() {
		return img, image.Pt(x, y)
	}
	if s.Factor() > 1 {
		return s.Image(img), image.Pt(s.Px(x), s.Px(y))
	}
	out, at := boxScaleRegion(premultiplied(img), x, y, docW, docH, max(s.Px(docW), 1), max(s.Px(docH), 1))
	return out, at
}
