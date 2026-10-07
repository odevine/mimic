package template

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"

	"github.com/odevine/impasto/blend"
	"github.com/odevine/impasto/raster"
)

// frameWithHole is a w by h opaque frame of the given thickness, transparent
// inside, encoded as a PNG
func frameWithHole(t *testing.T, w, h, thickness int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < thickness || y < thickness || x >= w-thickness || y >= h-thickness {
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 5), G: uint8(y * 3), B: 90, A: 255})
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func grayPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func layerAssets(t *testing.T) *memAssets {
	m := &memAssets{files: map[string][]byte{}}
	m.files["frame.png"] = frameWithHole(t, 120, 90, 12)
	m.files["gray.png"] = grayPNG(t)
	return m
}

func TestLoadIndexedReturnsTheImageWithItsIndex(t *testing.T) {
	c := NewCachedAssets(layerAssets(t), 1<<20)
	got, err := c.LoadIndexed("frame.png")
	if err != nil {
		t.Fatal(err)
	}
	ix, ok := got.(*blend.Indexed)
	if !ok {
		t.Fatalf("LoadIndexed gave %T, want *blend.Indexed", got)
	}
	plain, _ := c.LoadImage("frame.png")
	if ix.NRGBA != plain {
		t.Error("the indexed image is not the cached image")
	}
	again, _ := c.LoadIndexed("frame.png")
	if again != got {
		t.Error("the index was built again for a layer that was still cached")
	}
	if opens := c.AssetProvider.(*memAssets).opens.Load(); opens != 1 {
		t.Errorf("the layer was decoded %d times, want 1", opens)
	}
}

func TestLoadIndexedFallsBackToThePlainImage(t *testing.T) {
	m := layerAssets(t)
	// Not an NRGBA image, so there is nothing to index
	c := NewCachedAssets(m, 1<<20)
	got, err := c.LoadIndexed("gray.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(*image.Gray); !ok {
		t.Errorf("a gray layer came back as %T, want *image.Gray", got)
	}
	// No cache to keep an index in
	off := NewCachedAssets(layerAssets(t), 0)
	got, _ = off.LoadIndexed("frame.png")
	if _, ok := got.(*image.NRGBA); !ok {
		t.Errorf("with the cache off a layer came back as %T, want *image.NRGBA", got)
	}
	// Too big to keep
	small := NewCachedAssets(layerAssets(t), 100)
	got, _ = small.LoadIndexed("frame.png")
	if _, ok := got.(*image.NRGBA); !ok {
		t.Errorf("a layer over the budget came back as %T, want *image.NRGBA", got)
	}
	if _, err := c.LoadIndexed("missing.png"); err == nil {
		t.Error("a missing layer loaded")
	}
}

func TestLoadIndexedLosesTheIndexWithTheLayer(t *testing.T) {
	c := NewCachedAssets(layerAssets(t), 1<<20)
	first, _ := c.LoadIndexed("frame.png")
	c.SetBudget(0)
	c.SetBudget(1 << 20)
	second, _ := c.LoadIndexed("frame.png")
	if first == second {
		t.Error("an index outlived the layer it described")
	}
	if _, ok := second.(*blend.Indexed); !ok {
		t.Errorf("a layer cached again came back as %T", second)
	}
}

func TestLoadIndexedIsSafeForConcurrentUse(t *testing.T) {
	c := NewCachedAssets(layerAssets(t), 1<<20)
	var wg sync.WaitGroup
	results := make([]image.Image, 16)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = c.LoadIndexed("frame.png")
		}()
	}
	wg.Wait()
	for i, got := range results {
		if got != results[0] {
			t.Fatalf("goroutine %d got a different index", i)
		}
	}
}

func TestLoadIndexedImageUsesAProviderThatHasOne(t *testing.T) {
	m := layerAssets(t)
	got, err := LoadIndexedImage(m, "frame.png") // memAssets has no index of its own
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(*image.NRGBA); !ok {
		t.Errorf("a provider with no index gave %T, want *image.NRGBA", got)
	}
	cached, err := LoadIndexedImage(NewCachedAssets(m, 1<<20), "frame.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cached.(*blend.Indexed); !ok {
		t.Errorf("a cached provider gave %T, want *blend.Indexed", cached)
	}
}

