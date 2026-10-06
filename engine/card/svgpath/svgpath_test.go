package svgpath

import (
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/odevine/impasto/path"
	"github.com/odevine/impasto/raster"
)

// area is the filled area of p in square units, the sum of its pixel coverage
// on a w by h grid
func area(p *path.Path, rule path.FillRule, w, h int) float64 {
	var sum float64
	for _, c := range p.Coverage(w, h, rule, path.DefaultTolerance) {
		sum += float64(c)
	}
	return sum
}

func mustPath(t *testing.T, d string) *path.Path {
	t.Helper()
	p, err := pathData(d)
	if err != nil {
		t.Fatalf("pathData(%q): %v", d, err)
	}
	return p.build(1, 1, 0, 0)
}

func near(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

func TestPathDataAreas(t *testing.T) {
	tests := []struct {
		name string
		d    string
		want float64
	}{
		{"absolute square", "M2 2L8 2L8 8L2 8Z", 36},
		{"relative with H and V", "m2 2h6v6h-6z", 36},
		{"implicit lineto after moveto", "M2 2 8 2 8 8 2 8z", 36},
		{"implicit relative lineto", "m2 2 6 0 0 6 -6 0z", 36},
		{"compact numbers", "M.5.5L10.5.5 10.5 10.5.5 10.5z", 100},
		{"signs separate numbers", "M1 1h4v4h-4z", 16},
		{"exponent", "M0 0h1e1v1e1h-1e1z", 100},
		{"lineto after closepath starts at the subpath start", "M0 0h4v4h-4zl8 0v4h-8z", 32},
		{"two subpaths", "M0 0h4v4h-4zM10 10h4v4h-4z", 32},
		{"circle from two arcs", "M0 10a10 10 0 1 0 20 0a10 10 0 1 0 -20 0z", math.Pi * 100},
		{"circle with compact arc flags", "M0 10a10 10 0 1020 0a10 10 0 1020 0z", math.Pi * 100},
		{"rotated ellipse", "M0 10a5 10 90 1 0 20 0a5 10 90 1 0 -20 0z", math.Pi * 10 * 5},
		{"half circle", "M0 10a10 10 0 0 1 20 0z", math.Pi * 100 / 2},
		{"radii too small grow to fit", "M0 30a1 1 0 0 1 20 0z", math.Pi * 100 / 2},
		{"quadratic", "M0 0Q10 20 20 0z", 20 * 10 * 2 / 3.0},
		{"cubic matches the quadratic it raises", "M0 0C6.6667 13.3333 13.3333 13.3333 20 0z", 20 * 10 * 2 / 3.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := mustPath(t, tc.d)
			// Coordinates are shifted into the grid so arcs that dip below zero
			// still land on it
			if got := area(p, path.NonZero, 64, 64); !near(got, tc.want, tc.want*0.01+0.05) {
				t.Errorf("area of %q = %.3f, want %.3f", tc.d, got, tc.want)
			}
		})
	}
}

func TestSmoothCurvesReflectTheirControlPoint(t *testing.T) {
	// A smooth cubic reuses the reflection of the previous cubic's second
	// control point, so these two paths are the same curve
	smooth := mustPath(t, "M2 10C2 2 10 2 10 10S18 18 18 10z")
	explicit := mustPath(t, "M2 10C2 2 10 2 10 10C10 18 18 18 18 10z")
	compareCoverage(t, smooth, explicit)

	// A smooth quadratic reflects the previous quadratic's control point
	smoothQ := mustPath(t, "M2 10Q6 2 10 10T18 10z")
	explicitQ := mustPath(t, "M2 10Q6 2 10 10Q14 18 18 10z")
	compareCoverage(t, smoothQ, explicitQ)

	// With no curve before it, a smooth command's first control point is the
	// current point, so it is a plain cubic from there
	first := mustPath(t, "M2 10S10 2 18 10z")
	plain := mustPath(t, "M2 10C2 10 10 2 18 10z")
	compareCoverage(t, first, plain)
}

func compareCoverage(t *testing.T, a, b *path.Path) {
	t.Helper()
	ca := a.Coverage(24, 24, path.NonZero, path.DefaultTolerance)
	cb := b.Coverage(24, 24, path.NonZero, path.DefaultTolerance)
	for i := range ca {
		if !near(float64(ca[i]), float64(cb[i]), 1e-4) {
			t.Fatalf("coverage differs at pixel %d: %v vs %v", i, ca[i], cb[i])
		}
	}
}

