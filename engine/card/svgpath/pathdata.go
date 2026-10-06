package svgpath

import (
	"fmt"
	"math"
	"strconv"
)

// maxCoord bounds every coordinate a path reaches. Real icons live in boxes of
// a few thousand units, and a limit keeps a hostile file from handing the
// rasterizer geometry it cannot sensibly draw
const maxCoord = 1e6

// maxArcRadius bounds an arc's radius. An exporter writes a nearly straight curve as an
// arc of enormous radius, which draws no further than its end points
const maxArcRadius = 1e9

// pathData builds an impasto path from the contents of an SVG d attribute. It
// follows the grammar in https://www.w3.org/TR/SVG11/paths.html#PathData,
// where numbers may run together ("1.5.5", "-.5", "a1 1 0 00.5.5"), a command
// letter may be followed by several argument sets, and a moveto's extra sets
// are linetos
func pathData(d string) (*outline, error) {
	b := &pathBuilder{p: &outline{}}
	s := &scanner{s: d}
	var cmd byte
	for {
		s.skipSpace()
		if s.eof() {
			break
		}
		if c := s.s[s.i]; isCommand(c) {
			cmd = c
			s.i++
		} else if cmd == 0 {
			return nil, s.errorf("path data must start with a command")
		} else if cmd == 'z' || cmd == 'Z' {
			return nil, s.errorf("number after closepath")
		}
		if err := b.run(cmd, s); err != nil {
			return nil, err
		}
		// A moveto's further coordinate pairs are implicit linetos
		switch cmd {
		case 'M':
			cmd = 'L'
		case 'm':
			cmd = 'l'
		}
	}
	if b.p.Empty() {
		return nil, fmt.Errorf("svgpath: path data draws nothing")
	}
	return b.p, nil
}

func isCommand(c byte) bool {
	switch c {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's', 'Q', 'q', 'T', 't', 'A', 'a', 'Z', 'z':
		return true
	}
	return false
}

// pathBuilder carries the pen state a path's commands read: the current point, the
// start of the current subpath for closepath, and the last control point so a
// smooth curve can reflect it
type pathBuilder struct {
	p        *outline
	x, y     float64
	sx, sy   float64
	cx, cy   float64 // last control point of the previous curve
	prev     byte    // upper-case previous command, for smooth-curve reflection
	started  bool
	needMove bool // a closepath ended the subpath, so drawing needs a fresh moveto
}

// run reads one command's arguments from s and applies them
func (b *pathBuilder) run(cmd byte, s *scanner) error {
	rel := cmd >= 'a'
	up := cmd &^ 0x20
	if up != 'M' && up != 'Z' && !b.started {
		return s.errorf("path data must start with a moveto")
	}
	switch up {
	case 'Z':
		b.p.Close()
		b.x, b.y = b.sx, b.sy
		b.needMove = true
		b.prev = 'Z'
		return nil
	case 'M':
		x, y, err := b.pair(s, rel)
		if err != nil {
			return err
		}
		b.p.MoveTo(float32(x), float32(y))
		b.x, b.y, b.sx, b.sy = x, y, x, y
		b.started, b.needMove = true, false
		b.prev = 'M'
		return nil
	}
	if b.needMove {
		b.p.MoveTo(float32(b.sx), float32(b.sy))
		b.needMove = false
	}
	switch up {
	case 'L':
		x, y, err := b.pair(s, rel)
		if err != nil {
			return err
		}
		b.p.LineTo(float32(x), float32(y))
		b.x, b.y = x, y
	case 'H':
		v, err := s.number()
		if err != nil {
			return err
		}
		x := v
		if rel {
			x += b.x
		}
		if err := checkCoord(s, x); err != nil {
			return err
		}
		b.p.LineTo(float32(x), float32(b.y))
		b.x = x
	case 'V':
		v, err := s.number()
		if err != nil {
			return err
		}
		y := v
		if rel {
			y += b.y
		}
		if err := checkCoord(s, y); err != nil {
			return err
		}
		b.p.LineTo(float32(b.x), float32(y))
		b.y = y
	case 'C', 'S':
		var c1x, c1y float64
		if up == 'S' {
			c1x, c1y = b.x, b.y
			if b.prev == 'C' || b.prev == 'S' {
				c1x, c1y = 2*b.x-b.cx, 2*b.y-b.cy
			}
		} else {
			var err error
			if c1x, c1y, err = b.pair(s, rel); err != nil {
				return err
			}
		}
		c2x, c2y, err := b.pair(s, rel)
		if err != nil {
			return err
		}
		x, y, err := b.pair(s, rel)
		if err != nil {
			return err
		}
		b.p.CubicTo(float32(c1x), float32(c1y), float32(c2x), float32(c2y), float32(x), float32(y))
		b.cx, b.cy = c2x, c2y
		b.x, b.y = x, y
	case 'Q', 'T':
		var qx, qy float64
		if up == 'T' {
			qx, qy = b.x, b.y
			if b.prev == 'Q' || b.prev == 'T' {
				qx, qy = 2*b.x-b.cx, 2*b.y-b.cy
			}
		} else {
			var err error
			if qx, qy, err = b.pair(s, rel); err != nil {
				return err
			}
		}
		x, y, err := b.pair(s, rel)
		if err != nil {
			return err
		}
		b.p.QuadTo(float32(qx), float32(qy), float32(x), float32(y))
		b.cx, b.cy = qx, qy
		b.x, b.y = x, y
	case 'A':
		if err := b.arc(s, rel); err != nil {
			return err
		}
	}
	b.prev = up
	return nil
}

