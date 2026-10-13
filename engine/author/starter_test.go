package author

import (
	"bytes"
	"context"
	"image"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
)

func TestStarterBuildsAndRendersACard(t *testing.T) {
	files, err := Starter(StarterOptions{ID: "me.my-frame", Name: "My frame", DPI: 150})
	if err != nil {
		t.Fatal(err)
	}
	built, ds := Build(folder(files), Options{})
	if built == nil {
		t.Fatal(ds)
	}
	tm, _, err := template.Load(template.NewMemoryAssets(built.ManifestJSON, built.Assets))
	if err != nil {
		t.Fatal(err)
	}
	buf, err := render.Render(context.Background(), tm, template.RenderRequest{Card: testCard()})
	if err != nil {
		t.Fatal(err)
	}
	img := buf.ToImage(8)
	if got, want := img.Bounds().Size(), (image.Point{X: 408, Y: 555}); got != want {
		t.Fatalf("rendered %v, want %v for a card at 150 dpi", got, want)
	}
	if !hasDarkPixel(img, image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()/6)) {
		t.Error("the title area is blank, so the standard text boxes did not draw")
	}
}

func hasDarkPixel(img image.Image, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if cr, cg, cb, _ := img.At(x, y).RGBA(); cr < 0x4000 && cg < 0x4000 && cb < 0x4000 {
				return true
			}
		}
	}
	return false
}

func TestStarterDefaultsToTheStandardResolution(t *testing.T) {
	files, err := Starter(StarterOptions{ID: "me.my-frame", Name: "My frame"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["layouts/normal.json"]), `"dpi": 800`) {
		t.Errorf("layout = %s", files["layouts/normal.json"])
	}
}

func TestStarterRefusesWhatCannotBeBuilt(t *testing.T) {
	tests := []struct {
		name string
		opt  StarterOptions
		want string
	}{
		{"too coarse", StarterOptions{ID: "me.a", Name: "A", DPI: 10}, "dpi"},
		{"too fine", StarterOptions{ID: "me.a", Name: "A", DPI: 5000}, "dpi"},
		{"bad id", StarterOptions{ID: "Bad Id", Name: "A"}, "id"},
		{"no name", StarterOptions{ID: "me.a"}, "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Starter(tt.opt)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one that mentions %q", err, tt.want)
			}
		})
	}
}

func TestStarterIsStableAndStrictlyValid(t *testing.T) {
	a, err := Starter(StarterOptions{ID: "me.a", Name: "A", DPI: 100})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Starter(StarterOptions{ID: "me.a", Name: "A", DPI: 100})
	for name := range a {
		if !bytes.Equal(a[name], b[name]) {
			t.Errorf("%s differs between runs", name)
		}
	}
	if ds := Validate(folder(a), Options{}); ds.HasErrors() {
		t.Fatal(ds)
	}
}