func TestPathDataErrors(t *testing.T) {
	bad := map[string]string{
		"empty":                   "",
		"only whitespace":         "  ",
		"no moveto":               "L1 1",
		"number first":            "5 5",
		"missing argument":        "M1",
		"missing second argument": "M0 0L1",
		"trailing partial pair":   "M0 0L1 1 5",
		"unknown command":         "M0 0X1 1",
		"number after closepath":  "M0 0L1 1z 5 5",
		"bad arc flag":            "M0 0A1 1 0 2 0 1 1",
		"arc missing flag":        "M0 0A1 1 0",
		"huge number":             "M1e999 0",
		"coordinate out of range": "M0 0L2000000 0",
		"relative runs out":       "M0 0l1e6 0l1e6 0",
		"lone sign":               "M0 0L- 1",
		"closepath only":          "z",
	}
	for name, d := range bad {
		if _, err := pathData(d); err == nil {
			t.Errorf("%s: pathData(%q) succeeded, want an error", name, d)
		}
	}
}

func TestParseFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/4ed.svg")
	if err != nil {
		t.Fatal(err)
	}
	icon, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if icon.ViewBox != (Rect{0, 0, 1024, 1024}) {
		t.Errorf("ViewBox = %+v, want 0 0 1024 1024", icon.ViewBox)
	}
	if len(icon.Shapes) != 1 {
		t.Fatalf("Shapes = %d, want 1", len(icon.Shapes))
	}
	minX, minY, maxX, maxY := icon.Shapes[0].Path.Bounds()
	if minX < 0 || minY < 0 || maxX > 1024 || maxY > 1024 || maxX-minX < 900 {
		t.Errorf("bounds = %v %v %v %v, want the glyph to span most of the box", minX, minY, maxX, maxY)
	}
	// The icon is a solid shape, so a quarter-scale raster of it has real area
	if a := area(icon.Shapes[0].Path, icon.Shapes[0].Rule, 1024, 1024); a < 100000 {
		t.Errorf("area = %.0f, want a solid glyph", a)
	}
}

const wrap = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">%s</svg>`

func svg(body string) []byte { return []byte(strings.Replace(wrap, "%s", body, 1)) }

func TestParseShapes(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		shapes int
		area   float64
	}{
		{"path", `<path d="M0 0h10v10H0z"/>`, 1, 100},
		{"several paths", `<path d="M0 0h2v2H0z"/><path d="M4 4h2v2H4z"/>`, 2, 8},
		{"rect", `<rect x="1" y="1" width="4" height="3"/>`, 1, 12},
		{"circle", `<circle cx="50" cy="50" r="40"/>`, 1, math.Pi * 1600},
		{"ellipse", `<ellipse cx="50" cy="50" rx="40" ry="20"/>`, 1, math.Pi * 800},
		{"polygon", `<polygon points="0,0 4,0 4,4 0,4"/>`, 1, 16},
		{"group nesting", `<g><g><path d="M0 0h2v2H0z"/></g></g>`, 1, 4},
		{"fill none draws nothing", `<path d="M0 0h2v2H0z"/><path fill="none" d="M4 4h2v2H4z"/>`, 1, 4},
		{"fill from a group", `<g fill="#000"><path d="M0 0h2v2H0z"/></g>`, 1, 4},
		{"child fill overrides a group's none", `<g fill="none"><path fill="#000" d="M0 0h2v2H0z"/></g>`, 1, 4},
		{"stroke none is fine", `<path stroke="none" d="M0 0h2v2H0z"/>`, 1, 4},
		{"title and metadata are skipped", `<title>icon</title><metadata><x/></metadata><path d="M0 0h2v2H0z"/>`, 1, 4},
		{"other namespaces are skipped", `<x:thing xmlns:x="urn:x"><y/></x:thing><path d="M0 0h2v2H0z"/>`, 1, 4},
		{"empty path element is skipped", `<path d=""/><path d="M0 0h2v2H0z"/>`, 1, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			icon, err := Parse(svg(tc.body))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(icon.Shapes) != tc.shapes {
				t.Fatalf("Shapes = %d, want %d", len(icon.Shapes), tc.shapes)
			}
			// The raster is large so flattening error on the round shapes stays under a percent
			const g = 100
			var got float64
			for _, s := range icon.Shapes {
				got += area(s.Path, s.Rule, g, g)
			}
			if !near(got, tc.area, tc.area*0.01+0.05) {
				t.Errorf("area = %.3f, want %.3f", got, tc.area)
			}
		})
	}
}

