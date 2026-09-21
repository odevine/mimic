package template

import (
	"testing"

	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

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
