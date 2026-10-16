package template

import (
	"image"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/manifest"
)

func sectionRowsOf(texts ...string) []RowText {
	out := make([]RowText, len(texts))
	for i, s := range texts {
		out[i] = RowText{Parts: []TextPart{{Text: s, Src: scaleSource{}}}, X: 0, Width: 200}
	}
	return out
}

func sectionSpec(h int) (manifest.Section, TextBoxSpec) {
	sec := manifest.Section{Rect: manifest.Rect{X: 0, Y: 100, Width: 200, Height: h}, Gap: 10}
	return sec, TextBoxSpec{X: 0, Y: 100, Width: 200, Height: h, FontSize: 20, MinFontSize: 5}
}

func TestLayoutSectionSpreadsRowsEvenly(t *testing.T) {
	sec, style := sectionSpec(400)
	lay, err := LayoutSection(sec, style, image.Rectangle{}, sectionRowsOf("a", "b", "c"))
	if err != nil {
		t.Fatal(err)
	}
	if lay.Size != 20 || len(lay.Rows) != 3 {
		t.Fatalf("size %g, %d rows", lay.Size, len(lay.Rows))
	}
	h := lay.Rows[0].InkBottom - lay.Rows[0].InkTop
	if h <= 0 {
		t.Fatalf("row height %d", h)
	}
	above := lay.Rows[0].Top - 100
	between := lay.Rows[1].Top - lay.Rows[0].InkBottom
	below := 500 - lay.Rows[2].InkBottom
	if d := above - between; d < -1 || d > 1 {
		t.Errorf("above %d, between %d", above, between)
	}
	if d := below - between; d < -1 || d > 1 {
		t.Errorf("below %d, between %d", below, between)
	}
	if mid := lay.Between(0); mid <= lay.Rows[0].InkBottom || mid >= lay.Rows[1].Top {
		t.Errorf("between rows 0 and 1 is %d", mid)
	}
}

func TestLayoutSectionWeighsTheSpaceBetweenRows(t *testing.T) {
	sec, style := sectionSpec(400)
	sec.Spread = &manifest.Spread{Outside: 1, Inside: 2}
	lay, err := LayoutSection(sec, style, image.Rectangle{}, sectionRowsOf("a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	above := lay.Rows[0].Top - 100
	between := lay.Rows[1].Top - lay.Rows[0].InkBottom
	if d := between - 2*above; d < -2 || d > 2 {
		t.Errorf("above %d, between %d, want twice", above, between)
	}
}

func TestLayoutSectionShrinksEveryRowTogether(t *testing.T) {
	sec, style := sectionSpec(120)
	long := strings.Repeat("word ", 40)
	lay, err := LayoutSection(sec, style, image.Rectangle{}, sectionRowsOf(long, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if lay.Size >= 20 || lay.Size < 5 {
		t.Fatalf("size %g, want shrunk", lay.Size)
	}
	for _, r := range lay.Rows {
		if r.Box.FontSize != lay.Size {
			t.Errorf("a row is set at %g, not the shared %g", r.Box.FontSize, lay.Size)
		}
	}
	if last := lay.Rows[1]; last.InkBottom > 220 {
		t.Errorf("the last row ends at %d, past the region", last.InkBottom)
	}
}

func TestLayoutSectionHoldsRoomForAHeader(t *testing.T) {
	sec, style := sectionSpec(400)
	rows := sectionRowsOf("a", "b")
	rows[1].Header = 50
	lay, err := LayoutSection(sec, style, image.Rectangle{}, rows)
	if err != nil {
		t.Fatal(err)
	}
	if r := lay.Rows[1]; r.InkTop-r.Top != 50 {
		t.Errorf("row 1 text starts %d below its top, want the header's 50", r.InkTop-r.Top)
	}
}

func TestScaleSectionScalesItsPixels(t *testing.T) {
	sec := manifest.Section{
		Rect: manifest.Rect{X: 100, Y: 200, Width: 300, Height: 400}, Text: manifest.TextStyle{FontSize: 40},
		Gap: 20, Header: &manifest.Header{For: "cost", Height: 60},
		Items:   []manifest.RowItem{{Y: 1000, Step: 50, Rect: &manifest.Rect{X: 10, Y: 20, Width: 30, Height: 40}}},
		Divider: &manifest.RowPart{Part: "d", Y: 500},
	}
	got := ScaleSection(sec, 0.5)
	if got.Rect.X != 50 || got.Text.FontSize != 20 || got.Gap != 10 || got.Header.Height != 30 ||
		got.Items[0].Y != 500 || got.Items[0].Step != 25 || got.Items[0].Rect.Width != 15 || got.Divider.Y != 250 {
		t.Errorf("scaled = %+v", got)
	}
	if sec.Items[0].Y != 1000 || sec.Header.Height != 60 {
		t.Error("scaling changed the section it was given")
	}
}
