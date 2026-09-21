package normal

import (
	"image"
	"image/color"
	"math"
	"testing"

	"golang.org/x/image/math/fixed"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/template"
)

func TestLookupPipCoversScryfallCodes(t *testing.T) {
	cases := []struct {
		code   string
		icon   rune
		hybrid bool
	}{
		{code: "W", icon: glyphWhite},
		{code: "U", icon: glyphBlue},
		{code: "B", icon: glyphBlack},
		{code: "R", icon: glyphRed},
		{code: "G", icon: glyphGreen},
		{code: "C", icon: glyphColorless},
		{code: "S", icon: glyphSnow},
		{code: "T", icon: glyphTap},
		{code: "Q", icon: glyphUntap},
		{code: "X", icon: glyphX},
		{code: "Y", icon: glyphY},
		{code: "Z", icon: glyphZ},
		{code: "0", icon: glyphZero},
		{code: "15", icon: glyphZero + 15},
		{code: "16", icon: glyphSixteen},
		{code: "20", icon: glyphSixteen + 4},
		// Phyrexian mana is the phyrexian icon on the color's own disc.
		{code: "W/P", icon: glyphPhyrexian},
		{code: "G/P", icon: glyphPhyrexian},
		// Hybrids carry both icons and split the disc.
		{code: "W/U", icon: glyphWhite, hybrid: true},
		{code: "G/W", icon: glyphGreen, hybrid: true},
		{code: "2/W", icon: glyphZero + 2, hybrid: true},
		// The code is not case sensitive, since the parser passes it through.
		{code: "r", icon: glyphRed},
	}
	for _, c := range cases {
		p, ok := lookupPip(c.code)
		if !ok {
			t.Errorf("lookupPip(%q) reported no pip", c.code)
			continue
		}
		if p.icon[0] != c.icon {
			t.Errorf("lookupPip(%q) icon = U+%04X, want U+%04X", c.code, p.icon[0], c.icon)
		}
		if p.hybrid != c.hybrid {
			t.Errorf("lookupPip(%q) hybrid = %v, want %v", c.code, p.hybrid, c.hybrid)
		}
	}
}

func TestLookupPipColorsHybridHalves(t *testing.T) {
	// A hybrid's halves take their own colors, in the order the code writes
	// them, which is what the split disc paints.
	p, ok := lookupPip("R/G")
	if !ok {
		t.Fatal("lookupPip(R/G) reported no pip")
	}
	if p.back[0] != pipColors["R"] || p.back[1] != pipColors["G"] {
		t.Errorf("halves = %v and %v, want the red and green washes", p.back[0], p.back[1])
	}
	// A monocolored hybrid's generic half is the neutral gray.
	two, _ := lookupPip("2/U")
	if two.back[0] != pipGray || two.back[1] != pipColors["U"] {
		t.Errorf("halves = %v and %v, want gray then the blue wash", two.back[0], two.back[1])
	}
	// Phyrexian mana takes the color's disc, not a gray one.
	phy, _ := lookupPip("B/P")
	if phy.back[0] != pipColors["B"] || phy.hybrid {
		t.Errorf("phyrexian pip = %+v, want a solid black disc", phy)
	}
}

func TestLookupPipRejectsUnknownCodes(t *testing.T) {
	// Anything with no artwork reports false, which leaves the braces literal
	// rather than dropping the text.
	for _, code := range []string{"", "21", "-1", "HW", "W/U/P", "W/", "/W", "1.5"} {
		if p, ok := lookupPip(code); ok {
			t.Errorf("lookupPip(%q) = %+v, want no pip", code, p)
		}
	}
}

func TestSymbolMetricsScaleWithSize(t *testing.T) {
	s := newManaSymbols("")
	if s == nil {
		t.Fatal("no mana font resolved")
	}
	small, ok := s.Symbol("R", 100)
	if !ok {
		t.Fatal("Symbol(R) reported nothing")
	}
	large, _ := s.Symbol("R", 200)
	// The advance rounds to a fixed-point unit at each size on its own, so
	// doubling can land one unit off the doubled advance.
	if large.Box != 2*small.Box || abs(int(large.Advance-2*small.Advance)) > 1 {
		t.Errorf("doubling the size gave %+v from %+v", large, small)
	}
	// The pip is narrower than its advance, which is what keeps two adjacent
	// symbols from touching.
	if small.Advance.Round() <= small.Box {
		t.Errorf("advance %v does not clear the %d box", small.Advance, small.Box)
	}
	// It sits mostly above the baseline but dips a little below it, the way a
	// printed symbol sits on a line of text.
	if small.Ascent >= small.Box || small.Ascent <= small.Box/2 {
		t.Errorf("ascent %d is not inside the box of %d", small.Ascent, small.Box)
	}
	if _, ok := s.Symbol("nope", 100); ok {
		t.Error("an unknown code should report no metrics")
	}
}

