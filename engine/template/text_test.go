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
	total := 30 // block height
	cases := map[string]int{
		"":       100,             // default top
		"top":    100,             // top
		"center": 100 + (90-30)/2, // 130
		"bottom": 100 + 90 - 30,   // 160
	}
	for valign, want := range cases {
		box.VAlign = valign
		if got := blockTop(box, total); got != want {
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

func TestLayoutPartsInsertsDivider(t *testing.T) {
	box := TextBoxSpec{Width: 10000, Height: 10000, FontSize: 100}
	parts := []TextPart{
		{Text: "rules text", Src: scaleSource{}},
		{Text: "flavor text", Src: scaleSource{}},
	}
	lay, err := layoutParts(box, parts, 100)
	if err != nil {
		t.Fatalf("layoutParts: %v", err)
	}
	// One line per part with a divider between them.
	if len(lay.lines) != 3 {
		t.Fatalf("got %d lines, want 3 (rules, divider, flavor)", len(lay.lines))
	}
	if lay.lines[0].divider || !lay.lines[1].divider || lay.lines[2].divider {
		t.Errorf("divider is not the middle line: %+v", []bool{lay.lines[0].divider, lay.lines[1].divider, lay.lines[2].divider})
	}
}

func TestLayoutPartsNoDividerForOnePart(t *testing.T) {
	box := TextBoxSpec{Width: 10000, Height: 10000, FontSize: 100}
	// An empty part contributes nothing, so no divider is added.
	parts := []TextPart{
		{Text: "flavor only", Src: scaleSource{}},
		{Text: "", Src: scaleSource{}},
	}
	lay, err := layoutParts(box, parts, 100)
	if err != nil {
		t.Fatalf("layoutParts: %v", err)
	}
	for i, ln := range lay.lines {
		if ln.divider {
			t.Errorf("line %d is a divider, want none", i)
		}
	}
}

func TestFitLayoutShrinksToFit(t *testing.T) {
	// Ten four-glyph words at size 100 wrap to several lines taller than the
	// box, so the fit must pick a smaller size whose block fits.
	text := "aaaa aaaa aaaa aaaa aaaa aaaa aaaa aaaa aaaa aaaa"
	box := TextBoxSpec{Width: 1000, Height: 250, FontSize: 100, MinFontSize: 20}
	lay, err := fitLayout(box, []TextPart{{Text: text, Src: scaleSource{}}})
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
	lay, err := fitLayout(box, []TextPart{{Text: "aaaa", Src: scaleSource{}}})
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
	lay, err := fitLayout(box, []TextPart{{Text: "aaaa aaaa", Src: scaleSource{}}})
	if err != nil {
		t.Fatalf("fitLayout: %v", err)
	}
	if lay.size != box.FontSize {
		t.Errorf("size = %v, want the unshrunk max %v", lay.size, box.FontSize)
	}
}

func TestTrackingWidensTokens(t *testing.T) {
	// A 5-glyph token at size 100 with tracking 125 gains 5 tracking steps of
	// 0.125em, so it is wider than the untracked token by that much.
	track := trackingPx(125, 100)
	if track <= 0 {
		t.Fatalf("trackingPx(125, 100) = %v, want positive", track)
	}
	plain := tokenize("aaaaa", partStyle{face: basicfont.Face7x13})
	tracked := tokenize("aaaaa", partStyle{face: basicfont.Face7x13, track: track})
	if len(plain) != 1 || len(tracked) != 1 {
		t.Fatalf("want one token each, got %d and %d", len(plain), len(tracked))
	}
	if want := plain[0].advance + track*5; tracked[0].advance != want {
		t.Errorf("tracked advance = %v, want %v", tracked[0].advance, want)
	}
}

func TestTokenizeItalicizesReminderText(t *testing.T) {
	// Words inside parentheses draw in the emphasis face, the rest in the base
	// face, so reminder text italicizes inline.
	base := basicfont.Face7x13
	emph := scaleFace{size: 1}
	toks := tokenize("Deathtouch (Any damage.) done", partStyle{face: base, emph: emph})
	want := []bool{false, true, true, false}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(want))
	}
	for i, w := range want {
		if got := toks[i].runs[0].face == font.Face(emph); got != w {
			t.Errorf("token %d emph=%v, want %v", i, got, w)
		}
	}
}

