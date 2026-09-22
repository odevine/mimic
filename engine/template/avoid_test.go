package template

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/frame"
)

// writeTestPNG writes a docW by docH transparent image with fill painted into
// rect, the shape every AvoidRect test resolves a layer asset to
func writeTestPNG(t *testing.T, path string, docW, docH int, rect image.Rectangle) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, docW, docH))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0xFF, A: 0xFF})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestAvoidRect(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "pt_box.png"), 100, 100, image.Rect(60, 70, 90, 95))
	p := NewFSAssetProvider(dir)

	layers := map[string]LayerSpec{
		"pt_box": {
			Name:      "pt_box",
			Condition: "creature",
			ColorSlot: "ptBox",
			ColorVariants: map[string]LayerAsset{
				"gold": {Path: "pt_box.png"},
			},
		},
	}
	f := frame.Keys{Creature: true, PTBox: "gold"}

	r, ok := AvoidRect(p, layers, f, "pt_box")
	if !ok {
		t.Fatal("AvoidRect reported false for a creature with a matching variant")
	}
	if want := image.Rect(60, 70, 90, 95); r != want {
		t.Errorf("rect = %v, want %v", r, want)
	}

	// The layer's own condition gates it: a noncreature never draws pt_box, so
	// nothing needs avoiding.
	if _, ok := AvoidRect(p, layers, frame.Keys{Creature: false, PTBox: "gold"}, "pt_box"); ok {
		t.Error("AvoidRect reported true for a noncreature, want false")
	}

	// No color variant resolves for this key and there is no "any" fallback.
	if _, ok := AvoidRect(p, layers, frame.Keys{Creature: true, PTBox: "colorless"}, "pt_box"); ok {
		t.Error("AvoidRect reported true with no matching or \"any\" variant, want false")
	}

	// A name absent from the layer map reports false rather than panicking.
	if _, ok := AvoidRect(p, layers, f, "missing"); ok {
		t.Error("AvoidRect reported true for an unknown layer name, want false")
	}
}