func TestDrawSymbolPaintsTheDisc(t *testing.T) {
	s := newManaSymbols("")
	if s == nil {
		t.Fatal("no mana font resolved")
	}
	const box = 64
	dst := image.NewRGBA(image.Rect(0, 0, box, box))
	if err := s.DrawSymbol(dst, "R", dst.Bounds()); err != nil {
		t.Fatalf("DrawSymbol: %v", err)
	}
	// The disc fills the box, so the center is opaque and the corners are not.
	if _, _, _, a := dst.At(box/2, box/2).RGBA(); a == 0 {
		t.Error("the center of the pip is transparent")
	}
	if _, _, _, a := dst.At(0, 0).RGBA(); a != 0 {
		t.Error("the corner outside the disc was painted")
	}
	// An edge of the disc is the red wash rather than the icon's ink, so the
	// disc really is colored and not a monochrome glyph.
	r, g, b, a := dst.At(box/2, 2).RGBA()
	if a == 0 {
		t.Fatal("the top of the disc is transparent")
	}
	if r <= b || r <= g {
		t.Errorf("disc edge = (%d, %d, %d), want the red wash", r>>8, g>>8, b>>8)
	}
	// An unknown code paints nothing rather than failing.
	blank := image.NewRGBA(image.Rect(0, 0, box, box))
	if err := s.DrawSymbol(blank, "nope", blank.Bounds()); err != nil {
		t.Fatalf("DrawSymbol on an unknown code: %v", err)
	}
	if _, _, _, a := blank.At(box/2, box/2).RGBA(); a != 0 {
		t.Error("an unknown code painted something")
	}
}

func TestManaBoxDoesNotShrink(t *testing.T) {
	// A cost wider than its box keeps its size and grows leftward out of it, so
	// the symbols stay the size a printed card gives them.
	sym := newManaSymbols("")
	if sym == nil {
		t.Fatal("no mana font resolved")
	}
	box := template.TextBoxSpec{X: 2047, Y: 506, Width: 866, Height: 200, FontSize: 179, Align: "right", VAlign: "baseline"}
	cost := template.TextPart{Text: "{W}{W}{U}{U}{B}{B}{R}{R}{G}{G}", Src: fonts.ResolveFont(fonts.Body, ""), Sym: sym}

	ten, err := template.MeasureTextSpan(manaBox(box), cost)
	if err != nil {
		t.Fatalf("MeasureTextSpan: %v", err)
	}
	// The cost measures nine advances plus the last symbol's box, since the
	// trailing gap comes back off the end of a line. Give or take the rounding
	// at each end, that only holds if nothing shrank on the way.
	full, ok := sym.Symbol("R", box.FontSize)
	if !ok {
		t.Fatal("Symbol(R) reported nothing")
	}
	if want := (9*full.Advance + fixed.I(full.Box)).Round(); abs(ten.Right-ten.Left-want) > 2 {
		t.Errorf("ten symbols span %d, want about %d", ten.Right-ten.Left, want)
	}
	// They stay pinned to the box's right edge and run out past its left.
	if want := box.X + box.Width; ten.Right != want {
		t.Errorf("right edge = %d, want the box's at %d", ten.Right, want)
	}
	if ten.Left >= box.X {
		t.Errorf("left edge = %d, want it out past the box at %d", ten.Left, box.X)
	}
	// Left to itself the box would have shrunk, which is what manaBox switches
	// off.
	loose, err := template.MeasureTextSpan(box, cost)
	if err != nil {
		t.Fatalf("MeasureTextSpan: %v", err)
	}
	if loose.Right-loose.Left >= ten.Right-ten.Left {
		t.Error("the box did not shrink without manaBox, so the test proves nothing")
	}
}

