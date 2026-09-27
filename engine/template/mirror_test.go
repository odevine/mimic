package template

import (
	"image"
	"testing"

	"github.com/odevine/impasto/raster"
)

func TestMirror(t *testing.T) {
	mark := func() *raster.Buffer {
		buf := raster.MustNewBuffer(10, 4)
		for y := range 4 {
			i := (y*10 + 1) * 4
			buf.Pix[i+3] = float32(y + 1)
		}
		return buf
	}
	at := func(buf *raster.Buffer, x, y int) float32 { return buf.Pix[(y*10+x)*4+3] }

	whole := mark()
	Mirror(whole, image.Rectangle{})
	for y := range 4 {
		if at(whole, 1, y) != 0 || at(whole, 8, y) != float32(y+1) {
			t.Errorf("whole flip row %d: left %v right %v", y, at(whole, 1, y), at(whole, 8, y))
		}
	}

	// Rows 1 and 2 only, and a region past the center stops at it
	band := mark()
	Mirror(band, image.Rect(0, 1, 9, 3))
	for y, flipped := range []bool{false, true, true, false} {
		if got := at(band, 8, y) != 0; got != flipped {
			t.Errorf("band flip row %d: flipped %v, want %v", y, got, flipped)
		}
	}
}
