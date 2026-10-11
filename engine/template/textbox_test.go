package template

import (
	"math"
	"testing"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
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
	rows := map[string]TextBoxSpec{"artist": {Y: 900}}
	box := TextBoxSpec{X: 10, Y: 800, Width: 100}

	moved := MoveToRow(box, rows, "artist")
	if moved.Y != 900 {
		t.Errorf("Y = %d, want the artist row's 900", moved.Y)
	}
	if moved.X != box.X || moved.Width != box.Width {
		t.Error("MoveToRow changed more than Y")
	}

	// A layout missing the named row leaves box where it was.
	same := MoveToRow(box, rows, "missing")
	if same != box {
		t.Errorf("MoveToRow with an absent row = %+v, want box unchanged %+v", same, box)
	}
}

// override builds the override a manifest's "when" entry decodes to
func override(cond string, x, width int) manifest.Override {
	return manifest.Override{Condition: cond, Box: &manifest.TextBox{Rect: manifest.Rect{X: x, Width: width}}}
}

func boxWith(x, width int, when ...manifest.Override) *Box {
	b, err := compileBox("type", manifest.TextBox{Rect: manifest.Rect{X: x, Width: width}, When: when})
	if err != nil {
		panic(err)
	}
	return b
}

// widthFor is the width a box draws at for a card, the way a render picks it
func widthFor(b *Box, k frame.Keys) int {
	if i, ok := b.Pick(func(c frame.Condition) bool { return c.Met(k, nil) }); ok {
		return b.Overrides[i].Box.Rect.Width
	}
	return b.Base.Rect.Width
}

// A transform card's type line moves for a color indicator on either face, and the
// override that names the most conditions wins
func TestBoxOverridesTakeTheMostSpecificMatch(t *testing.T) {
	b := boxWith(0, 3,
		override("back", 0, 5),
		override("color_indicator", 0, 4),
		override("back,color_indicator", 0, 6),
	)
	cases := []struct {
		name string
		keys frame.Keys
		want int
	}{
		{"front", frame.Keys{Front: true}, 3},
		{"back", frame.Keys{Back: true}, 5},
		{"back with an indicator takes the most specific match", frame.Keys{Back: true, Indicator: frame.Named("u")}, 6},
		{"front with an indicator", frame.Keys{Front: true, Indicator: frame.Named("u")}, 4},
		{"single", frame.Keys{}, 3},
	}
	for _, c := range cases {
		if got := widthFor(b, c.keys); got != c.want {
			t.Errorf("%s: width = %d, want %d", c.name, got, c.want)
		}
	}
}

// The type line gives up room for the set symbol and for a color indicator, in
// four combinations
func TestBoxOverridesForTheSetSymbolAndAnIndicator(t *testing.T) {
	b := boxWith(0, 2531,
		override("set_symbol", 0, 2100),
		override("color_indicator,set_symbol", 0, 1971),
		override("color_indicator", 0, 2402),
	)
	tests := []struct {
		name string
		keys frame.Keys
		want int
	}{
		{"symbol", frame.Keys{SetSymbol: true}, 2100},
		{"no symbol takes the full width", frame.Keys{}, 2531},
		{"symbol and color indicator", frame.Keys{SetSymbol: true, Indicator: frame.Named("u")}, 1971},
		{"color indicator and no symbol", frame.Keys{Indicator: frame.Named("u")}, 2402},
	}
	for _, tc := range tests {
		if got := widthFor(b, tc.keys); got != tc.want {
			t.Errorf("%s: width = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestOverridesWithTheSameNumberOfConditionsKeepTheirWrittenOrder(t *testing.T) {
	b := boxWith(0, 1, override("land", 0, 2), override("legendary", 0, 3))
	if got := widthFor(b, frame.Keys{Land: true, Legendary: true}); got != 2 {
		t.Errorf("width = %d, want the first written override", got)
	}
}

func TestACompiledBoxNeedsItsOverridesMerged(t *testing.T) {
	_, err := compileBox("title", manifest.TextBox{When: []manifest.Override{{Condition: "land"}}})
	if err == nil {
		t.Error("an override that was never merged compiled")
	}
}
