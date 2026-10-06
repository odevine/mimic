package svgpath

import (
	"math"
	"testing"

	"github.com/odevine/impasto/raster"
)

// drawSVG parses an icon with a 100 by 100 viewBox and draws it one pixel to a
// unit
func drawSVG(t *testing.T, body string) (*Icon, *raster.Buffer) {
	t.Helper()
	icon, err := Parse(svg(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	buf := raster.MustNewBuffer(100, 100)
	icon.Draw(buf, Identity)
	return icon, buf
}

// srgb is the straight sRGB color at a pixel, each channel 0 to 255, and its alpha
func srgb(b *raster.Buffer, x, y int) (r, g, bl, a float64) {
	pr, pg, pb, pa := b.At(x, y)
	a = float64(pa)
	if pa > 0 {
		pr, pg, pb = pr/pa, pg/pa, pb/pa
	}
	return float64(raster.LinearToSRGB(pr)) * 255, float64(raster.LinearToSRGB(pg)) * 255, float64(raster.LinearToSRGB(pb)) * 255, a
}

func wantColor(t *testing.T, b *raster.Buffer, x, y int, r, g, bl, a float64) {
	t.Helper()
	gr, gg, gb, ga := srgb(b, x, y)
	// Pixel centers sit half a pixel along a gradient, and flat colors are exact
	const tol = 10.0
	if math.Abs(gr-r) > tol || math.Abs(gg-g) > tol || math.Abs(gb-bl) > tol || math.Abs(ga-a) > 0.02 {
		t.Errorf("pixel %d,%d = rgb(%.0f %.0f %.0f) a%.2f, want rgb(%.0f %.0f %.0f) a%.2f", x, y, gr, gg, gb, ga, r, g, bl, a)
	}
}

const square = `d="M0 0h100v100H0z"`

func TestDrawFlatFills(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		r, g, b, a float64
	}{
		{"default fill is black", `<path ` + square + `/>`, 0, 0, 0, 1},
		{"fill attribute", `<path fill="#ff8000" ` + square + `/>`, 255, 128, 0, 1},
		{"short hex", `<path fill="#f80" ` + square + `/>`, 255, 136, 0, 1},
		{"rgb function", `<path fill="rgb(10, 20, 30)" ` + square + `/>`, 10, 20, 30, 1},
		{"named color", `<path fill="red" ` + square + `/>`, 255, 0, 0, 1},
		{"fill from the group", `<g fill="#00f"><path ` + square + `/></g>`, 0, 0, 255, 1},
		{"child overrides the group", `<g fill="#00f"><path fill="#f00" ` + square + `/></g>`, 255, 0, 0, 1},
		{"currentColor", `<g color="#0f0"><path fill="currentColor" ` + square + `/></g>`, 0, 255, 0, 1},
		{"fill-opacity", `<path fill="#fff" fill-opacity=".5" ` + square + `/>`, 255, 255, 255, 0.5},
		{"opacity", `<path fill="#fff" opacity=".25" ` + square + `/>`, 255, 255, 255, 0.25},
		{"opacities multiply", `<g opacity=".5"><path fill-opacity=".5" ` + square + `/></g>`, 0, 0, 0, 0.25},
		{"style attribute", `<path style="fill:#123456" ` + square + `/>`, 0x12, 0x34, 0x56, 1},
		{"style beats the attribute", `<path fill="#f00" style="fill:#00f" ` + square + `/>`, 0, 0, 255, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, b := drawSVG(t, tc.body)
			wantColor(t, b, 50, 50, tc.r, tc.g, tc.b, tc.a)
		})
	}
}

func TestDrawStylesheet(t *testing.T) {
	css := `<style>.a{fill:#f00}.b,.c{fill:#0f0}path{fill:#00f}</style>`
	tests := []struct {
		name    string
		body    string
		r, g, b float64
	}{
		{"class rule", css + `<path class="a" ` + square + `/>`, 255, 0, 0},
		{"selector list", css + `<path class="c" ` + square + `/>`, 0, 255, 0},
		{"type rule", css + `<path ` + square + `/>`, 0, 0, 255},
		{"class beats type", css + `<path class="b" ` + square + `/>`, 0, 255, 0},
		{"rule beats the attribute", css + `<path class="a" fill="#fff" ` + square + `/>`, 255, 0, 0},
		{"style attribute beats the rule", css + `<path class="a" style="fill:#fff" ` + square + `/>`, 255, 255, 255},
		{"several classes", css + `<path class="x a" ` + square + `/>`, 255, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, b := drawSVG(t, tc.body)
			wantColor(t, b, 50, 50, tc.r, tc.g, tc.b, 1)
		})
	}
}

