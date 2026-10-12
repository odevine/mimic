package template

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

// Blender mixes a run of colors from left to right across a layer. Each seam is
// a transition between two neighbouring colors, and the weight of each color at a
// position follows from the seams before and after it. The colors mix as they are
// stored, in gamma encoded sRGB, which is how an image editor mixes a gradient and
// so how the art a template is cut from was made
type Blender struct {
	seams  []seamSpan
	smooth bool
	end    float64
}

// seamSpan is where one transition runs, as fractions of the authored document's
// width
type seamSpan struct{ from, to float64 }

// defaultSeam is where a pair's transition runs when a part gives none: centered,
// 650 pixels wide
var defaultSeam = manifest.Seam{Center: 0, Width: 650}

// NewBlender builds the mix of a run of n colors for a part's blend, in an authored
// document docW wide. A pair with no seams takes the default one, and it reports
// false for a longer run the part gives no seams for, which has no mix to draw
func NewBlender(n int, b *manifest.Blend, docW int) (Blender, bool) {
	bl := Blender{smooth: true, end: 1}
	var seams []manifest.Seam
	if b != nil {
		bl.smooth = b.Shape != "linear"
		if b.End > 0 {
			bl.end = b.End
		}
		seams = b.Seams[strconv.Itoa(n)]
	}
	if len(seams) == 0 && n == 2 {
		seams = []manifest.Seam{defaultSeam}
	}
	if n < 2 || n > 4 || len(seams) != n-1 || docW <= 0 {
		return Blender{}, false
	}
	w := float64(docW)
	for _, s := range seams {
		from := (w/2 + s.Center - s.Width/2) / w
		bl.seams = append(bl.seams, seamSpan{from: from, to: from + s.Width/w})
	}
	return bl, true
}

// Shifted moves every seam dx pixels to the right in an authored document docW
// wide, for an entry placed away from its part's origin, which carries its blend
// with it
func (bl Blender) Shifted(dx, docW int) Blender {
	if dx == 0 || docW <= 0 {
		return bl
	}
	d := float64(dx) / float64(docW)
	out := bl
	out.seams = make([]seamSpan, len(bl.seams))
	for i, s := range bl.seams {
		out.seams[i] = seamSpan{from: s.from + d, to: s.to + d}
	}
	return out
}

// Weights is how much of each color shows at a fraction f of the document's width,
// written into w, which holds one weight per color and sums to 1. The far right
// shows End of the last color, and the rest goes to the one before it
func (bl Blender) Weights(f float64, w []float64) {
	// t[k] is how far the transition after color k has come, so a color's weight is
	// how far the one before it has come minus how far the one after it has
	var t [3]float64
	for k, seam := range bl.seams {
		t[k] = bl.ramp(seam, f)
	}
	if n := len(bl.seams); n > 0 {
		t[n-1] *= bl.end
	}
	for k := range w {
		lo, hi := 1.0, 0.0
		if k > 0 {
			lo = t[k-1]
		}
		if k < len(bl.seams) {
			hi = t[k]
		}
		w[k] = lo - hi
	}
}

// ramp is how far a transition has come at fraction f, from 0 to 1
func (bl Blender) ramp(s seamSpan, f float64) float64 {
	if s.to <= s.from {
		if f >= s.to {
			return 1
		}
		return 0
	}
	v := math.Max(0, math.Min(1, (f-s.from)/(s.to-s.from)))
	if bl.smooth {
		v = v * v * (3 - 2*v)
	}
	return v
}

// LoadBlendedImage mixes the images at paths, one per color in the order they run,
// into one layer with the blender, for a layer that belongs at (x, y) in an authored
// document of docW by docH, such as a pair of pinlines or a fuse bar. It returns the
// layer as an 8-bit image with the origin it belongs at in the scaled document, so
// it costs the size of the layer and not of the document, and it can be composited
// without a float buffer. The images must be the same size. A provider that keeps a
// cache holds the result, so every card with the same pair shares one build. With
// indexed set the layer comes back with an index of its visible pixels when it is
// cached. It returns a nil image for a layer that lands on no pixel
func LoadBlendedImage(p AssetProvider, paths []string, bl Blender, s Scale, x, y, docW, docH int, indexed bool) (image.Image, image.Point, error) {
	build := func() (image.Image, image.Point, error) { return buildBlended(p, paths, bl, s, x, y, docW, docH) }
	if c, ok := p.(blendCache); ok {
		return c.LoadBlended(blendKey(paths, bl, s, x, y, docW, docH), indexed, build)
	}
	return build()
}

