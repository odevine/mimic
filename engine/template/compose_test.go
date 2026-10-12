package template

import (
	"image"
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
