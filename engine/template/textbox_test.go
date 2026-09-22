package template

import (
	"math"
	"testing"
)

func TestClearOf(t *testing.T) {
	box := TextBoxSpec{X: 386, Width: 2400, FontSize: 156}
	gap := int(math.Round(box.FontSize * 0.5))

	// A span reaching into the box narrows it to stop a gap short.
	narrowed := ClearOf(box, TextSpan{Left: 1338, Right: 2913})
	if want := 1338 - gap - box.X; narrowed.Width != want {
		t.Errorf("width = %d, want %d", narrowed.Width, want)
	}
	if narrowed.X != box.X {
		t.Errorf("X = %d, want the box left alone at %d", narrowed.X, box.X)
	}

	// An explicit ClearGap overrides the 0.5 default.
	box.ClearGap = 1.0
	tightGap := int(math.Round(box.FontSize * 1.0))
	custom := ClearOf(box, TextSpan{Left: 1338, Right: 2913})
	if want := 1338 - tightGap - box.X; custom.Width != want {
		t.Errorf("width with ClearGap 1.0 = %d, want %d", custom.Width, want)
	}
	box.ClearGap = 0

	// A span that already clears the box leaves it as the manifest wrote it,
	// so the box never grows.
	clear := ClearOf(box, TextSpan{Left: 2900, Right: 2913})
	if clear.Width != box.Width {
		t.Errorf("width = %d, want the manifest's %d", clear.Width, box.Width)
	}

	// An empty span means nothing to clear, so the box keeps its whole width.
	none := ClearOf(box, TextSpan{})
	if none.Width != box.Width {
		t.Errorf("width = %d, want the manifest's %d", none.Width, box.Width)
	}

	// A span swallowing the whole box leaves no room rather than a negative
	// width the layout would have to guard against.
	full := ClearOf(box, TextSpan{Left: 100, Right: 2913})
	if full.Width != 0 {
		t.Errorf("width = %d, want 0", full.Width)
	}
}

func TestMoveToRow(t *testing.T) {
	m := &Manifest{TextBoxes: map[string]TextBoxSpec{
		"artist": {Y: 900},
	}}
	box := TextBoxSpec{X: 10, Y: 800, Width: 100}

	moved := MoveToRow(box, m, "artist")
	if moved.Y != 900 {
		t.Errorf("Y = %d, want the artist row's 900", moved.Y)
	}
	if moved.X != box.X || moved.Width != box.Width {
		t.Error("MoveToRow changed more than Y")
	}

	// A manifest missing the named row leaves box where it was.
	same := MoveToRow(box, m, "missing")
	if same != box {
		t.Errorf("MoveToRow with an absent row = %+v, want box unchanged %+v", same, box)
	}
}
