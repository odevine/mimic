package svgpath

import (
	"image/color"
	"math"

	"github.com/odevine/impasto/path"
	"github.com/odevine/impasto/raster"
)

// paintSource is what fills or strokes a shape: a flat color or a gradient
type paintSource interface {
	// paint returns the per-pixel paint. local maps the shape's own coordinates,
	// the ones its gradient is defined in, to destination pixels, and alpha is
	// the opacity to apply on top of the paint's own
	paint(local Affine, alpha float64) path.Paint
}

// solid is a flat color
type solid struct{ c color.NRGBA }

func (s solid) paint(_ Affine, alpha float64) path.Paint {
	return flat(s.c, float64(s.c.A)/255*alpha)
}

// flat is a straight sRGB color at an opacity as a premultiplied linear paint
func flat(c color.NRGBA, a float64) path.FlatColor {
	return path.NewFlatColor(
		raster.SRGBToLinear(float32(c.R)/255),
		raster.SRGBToLinear(float32(c.G)/255),
		raster.SRGBToLinear(float32(c.B)/255),
		float32(a),
	)
}

// spread is what a gradient does outside its 0 to 1 range
type spread int

const (
	spreadPad spread = iota
	spreadReflect
	spreadRepeat
)

// stop is one color along a gradient, a straight sRGB color with its own opacity
type stop struct {
	offset  float64
	c       color.NRGBA
	opacity float64
}

// gradient is a linear or radial gradient as the file defines it
type gradient struct {
	radial bool
	// Linear gradients run from (x1, y1) to (x2, y2). Radial ones are the circle at
	// (cx, cy) with radius r, lit from the focal point (fx, fy)
	x1, y1, x2, y2 float64
	cx, cy, r      float64
	fx, fy         float64
	// bbox places the gradient in the shape's bounding box instead of its own
	// coordinate system
	bbox      bool
	bounds    Rect
	transform Affine
	spread    spread
	stops     []stop
}

func (g *gradient) paint(local Affine, alpha float64) path.Paint {
	last := g.stops[len(g.stops)-1]
	m := local
	if g.bbox {
		if g.bounds.W <= 0 || g.bounds.H <= 0 {
			// A bounding box with no width or height paints nothing
			return path.FlatColor{}
		}
		m = m.Mul(Affine{A: float64(g.bounds.W), D: float64(g.bounds.H), E: float64(g.bounds.X), F: float64(g.bounds.Y)})
	}
	m = m.Mul(g.transform)
	inv, ok := m.Invert()
	if !ok {
		return path.FlatColor{}
	}
	gp := &gradPaint{inv: inv, spread: g.spread, alpha: alpha, radial: g.radial}
	for _, s := range g.stops {
		gp.pos = append(gp.pos, float32(s.offset))
		gp.col = append(gp.col, [4]float32{float32(s.c.R) / 255, float32(s.c.G) / 255, float32(s.c.B) / 255, float32(s.c.A) / 255 * float32(s.opacity)})
	}
	if g.radial {
		if g.r <= 0 {
			return flat(last.c, float64(last.c.A)/255*last.opacity*alpha)
		}
		gp.cx, gp.cy, gp.r = g.cx, g.cy, g.r
		// The focal point is held just inside the circle, as SVG 1.1 asks, so every
		// point has a gradient parameter
		fx, fy := g.fx, g.fy
		if d := math.Hypot(g.cx-fx, g.cy-fy); d > g.r*0.999 {
			k := g.r * 0.999 / d
			fx, fy = g.cx+(fx-g.cx)*k, g.cy+(fy-g.cy)*k
		}
		gp.fx, gp.fy = fx, fy
		gp.ex, gp.ey = g.cx-fx, g.cy-fy
	} else {
		dx, dy := g.x2-g.x1, g.y2-g.y1
		len2 := dx*dx + dy*dy
		if len2 == 0 {
			// A gradient with no length paints its last stop everywhere
			return flat(last.c, float64(last.c.A)/255*last.opacity*alpha)
		}
		gp.x1, gp.y1, gp.dx, gp.dy, gp.len2 = g.x1, g.y1, dx, dy, len2
	}
	return gp
}

// gradPaint is a gradient bound to a position on the page
type gradPaint struct {
	inv    Affine // pixels to gradient coordinates
	spread spread
	alpha  float64
	radial bool
	pos    []float32
	col    [][4]float32 // straight sRGB with opacity last

	x1, y1, dx, dy, len2      float64
	cx, cy, r, fx, fy, ex, ey float64
}

func (g *gradPaint) ColorAt(x, y int) [4]float32 {
	px, py := g.inv.Apply(float64(x)+0.5, float64(y)+0.5)
	var t float64
	if g.radial {
		t = g.radialT(px, py)
	} else {
		t = ((px-g.x1)*g.dx + (py-g.y1)*g.dy) / g.len2
	}
	t = g.applySpread(t)
	r, gg, b, a := g.sample(float32(t))
	a *= float32(g.alpha)
	return [4]float32{
		raster.SRGBToLinear(r) * a,
		raster.SRGBToLinear(gg) * a,
		raster.SRGBToLinear(b) * a,
		a,
	}
}

// radialT is the parameter of the gradient circle that passes through (px, py),
// where the circles grow from the focal point out to the full one
func (g *gradPaint) radialT(px, py float64) float64 {
	dx, dy := px-g.fx, py-g.fy
	a := g.ex*g.ex + g.ey*g.ey - g.r*g.r
	b := -2 * (dx*g.ex + dy*g.ey)
	c := dx*dx + dy*dy
	disc := b*b - 4*a*c
	if disc < 0 {
		disc = 0
	}
	// a is negative with the focal point inside the circle
	return (-b - math.Sqrt(disc)) / (2 * a)
}

func (g *gradPaint) applySpread(t float64) float64 {
	switch g.spread {
	case spreadRepeat:
		return t - math.Floor(t)
	case spreadReflect:
		u := math.Mod(t, 2)
		if u < 0 {
			u += 2
		}
		if u > 1 {
			u = 2 - u
		}
		return u
	}
	return math.Min(1, math.Max(0, t))
}

// sample interpolates the stops in straight sRGB, which is how SVG blends them
func (g *gradPaint) sample(t float32) (r, gg, b, a float32) {
	n := len(g.pos)
	if t <= g.pos[0] {
		c := g.col[0]
		return c[0], c[1], c[2], c[3]
	}
	if t >= g.pos[n-1] {
		c := g.col[n-1]
		return c[0], c[1], c[2], c[3]
	}
	hi := 1
	for hi < n-1 && g.pos[hi] <= t {
		hi++
	}
	lo := hi - 1
	span := g.pos[hi] - g.pos[lo]
	var f float32
	if span > 0 {
		f = (t - g.pos[lo]) / span
	}
	c0, c1 := g.col[lo], g.col[hi]
	return c0[0] + (c1[0]-c0[0])*f, c0[1] + (c1[1]-c0[1])*f, c0[2] + (c1[2]-c0[2])*f, c0[3] + (c1[3]-c0[3])*f
}