// What LoadLayerImage hands the compositor must be what LoadLayerAt would have
// turned into a buffer, at native size and scaled, positioned or not
func TestLoadLayerImageMatchesLoadLayerAt(t *testing.T) {
	const docW, docH = 300, 220
	for _, tc := range []struct {
		name string
		s    Scale
		x, y int
	}{
		{"native at the origin", 1, 0, 0},
		{"native placed", 1, 40, 25},
		{"half size placed", 0.5, 40, 25},
		{"odd scale placed", 0.37, 41, 26},
	} {
		for _, cache := range []bool{false, true} {
			var p AssetProvider = layerAssets(t)
			if cache {
				p = NewCachedAssets(p, 1<<20)
			}
			img, at, err := LoadLayerImage(p, "frame.png", tc.s, tc.x, tc.y, docW, docH)
			if err != nil {
				t.Fatal(err)
			}
			want, wantAt, err := LoadLayerAt(p, "frame.png", tc.s, tc.x, tc.y, docW, docH)
			if err != nil {
				t.Fatal(err)
			}
			if at != wantAt {
				t.Errorf("%s (cache %v): origin %v, want %v", tc.name, cache, at, wantAt)
			}
			// An indexed image is an NRGBA underneath, which FromImage reads directly
			if ix, ok := img.(*blend.Indexed); ok {
				img = ix.NRGBA
			}
			got, err := raster.FromImage(img)
			if err != nil {
				t.Fatal(err)
			}
			if !got.SameSize(want) {
				t.Fatalf("%s (cache %v): %dx%d, want %dx%d", tc.name, cache, got.Width, got.Height, want.Width, want.Height)
			}
			for i := range want.Pix {
				if got.Pix[i] != want.Pix[i] {
					t.Fatalf("%s (cache %v): value %d differs", tc.name, cache, i)
				}
			}
		}
	}
}

func TestLoadLayerImageNativeKeepsTheIndexAndScaledDoesNot(t *testing.T) {
	c := NewCachedAssets(layerAssets(t), 1<<20)
	native, _, _ := LoadLayerImage(c, "frame.png", 1, 0, 0, 120, 90)
	if _, ok := native.(*blend.Indexed); !ok {
		t.Errorf("native scale gave %T, want *blend.Indexed", native)
	}
	scaled, _, _ := LoadLayerImage(c, "frame.png", 0.5, 0, 0, 120, 90)
	if _, ok := scaled.(*image.RGBA); !ok {
		t.Errorf("a scaled layer gave %T, want *image.RGBA", scaled)
	}
}

func TestLoadLayerImageTakesTheImagesOwnSizeForTheDocument(t *testing.T) {
	p := layerAssets(t)
	a, atA, err := LoadLayerImage(p, "frame.png", 0.5, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, atB, _ := LoadLayerImage(p, "frame.png", 0.5, 0, 0, 120, 90)
	if atA != atB || a.Bounds() != b.Bounds() {
		t.Errorf("a zero document size gave %v at %v, the image's own gave %v at %v", a.Bounds(), atA, b.Bounds(), atB)
	}
}

func TestLoadLayerImageOffTheDocumentIsNil(t *testing.T) {
	p := layerAssets(t)
	img, _, err := LoadLayerImage(p, "frame.png", 0.5, 1000, 1000, 120, 90)
	if err != nil || img != nil {
		t.Errorf("a layer past the document gave %v, %v, want nil and no error", img, err)
	}
	buf, _, err := LoadLayerAt(p, "frame.png", 0.5, 1000, 1000, 120, 90)
	if err != nil || buf != nil {
		t.Errorf("LoadLayerAt for a layer past the document gave %v, %v, want nil and no error", buf, err)
	}
}

func TestLoadLayerImageReportsAMissingLayer(t *testing.T) {
	p := layerAssets(t)
	if _, _, err := LoadLayerImage(p, "nope.png", 1, 0, 0, 10, 10); err == nil {
		t.Error("a missing layer loaded as an image")
	}
	if _, _, err := LoadLayerAt(p, "nope.png", 1, 0, 0, 10, 10); err == nil {
		t.Error("a missing layer loaded as a buffer")
	}
}
