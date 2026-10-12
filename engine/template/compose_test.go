package template

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestTurnRect(t *testing.T) {
	r := image.Rect(200, 300, 500, 360)
	tests := []struct {
		deg  int
		want image.Rectangle
	}{
		{0, r},
		{90, image.Rect(320, 180, 380, 480)},
		{180, r},
		{270, image.Rect(320, 180, 380, 480)},
		{-90, image.Rect(320, 180, 380, 480)},
		{360, r},
	}
	for _, tt := range tests {
		if got := TurnRect(r, 700, 660, tt.deg); got != tt.want {
			t.Errorf("TurnRect(%d) = %v, want %v", tt.deg, got, tt.want)
		}
	}
	// Off center, a quarter turn carries the corner around the point
	if got := TurnRect(image.Rect(10, 0, 20, 5), 0, 0, 90); got != image.Rect(-5, 10, 0, 20) {
		t.Errorf("TurnRect about the origin = %v", got)
	}
}

func TestDividerKeepsItsPlaceWhenItsPartIsCut(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 10))
	for y := 4; y < 6; y++ {
		for x := 10; x < 30; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	p := NewMemoryAssets(nil, map[string][]byte{"d.png": buf.Bytes()})
	// The image is the part cut at x 100, so its strip starts 10 in from there
	layer, err := Divider(p, "d.png", image.Pt(100, 0), 50, 1)
	if err != nil || layer == nil {
		t.Fatal(layer, err)
	}
	if layer.Origin.X != 110 {
		t.Errorf("divider starts at x %d, want 110", layer.Origin.X)
	}
}
