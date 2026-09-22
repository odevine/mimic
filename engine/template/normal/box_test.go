package normal

import (
	"math"
	"testing"

	"golang.org/x/image/math/fixed"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/mana"
	"github.com/odevine/mimic/engine/template"
)

func TestManaBoxDoesNotShrink(t *testing.T) {
	// A cost wider than its box keeps its size and grows leftward out of it, so
	// the symbols stay the size a printed card gives them.
	sym := mana.NewSymbols("")
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
