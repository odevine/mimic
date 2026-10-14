package render

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

var (
	crownRect = image.Rect(100, 50, 300, 130)
	badgeRect = image.Rect(400, 700, 440, 760)
)

// describeTemplate has a frame with an image for white and blue, a crown that draws
// for legendary cards, an image with nothing visible, a badge that draws for lands,
// and text boxes for the title, the power and toughness, and a land-only note
func describeTemplate(t *testing.T, edit func(m *manifest.Manifest)) *template.Template {
	t.Helper()
	files := map[string][]byte{
		"w.png":     flat(fullRect(), color.NRGBA{0xF0, 0xF0, 0xE0, 0xFF}),
		"u.png":     flat(fullRect(), color.NRGBA{0x20, 0x40, 0xC0, 0xFF}),
		"crown.png": flat(crownRect, color.NRGBA{0xD0, 0xA0, 0x20, 0xFF}),
		"badge.png": flat(badgeRect, color.NRGBA{0x10, 0x10, 0x10, 0xFF}),
		"ghost.png": flat(image.Rectangle{}, color.NRGBA{}),
	}
	title := box(48, 40, 520, 52, 38)
	title.Draws = "title"
	pt := box(560, 960, 120, 50, 38)
	note := box(48, 120, 300, 40, 24)
	note.Condition = "land"
	m := testManifest(manifest.Layout{
		Stack: []manifest.Entry{
			{ID: "frame", Part: "frame"},
			{ID: "crown", Part: "crown", Condition: "legendary"},
			{ID: "ghost", Part: "ghost"},
			{ID: "badge", Part: "badge", Condition: "land"},
		},
		TextBoxes: map[string]manifest.TextBox{"title": title, "pt": pt, "note": note},
		Art:       []manifest.ArtWindow{{X: 60, Y: 180, Width: 600, Height: 400, After: "frame"}},
		Symbols:   map[string]manifest.Symbol{"set": {X: 600, Y: 600, Width: 60, Height: 40}},
	}, map[string]manifest.Part{
		"frame": {Slot: "pinlines", Variants: map[string]manifest.Variant{"w": {Path: "w.png"}, "u": {Path: "u.png"}}},
		"crown": {Variants: anyImage("crown.png")},
		"ghost": {Variants: anyImage("ghost.png")},
		"badge": {Variants: anyImage("badge.png")},
	})
	if edit != nil {
		edit(&m)
	}
	return mustLoad(t, withManifest(t, nil, m, files))
}

func creature(colors ...card.Color) *card.Data {
	return &card.Data{
		Name: "Test", TypeLine: "Creature - Human", Power: "2", Toughness: "2",
		Colors: colors, ColorIdentity: colors, Layout: "normal",
	}
}

func items(t *testing.T, d *Description) map[string]Item {
	t.Helper()
	out := map[string]Item{}
	for _, it := range d.Items {
		out[string(it.Kind)+":"+it.Key] = it
	}
	return out
}

