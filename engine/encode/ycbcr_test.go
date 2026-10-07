package encode

import (
	"bytes"
	"image/color"
	"image/jpeg"
	"math"
	"testing"

	"github.com/odevine/impasto/raster"
)

// gradient fills a buffer with a smooth ramp in all three channels
func gradient(w, h int) *raster.Buffer {
	buf := raster.MustNewBuffer(w, h)
	for y := range h {
		for x := range w {
			buf.Set(x, y, float32(x)/float32(w), float32(y)/float32(h), float32(x+y)/float32(w+h), 1)
		}
	}
	return buf
}

func TestToYCbCrMatchesStdlibConversion(t *testing.T) {
	// Even and odd sizes, since an edge block averages fewer pixels
	for _, size := range [][2]int{{64, 48}, {63, 47}, {1, 1}, {2, 1}, {1, 2}} {
		buf := gradient(size[0], size[1])
		got := ToYCbCr(buf)
		ref := buf.ToImage(8)
		for y := range size[1] {
			for x := range size[0] {
				r, g, b, _ := ref.At(x, y).RGBA()
				want, _, _ := color.RGBToYCbCr(uint8(r>>8), uint8(g>>8), uint8(b>>8))
				if d := int(got.Y[y*got.YStride+x]) - int(want); d < -2 || d > 2 {
					t.Fatalf("%dx%d: Y at (%d,%d) = %d, want %d", size[0], size[1], x, y, got.Y[y*got.YStride+x], want)
				}
			}
		}
	}
}

func TestToYCbCrPrimaries(t *testing.T) {
	cases := []struct {
		name    string
		r, g, b float32
	}{
		{"black", 0, 0, 0}, {"white", 1, 1, 1}, {"red", 1, 0, 0}, {"blue", 0, 0, 1}, {"green", 0, 1, 0},
	}
	for _, c := range cases {
		buf := raster.MustNewBuffer(2, 2)
		for y := range 2 {
			for x := range 2 {
				buf.Set(x, y, c.r, c.g, c.b, 1)
			}
		}
		got := ToYCbCr(buf)
		wy, wcb, wcr := color.RGBToYCbCr(uint8(code(c.r)), uint8(code(c.g)), uint8(code(c.b)))
		if got.Y[0] != wy || got.Cb[0] != wcb || got.Cr[0] != wcr {
			t.Errorf("%s: got %d %d %d, want %d %d %d", c.name, got.Y[0], got.Cb[0], got.Cr[0], wy, wcb, wcr)
		}
	}
}

func TestToYCbCrEncodesAsJPEG(t *testing.T) {
	buf := gradient(100, 70)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, ToYCbCr(buf), &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	dec, err := jpeg.Decode(&out)
	if err != nil {
		t.Fatal(err)
	}
	ref := buf.ToImage(8)
	var sum float64
	for y := range 70 {
		for x := range 100 {
			r1, g1, b1, _ := dec.At(x, y).RGBA()
			r2, g2, b2, _ := ref.At(x, y).RGBA()
			for _, d := range []float64{float64(r1) - float64(r2), float64(g1) - float64(g2), float64(b1) - float64(b2)} {
				sum += d * d / (257 * 257)
			}
		}
	}
	mse := sum / float64(100*70*3)
	if psnr := 10 * math.Log10(255*255/max(mse, 1e-9)); psnr < 38 {
		t.Errorf("PSNR against the 8-bit render = %.1f dB, want at least 38", psnr)
	}
}
