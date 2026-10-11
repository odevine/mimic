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

func TestEntryBounds(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "pt_box.png"), 100, 100, image.Rect(60, 70, 90, 95))
	p := NewFSAssetProvider(dir)

	cond, err := frame.Compile("creature")
	if err != nil {
		t.Fatal(err)
	}
	e := &Entry{
		ID:        "pt_box",
		Condition: cond,
		Part: &Part{
			Name: "pt_box", Slot: frame.SlotPTBox,
			Variants: map[string]string{"gold": "pt_box.png"},
		},
	}
	creature := frame.Keys{Creature: true, PTBox: frame.Gold}

	r, ok := EntryBounds(p, e, creature, nil, 1, 0, 0)
	if !ok {
		t.Fatal("EntryBounds reported false for a creature with a matching variant")
	}
	if want := image.Rect(60, 70, 90, 95); r != want {
		t.Errorf("rect = %v, want %v", r, want)
	}

	// The entry's own condition gates it: a noncreature never draws pt_box, so
	// nothing needs avoiding.
	if _, ok := EntryBounds(p, e, frame.Keys{Creature: false, PTBox: frame.Gold}, nil, 1, 0, 0); ok {
		t.Error("EntryBounds reported true for a noncreature, want false")
	}

	// No variant resolves for this key and there is no "any" fallback.
	if _, ok := EntryBounds(p, e, frame.Keys{Creature: true, PTBox: frame.Colorless}, nil, 1, 0, 0); ok {
		t.Error("EntryBounds reported true with no matching or \"any\" variant, want false")
	}

	// A feature the entry's condition reads decides it too
	gated := *e
	if gated.Condition, err = frame.Compile("feature:ptStyle=dark"); err != nil {
		t.Fatal(err)
	}
	if _, ok := EntryBounds(p, &gated, creature, frame.Features{"ptStyle": "light"}, 1, 0, 0); ok {
		t.Error("EntryBounds reported true with the feature on another value")
	}
	if _, ok := EntryBounds(p, &gated, creature, frame.Features{"ptStyle": "dark"}, 1, 0, 0); !ok {
		t.Error("EntryBounds reported false with the feature on its value")
	}

	// An entry placed with At is measured where it sits, not at its part's origin
	moved := *e
	moved.At = &image.Point{X: 10, Y: 20}
	if r, ok := EntryBounds(p, &moved, creature, nil, 1, 0, 0); !ok || r != image.Rect(70, 90, 100, 115) {
		t.Errorf("a placed entry's rect = %v, %v, want 70,90 to 100,115", r, ok)
	}

	// An image that is not there reports false rather than panicking
	missing := *e
	missing.Part = &Part{Slot: frame.SlotPTBox, Variants: map[string]string{"gold": "nope.png"}}
	if _, ok := EntryBounds(p, &missing, creature, nil, 1, 0, 0); ok {
		t.Error("EntryBounds reported true for an image that does not exist")
	}
}