func TestTokenizeItalicizesLeadingAbilityWord(t *testing.T) {
	// A leading ability word from the set, and the em-dash, italicize; the rest
	// stays roman. A word not in the set (a keyword ability) does not.
	base := basicfont.Face7x13
	emph := scaleFace{size: 1}
	set := map[string]bool{"constellation": true}
	toks := tokenize("Constellation — Whenever an", partStyle{face: base, emph: emph, emphLead: set})
	want := []bool{true, true, false, false}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(want))
	}
	for i, w := range want {
		if got := toks[i].runs[0].face == font.Face(emph); got != w {
			t.Errorf("token %d emph=%v, want %v", i, got, w)
		}
	}
	// A word not in the set (a keyword ability) is left roman.
	roman := tokenize("Suspend 4 — Cost", partStyle{face: base, emph: emph, emphLead: set})
	if roman[0].runs[0].face == font.Face(emph) {
		t.Error("Suspend is a keyword ability and should stay roman")
	}

	// A numbered ability word prints its count, so the bare word still matches.
	numbered := tokenize("Descend 8 — When", partStyle{face: base, emph: emph, emphLead: map[string]bool{"descend": true}})
	for i, w := range []bool{true, true, true, false} {
		if got := numbered[i].runs[0].face == font.Face(emph); got != w {
			t.Errorf("Descend token %d emph=%v, want %v", i, got, w)
		}
	}
}

func TestRenderTextBoxReportsDivider(t *testing.T) {
	box := TextBoxSpec{X: 0, Y: 0, Width: 1000, Height: 1000, FontSize: 100, VAlign: "center", LineSpacing: 1.0}
	parts := []TextPart{
		{Text: "rules", Src: scaleSource{}},
		{Text: "flavor", Src: scaleSource{}},
	}
	res, err := RenderTextBox(box, 1000, 1000, parts...)
	if err != nil {
		t.Fatalf("RenderTextBox: %v", err)
	}
	if !res.HasDivider {
		t.Fatal("two parts should report a divider")
	}
	if res.DividerY <= box.Y || res.DividerY >= box.Y+box.Height {
		t.Errorf("divider Y %d is outside the box [%d, %d)", res.DividerY, box.Y, box.Y+box.Height)
	}

	// One part has nothing to divide, so no divider is reported.
	solo, err := RenderTextBox(box, 1000, 1000, TextPart{Text: "rules only", Src: scaleSource{}})
	if err != nil {
		t.Fatalf("RenderTextBox: %v", err)
	}
	if solo.HasDivider {
		t.Error("a single part should report no divider")
	}
}

func TestBaselineShrinksToOneLine(t *testing.T) {
	// Three tokens overflow the width at the max size, so a baseline box shrinks
	// until they sit on one line inside the box.
	box := TextBoxSpec{Width: 700, Height: 200, FontSize: 100, MinFontSize: 20, VAlign: "baseline"}
	lay, err := fitLayout(box, []TextPart{{Text: "aaaa aaaa aaaa", Src: scaleSource{}}})
	if err != nil {
		t.Fatalf("fitLayout: %v", err)
	}
	if lay.size >= box.FontSize {
		t.Errorf("size = %v, want shrunk below %v", lay.size, box.FontSize)
	}
	if !oneLineFits(lay, box) {
		t.Errorf("text did not shrink to one line within the box: %d lines", len(lay.lines))
	}
}

