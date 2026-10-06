package svgpath

import (
	"fmt"
	"math"
	"strings"
)

// Affine is a 2D affine transform in SVG's matrix(a b c d e f) order, mapping a
// point (x, y) to (A*x + C*y + E, B*x + D*y + F)
type Affine struct{ A, B, C, D, E, F float64 }

// Identity leaves every point where it is
var Identity = Affine{A: 1, D: 1}

// Translate moves points by (x, y)
func Translate(x, y float64) Affine { return Affine{A: 1, D: 1, E: x, F: y} }

// ScaleBy scales points about the origin
func ScaleBy(sx, sy float64) Affine { return Affine{A: sx, D: sy} }

// Mul returns the transform that applies n first and then m
func (m Affine) Mul(n Affine) Affine {
	return Affine{
		A: m.A*n.A + m.C*n.B,
		B: m.B*n.A + m.D*n.B,
		C: m.A*n.C + m.C*n.D,
		D: m.B*n.C + m.D*n.D,
		E: m.A*n.E + m.C*n.F + m.E,
		F: m.B*n.E + m.D*n.F + m.F,
	}
}

// Apply maps a point through the transform
func (m Affine) Apply(x, y float64) (float64, float64) {
	return m.A*x + m.C*y + m.E, m.B*x + m.D*y + m.F
}

// Invert returns the inverse transform, and false when the transform flattens
// the plane and has none
func (m Affine) Invert() (Affine, bool) {
	det := m.A*m.D - m.B*m.C
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return Identity, false
	}
	return Affine{
		A: m.D / det,
		B: -m.B / det,
		C: -m.C / det,
		D: m.A / det,
		E: (m.C*m.F - m.D*m.E) / det,
		F: (m.B*m.E - m.A*m.F) / det,
	}, true
}

// Scale is the factor the transform scales lengths by, the square root of its
// area scale. It is what a stroke width is multiplied by when the transform is
// close to a uniform scale
func (m Affine) Scale() float64 { return math.Sqrt(math.Abs(m.A*m.D - m.B*m.C)) }

// parseTransform reads an SVG transform list such as "translate(1 2) rotate(90)",
// applying each entry in turn, the first being the outermost
func parseTransform(s string) (Affine, error) {
	out := Identity
	s = strings.TrimSpace(s)
	for s != "" {
		open := strings.IndexByte(s, '(')
		closeIdx := strings.IndexByte(s, ')')
		if open < 0 || closeIdx < open {
			return Identity, fmt.Errorf("svgpath: transform %q is malformed", s)
		}
		name := strings.TrimSpace(s[:open])
		args, err := numberList(s[open+1 : closeIdx])
		if err != nil {
			return Identity, fmt.Errorf("svgpath: transform %s: %w", name, err)
		}
		t, err := transformOf(name, args)
		if err != nil {
			return Identity, err
		}
		out = out.Mul(t)
		s = strings.TrimLeft(s[closeIdx+1:], " \t\r\n,")
	}
	return out, nil
}

func transformOf(name string, a []float64) (Affine, error) {
	need := func(counts ...int) error {
		for _, c := range counts {
			if len(a) == c {
				return nil
			}
		}
		return fmt.Errorf("svgpath: transform %s takes a different number of arguments than %d", name, len(a))
	}
	switch name {
	case "matrix":
		if err := need(6); err != nil {
			return Identity, err
		}
		return Affine{a[0], a[1], a[2], a[3], a[4], a[5]}, nil
	case "translate":
		if err := need(1, 2); err != nil {
			return Identity, err
		}
		ty := 0.0
		if len(a) == 2 {
			ty = a[1]
		}
		return Translate(a[0], ty), nil
	case "scale":
		if err := need(1, 2); err != nil {
			return Identity, err
		}
		sy := a[0]
		if len(a) == 2 {
			sy = a[1]
		}
		return ScaleBy(a[0], sy), nil
	case "rotate":
		if err := need(1, 3); err != nil {
			return Identity, err
		}
		rad := a[0] * math.Pi / 180
		sin, cos := math.Sincos(rad)
		r := Affine{A: cos, B: sin, C: -sin, D: cos}
		if len(a) == 3 {
			return Translate(a[1], a[2]).Mul(r).Mul(Translate(-a[1], -a[2])), nil
		}
		return r, nil
	case "skewX":
		if err := need(1); err != nil {
			return Identity, err
		}
		return Affine{A: 1, C: math.Tan(a[0] * math.Pi / 180), D: 1}, nil
	case "skewY":
		if err := need(1); err != nil {
			return Identity, err
		}
		return Affine{A: 1, B: math.Tan(a[0] * math.Pi / 180), D: 1}, nil
	}
	return Identity, fmt.Errorf("%w: transform %q", ErrUnsupported, name)
}

// numberList reads numbers separated by whitespace or commas
func numberList(s string) ([]float64, error) {
	sc := &scanner{s: s}
	var out []float64
	for {
		sc.skipSpace()
		if sc.eof() {
			return out, nil
		}
		v, err := sc.number()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}
