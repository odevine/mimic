package render

import (
	"testing"

	"golang.org/x/image/math/fixed"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/mana"
	"github.com/odevine/mimic/engine/template"
)

func TestMinFontSizeStopsManaCostFromShrinking(t *testing.T) {
	// A manifest's mana box sets MinFontSize equal to its own FontSize, since a
	// symbol is a fixed size on a printed card: a cost wider than its box keeps
	// its size and grows leftward out of it rather than shrinking into it.
	sym := mana.NewSymbols("")
	if sym == nil {
		t.Fatal("no mana font resolved")
	}
	box := template.TextBoxSpec{X: 2047, Y: 506, Width: 866, Height: 200, FontSize: 179, MinFontSize: 179, Align: "right", VAlign: "baseline"}
	cost := template.TextPart{Text: "{W}{W}{U}{U}{B}{B}{R}{R}{G}{G}", Src: fonts.ResolveFont(fonts.Body, ""), Sym: sym}

	ten, err := template.MeasureTextSpan(box, cost)
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
	// Left without MinFontSize the box would have shrunk, so the box below
	// proves the field is what is holding it steady.
	box.MinFontSize = 0
	loose, err := template.MeasureTextSpan(box, cost)
	if err != nil {
		t.Fatalf("MeasureTextSpan: %v", err)
	}
	if loose.Right-loose.Left >= ten.Right-ten.Left {
		t.Error("the box did not shrink without MinFontSize, so the test proves nothing")
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