func describe(t *testing.T, tm *template.Template, req template.RenderRequest) *Description {
	t.Helper()
	d, err := Describe(tm, req)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDescribeSaysWhatDrawsAndWhy(t *testing.T) {
	d := describe(t, describeTemplate(t, nil), template.RenderRequest{Card: creature(card.White)})
	got := items(t, d)

	full := fullRect()
	if it := got["layer:frame"]; !it.Drawn || it.Authored != full || it.Delivered != full || len(it.Images) != 1 || it.Images[0] != "w.png" {
		t.Errorf("frame = %+v, want drawn over the whole canvas from w.png", it)
	}
	if it := got["layer:crown"]; it.Drawn || !strings.Contains(it.Why, "legendary") || !it.Authored.Empty() {
		t.Errorf("crown = %+v, want undrawn because the card is not legendary", it)
	}
	if it := got["layer:ghost"]; it.Drawn || !strings.Contains(it.Why, "visible") {
		t.Errorf("ghost = %+v, want undrawn because nothing in it is visible", it)
	}
	if it := got["layer:badge"]; it.Drawn || !strings.Contains(it.Why, "land") {
		t.Errorf("badge = %+v", it)
	}
	if it := got["art:0"]; it.Drawn || it.Why == "" || it.After != "frame" || it.Authored != image.Rect(60, 180, 660, 580) {
		t.Errorf("art = %+v, want an undrawn window above the frame because no art was given", it)
	}
	if it := got["text:title"]; !it.Drawn || it.Authored != image.Rect(48, 40, 568, 92) {
		t.Errorf("title = %+v", it)
	}
	if it := got["text:pt"]; !it.Drawn {
		t.Errorf("pt = %+v, want drawn for a creature", it)
	}
	if it := got["text:note"]; it.Drawn || !strings.Contains(it.Why, "land") {
		t.Errorf("note = %+v", it)
	}
	if it := got["symbol:set"]; it.Drawn || it.Why == "" {
		t.Errorf("set symbol = %+v, want undrawn because the card has none", it)
	}
}

func TestDescribeFollowsTheCard(t *testing.T) {
	tm := describeTemplate(t, nil)
	legendary := creature(card.White)
	legendary.TypeLine = "Legendary Creature - Human"
	got := items(t, describe(t, tm, template.RenderRequest{Card: legendary, Art: image.NewNRGBA(image.Rect(0, 0, 10, 10))}))
	if it := got["layer:crown"]; !it.Drawn || it.Authored != crownRect {
		t.Errorf("crown = %+v, want drawn at %v", it, crownRect)
	}
	if !got["art:0"].Drawn {
		t.Errorf("art = %+v, want drawn once art is given", got["art:0"])
	}

	land := &card.Data{Name: "Test Land", TypeLine: "Land", ProducedMana: []card.Color{card.White}, Layout: "normal"}
	got = items(t, describe(t, tm, template.RenderRequest{Card: land}))
	if it := got["layer:badge"]; !it.Drawn || it.Authored != badgeRect {
		t.Errorf("badge = %+v, want drawn for a land", it)
	}
	if got["text:pt"].Drawn {
		t.Errorf("pt = %+v, want undrawn for a land, which has no power", got["text:pt"])
	}
}

func TestDescribeNamesTheImagesOfABlendedPair(t *testing.T) {
	d := describe(t, describeTemplate(t, nil), template.RenderRequest{Card: creature(card.White, card.Blue)})
	it := items(t, d)["layer:frame"]
	if !it.Drawn || len(it.Images) != 2 || it.Images[0] != "w.png" || it.Images[1] != "u.png" {
		t.Fatalf("frame = %+v, want a blend of w.png and u.png", it)
	}
	if it.Authored != fullRect() {
		t.Errorf("bounds = %v", it.Authored)
	}
}

func TestDescribeSaysWhenNoImageFitsTheCard(t *testing.T) {
	d := describe(t, describeTemplate(t, nil), template.RenderRequest{Card: creature(card.Green)})
	if it := items(t, d)["layer:frame"]; it.Drawn || !strings.Contains(it.Why, `"g"`) {
		t.Fatalf("frame = %+v, want undrawn with no image for g", it)
	}
}

func TestDescribeKeepsToTheRenderDecision(t *testing.T) {
	tm := describeTemplate(t, nil)
	for _, c := range []*card.Data{creature(card.White), creature(card.Blue), creature(card.Green), creature(card.White, card.Blue)} {
		d := describe(t, tm, template.RenderRequest{Card: c})
		for _, it := range d.Items {
			if it.Kind != ItemLayer || !it.Drawn {
				continue
			}
			if it.Authored.Empty() || it.Delivered.Empty() {
				t.Errorf("%v: drawn layer %s has no bounds", c.Colors, it.Key)
			}
		}
	}
}

func TestDescribeGivesBoundsInBothCanvasesForARotatedLayout(t *testing.T) {
	const w, h = placeholderWidth, placeholderHeight
	tm := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		l.Canvas.Rotate = 90
		out := box(30, 20, 200, 40, 24)
		out.Draws = "artist"
		out.Space = template.SpaceOutput
		l.TextBoxes["legal"] = out
		m.Layouts["test"] = l
	})
	legendary := creature(card.White)
	legendary.TypeLine = "Legendary Creature - Human"
	legendary.Artist = "Someone"
	d := describe(t, tm, template.RenderRequest{Card: legendary})
	if d.Authored != image.Pt(w, h) || d.Delivered != image.Pt(h, w) {
		t.Fatalf("sizes %v and %v", d.Authored, d.Delivered)
	}
	got := items(t, d)

	crown := got["layer:crown"]
	if want := template.RotateRect(crownRect, w, h, 90); crown.Authored != crownRect || crown.Delivered != want {
		t.Errorf("crown authored %v delivered %v, want %v and %v", crown.Authored, crown.Delivered, crownRect, want)
	}
	legal := got["text:legal"]
	if want := image.Rect(30, 20, 230, 60); legal.Delivered != want {
		t.Errorf("an output box is laid out in the delivered canvas, got %v want %v", legal.Delivered, want)
	}
	if back := template.RotateRect(legal.Authored, w, h, 90); back != legal.Delivered {
		t.Errorf("legal authored %v turns to %v, want %v", legal.Authored, back, legal.Delivered)
	}
}