func TestDrawTransforms(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		in, out [2]int
	}{
		{"translate", `<path transform="translate(50 50)" d="M0 0h20v20H0z"/>`, [2]int{60, 60}, [2]int{10, 10}},
		{"scale", `<path transform="scale(2)" d="M10 10h10v10H10z"/>`, [2]int{30, 30}, [2]int{15, 15}},
		{"rotate about a point", `<path transform="rotate(90 50 50)" d="M50 50h30v10H50z"/>`, [2]int{45, 70}, [2]int{70, 55}},
		{"matrix", `<path transform="matrix(1 0 0 1 40 40)" d="M0 0h10v10H0z"/>`, [2]int{45, 45}, [2]int{5, 5}},
		{"nested", `<g transform="translate(10 0)"><path transform="translate(0 10)" d="M0 0h10v10H0z"/></g>`, [2]int{15, 15}, [2]int{5, 5}},
		{"list applies the first outermost", `<path transform="translate(50 0) scale(2)" d="M0 0h10v10H0z"/>`, [2]int{60, 10}, [2]int{25, 10}},
		{"use moves its target", `<defs><path id="p" d="M0 0h10v10H0z"/></defs><use href="#p" x="30" y="40"/>`, [2]int{35, 45}, [2]int{5, 5}},
		{"use with a transform", `<defs><path id="p" d="M0 0h10v10H0z"/></defs><use href="#p" transform="rotate(180 25 25)"/>`, [2]int{45, 45}, [2]int{5, 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, b := drawSVG(t, tc.body)
			if _, _, _, a := srgb(b, tc.in[0], tc.in[1]); a < 0.99 {
				t.Errorf("no ink at %v", tc.in)
			}
			if _, _, _, a := srgb(b, tc.out[0], tc.out[1]); a > 0.01 {
				t.Errorf("unexpected ink at %v", tc.out)
			}
		})
	}
}

