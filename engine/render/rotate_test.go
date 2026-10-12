package render

import (
	"context"
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

var flatGrey = color.NRGBA{0x80, 0x80, 0x80, 0xFF}

// flatTemplate is a grey frame with the layout edited by edit
func flatTemplate(t *testing.T, edit func(l *manifest.Layout)) *template.Template {
	t.Helper()
	dir := writeFaceTemplate(t, nil, nil, map[string]manifest.Part{
		"front": {Variants: anyImage("front.png")},
	}, []manifest.Entry{{Part: "front"}})
	if err := writeFlat(filepath.Join(dir, "front.png"), fullRect(), flatGrey); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, template.NewFSAssetProvider(dir))
	l := m.Layouts["test"]
	edit(&l)
	m.Layouts["test"] = l
	writeManifestFile(t, dir, m)
	return mustLoad(t, template.NewFSAssetProvider(dir))
}

func TestRenderTurnsATextBoxWithinItsRect(t *testing.T) {
	for _, deg := range []int{0, 90, 180, 270} {
		// The rect is where the box ends up, so a quarter turn is given a tall one
		want := image.Rect(200, 300, 500, 360)
		if deg == 90 || deg == 270 {
			want = image.Rect(320, 180, 380, 480)
		}
		tmpl := flatTemplate(t, func(l *manifest.Layout) {
			b := box(want.Min.X, want.Min.Y, want.Dx(), want.Dy(), 38)
			b.Draws = "title"
			b.Rotate = deg
			l.TextBoxes = map[string]manifest.TextBox{"title": b}
		})
		buf, err := Render(context.Background(), tmpl, template.RenderRequest{Card: turnCard()})
		if err != nil {
			t.Fatal(err)
		}
		got := ink(buf.ToImage(8), image.Rect(0, 0, 744, 1039), flatGrey)
		if got.Empty() || !got.In(want) {
			t.Errorf("rotate %d: ink at %v, want inside %v", deg, got, want)
		}
		if tall := got.Dy() > got.Dx(); tall != (deg == 90 || deg == 270) {
			t.Errorf("rotate %d: ink %v has the wrong orientation", deg, got)
		}
	}
}

func TestRenderTurnsArtWithinItsWindow(t *testing.T) {
	tmpl := flatTemplate(t, func(l *manifest.Layout) {
		l.Art = []manifest.ArtWindow{{X: 100, Y: 100, Width: 200, Height: 100, After: "front", Rotate: 90}}
	})
	// The art is red above and blue below, and a clockwise quarter turn puts the
	// top on the right
	art := image.NewNRGBA(image.Rect(0, 0, 100, 200))
	red, blue := color.NRGBA{0xFF, 0, 0, 0xFF}, color.NRGBA{0, 0, 0xFF, 0xFF}
	for y := range 200 {
		for x := range 100 {
			art.SetNRGBA(x, y, map[bool]color.NRGBA{true: red, false: blue}[y < 100])
		}
	}
	buf, err := Render(context.Background(), tmpl, template.RenderRequest{Card: turnCard(), Art: art})
	if err != nil {
		t.Fatal(err)
	}
	img := buf.ToImage(8)
	at := func(x, y int) color.NRGBA { return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) }
	if got := at(250, 150); !closeColor(got, red) {
		t.Errorf("right of the window = %v, want red", got)
	}
	if got := at(150, 150); !closeColor(got, blue) {
		t.Errorf("left of the window = %v, want blue", got)
	}
}

func turnCard() *card.Data {
	return &card.Data{Name: "Turned", TypeLine: "Creature", Layout: "transform", Faces: []card.Face{{Name: "Turned", TypeLine: "Creature"}, {Name: "B", TypeLine: "Creature"}}}
}
