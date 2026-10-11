package render

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
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

func splitTemplate(t *testing.T) *template.Template {
	t.Helper()
	dir := t.TempDir()
	writeSolid(t, dir, "red.png", halfW, halfH, red)
	writeSolid(t, dir, "blue.png", halfW, halfH, blue)
	writeSolid(t, dir, "bar_r.png", 900, 40, red)
	writeSolid(t, dir, "bar_u.png", 900, 40, blue)
	title := func(x, y, w int, face string) manifest.TextBox {
		b := box(x, y, w, 40, 30)
		b.Draws, b.Face = "title", face
		return b
	}
	legal := box(10, 940, 500, 40, 30)
	legal.Draws, legal.Space = "copyright", template.SpaceOutput
	legal.Text.Color = "#FFFFFF"
	// The bar runs two colors with a seam where the old fixed table put it: from
	// half the canvas to fifty-four hundredths, which is 40 pixels wide about 20
	// right of the middle
	bar := manifest.Blend{Shape: "linear", Seams: map[string][]manifest.Seam{"2": {{Center: 20, Width: 40}}}}
	writeManifestFile(t, dir, testManifest(manifest.Layout{
		Canvas: manifest.Canvas{Width: authoredW, Height: authoredH, Rotate: 270},
		Stack: []manifest.Entry{
			{ID: "bg_1", Part: "half", Face: "first", At: &manifest.Point{X: 50, Y: 50}},
			{ID: "bg_2", Part: "half", Face: "second", At: &manifest.Point{X: 550, Y: 50}},
			{ID: "bar", Part: "bar", Condition: "fuse"},
		},
		Art: []manifest.ArtWindow{
			{X: 60, Y: 60, Width: 100, Height: 100, After: "bg_2", Face: "first"},
			{X: 560, Y: 60, Width: 100, Height: 100, After: "bg_2", Face: "second"},
		},
		TextBoxes: map[string]manifest.TextBox{
			"title_1": title(60, 400, 300, "first"),
			"title_2": title(560, 400, 300, "second"),
			"legal":   legal,
		},
	}, map[string]manifest.Part{
		"half": {Slot: "background", Variants: map[string]manifest.Variant{"r": {Path: "red.png"}, "u": {Path: "blue.png"}}},
		"bar": {
			Slot: "fuse", Origin: manifest.Point{X: 50, Y: 550}, Blend: &bar,
			Variants: map[string]manifest.Variant{"r": {Path: "bar_r.png"}, "u": {Path: "bar_u.png"}},
		},
	}, frame.Shape{Role: frame.RoleSplit, Kind: frame.KindStandard}))
	return mustLoad(t, template.NewFSAssetProvider(dir))
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
	buf, err := Render(context.Background(), splitTemplate(t), template.RenderRequest{Card: d, Art: artImage()})
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