func TestParseFillRule(t *testing.T) {
	// Two squares wound the same way, the second inside the first
	d := `d="M0 0h10v10H0zM2 2h6v6H2z"`
	tests := []struct {
		name string
		body string
		rule path.FillRule
		area float64
	}{
		{"default is nonzero", `<path ` + d + `/>`, path.NonZero, 100},
		{"evenodd on the path", `<path fill-rule="evenodd" ` + d + `/>`, path.EvenOdd, 64},
		{"evenodd from a group", `<g fill-rule="evenodd"><path ` + d + `/></g>`, path.EvenOdd, 64},
		{"path overrides the group", `<g fill-rule="evenodd"><path fill-rule="nonzero" ` + d + `/></g>`, path.NonZero, 100},
		{"rule does not leak past the group", `<g fill-rule="evenodd"></g><path ` + d + `/>`, path.NonZero, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			icon, err := Parse(svg(tc.body))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			s := icon.Shapes[0]
			if s.Rule != tc.rule {
				t.Errorf("Rule = %v, want %v", s.Rule, tc.rule)
			}
			if got := area(s.Path, s.Rule, 10, 10); !near(got, tc.area, 0.1) {
				t.Errorf("area = %.3f, want %.3f", got, tc.area)
			}
		})
	}
}

func TestParseViewBox(t *testing.T) {
	tests := []struct {
		name string
		root string
		want Rect
	}{
		{"viewBox", `viewBox="0 0 964 432"`, Rect{0, 0, 964, 432}},
		{"commas and offset", `viewBox="-2,-3,20,30"`, Rect{-2, -3, 20, 30}},
		{"width and height stand in", `width="32" height="24"`, Rect{0, 0, 32, 24}},
		{"px units", `width="32px" height="24px"`, Rect{0, 0, 32, 24}},
		{"viewBox wins over width and height", `viewBox="0 0 8 8" width="32" height="24"`, Rect{0, 0, 8, 8}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := `<svg xmlns="http://www.w3.org/2000/svg" ` + tc.root + `><path d="M0 0h2v2H0z"/></svg>`
			icon, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if icon.ViewBox != tc.want {
				t.Errorf("ViewBox = %+v, want %+v", icon.ViewBox, tc.want)
			}
		})
	}
}

func TestParseUnsupported(t *testing.T) {
	tests := map[string]string{
		"filter":              `<path filter="url(#f)" d="M0 0h2v2H0z"/>`,
		"mask":                `<path style="mask:url(#m)" d="M0 0h2v2H0z"/>`,
		"use of a missing id": `<use href="#a"/>`,
		"nested svg":          `<svg><path d="M0 0h2v2H0z"/></svg>`,
		"unknown color":       `<path fill="chartreuse" d="M0 0h2v2H0z"/>`,
		"missing gradient":    `<path fill="url(#nope)" d="M0 0h2v2H0z"/>`,
		"unknown transform":   `<path transform="wobble(1)" d="M0 0h2v2H0z"/>`,
		"css at-rule":         `<style>@media print{path{fill:red}}</style><path d="M0 0h2v2H0z"/>`,
		"css descendant rule": `<style>g path{fill:red}</style><path d="M0 0h2v2H0z"/>`,
		"clip that cuts":      `<clipPath id="c"><path d="M0 0h1v1H0z"/></clipPath><path clip-path="url(#c)" d="M0 0h2v2H0z"/>`,
		"line":                `<line x1="0" y1="0" x2="5" y2="5"/>`,
		"text":                `<text>x</text>`,
		"rounded rect":        `<rect width="4" height="4" rx="1"/>`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(svg(body))
			if !errors.Is(err, ErrUnsupported) {
				t.Errorf("Parse error = %v, want one wrapping ErrUnsupported", err)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := map[string]string{
		"empty":                         ``,
		"not xml":                       `this is not xml`,
		"html page":                     `<!doctype html><html><head><title>Not Found</title></head></html>`,
		"unclosed":                      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path d="M0 0h1v1H0z"/>`,
		"no shapes":                     `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"></svg>`,
		"no size":                       `<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h1v1H0z"/></svg>`,
		"viewBox of three":              `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1"><path d="M0 0h1v1H0z"/></svg>`,
		"zero viewBox":                  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 0"><path d="M0 0h1v1H0z"/></svg>`,
		"bad path data":                 `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path d="M0 0L"/></svg>`,
		"root is not svg":               `<g><path d="M0 0h1v1H0z"/></g>`,
		"shape far outside the viewBox": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h900v10H0z"/></svg>`,
		"nested too deep":               `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1">` + strings.Repeat("<g>", 40) + `<path d="M0 0h1v1H0z"/>` + strings.Repeat("</g>", 40) + `</svg>`,
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(src)); err == nil {
				t.Error("Parse succeeded, want an error")
			}
		})
	}
}

