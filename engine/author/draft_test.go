package author

import (
	"context"
	"image"
	"image/color"
	"testing"
	"testing/fstest"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
)

func testCard() *card.Data {
	return &card.Data{
		Name: "Test Creature", ManaCost: "{W}", TypeLine: "Creature - Human", OracleText: "Vigilance",
		Power: "1", Toughness: "1", Colors: []card.Color{card.White}, ColorIdentity: []card.Color{card.White},
		Rarity: "common", CollectorNumber: "1", SetCode: "tst", Artist: "A. Artist", Language: "en", Layout: "normal",
	}
}

func renderWith(t *testing.T, tm *template.Template) image.Image {
	t.Helper()
	buf, err := render.Render(context.Background(), tm, template.RenderRequest{Card: testCard()})
	if err != nil {
		t.Fatal(err)
	}
	return buf.ToImage(8)
}

func samePixels(t *testing.T, a, b image.Image) bool {
	t.Helper()
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}

func TestDraftRendersLikeABuild(t *testing.T) {
	fsys := smallFS(t)
	built := build(t, fsys)
	packed, _, err := template.Load(template.NewMemoryAssets(built.ManifestJSON, built.Assets))
	if err != nil {
		t.Fatal(err)
	}
	d := NewDraft(fsys, 64<<20)
	defer d.Close()
	opened, ds, err := d.Open(Options{})
	if err != nil {
		t.Fatalf("%v: %v", err, ds)
	}
	if !samePixels(t, renderWith(t, packed), renderWith(t, opened)) {
		t.Fatal("a draft renders differently from the bundle built from the same folder")
	}
}

func TestDraftReadsAChangedImageOnlyAfterForget(t *testing.T) {
	fsys := smallFS(t)
	d := NewDraft(fsys, 64<<20)
	defer d.Close()
	open := func() image.Image {
		tm, ds, err := d.Open(Options{})
		if err != nil {
			t.Fatalf("%v: %v", err, ds)
		}
		return renderWith(t, tm)
	}
	before := open()

	green := color.NRGBA{0, 255, 0, 255}
	fsys["bg/w.png"] = &fstest.MapFile{Data: pngOf(t, 40, image.Rect(10, 10, 20, 30), green)}
	if !samePixels(t, before, open()) {
		t.Fatal("the cache served a changed file without being told")
	}
	d.Forget("bg/w.png")
	if samePixels(t, before, open()) {
		t.Fatal("the changed image was not read again after Forget")
	}
}

func TestDraftOpenReportsErrorsAndNoTemplate(t *testing.T) {
	fsys := smallFS(t)
	fsys["layouts/normal.json"] = &fstest.MapFile{Data: []byte(`{"serves": []}`)}
	d := NewDraft(fsys, 64<<20)
	defer d.Close()
	tm, ds, err := d.Open(Options{})
	if tm != nil || err == nil || !ds.HasErrors() {
		t.Fatalf("got template %v, error %v, diagnostics %v", tm, err, ds)
	}
}

func TestDraftTemplatesSurviveALaterOpen(t *testing.T) {
	fsys := smallFS(t)
	d := NewDraft(fsys, 64<<20)
	defer d.Close()
	first, _, err := d.Open(Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := renderWith(t, first)
	fsys["layouts/normal.json"] = &fstest.MapFile{Data: []byte(stdLayoutMoved)}
	if _, _, err := d.Open(Options{}); err != nil {
		t.Fatal(err)
	}
	if !samePixels(t, want, renderWith(t, first)) {
		t.Fatal("an earlier template changed when the draft was opened again")
	}
}

// stdLayoutMoved is stdLayout with the title box moved
const stdLayoutMoved = `{
  "serves": [{"role": "single", "kind": "standard"}],
  "canvas": {"width": 100, "height": 100},
  "stack": [{"part": "bg"}, {"id": "mark", "part": "mark", "at": {"x": 100, "y": 50}}],
  "textBoxes": {"title": {"rect": {"x": 30, "y": 40, "width": 50, "height": 10}, "text": {"fontSize": 12}}}
}`
