package template

import (
	"image"
	"math/rand/v2"
	"testing"
)

// sparse fills a rectangle of img's pixel bytes with a few random opaque
// pixels, leaving the rest transparent
func sparse(pix []uint8, stride, bpp int, r image.Rectangle, rng *rand.Rand) {
	n := rng.IntN(6)
	for range n {
		x, y := rng.IntN(r.Dx()), rng.IntN(r.Dy())
		off := y*stride + x*bpp
		for c := 0; c < bpp; c++ {
			pix[off+c] = uint8(1 + rng.IntN(255))
		}
	}
}

func TestOpaqueBoundsMatchesAt(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for i := 0; i < 300; i++ {
		r := image.Rect(rng.IntN(5), rng.IntN(5), 5+rng.IntN(40), 5+rng.IntN(40))
		nrgba := image.NewNRGBA(r)
		rgba := image.NewRGBA(r)
		nrgba64 := image.NewNRGBA64(r)
		sparse(nrgba.Pix, nrgba.Stride, 4, r, rng)
		sparse(rgba.Pix, rgba.Stride, 4, r, rng)
		sparse(nrgba64.Pix, nrgba64.Stride, 8, r, rng)
		for _, img := range []image.Image{nrgba, rgba, nrgba64} {
			if got, want := OpaqueBounds(img), opaqueBoundsAt(img); got != want {
				t.Fatalf("%T %v: got %v, want %v", img, r, got, want)
			}
		}
	}
}

// A single set alpha byte decides visibility on its own, whichever byte of a
// 16-bit alpha it is
func TestOpaqueBoundsReadsEveryAlphaByte(t *testing.T) {
	img := image.NewNRGBA64(image.Rect(0, 0, 4, 3))
	img.Pix[2*img.Stride+2*8+6] = 1
	if got, want := OpaqueBounds(img), image.Rect(2, 2, 3, 3); got != want {
		t.Errorf("high byte: got %v, want %v", got, want)
	}
	img.Pix[2*img.Stride+2*8+6] = 0
	img.Pix[1*img.Stride+1*8+7] = 1
	if got, want := OpaqueBounds(img), image.Rect(1, 1, 2, 2); got != want {
		t.Errorf("low byte: got %v, want %v", got, want)
	}
}

func TestOpaqueBoundsOfSubImageAndEmpty(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	img.Pix[7*img.Stride+9*4+3] = 255
	if got, want := OpaqueBounds(img.SubImage(image.Rect(5, 5, 15, 15))), image.Rect(9, 7, 10, 8); got != want {
		t.Errorf("sub-image: got %v, want %v", got, want)
	}
	if got := OpaqueBounds(img.SubImage(image.Rect(0, 0, 4, 4))); !got.Empty() {
		t.Errorf("transparent region: got %v, want empty", got)
	}
	if got := OpaqueBounds(image.NewNRGBA(image.Rectangle{})); !got.Empty() {
		t.Errorf("zero size: got %v, want empty", got)
	}
}

func BenchmarkOpaqueBounds(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 3264, 4440))
	img.Pix[2000*img.Stride+1500*4+3] = 255
	for b.Loop() {
		OpaqueBounds(img)
	}
}

func BenchmarkOpaqueBoundsAt(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 3264, 4440))
	img.Pix[2000*img.Stride+1500*4+3] = 255
	for b.Loop() {
		opaqueBoundsAt(img)
	}
}
