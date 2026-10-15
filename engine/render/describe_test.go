package render

import (
	"context"
	"image"
	"image/color"
	"math"
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
	// The box's own place is the baseline across its width, which the ink does not say
	if want := image.Rect(48, 120, 568, 120); title.BaselineAuthored != want || title.BaselineDelivered != want {
		t.Errorf("title baseline is %v and %v, want %v", title.BaselineAuthored, title.BaselineDelivered, want)
	}
	if oracle := got["text:oracle"]; !oracle.BaselineAuthored.Empty() || !oracle.BaselineDelivered.Empty() || oracle.BaselineAuthored != (image.Rectangle{}) {
		t.Errorf("a box that is not set on its baseline has one: %v", oracle.BaselineAuthored)
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

func TestDescribeTurnsABaselineWithTheLayout(t *testing.T) {
	const w, h = placeholderWidth, placeholderHeight
	tm := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		l.Canvas.Rotate = 90
		title := l.TextBoxes["title"]
		title.Rect = manifest.Rect{X: 48, Y: 120, Width: 520, Height: 52}
		title.Text.VAlign = "baseline"
		l.TextBoxes["title"] = title
		out := box(30, 40, 200, 40, 24)
		out.Draws = "artist"
		out.Text.VAlign = "baseline"
		out.Space = template.SpaceOutput
		l.TextBoxes["legal"] = out
		m.Layouts["test"] = l
	})
	c := creature(card.White)
	c.Name = "Gopher Guard"
	c.Artist = "Someone"
	got := items(t, describe(t, tm, template.RenderRequest{Card: c}))

	title := got["text:title"]
	if !title.Ink {
		t.Fatalf("title = %+v", title)
	}
	line := image.Rect(48, 120, 568, 120)
	if title.BaselineAuthored != line {
		t.Errorf("authored baseline %v, want %v", title.BaselineAuthored, line)
	}
	// A quarter turn makes the horizontal line a vertical one in the finished image, and
	// it turns as the box's ink does
	want := template.RotateRect(line, w, h, 90)
	if title.BaselineDelivered != want || want.Dx() != 0 || want.Dy() != 520 {
		t.Errorf("delivered baseline %v, want %v, a vertical line", title.BaselineDelivered, want)
	}

	// A box laid out in the finished image has its baseline there, and in the layout's
	// own canvas it turns back
	legal := got["text:legal"]
	if !legal.Ink {
		t.Fatalf("legal = %+v", legal)
	}
	if want := image.Rect(30, 40, 230, 40); legal.BaselineDelivered != want {
		t.Errorf("an output box's baseline is %v, want %v", legal.BaselineDelivered, want)
	}
	if back := template.RotateRect(legal.BaselineAuthored, w, h, 90); back != legal.BaselineDelivered {
		t.Errorf("legal baseline authored %v turns to %v, want %v", legal.BaselineAuthored, back, legal.BaselineDelivered)
	}
}

func TestDescribeSaysHowTextFitted(t *testing.T) {
	tm := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		short := box(48, 300, 520, 200, 28)
		short.Draws = "oracle"
		l.TextBoxes["oracle"] = short
		// A one line title with room, and one in a box far too narrow for its name
		wide := l.TextBoxes["title"]
		wide.Text.VAlign = "baseline"
		l.TextBoxes["title"] = wide
		tight := box(48, 600, 60, 40, 38)
		tight.Draws = "artist"
		tight.Text.VAlign = "baseline"
		l.TextBoxes["artist"] = tight
		m.Layouts["test"] = l
	})
	c := creature(card.White)
	c.Name = "Gopher"
	c.Artist = "A Name Far Too Long To Fit In So Small A Box At Any Size"
	c.OracleText = "Vigilance"
	got := items(t, describe(t, tm, template.RenderRequest{Card: c}))

	title := got["text:title"].Fit
	if title == nil || title.Size != title.Wanted || title.Wanted != 38 || title.Overflows || title.Lines != 1 {
		t.Errorf("a name with room fitted as %+v", title)
	}
	rules := got["text:oracle"].Fit
	if rules == nil || rules.Size != 28 || rules.Overflows {
		t.Errorf("a line of rules in a roomy box fitted as %+v", rules)
	}
	artist := got["text:artist"].Fit
	if artist == nil || artist.Size >= artist.Wanted || artist.Size > artist.Floor+0.5 || !artist.Overflows {
		t.Errorf("a name in a box too narrow for it fitted as %+v, want the floor and overflowing", artist)
	}
	if want := 38 * 0.55; math.Abs(artist.Floor-want) > 1e-9 {
		t.Errorf("the floor is %v, want %v, a share of the size the box asks for", artist.Floor, want)
	}
	// What is not drawn, and what is not a text box, has no fit
	if it := got["text:note"]; it.Fit != nil || it.Drawn {
		t.Errorf("a box that does not draw has a fit: %+v", it.Fit)
	}
	if got["layer:frame"].Fit != nil {
		t.Error("a layer has a fit")
	}

	// Text that needs the room shrinks to fit and no further
	c.OracleText = strings.Repeat("Whenever this creature attacks, draw a card. ", 6)
	crowded := items(t, describe(t, tm, template.RenderRequest{Card: c}))["text:oracle"].Fit
	if crowded == nil || crowded.Size >= crowded.Wanted || crowded.Overflows || crowded.Lines < 2 {
		t.Errorf("rules that need the room fitted as %+v, want smaller than asked for and fitting", crowded)
	}
}