// pair reads an x y coordinate pair, resolving it against the current point
// for a relative command
func (b *pathBuilder) pair(s *scanner, rel bool) (x, y float64, err error) {
	if x, err = s.number(); err != nil {
		return
	}
	if y, err = s.number(); err != nil {
		return
	}
	if rel {
		x += b.x
		y += b.y
	}
	if err = checkCoord(s, x); err != nil {
		return
	}
	err = checkCoord(s, y)
	return
}

func checkCoord(s *scanner, v float64) error {
	if math.Abs(v) > maxCoord {
		return s.errorf("coordinate %g is out of range", v)
	}
	return nil
}

// arc reads an elliptical arc's arguments and appends it as cubic curves
func (b *pathBuilder) arc(s *scanner, rel bool) error {
	rx, err := s.number()
	if err != nil {
		return err
	}
	ry, err := s.number()
	if err != nil {
		return err
	}
	rot, err := s.number()
	if err != nil {
		return err
	}
	large, err := s.flag()
	if err != nil {
		return err
	}
	sweep, err := s.flag()
	if err != nil {
		return err
	}
	x, y, err := b.pair(s, rel)
	if err != nil {
		return err
	}
	if math.Abs(rx) > maxArcRadius || math.Abs(ry) > maxArcRadius {
		return s.errorf("arc radius is out of range")
	}
	arcToCubics(b.p, b.x, b.y, rx, ry, rot, large, sweep, x, y)
	b.x, b.y = x, y
	return nil
}