func TestDrawLinearGradient(t *testing.T) {
	grad := func(attrs string) string {
		return `<defs><linearGradient id="g" ` + attrs + `><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient></defs><path fill="url(#g)" ` + square + `/>`
	}
	t.Run("user space", func(t *testing.T) {
		_, b := drawSVG(t, grad(`gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="100" y2="0"`))
		// Stops are blended in sRGB, so the middle is mid gray
		wantColor(t, b, 1, 50, 1, 1, 1, 1)
		wantColor(t, b, 50, 50, 128, 128, 128, 1)
		wantColor(t, b, 98, 50, 253, 253, 253, 1)
		// Constant down the gradient's cross axis
		wantColor(t, b, 50, 5, 128, 128, 128, 1)
	})
	t.Run("bounding box is the default", func(t *testing.T) {
		_, b := drawSVG(t, `<defs><linearGradient id="g"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient></defs><path fill="url(#g)" d="M20 0h60v100H20z"/>`)
		wantColor(t, b, 21, 50, 2, 2, 2, 1)
		wantColor(t, b, 50, 50, 128, 128, 128, 1)
		wantColor(t, b, 79, 50, 253, 253, 253, 1)
	})
	t.Run("percentages", func(t *testing.T) {
		_, b := drawSVG(t, grad(`x1="0%" x2="100%"`))
		wantColor(t, b, 50, 50, 128, 128, 128, 1)
	})
	t.Run("vertical", func(t *testing.T) {
		_, b := drawSVG(t, grad(`x1="0" y1="0" x2="0" y2="1"`))
		wantColor(t, b, 50, 25, 64, 64, 64, 1)
		wantColor(t, b, 5, 75, 192, 192, 192, 1)
	})
	t.Run("gradientTransform", func(t *testing.T) {
		// Turning the default left to right gradient a quarter turn makes it run down
		_, b := drawSVG(t, grad(`gradientTransform="rotate(90 .5 .5)"`))
		wantColor(t, b, 50, 25, 64, 64, 64, 1)
		wantColor(t, b, 5, 75, 192, 192, 192, 1)
	})
	t.Run("href inherits stops and geometry", func(t *testing.T) {
		_, b := drawSVG(t, `<defs>
			<linearGradient id="base" gradientUnits="userSpaceOnUse" x1="0" x2="100"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
			<linearGradient id="g" href="#base"/></defs><path fill="url(#g)" `+square+`/>`)
		wantColor(t, b, 50, 50, 128, 128, 128, 1)
	})
	t.Run("href attributes override", func(t *testing.T) {
		_, b := drawSVG(t, `<defs>
			<linearGradient id="base" gradientUnits="userSpaceOnUse" x1="0" x2="100"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
			<linearGradient id="g" href="#base" x2="50"/></defs><path fill="url(#g)" `+square+`/>`)
		// Reaching white at 50, so 25 is the middle
		wantColor(t, b, 25, 50, 128, 128, 128, 1)
		wantColor(t, b, 75, 50, 255, 255, 255, 1)
	})
	t.Run("stop styles and opacity", func(t *testing.T) {
		_, b := drawSVG(t, `<defs><linearGradient id="g"><stop offset="0" style="stop-color:#f00;stop-opacity:1"/><stop offset="1" style="stop-color:#f00;stop-opacity:0"/></linearGradient></defs><path fill="url(#g)" `+square+`/>`)
		wantColor(t, b, 1, 50, 255, 0, 0, 0.99)
		wantColor(t, b, 50, 50, 255, 0, 0, 0.5)
	})
	t.Run("three stops", func(t *testing.T) {
		_, b := drawSVG(t, `<defs><linearGradient id="g"><stop offset="0" stop-color="#f00"/><stop offset=".5" stop-color="#0f0"/><stop offset="1" stop-color="#00f"/></linearGradient></defs><path fill="url(#g)" `+square+`/>`)
		wantColor(t, b, 50, 50, 0, 255, 0, 1)
	})
	t.Run("a single stop is flat", func(t *testing.T) {
		_, b := drawSVG(t, `<defs><linearGradient id="g"><stop offset="0" stop-color="#336699"/></linearGradient></defs><path fill="url(#g)" `+square+`/>`)
		wantColor(t, b, 50, 50, 0x33, 0x66, 0x99, 1)
	})
	t.Run("stops stay in order", func(t *testing.T) {
		// The second stop's offset is before the first's, and is held at it
		_, b := drawSVG(t, `<defs><linearGradient id="g"><stop offset=".6" stop-color="#000"/><stop offset=".2" stop-color="#fff"/></linearGradient></defs><path fill="url(#g)" `+square+`/>`)
		wantColor(t, b, 20, 50, 0, 0, 0, 1)
		wantColor(t, b, 80, 50, 255, 255, 255, 1)
	})
}

func TestDrawGradientSpread(t *testing.T) {
	grad := func(spread string) string {
		return `<defs><linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" x2="50" ` + spread + `><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient></defs><path fill="url(#g)" ` + square + `/>`
	}
	_, pad := drawSVG(t, grad(``))
	wantColor(t, pad, 90, 50, 255, 255, 255, 1)
	_, repeat := drawSVG(t, grad(`spreadMethod="repeat"`))
	wantColor(t, repeat, 75, 50, 128, 128, 128, 1)
	wantColor(t, repeat, 52, 50, 5, 5, 5, 1)
	_, reflect := drawSVG(t, grad(`spreadMethod="reflect"`))
	wantColor(t, reflect, 75, 50, 128, 128, 128, 1)
	wantColor(t, reflect, 52, 50, 250, 250, 250, 1)
}

func TestDrawGradientInTheElementsOwnSpace(t *testing.T) {
	// A user space gradient is laid out in the coordinates of the element that
	// uses it, transform included. This path is moved right by 50, so its
	// gradient, from 0 to 50 in its own space, covers the moved square
	_, b := drawSVG(t, `<defs><linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" x2="50"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient></defs>
		<path transform="translate(50 0)" fill="url(#g)" d="M0 0h50v100H0z"/>`)
	wantColor(t, b, 51, 50, 3, 3, 3, 1)
	wantColor(t, b, 75, 50, 128, 128, 128, 1)
	wantColor(t, b, 98, 50, 252, 252, 252, 1)
}