func TestParagraphGapAddsSpace(t *testing.T) {
	// An explicit newline opens a new paragraph, marked on its first line and
	// counted as a paragraph gap plus a line in the block height.
	box := TextBoxSpec{Width: 1000, Height: 1000, FontSize: 100, LineSpacing: 1.0}
	lay := layoutText(box, "one\ntwo", basicfont.Face7x13)
	if len(lay.lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lay.lines))
	}
	if lay.lines[0].paraStart {
		t.Error("first paragraph should not be marked paraStart")
	}
	if !lay.lines[1].paraStart {
		t.Error("second paragraph should be marked paraStart")
	}
	if got, want := blockHeight(lay, box), 2*100+paragraphGap(100); got != want {
		t.Errorf("blockHeight = %d, want %d (two lines plus a paragraph gap)", got, want)
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

// fakeSymbols draws nothing and knows two codes, so layout tests can exercise
// symbol runs without a font.
type fakeSymbols struct{ drawn []image.Rectangle }

func (f *fakeSymbols) Symbol(code string, size float64) (SymbolMetrics, bool) {
	if code != "R" && code != "T" {
		return SymbolMetrics{}, false
	}
	box := int(size)
	return SymbolMetrics{Advance: fixed.I(box), Box: box, Ascent: box}, true
}

func (f *fakeSymbols) DrawSymbol(_ *image.RGBA, _ string, at image.Rectangle) error {
	f.drawn = append(f.drawn, at)
	return nil
}

func TestSplitSymbols(t *testing.T) {
	codes := func(word string) []string {
		var out []string
		for _, p := range splitSymbols(word) {
			if p.code != "" {
				out = append(out, "<"+p.code+">")
			} else {
				out = append(out, p.text)
			}
		}
		return out
	}
	cases := []struct {
		word string
		want []string
	}{
		{"plain", []string{"plain"}},
		{"{R}", []string{"<R>"}},
		{"{T}:", []string{"<T>", ":"}},
		{"{1}{G},", []string{"<1>", "<G>", ","}},
		{"{W/U}", []string{"<W/U>"}},
		{"add{2/W}here", []string{"add", "<2/W>", "here"}},
		// An unclosed brace and an empty pair have no code to look up, so they
		// stay literal.
		{"{oops", []string{"{oops"}},
		{"a{}b", []string{"a{}", "b"}},
		{"{R}{oops", []string{"<R>", "{oops"}},
	}
	for _, c := range cases {
		got := codes(c.word)
		if len(got) != len(c.want) {
			t.Errorf("splitSymbols(%q) = %q, want %q", c.word, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitSymbols(%q) = %q, want %q", c.word, got, c.want)
				break
			}
		}
	}
}

func TestTokenizeBuildsSymbolRuns(t *testing.T) {
	// "{T}:" is one token of two runs, the symbol and the colon, so the symbol
	// never breaks away from the text it sits against.
	st := partStyle{face: basicfont.Face7x13, sym: &fakeSymbols{}, size: 10}
	toks := tokenize("{T}: done", st)
	if len(toks) != 2 {
		t.Fatalf("got %d tokens, want 2", len(toks))
	}
	if len(toks[0].runs) != 2 {
		t.Fatalf("got %d runs in the first token, want 2", len(toks[0].runs))
	}
	if toks[0].runs[0].sym != "T" {
		t.Errorf("first run sym = %q, want T", toks[0].runs[0].sym)
	}
	if toks[0].runs[1].text != ":" {
		t.Errorf("second run text = %q, want :", toks[0].runs[1].text)
	}
	// The token's advance is its runs', the symbol's plus the colon's.
	if want := toks[0].runs[0].advance + toks[0].runs[1].advance; toks[0].advance != want {
		t.Errorf("token advance = %v, want %v", toks[0].advance, want)
	}
}

func TestTokenizeKeepsUnknownSymbolLiteral(t *testing.T) {
	// A code the renderer does not draw keeps its braces and measures as text,
	// so nothing vanishes from the card.
	st := partStyle{face: basicfont.Face7x13, sym: &fakeSymbols{}, size: 10}
	toks := tokenize("{ZZZ}", st)
	if len(toks) != 1 || len(toks[0].runs) != 1 {
		t.Fatalf("got %d tokens, want one of one run", len(toks))
	}
	run := toks[0].runs[0]
	if run.sym != "" || run.text != "{ZZZ}" {
		t.Errorf("run = %+v, want the literal {ZZZ}", run)
	}
	if want := fixed.I(len("{ZZZ}") * glyphW); run.advance != want {
		t.Errorf("advance = %v, want the text width %v", run.advance, want)
	}
}

func TestTokenizeWithoutRendererKeepsBraces(t *testing.T) {
	// A part with no renderer leaves every code literal, which is how boxes
	// that carry no symbols behave.
	toks := tokenize("{R}", partStyle{face: basicfont.Face7x13})
	if len(toks) != 1 || toks[0].runs[0].text != "{R}" {
		t.Errorf("tokens = %+v, want the literal {R}", toks)
	}
}

func TestSymbolRunsDrawAtTheirBox(t *testing.T) {
	// A cost of three symbols draws three boxes, each one advance further right
	// and all on the same row.
	sym := &fakeSymbols{}
	box := TextBoxSpec{Width: 1000, Height: 1000, FontSize: 20, VAlign: "top"}
	_, err := RenderTextBox(box, 1000, 1000, TextPart{Text: "{R}{R}{T}", Src: scaleSource{}, Sym: sym})
	if err != nil {
		t.Fatalf("RenderTextBox: %v", err)
	}
	if len(sym.drawn) != 3 {
		t.Fatalf("drew %d symbols, want 3", len(sym.drawn))
	}
	for i, at := range sym.drawn {
		if at.Dx() != 20 || at.Dy() != 20 {
			t.Errorf("symbol %d box = %v, want 20 by 20", i, at)
		}
		if at.Min.Y != sym.drawn[0].Min.Y {
			t.Errorf("symbol %d top = %d, want %d", i, at.Min.Y, sym.drawn[0].Min.Y)
		}
		if want := sym.drawn[0].Min.X + 20*i; at.Min.X != want {
			t.Errorf("symbol %d left = %d, want %d", i, at.Min.X, want)
		}
	}
}

func TestMeasureTextSpan(t *testing.T) {
	// A right-aligned line ends at the box's right edge and starts its own
	// width short of it, which is what lets a caller size another box around it.
	box := TextBoxSpec{X: 100, Width: 500, Height: 200, FontSize: 13, Align: "right", VAlign: "baseline"}
	span, err := MeasureTextSpan(box, TextPart{Text: "abcd", Src: fixedSource{}})
	if err != nil {
		t.Fatalf("MeasureTextSpan: %v", err)
	}
	if span.Right != box.X+box.Width {
		t.Errorf("right = %d, want the box edge at %d", span.Right, box.X+box.Width)
	}
	if want := box.X + box.Width - 4*glyphW; span.Left != want {
		t.Errorf("left = %d, want %d", span.Left, want)
	}
	if span.Empty() {
		t.Error("a measured line reports empty")
	}

	// A left-aligned box starts at its own left edge regardless of the text.
	box.Align = "left"
	left, err := MeasureTextSpan(box, TextPart{Text: "abcd", Src: fixedSource{}})
	if err != nil {
		t.Fatalf("MeasureTextSpan: %v", err)
	}
	if left.Left != box.X {
		t.Errorf("left = %d, want the box edge at %d", left.Left, box.X)
	}

	// Nothing to draw measures to an empty span rather than to the box.
	blank, err := MeasureTextSpan(box, TextPart{Text: "", Src: fixedSource{}})
	if err != nil {
		t.Fatalf("MeasureTextSpan: %v", err)
	}
	if !blank.Empty() {
		t.Errorf("empty text measured %+v, want an empty span", blank)
	}
}

// fixedSource hands out basicfont.Face7x13 at every size, so a span is exact
// glyph counts and does not move when a fit tries another size.
type fixedSource struct{}

func (fixedSource) Face(float64) (font.Face, error) { return basicfont.Face7x13, nil }

func TestInsetBoxPadsPerAxis(t *testing.T) {
	box := TextBoxSpec{X: 100, Y: 200, Width: 1000, Height: 400, Padding: 40}
	ptr := func(v int) *int { return &v }

	cases := []struct {
		name string
		box  TextBoxSpec
		want TextBoxSpec
	}{
		{
			name: "padding alone insets every side",
			box:  box,
			want: TextBoxSpec{X: 140, Y: 240, Width: 920, Height: 320},
		},
		{
			// The case the oracle box wants: a side margin with the text free to
			// use the panel's full height
			name: "a zero on one axis really is none",
			box:  withPad(box, nil, ptr(0)),
			want: TextBoxSpec{X: 140, Y: 200, Width: 920, Height: 400},
		},
		{
			name: "each axis overrides on its own",
			box:  withPad(box, ptr(10), ptr(60)),
			want: TextBoxSpec{X: 110, Y: 260, Width: 980, Height: 280},
		},
		{
			name: "an axis padding stands without an all-round one",
			box:  withPad(TextBoxSpec{X: 100, Y: 200, Width: 1000, Height: 400}, ptr(25), nil),
			want: TextBoxSpec{X: 125, Y: 200, Width: 950, Height: 400},
		},
		{
			// Growing the box would put text outside the panel it belongs to
			name: "a negative padding does not grow the box",
			box:  withPad(box, ptr(-30), nil),
			want: TextBoxSpec{X: 100, Y: 240, Width: 1000, Height: 320},
		},
		{
			name: "padding wider than the box leaves nothing rather than a negative",
			box:  TextBoxSpec{X: 100, Y: 200, Width: 50, Height: 60, Padding: 40},
			want: TextBoxSpec{X: 140, Y: 240, Width: 0, Height: 0},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := insetBox(c.box)
			if got.X != c.want.X || got.Y != c.want.Y || got.Width != c.want.Width || got.Height != c.want.Height {
				t.Errorf("insetBox gave %d,%d %dx%d, want %d,%d %dx%d",
					got.X, got.Y, got.Width, got.Height,
					c.want.X, c.want.Y, c.want.Width, c.want.Height)
			}
		})
	}
}

