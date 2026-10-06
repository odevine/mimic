package template

import (
	"math"
	"testing"

	"github.com/odevine/mimic/engine/frame"
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

func TestResolveTextBoxes(t *testing.T) {
	boxes := map[string]TextBoxSpec{
		"title":           {X: 1, Condition: "front"},
		"title_back":      {X: 2, Box: "title", Condition: "back"},
		"type":            {X: 3},
		"type_shift":      {X: 4, Box: "type", Condition: "color_indicator"},
		"type_back":       {X: 5, Box: "type", Condition: "back"},
		"type_back_shift": {X: 6, Box: "type", Condition: "back,color_indicator"},
	}
	cases := []struct {
		name        string
		keys        frame.Keys
		title, typ  int
		wantTitleOK bool
	}{
		{"front", frame.Keys{Front: true}, 1, 3, true},
		{"back", frame.Keys{Back: true}, 2, 5, true},
		{"back with an indicator takes the most specific match", frame.Keys{Back: true, Indicator: "u"}, 2, 6, true},
		{"front with an indicator", frame.Keys{Front: true, Indicator: "u"}, 1, 4, true},
		{"single has no title here", frame.Keys{}, 0, 3, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveTextBoxes(boxes, c.keys)
			title, ok := got["title"]
			if ok != c.wantTitleOK || title.X != c.title {
				t.Errorf("title = %+v (present %v), want X %d", title, ok, c.title)
			}
			if got["type"].X != c.typ {
				t.Errorf("type X = %d, want %d", got["type"].X, c.typ)
			}
			for _, alias := range []string{"title_back", "type_shift", "type_back", "type_back_shift"} {
				if _, ok := got[alias]; ok {
					t.Errorf("resolved boxes kept the spec key %q", alias)
				}
			}
		})
	}
}

func TestResolveTextBoxesSetSymbol(t *testing.T) {
	boxes := map[string]TextBoxSpec{
		"type":            {Width: 2100, Condition: "set_symbol"},
		"type_full":       {Box: "type", Width: 2531},
		"type_shift":      {Box: "type", Width: 1971, Condition: "color_indicator,set_symbol"},
		"type_shift_full": {Box: "type", Width: 2402, Condition: "color_indicator"},
	}
	tests := []struct {
		name string
		keys frame.Keys
		want int
	}{
		{"symbol", frame.Keys{SetSymbol: true}, 2100},
		{"no symbol takes the full width", frame.Keys{}, 2531},
		{"symbol and color indicator", frame.Keys{SetSymbol: true, Indicator: "u"}, 1971},
		{"color indicator and no symbol", frame.Keys{Indicator: "u"}, 2402},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTextBoxes(boxes, tc.keys)
			if len(got) != 1 || got["type"].Width != tc.want {
				t.Errorf("type width = %d in %v, want %d", got["type"].Width, got, tc.want)
			}
		})
	}
}
