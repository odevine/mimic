package template

import (
	"image"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// scaleFace is a fake face whose glyph advance and line height scale with the
// point size, so a layout gets narrower and shorter as the size shrinks. It
// draws nothing, so it is for layout and fit tests only.
type scaleFace struct{ size float64 }

func (f scaleFace) Close() error { return nil }
func (f scaleFace) Glyph(fixed.Point26_6, rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	return image.Rectangle{}, nil, image.Point{}, 0, false
}
func (f scaleFace) GlyphBounds(rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	return fixed.Rectangle26_6{}, 0, false
}
func (f scaleFace) GlyphAdvance(rune) (fixed.Int26_6, bool) { return fixed.I(int(f.size)), true }
func (f scaleFace) Kern(rune, rune) fixed.Int26_6           { return 0 }
func (f scaleFace) Metrics() font.Metrics {
	return font.Metrics{
		Height:  fixed.I(int(f.size)),
		Ascent:  fixed.I(int(f.size * 0.8)),
		Descent: fixed.I(int(f.size * 0.2)),
	}
}

// scaleSource hands out scaleFaces, so a fit search sees the block shrink.
type scaleSource struct{}

func (scaleSource) Face(size float64) (font.Face, error) { return scaleFace{size: size}, nil }

// basicfont.Face7x13 gives every glyph a 7px advance, so widths are exact and
// wrapping is predictable in these tests.
const glyphW = 7

func lineTexts(lay textLayout) []string {
	out := make([]string, len(lay.lines))
	for i, ln := range lay.lines {
		var s string
		for j, tk := range ln.tokens {
			if j > 0 {
				s += " "
			}
			for _, run := range tk.runs {
				s += run.text
			}
		}
		out[i] = s
	}
	return out
}

func TestLayoutWrapsGreedily(t *testing.T) {
	// "aa bb cc" is three 2-char tokens. A width holding two tokens plus their
	// space (5 glyphs) wraps after the second.
	box := TextBoxSpec{Width: 5 * glyphW, Height: 1000}
	got := lineTexts(layoutText(box, "aa bb cc", basicfont.Face7x13))
	want := []string{"aa bb", "cc"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLayoutKeepsParagraphs(t *testing.T) {
	// An explicit newline always breaks, even when the words would fit on one
	// line together.
	box := TextBoxSpec{Width: 1000, Height: 1000}
	got := lineTexts(layoutText(box, "one\ntwo", basicfont.Face7x13))
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("paragraphs = %q, want [one two]", got)
	}
}

func TestLayoutLongWordGetsOwnLine(t *testing.T) {
	// A token wider than the box still occupies a line on its own rather than
	// being dropped, since there is no break point inside a token.
	box := TextBoxSpec{Width: 3 * glyphW, Height: 1000}
	got := lineTexts(layoutText(box, "wide", basicfont.Face7x13))
	if len(got) != 1 || got[0] != "wide" {
		t.Errorf("long word = %q, want [wide]", got)
	}
}

func TestBlockTopVAlign(t *testing.T) {
	box := TextBoxSpec{Y: 100, Height: 90}
	lineHeight := 10
	nLines := 3 // block is 30 tall
	cases := map[string]int{
		"":       100,             // default top
		"top":    100,             // top
		"center": 100 + (90-30)/2, // 130
		"bottom": 100 + 90 - 30,   // 160
	}
	for valign, want := range cases {
		box.VAlign = valign
		if got := blockTop(box, nLines, lineHeight); got != want {
			t.Errorf("blockTop(%q) = %d, want %d", valign, got, want)
		}
	}
}

func TestLineHeightSpacing(t *testing.T) {
	m := basicfont.Face7x13.Metrics()
	if natural := lineHeightPx(m, 100, 0); natural <= 0 {
		t.Fatalf("natural line height = %d, want positive", natural)
	}
	// A positive spacing is a multiple of the em, independent of the face.
	if solid := lineHeightPx(m, 100, 1.0); solid != 100 {
		t.Errorf("solid spacing = %d, want 100", solid)
	}
	if loose := lineHeightPx(m, 100, 1.2); loose != 120 {
		t.Errorf("1.2x spacing = %d, want 120", loose)
	}
}

func TestFirstBaselineAnchor(t *testing.T) {
	m := basicfont.Face7x13.Metrics()
	ascent := m.Ascent.Ceil()
	// A baseline anchor puts the first baseline exactly at Y.
	base := TextBoxSpec{Y: 500, Height: 100, VAlign: "baseline"}
	if got := firstBaseline(base, m, 1, 20); got != 500 {
		t.Errorf("baseline anchor = %d, want 500", got)
	}
	// A top anchor drops from the box top to the baseline by the ascent.
	top := TextBoxSpec{Y: 500, Height: 100}
	if got := firstBaseline(top, m, 1, 20); got != 500+ascent {
		t.Errorf("top anchor = %d, want %d", got, 500+ascent)
	}
}

func TestFitLayoutShrinksToFit(t *testing.T) {
	// Ten four-glyph words at size 100 wrap to several lines taller than the
	// box, so the fit must pick a smaller size whose block fits.
	text := "aaaa aaaa aaaa aaaa aaaa aaaa aaaa aaaa aaaa aaaa"
	box := TextBoxSpec{Width: 1000, Height: 250, FontSize: 100, MinFontSize: 20}
	lay, err := fitLayout(box, text, scaleSource{})
	if err != nil {
		t.Fatalf("fitLayout: %v", err)
	}
	if lay.size >= box.FontSize {
		t.Errorf("size = %v, want less than the max %v", lay.size, box.FontSize)
	}
	if h := blockHeight(lay, box); h > box.Height {
		t.Errorf("block height %d overflows box height %d", h, box.Height)
	}
}

func TestFitLayoutKeepsMaxWhenItFits(t *testing.T) {
	// A short text fits at the max size, so no shrinking happens.
	box := TextBoxSpec{Width: 1000, Height: 500, FontSize: 100, MinFontSize: 20}
	lay, err := fitLayout(box, "aaaa", scaleSource{})
	if err != nil {
		t.Fatalf("fitLayout: %v", err)
	}
	if lay.size != box.FontSize {
		t.Errorf("size = %v, want the max %v", lay.size, box.FontSize)
	}
}

func TestFitLayoutBaselineDoesNotShrink(t *testing.T) {
	// A baseline-anchored box is single-line point text, so it keeps its size
	// even when the nominal block is taller than the box.
	box := TextBoxSpec{Width: 1000, Height: 10, FontSize: 100, MinFontSize: 20, VAlign: "baseline"}
	lay, err := fitLayout(box, "aaaa aaaa", scaleSource{})
	if err != nil {
		t.Fatalf("fitLayout: %v", err)
	}
	if lay.size != box.FontSize {
		t.Errorf("size = %v, want the unshrunk max %v", lay.size, box.FontSize)
	}
}

func TestLineStartXAlign(t *testing.T) {
	box := TextBoxSpec{X: 10, Width: 100}
	width := fixed.I(20)
	cases := map[string]int{
		"left":   10,
		"center": 10 + (100-20)/2, // 50
		"right":  10 + 100 - 20,   // 90
	}
	for align, want := range cases {
		box.Align = align
		if got := lineStartX(box, width).Round(); got != want {
			t.Errorf("lineStartX(%q) = %d, want %d", align, got, want)
		}
	}
}