// withPad copies box with the two axis paddings set, so a table case can read
// as one line
func withPad(box TextBoxSpec, x, y *int) TextBoxSpec {
	box.PaddingX, box.PaddingY = x, y
	return box
}

// inkFace is a fake face whose glyphs ink a fixed fraction of the space its
// metrics claim, so a test can tell an ink measurement from a metric one. Every
// glyph reaches half the ascent and nothing below the baseline.
type inkFace struct{ scaleFace }

func (f inkFace) GlyphBounds(rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	top := -fixed.I(int(f.size * 0.4))
	return fixed.Rectangle26_6{Min: fixed.Point26_6{Y: top}}, fixed.I(int(f.size)), true
}

type inkSource struct{}

func (inkSource) Face(size float64) (font.Face, error) { return inkFace{scaleFace{size: size}}, nil }

func TestBlockInkMeasuresWhatIsDrawn(t *testing.T) {
	// The face claims 0.8 of the size above the baseline and 0.2 below, while
	// its glyphs ink 0.4 above and nothing below.
	box := TextBoxSpec{Width: 1000, Height: 1000, FontSize: 100, LineSpacing: 1.0}
	face := inkFace{scaleFace{size: 100}}
	lay := layoutText(box, "one\ntwo", face)

	above, below := blockInk(lay, face)
	if above != 40 || below != 0 {
		t.Errorf("blockInk = %d above, %d below, want 40 and 0", above, below)
	}
	// The fit measure still counts whole slots, so a box is asked to hold the
	// leading the face wants.
	if got, want := blockHeight(lay, box), 2*100+paragraphGap(100); got != want {
		t.Errorf("blockHeight = %d, want %d", got, want)
	}
	// The anchor measure runs from the first line's ink to the last line's, so
	// it drops the slot the last line leaves unused.
	ink, first := blockInkHeight(lay, box, face)
	if want := 100 + paragraphGap(100) + 40; ink != want {
		t.Errorf("blockInkHeight = %d, want %d", ink, want)
	}
	if first != 40 {
		t.Errorf("first line reaches %d above its baseline, want 40", first)
	}
}

func TestVerticalCenterUsesTheInk(t *testing.T) {
	// A line of one symbol reaches its whole box above the baseline and nothing
	// below it, so a block centered on the face's descent would sit high.
	sym := &fakeSymbols{}
	box := TextBoxSpec{Y: 200, Width: 1000, Height: 1000, FontSize: 100, VAlign: "center"}
	if _, err := RenderTextBox(box, 2000, 2000, TextPart{Text: "{R}", Src: scaleSource{}, Sym: sym}); err != nil {
		t.Fatalf("RenderTextBox: %v", err)
	}
	if len(sym.drawn) != 1 {
		t.Fatalf("drew %d symbols, want 1", len(sym.drawn))
	}
	at := sym.drawn[0]
	above, below := at.Min.Y-box.Y, box.Y+box.Height-at.Max.Y
	if above-below > 1 || below-above > 1 {
		t.Errorf("symbol sits %d below the top and %d above the bottom, want them level", above, below)
	}
}
