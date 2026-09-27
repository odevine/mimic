package render

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// writeFaceAssets writes a manifest whose layers each paint their own flat
// color into a corner, so a render shows which of them its face chose
func writeFaceAssets(t *testing.T, fills map[string]image.Rectangle, colors map[string]color.NRGBA, layers []template.LayerSpec) string {
	t.Helper()
	dir := t.TempDir()
	for name, rect := range fills {
		if err := writeFlat(filepath.Join(dir, name+".png"), rect, colors[name]); err != nil {
			t.Fatal(err)
		}
	}
	m := template.Manifest{
		Template: "test",
		Width:    placeholderWidth,
		Height:   placeholderHeight,
		Layers:   layers,
		TextBoxes: map[string]template.TextBoxSpec{
			"title":      {X: 48, Y: 40, Width: 520, Height: 52, FontSize: 38, Color: "#000000", Condition: "front"},
			"title_back": {X: 48, Y: 40, Width: 520, Height: 52, FontSize: 38, Color: "#FFFFFF", Box: "title", Condition: "back"},
		},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
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
	variant := func(name string) map[string]template.LayerAsset {
		return map[string]template.LayerAsset{"any": {Path: name + ".png"}}
	}
	dir := writeFaceAssets(t, fills, colors, []template.LayerSpec{
		{Name: "front", Condition: "front", ColorVariants: variant("front")},
		{Name: "back", Condition: "back", ColorVariants: variant("back")},
		{Name: "backland", Condition: "back,land", ColorVariants: variant("backland")},
		{Name: "icon", ColorSlot: "transform_icon", ColorVariants: map[string]template.LayerAsset{
			"sunmoondfc":     {Path: "sunmoon.png"},
			"compasslanddfc": {Path: "compass.png"},
		}},
		{Name: "indicator", Condition: "color_indicator", ColorSlot: "indicator", ColorVariants: map[string]template.LayerAsset{
			"u": {Path: "ind_u.png"},
		}},
	})

	d := &card.Data{
		Name: "Front Face", TypeLine: "Creature — Human", Power: "1", Toughness: "1",
		Layout: "transform", FrameEffects: []string{"legendary", "compasslanddfc"},
		Faces: []card.Face{
			{Name: "Front Face", TypeLine: "Creature — Human", Power: "1", Toughness: "1"},
			{Name: "Back Face", TypeLine: "Land", ColorIndicator: []card.Color{card.Blue}},
		},
	}
	p := template.NewFSAssetProvider(dir)
	m, err := p.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	// Each face renders once at the smallest resolution, and every case samples
	// that render at its native coordinate scaled down
	s := m.ScaleForDPI(template.MinDPI)
	renders := map[int]image.Image{}
	at := func(face int, x, y int) color.NRGBA {
		t.Helper()
		img, ok := renders[face]
		if !ok {
			buf, err := New("test").Render(context.Background(), template.RenderRequest{
				Card: d, Face: face, Assets: p, DPI: template.MinDPI,
			})
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
	// skips that layer, still gets its art above its own background
	colors := map[string]color.NRGBA{"front": {0xE0, 0x20, 0x20, 0xFF}, "back": {0x20, 0x20, 0xE0, 0xFF}}
	fills := map[string]image.Rectangle{"front": fullRect(), "back": fullRect()}
	dir := writeFaceAssets(t, fills, colors, []template.LayerSpec{
		{Name: "front", Condition: "front", ColorVariants: map[string]template.LayerAsset{"any": {Path: "front.png"}}},
		{Name: "back", Condition: "back", ColorVariants: map[string]template.LayerAsset{"any": {Path: "back.png"}}},
	})
	p := template.NewFSAssetProvider(dir)
	m, err := p.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	m.Art = template.ArtSlot{X: 60, Y: 132, Width: 624, Height: 424, After: "back"}
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	art := image.NewNRGBA(image.Rect(0, 0, 624, 424))
	magenta := color.NRGBA{0xFF, 0x00, 0xFF, 0xFF}
	for y := range 424 {
		for x := range 624 {
			art.SetNRGBA(x, y, magenta)
		}
	}
	d := &card.Data{Name: "A", TypeLine: "Creature", Layout: "transform", Faces: []card.Face{{Name: "A", TypeLine: "Creature"}, {Name: "B", TypeLine: "Creature"}}}
	s := m.ScaleForDPI(template.MinDPI)
	buf, err := New("test").Render(context.Background(), template.RenderRequest{Card: d, Art: art, Assets: template.NewFSAssetProvider(dir), DPI: template.MinDPI})
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(buf.ToImage(8).At(s.Px(300), s.Px(300))).(color.NRGBA); !closeColor(got, magenta) {
		t.Errorf("art region on the front = %v, want the art %v", got, magenta)
	}
}

func TestRenderMirrorsALayerOnItsCondition(t *testing.T) {
	// The corner mark sits on the left, and a triangle back flips the top
	// band so it lands on the right while the lower mark stays put
	colors := map[string]color.NRGBA{"mark": {0xE0, 0x20, 0x20, 0xFF}}
	fills := map[string]image.Rectangle{"mark": image.Rect(0, 0, 40, 40)}
	low := image.Rect(0, 900, 40, 940)
	dir := writeFaceAssets(t, fills, colors, []template.LayerSpec{
		{
			Name: "mark", ColorVariants: map[string]template.LayerAsset{"any": {Path: "mark.png"}},
			Mirror: &template.LayerMirror{Condition: "icon_right", Width: 100, Height: 100},
		},
	})
	lowPath := filepath.Join(dir, "low.png")
	if err := writeFlat(lowPath, low, colors["mark"]); err != nil {
		t.Fatal(err)
	}
	p := template.NewFSAssetProvider(dir)
	m, err := p.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	m.Layers = append(m.Layers, template.LayerSpec{
		Name: "low", ColorVariants: map[string]template.LayerAsset{"any": {Path: "low.png"}},
		Mirror: &template.LayerMirror{Condition: "icon_right", Width: 100, Height: 100},
	})
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	d := &card.Data{Name: "A", TypeLine: "Creature", Layout: "transform", Faces: []card.Face{{Name: "A", TypeLine: "Creature"}, {Name: "B", TypeLine: "Creature"}}}
	// Each face renders once and every check samples that render
	renders := map[int]image.Image{}
	alpha := func(face, x, y int) uint8 {
		t.Helper()
		img, ok := renders[face]
		if !ok {
			buf, err := New("test").Render(context.Background(), template.RenderRequest{Card: d, Face: face, Assets: template.NewFSAssetProvider(dir)})
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