func TestTitleClearOfCost(t *testing.T) {
	box := template.TextBoxSpec{X: 386, Width: 2400, FontSize: 156}
	gap := int(math.Round(box.FontSize * titleCostGap))

	// A cost reaching into the name box narrows it to stop a gap short.
	narrowed := titleClearOf(box, template.TextSpan{Left: 1338, Right: 2913})
	if want := 1338 - gap - box.X; narrowed.Width != want {
		t.Errorf("width = %d, want %d", narrowed.Width, want)
	}
	if narrowed.X != box.X {
		t.Errorf("X = %d, want the box left alone at %d", narrowed.X, box.X)
	}

	// A cost that already clears the box leaves it as the manifest wrote it,
	// so the name box never grows.
	clear := titleClearOf(box, template.TextSpan{Left: 2900, Right: 2913})
	if clear.Width != box.Width {
		t.Errorf("width = %d, want the manifest's %d", clear.Width, box.Width)
	}

	// A land has no cost, so the name keeps the whole bar.
	none := titleClearOf(box, template.TextSpan{})
	if none.Width != box.Width {
		t.Errorf("width = %d, want the manifest's %d", none.Width, box.Width)
	}

	// A cost swallowing the whole bar leaves no room rather than a negative
	// width the layout would have to guard against.
	full := titleClearOf(box, template.TextSpan{Left: 100, Right: 2913})
	if full.Width != 0 {
		t.Errorf("width = %d, want 0", full.Width)
	}
}

// abs gives a pixel difference without its sign, for assertions that allow the
// rounding at a span's two ends.
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestArtistNibMetrics(t *testing.T) {
	ms := newManaSymbols("")
	if ms == nil {
		t.Fatal("no mana font resolved")
	}
	nib := artistNib{sym: ms, ink: color.White}

	m, ok := nib.Symbol(nibCode, 77)
	if !ok {
		t.Fatalf("Symbol(%q) reported nothing", nibCode)
	}
	// The glyph inks its whole em across, so the box is the em and the advance
	// is what carries the gap to the name.
	if want := int(math.Round(77 * nibEm)); m.Box != want {
		t.Errorf("box = %d, want %d", m.Box, want)
	}
	if want := f26(77 * nibEm * (1 + nibGap)); m.Advance != want {
		t.Errorf("advance = %v, want %v", m.Advance, want)
	}
	// It sits wholly above the baseline, the way a printed nib does.
	if m.Ascent <= m.Box/2 || m.Ascent > m.Box {
		t.Errorf("ascent %d does not hold the nib above the baseline of a %d box", m.Ascent, m.Box)
	}
	// Nothing else draws through this renderer, so a name with braces in it
	// keeps them rather than losing a word.
	for _, code := range []string{"R", "W/U", "1", "T", ""} {
		if _, ok := nib.Symbol(code, 77); ok {
			t.Errorf("Symbol(%q) drew through the nib renderer", code)
		}
	}
}

func TestArtistNibDrawsFlatInk(t *testing.T) {
	ms := newManaSymbols("")
	if ms == nil {
		t.Fatal("no mana font resolved")
	}
	const box = 64
	dst := image.NewRGBA(image.Rect(0, 0, box, box))
	nib := artistNib{sym: ms, ink: color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}}
	if err := nib.DrawSymbol(dst, nibCode, dst.Bounds()); err != nil {
		t.Fatalf("DrawSymbol: %v", err)
	}
	// The nib is flat ink in the color it was given, with no disc behind it, so
	// the middle is white and the corners stay clear.
	r, g, b, a := dst.At(box/2, box/2).RGBA()
	if a == 0 {
		t.Fatal("the middle of the nib is transparent")
	}
	if r>>8 != 0xFF || g>>8 != 0xFF || b>>8 != 0xFF {
		t.Errorf("nib ink = (%d, %d, %d), want the white it was given", r>>8, g>>8, b>>8)
	}
	if _, _, _, a := dst.At(0, 0).RGBA(); a != 0 {
		t.Error("a corner was painted, so something drew a disc")
	}
	// An unknown code paints nothing rather than the nib.
	blank := image.NewRGBA(image.Rect(0, 0, box, box))
	if err := nib.DrawSymbol(blank, "R", blank.Bounds()); err != nil {
		t.Fatalf("DrawSymbol on another code: %v", err)
	}
	if _, _, _, a := blank.At(box/2, box/2).RGBA(); a != 0 {
		t.Error("another code painted the nib")
	}
}