func TestDrawRadialGradient(t *testing.T) {
	grad := func(attrs string) string {
		return `<defs><radialGradient id="g" ` + attrs + `><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></radialGradient></defs><path fill="url(#g)" ` + square + `/>`
	}
	t.Run("centered", func(t *testing.T) {
		_, b := drawSVG(t, grad(`gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40"`))
		wantColor(t, b, 50, 50, 255, 255, 255, 1)
		wantColor(t, b, 70, 50, 128, 128, 128, 1)
		wantColor(t, b, 50, 70, 128, 128, 128, 1)
		wantColor(t, b, 95, 50, 0, 0, 0, 1)
	})
	t.Run("bounding box defaults", func(t *testing.T) {
		// Centered, with a radius of half the box
		_, b := drawSVG(t, grad(``))
		wantColor(t, b, 50, 50, 255, 255, 255, 1)
		wantColor(t, b, 75, 50, 128, 128, 128, 1)
	})
	t.Run("focal point", func(t *testing.T) {
		// Lit from the left, so the left is bright for longer than the right
		_, b := drawSVG(t, grad(`gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40" fx="30" fy="50"`))
		wantColor(t, b, 30, 50, 255, 255, 255, 1)
		l, _, _, _ := srgb(b, 40, 50)
		r, _, _, _ := srgb(b, 60, 50)
		if l <= r {
			t.Errorf("left %v should be brighter than right %v with the focal point on the left", l, r)
		}
	})
	t.Run("gradientTransform stretches the circle", func(t *testing.T) {
		_, b := drawSVG(t, grad(`gradientUnits="userSpaceOnUse" cx="0" cy="0" r="1" gradientTransform="matrix(40 0 0 20 50 50)"`))
		// 20 across is as far along as 10 down
		x, _, _, _ := srgb(b, 70, 50)
		y, _, _, _ := srgb(b, 50, 60)
		if math.Abs(x-y) > 8 {
			t.Errorf("ellipse is not stretched as asked: %v across, %v down", x, y)
		}
		wantColor(t, b, 50, 50, 255, 255, 255, 1)
	})
	t.Run("href from a linear gradient for the stops", func(t *testing.T) {
		_, b := drawSVG(t, `<defs>
			<linearGradient id="s"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></linearGradient>
			<radialGradient id="g" href="#s" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40"/></defs><path fill="url(#g)" `+square+`/>`)
		wantColor(t, b, 50, 50, 255, 255, 255, 1)
		wantColor(t, b, 70, 50, 128, 128, 128, 1)
	})
	t.Run("no radius is the last stop", func(t *testing.T) {
		_, b := drawSVG(t, grad(`gradientUnits="userSpaceOnUse" cx="50" cy="50" r="0"`))
		wantColor(t, b, 50, 50, 0, 0, 0, 1)
	})
}

func TestDrawStroke(t *testing.T) {
	t.Run("stroke around a fill", func(t *testing.T) {
		_, b := drawSVG(t, `<path fill="#fff" stroke="#f00" stroke-width="10" d="M30 30h40v40H30z"/>`)
		wantColor(t, b, 50, 50, 255, 255, 255, 1)
		// Centered on the edge, so 5 either side
		wantColor(t, b, 27, 50, 255, 0, 0, 1)
		wantColor(t, b, 33, 50, 255, 0, 0, 1)
		wantColor(t, b, 20, 50, 0, 0, 0, 0)
	})
	t.Run("stroke only", func(t *testing.T) {
		_, b := drawSVG(t, `<path fill="none" stroke="#00f" stroke-width="6" d="M10 50h80"/>`)
		wantColor(t, b, 50, 50, 0, 0, 255, 1)
		wantColor(t, b, 50, 60, 0, 0, 0, 0)
	})
	t.Run("width follows the transform", func(t *testing.T) {
		_, b := drawSVG(t, `<path transform="scale(2)" fill="none" stroke="#000" stroke-width="5" d="M5 25h40"/>`)
		wantColor(t, b, 50, 52, 0, 0, 0, 1)
		wantColor(t, b, 50, 58, 0, 0, 0, 0)
	})
	t.Run("zero width draws nothing", func(t *testing.T) {
		icon, _ := drawSVG(t, `<path fill="none" stroke="#000" stroke-width="0" d="M10 50h80"/>`+`<path d="M0 0h1v1H0z"/>`)
		if len(icon.Shapes) != 1 {
			t.Errorf("shapes = %d, want only the filled one", len(icon.Shapes))
		}
	})
	t.Run("dashes", func(t *testing.T) {
		_, b := drawSVG(t, `<path fill="none" stroke="#000" stroke-width="10" stroke-dasharray="20 20" d="M0 50h100"/>`)
		wantColor(t, b, 10, 50, 0, 0, 0, 1)
		wantColor(t, b, 30, 50, 0, 0, 0, 0)
		wantColor(t, b, 50, 50, 0, 0, 0, 1)
	})
	t.Run("stroke opacity", func(t *testing.T) {
		_, b := drawSVG(t, `<path fill="none" stroke="#000" stroke-opacity=".5" stroke-width="10" d="M0 50h100"/>`)
		wantColor(t, b, 50, 50, 0, 0, 0, 0.5)
	})
	t.Run("gradient stroke", func(t *testing.T) {
		_, b := drawSVG(t, `<defs><linearGradient id="g" gradientUnits="userSpaceOnUse" x2="100"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient></defs>
			<path fill="none" stroke="url(#g)" stroke-width="10" d="M0 50h100"/>`)
		wantColor(t, b, 50, 50, 128, 128, 128, 1)
	})
	t.Run("css stroke width", func(t *testing.T) {
		_, b := drawSVG(t, `<style>.s{stroke:#000;stroke-width:10;fill:none}</style><path class="s" d="M0 50h100"/>`)
		wantColor(t, b, 50, 53, 0, 0, 0, 1)
	})
}

