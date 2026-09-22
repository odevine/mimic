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
	// Averaging happens in premultiplied alpha, or the color under a transparent
	// pixel bleeds into its neighbors as a halo
	rgba, ok := src.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
		xdraw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, xdraw.Src)
	}
	sw, sh := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	// The source span each destination column covers, worked out once so the
	// inner loop divides nothing
	cols := make([]int, w+1)
	for x := range cols {
		cols[x] = x * sw / w
	}
	for y := range h {
		sy0, sy1 := y*sh/h, (y+1)*sh/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		row := dst.Pix[y*dst.Stride:]
		for x := range w {
			sx0, sx1 := cols[x], cols[x+1]
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, b, a uint32
			for sy := sy0; sy < sy1; sy++ {
				line := rgba.Pix[sy*rgba.Stride+sx0*4 : sy*rgba.Stride+sx1*4]
				for i := 0; i < len(line); i += 4 {
					r += uint32(line[i])
					g += uint32(line[i+1])
					b += uint32(line[i+2])
					a += uint32(line[i+3])
				}
			}
			n := uint32((sx1 - sx0) * (sy1 - sy0))
			o := x * 4
			row[o] = uint8(r / n)
			row[o+1] = uint8(g / n)
			row[o+2] = uint8(b / n)
			row[o+3] = uint8(a / n)
		}
	}
	return dst
}

// NativeDPI is the resolution a manifest's canvas was authored at. One that
// states its own DPI reports that; one that does not infers it from its width
// and the physical width of a card with bleed
func (m *Manifest) NativeDPI() int {
	if m.DPI > 0 {
		return m.DPI
	}
	return int(math.Round(float64(m.Width) / CardWidthInches))
}

// ClampDPI holds dpi within the range m renders at: no less than MinDPI, and no
// more than the authored resolution. A non-positive dpi means native
func (m *Manifest) ClampDPI(dpi int) int {
	native := m.NativeDPI()
	switch {
	case dpi <= 0 || dpi > native:
		return native
	case dpi < MinDPI:
		return min(MinDPI, native)
	default:
		return dpi
	}
}

// ScaleForDPI is the factor that renders m at dpi, which is clamped first, so a
// caller can pass a saved preference through without checking it against the
// template that happens to be active
func (m *Manifest) ScaleForDPI(dpi int) Scale {
	native := m.NativeDPI()
	if native <= 0 {
		return 1
	}
	dpi = m.ClampDPI(dpi)
	if dpi == native {
		return 1
	}
	return Scale(float64(dpi) / float64(native))
}

// Resolution is one render size: the pixel dimensions it produces and the dpi
// those dimensions print at
type Resolution struct {
	DPI    int  `json:"dpi"`
	Width  int  `json:"width"`
	Height int  `json:"height"`
	Native bool `json:"native,omitempty"`
}

// Resolution reports the pixel size m renders at for dpi, clamped the way
// ScaleForDPI clamps it
func (m *Manifest) Resolution(dpi int) Resolution {
	dpi = m.ClampDPI(dpi)
	s := m.ScaleForDPI(dpi)
	return Resolution{
		DPI:    dpi,
		Width:  s.Px(m.Width),
		Height: s.Px(m.Height),
		Native: dpi == m.NativeDPI(),
	}
}

// Presets lists the resolutions a ui offers for m, smallest first and ending
// with the template's own
func (m *Manifest) Presets() []Resolution {
	native := m.NativeDPI()
	out := make([]Resolution, 0, len(PresetDPIs)+1)
	for _, dpi := range PresetDPIs {
		if dpi >= native || dpi < MinDPI {
			continue
		}
		out = append(out, m.Resolution(dpi))
	}
	return append(out, m.Resolution(native))
}

// Scaled returns a copy of m with every pixel measurement multiplied by s.
// Measurements the manifest states as ratios, such as line spacing in ems or a
// shadow distance in font sizes, are already resolution independent and carry
// over untouched. The layer list is shared with m, since it holds asset paths
// rather than geometry
func (m *Manifest) Scaled(s Scale) *Manifest {
	if m == nil || s.Native() {
		return m
	}
	out := *m
	out.Width, out.Height = s.Px(m.Width), s.Px(m.Height)
	out.DPI = int(math.Round(float64(m.NativeDPI()) * s.Factor()))
	out.Art.X, out.Art.Y = s.Px(m.Art.X), s.Px(m.Art.Y)
	out.Art.Width, out.Art.Height = s.Px(m.Art.Width), s.Px(m.Art.Height)
	out.TextBoxes = make(map[string]TextBoxSpec, len(m.TextBoxes))
	for name, box := range m.TextBoxes {
		out.TextBoxes[name] = scaleBox(box, s)
	}
	return &out
}

// scaleBox multiplies one text box's pixel geometry. Tracking is in thousandths
// of an em, line spacing and the clear gap are multiples of the font size, and
// a shadow's distance is a fraction of it, so all four scale with the font on
// their own
func scaleBox(box TextBoxSpec, s Scale) TextBoxSpec {
	box.X, box.Y = s.Px(box.X), s.Px(box.Y)
	box.Width, box.Height = s.Px(box.Width), s.Px(box.Height)
	box.FontSize = s.F(box.FontSize)
	box.MinFontSize = s.F(box.MinFontSize)
	box.Padding = s.Px(box.Padding)
	box.PaddingX = scalePad(box.PaddingX, s)
	box.PaddingY = scalePad(box.PaddingY, s)
	box.Avoid = s.Rect(box.Avoid)
	return box
}

// scalePad scales an axis padding override, keeping nil as nil so the box still
// falls back to its all-round padding
func scalePad(p *int, s Scale) *int {
	if p == nil {
		return nil
	}
	v := s.Px(*p)
	return &v
}
