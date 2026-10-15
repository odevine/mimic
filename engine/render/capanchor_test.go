package render

import (
	"testing"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/mana"
	"github.com/odevine/mimic/engine/template"
)

// capMidline is the vertical middle of the capitals of a box's text. The ink a box
// measures to takes a letter's bounds from the font, which for some letters reach
// below the baseline, so the middle is taken from the top of the ink and the cap
// height of the face at the size the text was laid out at
func capMidline(t *testing.T, box template.TextBoxSpec, part template.TextPart) float64 {
	t.Helper()
	ink, err := template.MeasureTextBox(box, part)
	if err != nil {
		t.Fatal(err)
	}
	fit, err := template.FitTextBox(box, part)
	if err != nil {
		t.Fatal(err)
	}
	face, err := part.Src.Face(fit.Size)
	if err != nil {
		t.Fatal(err)
	}
	return float64(ink.Min.Y) + float64(face.Metrics().CapHeight)/64/2
}

// A title set on the middle of its capitals keeps them on Y however far it shrinks to fit
// its width, which a baseline box does not
func TestCapCenterKeepsATitleOnItsLineAsItShrinks(t *testing.T) {
	const y = 500
	title := template.TextPart{Text: "HEH", Src: fonts.Default(fonts.Title)}
	for _, width := range []int{4000, 300, 150} {
		box := template.TextBoxSpec{X: 100, Y: y, Width: width, Height: 200, FontSize: 156, MinFontSize: 20, VAlign: "cap-center"}
		if got := capMidline(t, box, title); got < y-3 || got > y+3 {
			t.Errorf("width %d: capitals centered at %.1f, want %d", width, got, y)
		}
	}
	box := template.TextBoxSpec{X: 100, Y: y, Width: 150, Height: 200, FontSize: 156, MinFontSize: 20, VAlign: "baseline"}
	if got := capMidline(t, box, title); got > y-20 {
		t.Errorf("a shrunk baseline title is centered at %.1f, the test expects it well above Y", got)
	}
}

func TestCapTopHangsTheCapitalsFromY(t *testing.T) {
	const y = 500
	title := template.TextPart{Text: "HEH", Src: fonts.Default(fonts.Title)}
	for _, width := range []int{4000, 150} {
		box := template.TextBoxSpec{X: 100, Y: y, Width: width, Height: 200, FontSize: 156, MinFontSize: 20, VAlign: "cap-top"}
		ink, err := template.MeasureTextBox(box, title)
		if err != nil {
			t.Fatal(err)
		}
		if ink.Min.Y < y-2 || ink.Min.Y > y+2 {
			t.Errorf("width %d: capitals start at %d, want %d", width, ink.Min.Y, y)
		}
	}
}

// Mana symbols and a title on the same Y are centered on each other, at the sizes a card
// sets them in
func TestManaSymbolsAndTitleShareACapCenter(t *testing.T) {
	sym := mana.NewSymbols(fonts.Default(fonts.Mana))
	if sym == nil {
		t.Fatal("no mana font resolved")
	}
	const y = 500
	title := template.TextBoxSpec{X: 100, Y: y, Width: 2400, Height: 200, FontSize: 156, MinFontSize: 20, VAlign: "cap-center"}
	cost := template.TextBoxSpec{X: 2047, Y: y, Width: 866, Height: 200, FontSize: 179, MinFontSize: 179, Align: "right", VAlign: "cap-center"}
	a := capMidline(t, title, template.TextPart{Text: "HEH", Src: fonts.Default(fonts.Title)})
	pips, err := template.MeasureTextBox(cost, template.TextPart{Text: "{W}{U}", Src: fonts.Default(fonts.Body), Sym: sym})
	if err != nil {
		t.Fatal(err)
	}
	b := float64(pips.Min.Y+pips.Max.Y) / 2
	t.Logf("title capitals centered at %.1f, mana pips at %.1f", a, b)
	if d := a - b; d < -6 || d > 6 {
		t.Errorf("title centered at %.1f and mana at %.1f, want them within 6px", a, b)
	}
}