func TestBoundsIncludeStrokes(t *testing.T) {
	icon, err := Parse(svg(`<path fill="none" stroke="#000" stroke-width="10" d="M20 20h40v40H20z"/>`))
	if err != nil {
		t.Fatal(err)
	}
	got := icon.Bounds()
	want := Rect{15, 15, 50, 50}
	if got != want {
		t.Errorf("Bounds = %+v, want %+v", got, want)
	}
}

func TestBoundsFollowTransforms(t *testing.T) {
	icon, err := Parse(svg(`<path transform="translate(10 20) scale(2)" d="M0 0h10v10H0z"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := icon.Bounds(), (Rect{10, 20, 20, 20}); got != want {
		t.Errorf("Bounds = %+v, want %+v", got, want)
	}
}

func TestDrawClip(t *testing.T) {
	// A clip that holds everything the art draws changes nothing
	icon, err := Parse(svg(`<clipPath id="c"><path d="M0 0h100v100H0z"/></clipPath><g clip-path="url(#c)"><path d="M10 10h20v20H10z"/></g>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(icon.Shapes) != 1 {
		t.Errorf("shapes = %d, want 1", len(icon.Shapes))
	}
}

func TestDrawHidden(t *testing.T) {
	for name, body := range map[string]string{
		"display none":      `<path style="display:none" ` + square + `/><path d="M0 0h1v1H0z"/>`,
		"group display":     `<g display="none"><path ` + square + `/></g><path d="M0 0h1v1H0z"/>`,
		"visibility hidden": `<path visibility="hidden" ` + square + `/><path d="M0 0h1v1H0z"/>`,
		"foreignObject":     `<switch><foreignObject width="1" height="1"/><g><path d="M0 0h1v1H0z"/></g></switch>`,
		"defs":              `<defs><path ` + square + `/></defs><path d="M0 0h1v1H0z"/>`,
	} {
		t.Run(name, func(t *testing.T) {
			icon, err := Parse(svg(body))
			if err != nil {
				t.Fatal(err)
			}
			if len(icon.Shapes) != 1 {
				t.Errorf("shapes = %d, want only the small square", len(icon.Shapes))
			}
		})
	}
}

func TestParseTransformList(t *testing.T) {
	m, err := parseTransform("translate(10, 20) scale(2) rotate(90)")
	if err != nil {
		t.Fatal(err)
	}
	// (1, 0) is turned to (0, 1), doubled to (0, 2), and moved to (10, 22)
	if x, y := m.Apply(1, 0); math.Abs(x-10) > 1e-9 || math.Abs(y-22) > 1e-9 {
		t.Errorf("(1,0) -> (%v, %v), want (10, 22)", x, y)
	}
	inv, ok := m.Invert()
	if !ok {
		t.Fatal("no inverse")
	}
	if x, y := inv.Apply(10, 22); math.Abs(x-1) > 1e-9 || math.Abs(y) > 1e-9 {
		t.Errorf("inverse gives (%v, %v), want (1, 0)", x, y)
	}
	for _, bad := range []string{"translate(1", "scale(1 2 3)", "matrix(1 2 3)", "wobble(1)", "rotate()"} {
		if _, err := parseTransform(bad); err == nil {
			t.Errorf("parseTransform(%q) succeeded, want an error", bad)
		}
	}
	if _, ok := (Affine{}).Invert(); ok {
		t.Error("a flattening transform should have no inverse")
	}
}