func TestParseTooLarge(t *testing.T) {
	big := append([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><!--`), make([]byte, MaxBytes)...)
	if _, err := Parse(big); err == nil {
		t.Error("Parse of an oversize file succeeded, want an error")
	}
}

// FuzzParse feeds arbitrary bytes to Parse, which reads network input. It must
// return an error or an icon without panicking, and what it returns must hold
// no coordinate the rasterizer cannot handle
func FuzzParse(f *testing.F) {
	if data, err := os.ReadFile("testdata/4ed.svg"); err == nil {
		f.Add(data)
	}
	f.Add(svg(`<path d="M0 0a1 1 0 1020 0z"/>`))
	f.Add(svg(`<path d="M1 1C2 2 3 3 4 4S5 5 6 6Q7 7 8 8T9 9z"/>`))
	f.Add(svg(`<polygon points="0,0 1,0 1,1"/><circle cx="1" cy="1" r="1"/>`))
	f.Add(svg(`<defs><linearGradient id="g" x2="1" gradientTransform="rotate(30)" spreadMethod="reflect"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f" stop-opacity=".5"/></linearGradient><radialGradient id="r" href="#g" fx=".2"/></defs><path fill="url(#r)" stroke="url(#g)" stroke-width="3" stroke-dasharray="4 2" d="M10 10h50v50H10z"/>`))
	f.Add(svg(`<style>.a{fill:#f00;stroke:#000;stroke-width:2}</style><defs><path id="p" class="a" d="M0 0h10v10H0z"/></defs><use href="#p" x="5" transform="rotate(30 5 5)"/>`))
	f.Add(svg(`<clipPath id="c"><path d="M0 0h100v100H0z"/></clipPath><g clip-path="url(#c)" opacity=".5"><circle cx="50" cy="50" r="20"/></g>`))
	f.Add([]byte(`M0 0L1 1`))
	f.Fuzz(func(t *testing.T, data []byte) {
		icon, err := Parse(data)
		if err != nil {
			return
		}
		if len(icon.Shapes) == 0 {
			t.Fatal("Parse returned an icon with no shapes and no error")
		}
		// What parses must also draw, at a small size, without hanging
		k := 32 / float64(max(icon.ViewBox.W, icon.ViewBox.H))
		icon.Draw(raster.MustNewBuffer(32, 32), ScaleBy(k, k))
		for _, s := range icon.Shapes {
			minX, minY, maxX, maxY := s.Path.Bounds()
			for _, v := range []float32{minX, minY, maxX, maxY} {
				if math.IsNaN(float64(v)) || math.Abs(float64(v)) > 4*maxCoord {
					t.Fatalf("path bound %v is out of range", v)
				}
			}
		}
	})
}

// FuzzPathData targets the path data grammar directly, where most of the
// parser's arithmetic lives
func FuzzPathData(f *testing.F) {
	for _, d := range []string{"M0 0h10v10H0z", "M0 10a10 10 0 1020 0z", "m1 1 2 2 3 3", "M0 0C1 1 2 2 3 3S4 4 5 5", "M1e5 1e5L-1e5 1e5"} {
		f.Add(d)
	}
	f.Fuzz(func(t *testing.T, d string) {
		o, err := pathData(d)
		if err != nil {
			return
		}
		p := o.build(1, 1, 0, 0)
		// Rasterizing must finish for any path of sane size. Parse bounds the size
		// of a whole file relative to its viewBox, so a bare path is only drawn
		// when it is small
		minX, minY, maxX, maxY := p.Bounds()
		if maxX-minX <= 1000 && maxY-minY <= 1000 {
			p.Coverage(8, 8, path.NonZero, path.DefaultTolerance)
		}
	})
}

func TestIconBounds(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Rect
		tol  float32
	}{
		{"art smaller than the viewBox", `<path d="M10 20h30v40H10z"/>`, Rect{10, 20, 30, 40}, 0.001},
		{"several shapes", `<path d="M0 0h2v2H0z"/><path d="M8 6h2v4H8z"/>`, Rect{0, 0, 10, 10}, 0.001},
		{"curve bulges less than its control point", `<path d="M10 10Q20 50 30 10z"/>`, Rect{10, 10, 20, 20}, 0.2},
		{"circle", `<circle cx="50" cy="40" r="10"/>`, Rect{40, 30, 20, 20}, 0.2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			icon, err := Parse(svg(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			got := icon.Bounds()
			for i, d := range []float32{got.X - tc.want.X, got.Y - tc.want.Y, got.W - tc.want.W, got.H - tc.want.H} {
				if d < -tc.tol || d > tc.tol {
					t.Fatalf("Bounds = %+v, want %+v", got, tc.want)
				}
				_ = i
			}
		})
	}
}