// blendCache is a provider that keeps layers it built
type blendCache interface {
	LoadBlended(key string, indexed bool, build func() (image.Image, image.Point, error)) (image.Image, image.Point, error)
}

// blendKey names a blended layer by everything it is built from
func blendKey(paths []string, bl Blender, s Scale, x, y, docW, docH int) string {
	var b strings.Builder
	b.WriteString("blend:")
	for _, p := range paths {
		b.WriteString(strconv.Quote(p))
		b.WriteByte(',')
	}
	fmt.Fprintf(&b, "%v|%v|%v|%v|%v,%v|%v,%v|", bl.seams, bl.smooth, bl.end, s.Factor(), x, y, docW, docH)
	return b.String()
}

func buildBlended(p AssetProvider, paths []string, bl Blender, s Scale, x, y, docW, docH int) (image.Image, image.Point, error) {
	srcs := make([]blendSource, 0, len(paths))
	var origin image.Point
	for _, path := range paths {
		img, at, err := loadLayerImage(p, path, s, x, y, docW, docH, false)
		if err != nil {
			return nil, image.Point{}, err
		}
		if img == nil {
			return nil, at, nil
		}
		origin = at
		src := newBlendSource(img)
		if len(srcs) > 0 && src.rect.Size() != srcs[0].rect.Size() {
			return nil, image.Point{}, fmt.Errorf("template: blended layers %q and %q differ in size", paths[0], path)
		}
		srcs = append(srcs, src)
	}
	w, h := srcs[0].rect.Dx(), srcs[0].rect.Dy()
	docWide := max(s.Px(docW), 1)
	if s.Native() {
		docWide = docW
	}
	// A column's weights are the same all the way down it
	weights := make([][4]float32, w)
	col := make([]float64, len(srcs))
	for sx := range weights {
		bl.Weights((float64(origin.X+sx)+0.5)/float64(docWide), col)
		for k, v := range col {
			weights[sx][k] = float32(v)
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for sy := 0; sy < h; sy++ {
		dst := out.Pix[sy*out.Stride : sy*out.Stride+4*w]
		for sx := 0; sx < w; sx++ {
			var r, g, b, a float32
			for k := range srcs {
				wt := weights[sx][k]
				if wt == 0 {
					continue
				}
				pr, pg, pb, pa := srcs[k].premultiplied(sx, sy)
				r += wt * pr
				g += wt * pg
				b += wt * pb
				a += wt * pa
			}
			if a <= 0 {
				continue
			}
			// The mix is premultiplied, so it is divided by its alpha to store it straight
			d := dst[4*sx : 4*sx+4]
			scale := 255 / a
			d[0], d[1], d[2] = clamp8(r*scale), clamp8(g*scale), clamp8(b*scale)
			d[3] = clamp8(a)
		}
	}
	return out, origin, nil
}

func clamp8(v float32) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v + 0.5)
}

// blendSource reads the premultiplied color of one layer's pixels
type blendSource struct {
	pix      []uint8
	stride   int
	rect     image.Rectangle
	straight bool
}

func newBlendSource(img image.Image) blendSource {
	switch m := img.(type) {
	case *image.NRGBA:
		return blendSource{pix: m.Pix, stride: m.Stride, rect: m.Rect, straight: true}
	case *image.RGBA:
		return blendSource{pix: m.Pix, stride: m.Stride, rect: m.Rect}
	}
	m := premultiplied(img)
	return blendSource{pix: m.Pix, stride: m.Stride, rect: m.Rect}
}

// premultiplied is the pixel at (x, y) of the layer, relative to its corner, with
// each color already multiplied by its alpha
func (b blendSource) premultiplied(x, y int) (r, g, bl, a float32) {
	i := y*b.stride + 4*x
	px := b.pix[i : i+4]
	a = float32(px[3])
	if !b.straight || px[3] == 255 {
		return float32(px[0]), float32(px[1]), float32(px[2]), a
	}
	f := a / 255
	return float32(px[0]) * f, float32(px[1]) * f, float32(px[2]) * f, a
}

// BlendedPaths lists the variants a run of colors blends, one per color in the
// order they run, or nil when the material is not a run or a color has no variant
// of its own
func BlendedPaths(variants map[string]string, m frame.Material) []string {
	if m.Kind() != frame.MatRun {
		return nil
	}
	var paths []string
	for _, l := range m.Letters() {
		path, ok := variants[l]
		if !ok {
			return nil
		}
		paths = append(paths, path)
	}
	return paths
}