func TestDescribePlacesABoxWhereTheCardPutsIt(t *testing.T) {
	tm := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		title := l.TextBoxes["title"]
		title.Flow = manifest.Flow{ClearOf: "pt"}
		l.TextBoxes["title"] = title
		// The power and toughness sit on the title's row, at its right
		pt := box(400, 40, 168, 52, 38)
		pt.Text.Align = "right"
		l.TextBoxes["pt"] = pt
		m.Layouts["test"] = l
	})
	plain := items(t, describe(t, tm, template.RenderRequest{Card: creature(card.White)}))["text:title"]
	if plain.Moved || !plain.Narrowed {
		t.Fatalf("a title beside the stats, moved %v narrowed %v", plain.Moved, plain.Narrowed)
	}
	if own := image.Rect(48, 40, 568, 92); plain.Authored != own {
		t.Fatalf("the box is %v, want its own rectangle %v", plain.Authored, own)
	}
	if p := plain.PlacedAuthored; p.Min != image.Pt(48, 40) || p.Dx() >= 520 || p.Dx() <= 0 || p.Dy() != 52 {
		t.Errorf("the title is placed at %v, want the same row and narrower than 520", p)
	}
	if plain.PlacedDelivered != plain.PlacedAuthored {
		t.Errorf("placed %v and %v in an unturned layout", plain.PlacedAuthored, plain.PlacedDelivered)
	}
}

func TestDescribeMovesATitleToAnotherRow(t *testing.T) {
	tm := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		title := l.TextBoxes["title"]
		title.Flow = manifest.Flow{Duck: &manifest.Duck{Layer: "crown", Row: "pt"}}
		l.TextBoxes["title"] = title
		m.Layouts["test"] = l
	})
	still := items(t, describe(t, tm, template.RenderRequest{Card: creature(card.White)}))["text:title"]
	if still.Moved || still.Narrowed || still.PlacedAuthored != still.Authored {
		t.Errorf("a title for a card without the crown moved: %+v", still)
	}
	moved := items(t, describe(t, tm, template.RenderRequest{Card: legendary()}))["text:title"]
	if !moved.Moved || moved.Narrowed {
		t.Fatalf("moved %v narrowed %v for a card the crown draws for", moved.Moved, moved.Narrowed)
	}
	// The pt box's row is at 960, and the box keeps its size and place across
	if want := image.Rect(48, 960, 568, 1012); moved.PlacedAuthored != want {
		t.Errorf("the title is placed at %v, want %v", moved.PlacedAuthored, want)
	}
	if moved.Authored != image.Rect(48, 40, 568, 92) {
		t.Errorf("the box's own rectangle changed to %v", moved.Authored)
	}
}

func TestDescribeSaysWhichOverrideIsInForce(t *testing.T) {
	tm := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		title := l.TextBoxes["title"]
		title.When = []manifest.Override{
			when("land", `{"rect":{"x":10}}`),
			when("legendary", `{"rect":{"x":60}}`),
			when("legendary, creature", `{"rect":{"x":70}}`),
		}
		l.TextBoxes["title"] = title
		m.Layouts["test"] = l
	})
	none := items(t, describe(t, tm, template.RenderRequest{Card: creature(card.White)}))["text:title"]
	if none.Override != nil || none.Authored.Min.X != 48 {
		t.Errorf("a card no override applies to has %+v at %v", none.Override, none.Authored)
	}
	// Two overrides hold, and the one naming more conditions wins, though it is written
	// last, so its place in the layout's list is not its place in the order tried
	got := items(t, describe(t, tm, template.RenderRequest{Card: legendary()}))["text:title"]
	if got.Override == nil || got.Override.When != "legendary, creature" || got.Override.Index != 2 {
		t.Fatalf("override = %+v, want the one for legendary, creature at index 2", got.Override)
	}
	if got.Authored.Min.X != 70 {
		t.Errorf("the box is at %v, want the override's x of 70", got.Authored)
	}
	land := items(t, describe(t, tm, template.RenderRequest{Card: &card.Data{Name: "Land", TypeLine: "Land", Layout: "normal"}}))["text:title"]
	if land.Override == nil || land.Override.Index != 0 || land.Override.When != "land" {
		t.Errorf("override = %+v for a land", land.Override)
	}
}