// arcToCubics appends the arc from (x1,y1) to (x2,y2) as at most four cubics
// per quarter turn, following the endpoint to center conversion in
// https://www.w3.org/TR/SVG11/implnote.html#ArcImplementationNotes
func arcToCubics(p *outline, x1, y1, rx, ry, rotDeg float64, large, sweep bool, x2, y2 float64) {
	if x1 == x2 && y1 == y2 {
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		p.LineTo(float32(x2), float32(y2))
		return
	}
	phi := rotDeg * math.Pi / 180
	cosP, sinP := math.Cos(phi), math.Sin(phi)
	dx, dy := (x1-x2)/2, (y1-y2)/2
	x1p := cosP*dx + sinP*dy
	y1p := -sinP*dx + cosP*dy

	// Radii too small to span the endpoints grow until they just do
	if l := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry); l > 1 {
		sq := math.Sqrt(l)
		rx, ry = rx*sq, ry*sq
	}
	rx2, ry2 := rx*rx, ry*ry
	num := rx2*ry2 - rx2*y1p*y1p - ry2*x1p*x1p
	den := rx2*y1p*y1p + ry2*x1p*x1p
	coef := 0.0
	if den != 0 && num > 0 {
		coef = math.Sqrt(num / den)
	}
	if large == sweep {
		coef = -coef
	}
	cxp := coef * rx * y1p / ry
	cyp := -coef * ry * x1p / rx
	cx := cosP*cxp - sinP*cyp + (x1+x2)/2
	cy := sinP*cxp + cosP*cyp + (y1+y2)/2

	ux, uy := (x1p-cxp)/rx, (y1p-cyp)/ry
	vx, vy := (-x1p-cxp)/rx, (-y1p-cyp)/ry
	theta := math.Atan2(uy, ux)
	delta := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	} else if sweep && delta < 0 {
		delta += 2 * math.Pi
	}

	n := int(math.Ceil(math.Abs(delta) / (math.Pi / 2)))
	if n < 1 {
		n = 1
	}
	step := delta / float64(n)
	k := 4.0 / 3.0 * math.Tan(step/4)
	at := func(ux, uy float64) (float64, float64) {
		return cx + cosP*rx*ux - sinP*ry*uy, cy + sinP*rx*ux + cosP*ry*uy
	}
	for i := 0; i < n; i++ {
		a1 := theta + float64(i)*step
		a2 := a1 + step
		s1, c1 := math.Sincos(a1)
		s2, c2 := math.Sincos(a2)
		p1x, p1y := at(c1-k*s1, s1+k*c1)
		p2x, p2y := at(c2+k*s2, s2-k*c2)
		ex, ey := at(c2, s2)
		if i == n-1 {
			ex, ey = x2, y2
		}
		p.CubicTo(float32(p1x), float32(p1y), float32(p2x), float32(p2y), float32(ex), float32(ey))
	}
}

// scanner reads the numbers and flags of a path data string
type scanner struct {
	s string
	i int
}

func (s *scanner) eof() bool { return s.i >= len(s.s) }

func (s *scanner) errorf(format string, args ...any) error {
	return fmt.Errorf("svgpath: path data at offset %d: %s", s.i, fmt.Sprintf(format, args...))
}

// skipSpace passes whitespace and the commas that separate arguments
func (s *scanner) skipSpace() {
	for s.i < len(s.s) {
		switch s.s[s.i] {
		case ' ', '\t', '\n', '\r', '\f', ',':
			s.i++
		default:
			return
		}
	}
}

// number reads one number, leaving the scanner after it. A second decimal point
// or a sign starts the next number, which is how compact data runs together
func (s *scanner) number() (float64, error) {
	s.skipSpace()
	start := s.i
	if s.i < len(s.s) && (s.s[s.i] == '+' || s.s[s.i] == '-') {
		s.i++
	}
	digits := 0
	for s.i < len(s.s) && isDigit(s.s[s.i]) {
		s.i++
		digits++
	}
	if s.i < len(s.s) && s.s[s.i] == '.' {
		s.i++
		for s.i < len(s.s) && isDigit(s.s[s.i]) {
			s.i++
			digits++
		}
	}
	if digits == 0 {
		s.i = start
		return 0, s.errorf("expected a number")
	}
	if s.i < len(s.s) && (s.s[s.i] == 'e' || s.s[s.i] == 'E') {
		j := s.i + 1
		if j < len(s.s) && (s.s[j] == '+' || s.s[j] == '-') {
			j++
		}
		if j < len(s.s) && isDigit(s.s[j]) {
			for j < len(s.s) && isDigit(s.s[j]) {
				j++
			}
			s.i = j
		}
	}
	v, err := strconv.ParseFloat(s.s[start:s.i], 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, s.errorf("number %q is out of range", s.s[start:s.i])
	}
	return v, nil
}

// flag reads an arc flag, which is a single 0 or 1 character and so may sit
// directly against the next argument
func (s *scanner) flag() (bool, error) {
	s.skipSpace()
	if s.i < len(s.s) && (s.s[s.i] == '0' || s.s[s.i] == '1') {
		s.i++
		return s.s[s.i-1] == '1', nil
	}
	return false, s.errorf("expected an arc flag, 0 or 1")
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