func TestDescribeRefusesWhatRenderRefuses(t *testing.T) {
	tm := describeTemplate(t, nil)
	if _, err := Describe(tm, template.RenderRequest{}); err == nil {
		t.Error("a request with no card was described")
	}
	if _, err := Describe(tm, template.RenderRequest{Card: creature(card.White), Face: 3}); err == nil {
		t.Error("a face the card does not have was described")
	}
	if _, err := Describe(nil, template.RenderRequest{Card: creature(card.White)}); err == nil {
		t.Error("no template was described")
	}
}

// inkBounds is the smallest rectangle in area that holds a pixel unlike the flat
// color the frame is
func inkBounds(img image.Image, area image.Rectangle, flat color.Color) image.Rectangle {
	fr, fg, fb, _ := flat.RGBA()
	var out image.Rectangle
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r != fr || g != fg || b != fb {
				out = out.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return out
}

// A box set on its baseline is shown where its text is drawn, which the box's own
// rectangle does not say, and the rectangle stays the box for the others
func TestDescribeShowsABaselineBoxWhereItsTextIs(t *testing.T) {
	tm := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		title := l.TextBoxes["title"]
		title.Rect = manifest.Rect{X: 48, Y: 120, Width: 520, Height: 52}
		title.Text.VAlign = "baseline"
		l.TextBoxes["title"] = title
		rules := box(48, 300, 520, 200, 28)
		rules.Draws = "oracle"
		l.TextBoxes["oracle"] = rules
		m.Layouts["test"] = l
	})
	c := creature(card.White)
	c.Name = "Gopher Guard"
	c.OracleText = "Vigilance"
	req := template.RenderRequest{Card: c}
	got := items(t, describe(t, tm, req))

	title := got["text:title"]
	if !title.Drawn || !title.Ink {
		t.Fatalf("title = %+v, want drawn and measured by its ink", title)
	}
	// The box's top is the baseline, so the text sits mostly above it, with only the
	// descender of a letter like the p below
	if title.Authored.Min.Y >= 100 || title.Authored.Max.Y < 120 || title.Authored.Max.Y > 140 {
		t.Errorf("title ink %v does not sit on the baseline at 120", title.Authored)
	}
	if oracle := got["text:oracle"]; oracle.Ink || oracle.Authored != image.Rect(48, 300, 568, 500) {
		t.Errorf("oracle = %+v, want the box's own rectangle", oracle)
	}

	// What it says matches what is drawn, to a pixel or two for the antialiasing
	buf, err := Render(context.Background(), tm, req)
	if err != nil {
		t.Fatal(err)
	}
	drawn := inkBounds(buf.ToImage(8), image.Rect(40, 60, 580, 200), color.NRGBA{0xF0, 0xF0, 0xE0, 0xFF})
	near := func(a, b int) bool { return a-b <= 2 && b-a <= 2 }
	if !near(title.Authored.Min.X, drawn.Min.X) || !near(title.Authored.Max.X, drawn.Max.X) || !near(title.Authored.Min.Y, drawn.Min.Y) || !near(title.Authored.Max.Y, drawn.Max.Y) {
		t.Errorf("Describe says the title is at %v, and its ink is at %v", title.Authored, drawn)
	}
}

func TestDescribeKeepsAnUndrawnTextBoxItsOwnRectangle(t *testing.T) {
	tm := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		pt := l.TextBoxes["pt"]
		pt.Text.VAlign = "baseline"
		l.TextBoxes["pt"] = pt
		m.Layouts["test"] = l
	})
	land := &card.Data{Name: "Test Land", TypeLine: "Land", Layout: "normal"}
	it := items(t, describe(t, tm, template.RenderRequest{Card: land}))["text:pt"]
	if it.Drawn || it.Ink || it.Authored != image.Rect(560, 960, 680, 1010) {
		t.Errorf("pt = %+v, want the box's rectangle", it)
	}
}
