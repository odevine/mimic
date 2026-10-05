package render

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// The split manifest below is authored on a 1000 by 600 canvas and delivered
// 600 by 1000, turned 270 degrees clockwise, so an authored point (x, y) lands
// at (y, 999-x) in the delivered image
const (
	authoredW, authoredH = 1000, 600
	halfW, halfH         = 400, 500
)

var (
	red     = color.NRGBA{0xE0, 0x20, 0x20, 0xFF}
	blue    = color.NRGBA{0x20, 0x20, 0xE0, 0xFF}
	yellow  = color.NRGBA{0xE0, 0xE0, 0x20, 0xFF}
	green   = color.NRGBA{0x20, 0xE0, 0x20, 0xFF}
	magenta = color.NRGBA{0xE0, 0x20, 0xE0, 0xFF}
)

func writeSolid(t *testing.T, dir, name string, w, h int, c color.NRGBA) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func splitAssets(t *testing.T) *template.FSAssetProvider {
	t.Helper()
	dir := t.TempDir()
	writeSolid(t, dir, "red.png", halfW, halfH, red)
	writeSolid(t, dir, "blue.png", halfW, halfH, blue)
	writeSolid(t, dir, "bar_r.png", 900, 40, red)
	writeSolid(t, dir, "bar_u.png", 900, 40, blue)
	variants := func(r, u string) map[string]template.LayerAsset {
		return map[string]template.LayerAsset{"r": {Path: r}, "u": {Path: u}}
	}
	black := func(x, y, w int, half int) template.TextBoxSpec {
		return template.TextBoxSpec{
			X: x, Y: y, Width: w, Height: 40, FontSize: 30, Color: "#000000", Half: half, Box: "title",
		}
	}
	m := template.Manifest{
		Template: "test", Width: authoredW, Height: authoredH, Rotate: 270,
		Layers: []template.LayerSpec{
			{Name: "bg_1", Half: 1, X: 50, Y: 50, ColorSlot: "background", ColorVariants: variants("red.png", "blue.png")},
			{Name: "bg_2", Half: 2, X: 550, Y: 50, ColorSlot: "background", ColorVariants: variants("red.png", "blue.png")},
			{
				Name: "bar", Condition: "fuse", X: 50, Y: 550, ColorSlot: "fuse", ColorBlend: true,
				ColorVariants: map[string]template.LayerAsset{"r": {Path: "bar_r.png"}, "u": {Path: "bar_u.png"}},
			},
		},
		Arts: []template.ArtSlot{
			{X: 60, Y: 60, Width: 100, Height: 100, After: "bg_2"},
			{X: 560, Y: 60, Width: 100, Height: 100, After: "bg_2"},
		},
		TextBoxes: map[string]template.TextBoxSpec{
			"title_1": black(60, 400, 300, 1),
			"title_2": black(560, 400, 300, 2),
			"legal": {
				X: 10, Y: 940, Width: 500, Height: 40, FontSize: 30, Color: "#FFFFFF",
				Box: "copyright", Space: template.SpaceOutput,
			},
		},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return template.NewFSAssetProvider(dir)
}

// artImage is 200 by 100 with a green left half and a magenta right half, so a
// slot's color shows which half of the one art image it was cut from
func artImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			c := green
			if x >= 100 {
				c = magenta
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func renderSplit(t *testing.T, d *card.Data) image.Image {
	t.Helper()
	buf, err := New("test").Render(context.Background(), template.RenderRequest{
		Card: d, Assets: splitAssets(t), Art: artImage(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return buf.ToImage(8)
}

func splitCard(fuse bool) *card.Data {
	d := &card.Data{
		Name: "Fire // Ice", Layout: "split", Colors: []card.Color{card.Blue, card.Red},
		Faces: []card.Face{
			{Name: "Fire", ManaCost: "{1}{R}", TypeLine: "Instant", OracleText: "Fire deals 2 damage."},
			{Name: "", ManaCost: "{1}{U}", TypeLine: "Instant", OracleText: "Tap target permanent."},
		},
		SetCode: "dmr", ReleasedAt: "2023-01-01",
	}
	if fuse {
		d.Keywords = []string{"Fuse"}
	}
	return d
}

func at(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(y, authoredW-1-x)).(color.NRGBA)
}

func TestRenderSplitCard(t *testing.T) {
	img := renderSplit(t, splitCard(false))
	if got := img.Bounds(); got.Dx() != authoredH || got.Dy() != authoredW {
		t.Fatalf("delivered %v, want 600 by 1000", got)
	}

	cases := []struct {
		name string
		x, y int
		want color.NRGBA
	}{
		{"first half frames from its own color", 300, 300, red},
		{"second half frames from its own color", 800, 300, blue},
		{"first art slot is the left of the art", 110, 110, green},
		{"second art slot is the right of the art", 610, 110, magenta},
		{"a half's layer sits at its own offset", 40, 300, color.NRGBA{}},
		{"the fuse bar is off for an ordinary card", 500, 570, color.NRGBA{}},
	}
	for _, c := range cases {
		if got := at(img, c.x, c.y); got != c.want {
			t.Errorf("%s: authored (%d,%d) is %v, want %v", c.name, c.x, c.y, got, c.want)
		}
	}
}

func TestRenderSplitFuseBar(t *testing.T) {
	img := renderSplit(t, splitCard(true))
	// The first half is red and the second blue, so the bar runs from one to the
	// other, with both showing in the seam between
	if got := at(img, 200, 570); got != red {
		t.Errorf("bar left of the seam = %v, want %v", got, red)
	}
	if got := at(img, 800, 570); got != blue {
		t.Errorf("bar right of the seam = %v, want %v", got, blue)
	}
	if got := at(img, 520, 570); got == red || got == blue || got.A == 0 {
		t.Errorf("bar in the seam = %v, want a blend of both", got)
	}
}

// ink reports the bounds of the pixels in r the render has drawn over what was
// there, which is a text box's footprint on a flat frame
func ink(img image.Image, r image.Rectangle, flat color.NRGBA) image.Rectangle {
	var out image.Rectangle
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) != flat {
				out = out.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return out
}

func TestRenderSplitText(t *testing.T) {
	img := renderSplit(t, splitCard(false))

	// The first half's title runs along authored y 400..440 from x 60, which the
	// turn puts at delivered x 400..440 and y 640..940. Its second half's name is
	// empty, so it draws nothing
	first := ink(img, image.Rect(400, 540, 440, 940), red)
	second := ink(img, image.Rect(400, 140, 440, 440), blue)
	if first.Empty() {
		t.Error("the first half's title drew nothing")
	}
	if !second.Empty() {
		t.Errorf("the second half drew %v, but its face has no name", second)
	}
	if first.Dy() <= first.Dx() {
		t.Errorf("the authored text is %v, want it turned onto its side", first)
	}

	// The legal line is laid out after the turn, so it reads across the card
	legal := ink(img, image.Rect(0, 900, 600, 1000), color.NRGBA{})
	if legal.Empty() || legal.Dx() <= legal.Dy() {
		t.Errorf("legal line is %v, want it upright", legal)
	}
}
