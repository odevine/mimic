package svgpath

import (
	"math"

	"github.com/odevine/impasto/path"
)

// outline is a path kept as the commands that drew it. impasto's Path cannot be
// moved or scaled once built, so a shape holds its commands and rebuilds the
// path through whatever transform its caller needs
type outline struct{ segs []segment }

// segment is one drawing command with its points: one for a move or line, two
// for a quadratic, three for a cubic, and none for a close
type segment struct {
	op  byte // 'M', 'L', 'Q', 'C' or 'Z'
	pts [3]path.Point
}

func (o *outline) MoveTo(x, y float32) *outline {
	o.segs = append(o.segs, segment{op: 'M', pts: [3]path.Point{{X: x, Y: y}}})
	return o
}

func (o *outline) LineTo(x, y float32) *outline {
	o.segs = append(o.segs, segment{op: 'L', pts: [3]path.Point{{X: x, Y: y}}})
	return o
}

func (o *outline) QuadTo(cx, cy, x, y float32) *outline {
	o.segs = append(o.segs, segment{op: 'Q', pts: [3]path.Point{{X: cx, Y: cy}, {X: x, Y: y}}})
	return o
}

func (o *outline) CubicTo(c1x, c1y, c2x, c2y, x, y float32) *outline {
	o.segs = append(o.segs, segment{op: 'C', pts: [3]path.Point{{X: c1x, Y: c1y}, {X: c2x, Y: c2y}, {X: x, Y: y}}})
	return o
}

func (o *outline) Close() *outline {
	if len(o.segs) == 0 {
		return o
	}
	o.segs = append(o.segs, segment{op: 'Z'})
	return o
}

// Rect appends an axis-aligned rectangle as a closed subpath
func (o *outline) Rect(x0, y0, x1, y1 float32) *outline {
	return o.MoveTo(x0, y0).LineTo(x1, y0).LineTo(x1, y1).LineTo(x0, y1).Close()
}

// Ellipse appends an ellipse as a closed subpath of four cubics
func (o *outline) Ellipse(cx, cy, rx, ry float32) *outline {
	const k = 0.5522847498307936 // 4/3 * (sqrt(2)-1), the circle-to-cubic constant
	ox, oy := rx*k, ry*k
	o.MoveTo(cx+rx, cy)
	o.CubicTo(cx+rx, cy+oy, cx+ox, cy+ry, cx, cy+ry)
	o.CubicTo(cx-ox, cy+ry, cx-rx, cy+oy, cx-rx, cy)
	o.CubicTo(cx-rx, cy-oy, cx-ox, cy-ry, cx, cy-ry)
	o.CubicTo(cx+ox, cy-ry, cx+rx, cy-oy, cx+rx, cy)
	return o.Close()
}

// Empty reports whether nothing has been drawn
func (o *outline) Empty() bool { return len(o.segs) == 0 }

// build replays the commands into an impasto path, mapping each point (x, y) to
// (x*sx+dx, y*sy+dy)
func (o *outline) build(sx, sy, dx, dy float32) *path.Path {
	p := path.New()
	at := func(pt path.Point) (float32, float32) { return pt.X*sx + dx, pt.Y*sy + dy }
	for _, s := range o.segs {
		switch s.op {
		case 'M':
			p.MoveTo(at(s.pts[0]))
		case 'L':
			p.LineTo(at(s.pts[0]))
		case 'Q':
			cx, cy := at(s.pts[0])
			x, y := at(s.pts[1])
			p.QuadTo(cx, cy, x, y)
		case 'C':
			c1x, c1y := at(s.pts[0])
			c2x, c2y := at(s.pts[1])
			x, y := at(s.pts[2])
			p.CubicTo(c1x, c1y, c2x, c2y, x, y)
		case 'Z':
			p.Close()
		}
	}
	return p
}

// bounds is the tight box around the drawn outline. Curves are sampled, since
// the box around a curve's control points can sit well outside the curve
func (o *outline) bounds() (minX, minY, maxX, maxY float32, ok bool) {
	minX, minY = float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY = float32(math.Inf(-1)), float32(math.Inf(-1))
	add := func(x, y float32) {
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
		ok = true
	}
	var cur path.Point
	for _, s := range o.segs {
		switch s.op {
		case 'M', 'L':
			cur = s.pts[0]
			add(cur.X, cur.Y)
		case 'Q':
			for i := 1; i <= curveSamples; i++ {
				t := float32(i) / curveSamples
				u := 1 - t
				add(u*u*cur.X+2*u*t*s.pts[0].X+t*t*s.pts[1].X, u*u*cur.Y+2*u*t*s.pts[0].Y+t*t*s.pts[1].Y)
			}
			cur = s.pts[1]
		case 'C':
			for i := 1; i <= curveSamples; i++ {
				t := float32(i) / curveSamples
				u := 1 - t
				a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
				add(a*cur.X+b*s.pts[0].X+c*s.pts[1].X+d*s.pts[2].X, a*cur.Y+b*s.pts[0].Y+c*s.pts[1].Y+d*s.pts[2].Y)
			}
			cur = s.pts[2]
		}
	}
	return
}

// curveSamples is how many points of each curve bounds checks
const curveSamples = 24

// apply returns the outline with every point moved through m
func (o *outline) apply(m Affine) *outline {
	out := &outline{segs: make([]segment, len(o.segs))}
	for i, s := range o.segs {
		for j := range s.pts {
			x, y := m.Apply(float64(s.pts[j].X), float64(s.pts[j].Y))
			s.pts[j] = path.Point{X: float32(x), Y: float32(y)}
		}
		out.segs[i] = s
	}
	return out
}

// isAxisRect reports whether the outline is a single axis-aligned rectangle, and
// its bounds when it is
func (o *outline) isAxisRect() (Rect, bool) {
	var pts []path.Point
	for _, s := range o.segs {
		switch s.op {
		case 'M', 'L':
			pts = append(pts, s.pts[0])
		case 'Z':
		default:
			return Rect{}, false
		}
	}
	if len(pts) == 5 && pts[4] == pts[0] {
		pts = pts[:4]
	}
	if len(pts) != 4 {
		return Rect{}, false
	}
	for i := range pts {
		a, b := pts[i], pts[(i+1)%4]
		if a.X != b.X && a.Y != b.Y {
			return Rect{}, false
		}
	}
	minX, minY, maxX, maxY, _ := o.bounds()
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}, true
}

// buildAffine replays the commands into an impasto path, moving each point
// through m
func (o *outline) buildAffine(m Affine) *path.Path {
	return o.apply(m).build(1, 1, 0, 0)
}
