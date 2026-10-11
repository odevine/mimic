package render

import (
	"context"
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

// transformShapes are the two faces of a double-faced card
var transformShapes = []frame.Shape{
	{Role: frame.RoleTransformFront, Kind: frame.KindStandard},
	{Role: frame.RoleTransformBack, Kind: frame.KindStandard},
}

// writeFaceTemplate writes each image of fills into a folder, flat in its own
// color, and a manifest over them, so a render shows which of the entries its face
// chose
func writeFaceTemplate(t *testing.T, fills map[string]image.Rectangle, colors map[string]color.NRGBA, parts map[string]manifest.Part, stack []manifest.Entry) string {
	t.Helper()
	dir := t.TempDir()
	for name, rect := range fills {
		if err := writeFlat(filepath.Join(dir, name+".png"), rect, colors[name]); err != nil {
			t.Fatal(err)
		}
	}
	title := box(48, 40, 520, 52, 38)
	title.When = []manifest.Override{when("back", `{"text":{"color":"#FFFFFF"}}`)}
	writeManifestFile(t, dir, testManifest(manifest.Layout{
		Stack:     stack,
		TextBoxes: map[string]manifest.TextBox{"title": title},
	}, parts, transformShapes...))
	return dir
}

func TestRenderChoosesEachFacesLayers(t *testing.T) {
	colors := map[string]color.NRGBA{
		"front":    {0xE0, 0x20, 0x20, 0xFF},
		"back":     {0x20, 0x20, 0xE0, 0xFF},
		"sunmoon":  {0x20, 0xE0, 0x20, 0xFF},
		"compass":  {0xE0, 0xE0, 0x20, 0xFF},
		"ind_u":    {0x20, 0xE0, 0xE0, 0xFF},
		"backland": {0xE0, 0x20, 0xE0, 0xFF},
	}
	full := fullRect()
	corner := image.Rect(0, 0, 40, 40)
	dot := image.Rect(100, 500, 140, 540)
	strip := image.Rect(0, 900, placeholderWidth, 940)
	fills := map[string]image.Rectangle{
		"front": full, "back": full, "sunmoon": corner, "compass": corner, "ind_u": dot, "backland": strip,
	}
	dir := writeFaceTemplate(t, fills, colors, map[string]manifest.Part{
		"front":    {Variants: anyImage("front.png")},
		"back":     {Variants: anyImage("back.png")},
		"backland": {Variants: anyImage("backland.png")},
		"icon": {Slot: "transform_icon", Variants: map[string]manifest.Variant{
			"sunmoondfc":     {Path: "sunmoon.png"},
			"compasslanddfc": {Path: "compass.png"},
		}},
		"indicator": {Slot: "indicator", Variants: map[string]manifest.Variant{"u": {Path: "ind_u.png"}}},
	}, []manifest.Entry{
		{Part: "front", Condition: "front"},
		{Part: "back", Condition: "back"},
		{Part: "backland", Condition: "back,land"},
		{Part: "icon"},
		{Part: "indicator", Condition: "color_indicator"},
	})

	d := &card.Data{
		Name: "Front Face", TypeLine: "Creature — Human", Power: "1", Toughness: "1",
		Layout: "transform", FrameEffects: []string{"legendary", "compasslanddfc"},
		Faces: []card.Face{
			{Name: "Front Face", TypeLine: "Creature — Human", Power: "1", Toughness: "1"},
			{Name: "Back Face", TypeLine: "Land", ColorIndicator: []card.Color{card.Blue}},
		},
	}
	tmpl := mustLoad(t, template.NewFSAssetProvider(dir))
	// Each face renders once at the smallest resolution, and every case samples
	// that render at its native coordinate scaled down
	s := tmpl.Primary().ScaleForDPI(template.MinDPI)
	renders := map[int]image.Image{}
	at := func(face int, x, y int) color.NRGBA {
		t.Helper()
		img, ok := renders[face]
		if !ok {
			buf, err := Render(context.Background(), tmpl, template.RenderRequest{Card: d, Face: face, DPI: template.MinDPI})
			if err != nil {
				t.Fatalf("Render face %d: %v", face, err)
			}
			img = buf.ToImage(8)
			renders[face] = img
		}
		return color.NRGBAModel.Convert(img.At(s.Px(x), s.Px(y))).(color.NRGBA)
	}

	cases := []struct {
		name string
		face int
		x, y int
		want color.NRGBA
	}{
		{"front body", 0, 400, 700, colors["front"]},
		{"back body", 1, 400, 700, colors["back"]},
		{"front icon from frame effects", 0, 10, 10, colors["compass"]},
		{"back icon", 1, 10, 10, colors["compass"]},
		{"front has no indicator", 0, 120, 520, colors["front"]},
		{"back indicator", 1, 120, 520, colors["ind_u"]},
		{"front skips the back land layer", 0, 400, 920, colors["front"]},
		{"back land layer", 1, 400, 920, colors["backland"]},
	}
	for _, c := range cases {
		if got := at(c.face, c.x, c.y); !closeColor(got, c.want) {
			t.Errorf("%s: pixel (%d,%d) = %v, want %v", c.name, c.x, c.y, got, c.want)
		}
	}
}

func TestRenderPlacesArtAfterASkippedLayer(t *testing.T) {
	// The art follows the back's background in the stack, so the front, which
	// skips that entry, still gets its art above its own background
	colors := map[string]color.NRGBA{"front": {0xE0, 0x20, 0x20, 0xFF}, "back": {0x20, 0x20, 0xE0, 0xFF}}
	fills := map[string]image.Rectangle{"front": fullRect(), "back": fullRect()}
	dir := writeFaceTemplate(t, fills, colors, map[string]manifest.Part{
		"front": {Variants: anyImage("front.png")},
		"back":  {Variants: anyImage("back.png")},
	}, []manifest.Entry{
		{Part: "front", Condition: "front"},
		{Part: "back", Condition: "back"},
	})
	m := readManifest(t, template.NewFSAssetProvider(dir))
	l := m.Layouts["test"]
	l.Art = []manifest.ArtWindow{{X: 60, Y: 132, Width: 624, Height: 424, After: "back"}}
	m.Layouts["test"] = l
	writeManifestFile(t, dir, m)

	art := image.NewNRGBA(image.Rect(0, 0, 624, 424))
	magenta := color.NRGBA{0xFF, 0x00, 0xFF, 0xFF}
	for y := range 424 {
		for x := range 624 {
			art.SetNRGBA(x, y, magenta)
		}
	}
	d := &card.Data{Name: "A", TypeLine: "Creature", Layout: "transform", Faces: []card.Face{{Name: "A", TypeLine: "Creature"}, {Name: "B", TypeLine: "Creature"}}}
	tmpl := mustLoad(t, template.NewFSAssetProvider(dir))
	s := tmpl.Primary().ScaleForDPI(template.MinDPI)
	buf, err := Render(context.Background(), tmpl, template.RenderRequest{Card: d, Art: art, DPI: template.MinDPI})
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(buf.ToImage(8).At(s.Px(300), s.Px(300))).(color.NRGBA); !closeColor(got, magenta) {
		t.Errorf("art region on the front = %v, want the art %v", got, magenta)
	}
}

func TestRenderMirrorsAnEntryOnItsCondition(t *testing.T) {
	// The corner mark sits on the left, and a triangle back flips the top
	// band so it lands on the right while the lower mark stays put
	colors := map[string]color.NRGBA{"mark": {0xE0, 0x20, 0x20, 0xFF}}
	fills := map[string]image.Rectangle{"mark": image.Rect(0, 0, 40, 40), "low": image.Rect(0, 900, 40, 940)}
	colors["low"] = colors["mark"]
	mirror := &manifest.Mirror{Condition: "icon_right", Width: 100, Height: 100}
	dir := writeFaceTemplate(t, fills, colors, map[string]manifest.Part{
		"mark": {Variants: anyImage("mark.png")},
		"low":  {Variants: anyImage("low.png")},
	}, []manifest.Entry{
		{Part: "mark", Mirror: mirror},
		{Part: "low", Mirror: mirror},
	})

	d := &card.Data{Name: "A", TypeLine: "Creature", Layout: "transform", Faces: []card.Face{{Name: "A", TypeLine: "Creature"}, {Name: "B", TypeLine: "Creature"}}}
	tmpl := mustLoad(t, template.NewFSAssetProvider(dir))
	// Each face renders once and every check samples that render
	renders := map[int]image.Image{}
	alpha := func(face, x, y int) uint8 {
		t.Helper()
		img, ok := renders[face]
		if !ok {
			buf, err := Render(context.Background(), tmpl, template.RenderRequest{Card: d, Face: face})
			if err != nil {
				t.Fatal(err)
			}
			img = buf.ToImage(8)
			renders[face] = img
		}
		return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A
	}
	right := placeholderWidth - 10
	if alpha(0, 10, 10) == 0 || alpha(0, right, 10) != 0 {
		t.Error("the front mirrored its mark")
	}
	if alpha(1, 10, 10) != 0 || alpha(1, right, 10) == 0 {
		t.Error("the back did not mirror its mark into the right corner")
	}
	if alpha(1, 10, 920) == 0 {
		t.Error("the back mirrored a mark outside the region")
	}
}

// A box that reads the other face draws what the face being drawn does not print
func TestRenderDrawsAnotherFacesTextInABox(t *testing.T) {
	dir := writeFaceTemplate(t, nil, nil, map[string]manifest.Part{
		"front": {Variants: anyImage("front.png")},
	}, []manifest.Entry{{Part: "front"}})
	// A flat grey frame, with a title box that reads the face not being drawn
	if err := writeFlat(filepath.Join(dir, "front.png"), fullRect(), color.NRGBA{0x80, 0x80, 0x80, 0xFF}); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, template.NewFSAssetProvider(dir))
	l := m.Layouts["test"]
	other := box(48, 40, 520, 52, 38)
	other.Draws = "title"
	other.Face = "other"
	l.TextBoxes = map[string]manifest.TextBox{"title": other}
	m.Layouts["test"] = l
	writeManifestFile(t, dir, m)

	tmpl := mustLoad(t, template.NewFSAssetProvider(dir))
	drawn := func(d *card.Data, face int) bool {
		buf, err := Render(context.Background(), tmpl, template.RenderRequest{Card: d, Face: face})
		if err != nil {
			t.Fatal(err)
		}
		return !ink(buf.ToImage(8), image.Rect(48, 40, 568, 92), color.NRGBA{0x80, 0x80, 0x80, 0xFF}).Empty()
	}
	// The back face has a name and the front has none, so asking for the front draws
	// the back's name through a box that reads the other face
	d := &card.Data{Layout: "transform", Faces: []card.Face{
		{Name: "", TypeLine: "Creature"}, {Name: "Back Face", TypeLine: "Creature"},
	}}
	if !drawn(d, 0) {
		t.Error("the front's box read nothing from the back face")
	}
	if drawn(d, 1) {
		t.Error("the back's box drew, but the front face has no name")
	}
}
