package template

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"testing"

	"github.com/odevine/impasto/blend"

	"github.com/odevine/mimic/engine/manifest"
)

// solidPNG is a w by h image of one color
func solidPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pairAssets(t *testing.T, w, h int, left, right color.NRGBA) *memAssets {
	t.Helper()
	return &memAssets{files: map[string][]byte{"l.png": solidPNG(t, w, h, left), "r.png": solidPNG(t, w, h, right)}}
}

// pairBlender is a smooth pair on a document docW wide, with a seam of the given
// center and width, that ends at end of the right color
func pairBlender(t *testing.T, docW int, center, width, end float64) Blender {
	t.Helper()
	b := &manifest.Blend{End: end, Seams: map[string][]manifest.Seam{"2": {{Center: center, Width: width}}}}
	bl, ok := NewBlender(2, b, docW)
	if !ok {
		t.Fatal("no blender")
	}
	return bl
}

func nrgbaAt(t *testing.T, img image.Image, x, y int) color.NRGBA {
	t.Helper()
	n, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("layer is %T, want an *image.NRGBA", img)
	}
	return n.NRGBAAt(x, y)
}

// Colors mix as they are stored, the way an image editor mixes a gradient. In linear
// light, the same half and half mix of red and blue would be a brighter 188
func TestBlendedImageMixesInGammaSpace(t *testing.T) {
	p := pairAssets(t, 200, 10, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	bl := pairBlender(t, 200, 0, 100, 0)
	img, at, err := LoadBlendedImage(p, []string{"l.png", "r.png"}, bl, 1, 0, 0, 200, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if at != (image.Point{}) || img.Bounds().Dx() != 200 {
		t.Fatalf("layer at %v is %v", at, img.Bounds())
	}
	if got := nrgbaAt(t, img, 100, 5); got.R < 126 || got.R > 129 || got.B < 126 || got.B > 129 || got.G != 0 || got.A != 255 {
		t.Errorf("the middle = %v, want an even mix near (128, 0, 128)", got)
	}
	if got := nrgbaAt(t, img, 10, 5); got != (color.NRGBA{R: 255, A: 255}) {
		t.Errorf("left of the seam = %v, want the left color", got)
	}
	if got := nrgbaAt(t, img, 190, 5); got != (color.NRGBA{B: 255, A: 255}) {
		t.Errorf("right of the seam = %v, want the right color", got)
	}
}

func TestBlendedImageEndLeavesTheRestOnTheLeftColor(t *testing.T) {
	p := pairAssets(t, 200, 4, color.NRGBA{R: 200, A: 255}, color.NRGBA{B: 200, A: 255})
	bl := pairBlender(t, 200, 0, 100, 0.94)
	img, _, err := LoadBlendedImage(p, []string{"l.png", "r.png"}, bl, 1, 0, 0, 200, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	got := nrgbaAt(t, img, 199, 1)
	// 0.06 of 200 red and 0.94 of 200 blue
	if got.R < 11 || got.R > 13 || got.B < 187 || got.B > 189 {
		t.Errorf("the far right = %v, want 0.94 of the right color over 0.06 of the left", got)
	}
}

// A mix is premultiplied, so a color that is transparent where the other is
// opaque takes only its share of the alpha and does not darken the other
func TestBlendedImageKeepsColorWhereOneLayerIsTransparent(t *testing.T) {
	p := pairAssets(t, 200, 4, color.NRGBA{A: 0}, color.NRGBA{B: 255, A: 255})
	bl := pairBlender(t, 200, 0, 100, 0)
	img, _, err := LoadBlendedImage(p, []string{"l.png", "r.png"}, bl, 1, 0, 0, 200, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	got := nrgbaAt(t, img, 100, 1)
	if got.A < 126 || got.A > 129 || got.B != 255 || got.R != 0 {
		t.Errorf("the middle = %v, want half transparent blue and not a darker blue", got)
	}
	if got := nrgbaAt(t, img, 10, 1); got.A != 0 {
		t.Errorf("where both are clear or only the left shows = %v, want transparent", got)
	}
}

func TestBlendedImageMixesPartlyTransparentPixels(t *testing.T) {
	p := pairAssets(t, 200, 4, color.NRGBA{R: 255, A: 128}, color.NRGBA{R: 255, A: 128})
	bl := pairBlender(t, 200, 0, 100, 0)
	img, _, err := LoadBlendedImage(p, []string{"l.png", "r.png"}, bl, 1, 0, 0, 200, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := nrgbaAt(t, img, 100, 1); got.R != 255 || got.A != 128 {
		t.Errorf("two equal half transparent reds mix to %v, want the same", got)
	}
}

// A layer cut smaller than the document is mixed by the columns it covers there
func TestBlendedImageWeighsACutLayerByItsPlaceInTheDocument(t *testing.T) {
	// The cut sits at x 150 to 250 of a 400 wide document, and the seam covers 150 to
	// 250 too, which is 100 wide about the middle
	p := pairAssets(t, 100, 4, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	bl := pairBlender(t, 400, 0, 100, 0)
	img, at, err := LoadBlendedImage(p, []string{"l.png", "r.png"}, bl, 1, 150, 20, 400, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if at != image.Pt(150, 20) {
		t.Errorf("origin = %v, want 150,20", at)
	}
	if got := nrgbaAt(t, img, 1, 1); got.R < 250 {
		t.Errorf("the cut's left edge = %v, want nearly all left color", got)
	}
	if got := nrgbaAt(t, img, 98, 1); got.B < 250 {
		t.Errorf("the cut's right edge = %v, want nearly all right color", got)
	}
	if got := nrgbaAt(t, img, 50, 1); got.R < 120 || got.R > 135 || got.B < 120 || got.B > 135 {
		t.Errorf("the cut's middle = %v, want an even mix", got)
	}
}

func TestBlendedImageAtAReducedScaleKeepsTheSameMix(t *testing.T) {
	p := pairAssets(t, 200, 20, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	bl := pairBlender(t, 200, 0, 100, 0)
	img, _, err := LoadBlendedImage(p, []string{"l.png", "r.png"}, bl, 0.5, 0, 0, 200, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 100 || b.Dy() != 10 {
		t.Fatalf("scaled layer is %v, want 100 by 10", b)
	}
	// Scaling averages in premultiplied color, so a flat color stays flat
	got := nrgbaAt(t, img, 50, 5)
	if got.R < 120 || got.R > 135 || got.B < 120 || got.B > 135 {
		t.Errorf("the scaled middle = %v, want an even mix", got)
	}
}

func TestBlendedImageRefusesLayersOfDifferentSizes(t *testing.T) {
	p := &memAssets{files: map[string][]byte{
		"l.png": solidPNG(t, 100, 10, color.NRGBA{A: 255}),
		"r.png": solidPNG(t, 100, 12, color.NRGBA{A: 255}),
	}}
	if _, _, err := LoadBlendedImage(p, []string{"l.png", "r.png"}, pairBlender(t, 100, 0, 50, 0), 1, 0, 0, 100, 12, false); err == nil {
		t.Error("layers of different sizes were mixed")
	}
}

func TestBlendedImageReportsAMissingLayer(t *testing.T) {
	p := &memAssets{files: map[string][]byte{"l.png": solidPNG(t, 10, 10, color.NRGBA{A: 255})}}
	if _, _, err := LoadBlendedImage(p, []string{"l.png", "gone.png"}, pairBlender(t, 10, 0, 5, 0), 1, 0, 0, 10, 10, false); err == nil {
		t.Error("a missing layer did not fail the blend")
	}
}

func TestBlendedImageMixesThreeAndFourColors(t *testing.T) {
	files := map[string][]byte{}
	for name, c := range map[string]color.NRGBA{
		"a.png": {R: 255, A: 255}, "b.png": {G: 255, A: 255}, "c.png": {B: 255, A: 255}, "d.png": {R: 255, G: 255, A: 255},
	} {
		files[name] = solidPNG(t, 400, 4, c)
	}
	p := &memAssets{files: files}
	bl3, _ := NewBlender(3, &manifest.Blend{Shape: "linear", Seams: map[string][]manifest.Seam{"3": {{Center: -100, Width: 40}, {Center: 100, Width: 40}}}}, 400)
	img, _, err := LoadBlendedImage(p, []string{"a.png", "b.png", "c.png"}, bl3, 1, 0, 0, 400, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := nrgbaAt(t, img, 20, 1); got != (color.NRGBA{R: 255, A: 255}) {
		t.Errorf("first third = %v, want red", got)
	}
	if got := nrgbaAt(t, img, 200, 1); got != (color.NRGBA{G: 255, A: 255}) {
		t.Errorf("middle third = %v, want green", got)
	}
	if got := nrgbaAt(t, img, 380, 1); got != (color.NRGBA{B: 255, A: 255}) {
		t.Errorf("last third = %v, want blue", got)
	}
	bl4, ok := NewBlender(4, &manifest.Blend{Shape: "linear", Seams: map[string][]manifest.Seam{
		"4": {{Center: -150, Width: 40}, {Center: 0, Width: 40}, {Center: 150, Width: 40}},
	}}, 400)
	if !ok {
		t.Fatal("no four color blender")
	}
	img, _, err = LoadBlendedImage(p, []string{"a.png", "b.png", "c.png", "d.png"}, bl4, 1, 0, 0, 400, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := nrgbaAt(t, img, 390, 1); got != (color.NRGBA{R: 255, G: 255, A: 255}) {
		t.Errorf("last quarter = %v, want yellow", got)
	}
}

// With a cache behind it, a card that asks for the blend again is handed the layer
// that was built, and the two mono layers are decoded once
func TestACachedBlendIsBuiltOnce(t *testing.T) {
	m := pairAssets(t, 100, 10, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	c := NewCachedAssets(m, 1<<30)
	bl := pairBlender(t, 100, 0, 50, 0)
	paths := []string{"l.png", "r.png"}

	first, at1, err := LoadBlendedImage(c, paths, bl, 1, 0, 0, 100, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	second, at2, err := LoadBlendedImage(c, paths, bl, 1, 0, 0, 100, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || at1 != at2 {
		t.Error("the second ask built the layer again")
	}
	if opens := m.opens.Load(); opens != 2 {
		t.Errorf("the layers were opened %d times, want each once", opens)
	}
	if s := c.Stats(); s.Misses != 3 || s.Hits != 1 || s.Layers != 3 {
		t.Errorf("stats = %+v, want two decodes and one build, then one hit", s)
	}
}

func TestACachedBlendIsKeyedByWhatItIsBuiltFrom(t *testing.T) {
	m := pairAssets(t, 100, 10, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	c := NewCachedAssets(m, 1<<30)
	base := pairBlender(t, 100, 0, 50, 0)
	load := func(paths []string, bl Blender, s Scale) image.Image {
		t.Helper()
		img, _, err := LoadBlendedImage(c, paths, bl, s, 0, 0, 100, 10, false)
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	ref := load([]string{"l.png", "r.png"}, base, 1)
	if load([]string{"r.png", "l.png"}, base, 1) == ref {
		t.Error("the same colors the other way round shared a layer")
	}
	if load([]string{"l.png", "r.png"}, pairBlender(t, 100, 10, 50, 0), 1) == ref {
		t.Error("a seam in another place shared a layer")
	}
	if load([]string{"l.png", "r.png"}, pairBlender(t, 100, 0, 50, 0.9), 1) == ref {
		t.Error("another end shared a layer")
	}
	if load([]string{"l.png", "r.png"}, base, 0.5) == ref {
		t.Error("another scale shared a layer")
	}
	if load([]string{"l.png", "r.png"}, base, 1) != ref {
		t.Error("the same ask no longer finds its layer")
	}
}

func TestACachedBlendComesBackIndexedWhenAskedFor(t *testing.T) {
	c := NewCachedAssets(pairAssets(t, 100, 10, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}), 1<<30)
	bl := pairBlender(t, 100, 0, 50, 0)
	img, _, err := LoadBlendedImage(c, []string{"l.png", "r.png"}, bl, 1, 0, 0, 100, 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*blend.Indexed); !ok {
		t.Errorf("an indexed ask gave %T, want a *blend.Indexed", img)
	}
	plain, _, err := LoadBlendedImage(c, []string{"l.png", "r.png"}, bl, 1, 0, 0, 100, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain.(*image.NRGBA); !ok {
		t.Errorf("a plain ask gave %T, want an *image.NRGBA", plain)
	}
}

func TestConcurrentAsksForOneBlendShareASingleBuild(t *testing.T) {
	m := pairAssets(t, 400, 40, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	c := NewCachedAssets(m, 1<<30)
	bl := pairBlender(t, 400, 0, 100, 0)
	var wg sync.WaitGroup
	got := make([]image.Image, 8)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			img, _, err := LoadBlendedImage(c, []string{"l.png", "r.png"}, bl, 1, 0, 0, 400, 40, false)
			if err != nil {
				t.Error(err)
			}
			got[i] = img
		}()
	}
	wg.Wait()
	for i := 1; i < len(got); i++ {
		if got[i] != got[0] {
			t.Fatalf("caller %d was handed another layer", i)
		}
	}
	if s := c.Stats(); s.Misses != 3 {
		t.Errorf("misses = %d, want two decodes and one build", s.Misses)
	}
}

func TestABlendTooBigForTheCacheIsStillBuiltCorrectly(t *testing.T) {
	m := pairAssets(t, 100, 10, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255})
	c := NewCachedAssets(m, 4)
	bl := pairBlender(t, 100, 0, 50, 0)
	img, _, err := LoadBlendedImage(c, []string{"l.png", "r.png"}, bl, 1, 0, 0, 100, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := nrgbaAt(t, img, 5, 5); got != (color.NRGBA{R: 255, A: 255}) {
		t.Errorf("left = %v, want the left color", got)
	}
	if s := c.Stats(); s.Layers != 0 || s.Bytes != 0 {
		t.Errorf("a layer bigger than the budget was kept: %+v", s)
	}
	off := NewCachedAssets(pairAssets(t, 100, 10, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}), 0)
	a, _, _ := LoadBlendedImage(off, []string{"l.png", "r.png"}, bl, 1, 0, 0, 100, 10, false)
	b, _, _ := LoadBlendedImage(off, []string{"l.png", "r.png"}, bl, 1, 0, 0, 100, 10, false)
	if a == nil || a == b {
		t.Error("a cache with no budget kept a layer")
	}
}

// A scaled render drops a layer that lands off the document, as it does for any layer
func TestABlendThatLandsOnNoPixelIsNil(t *testing.T) {
	p := pairAssets(t, 10, 10, color.NRGBA{A: 255}, color.NRGBA{A: 255})
	img, _, err := LoadBlendedImage(p, []string{"l.png", "r.png"}, pairBlender(t, 100, 0, 50, 0), 0.5, 500, 500, 100, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if img != nil {
		t.Errorf("a layer off the document is %v, want nil", img.Bounds())
	}
}

func TestBlenderShiftedMovesItsSeams(t *testing.T) {
	bl, ok := NewBlender(2, &manifest.Blend{Shape: "linear"}, 1000)
	if !ok {
		t.Fatal("no blender")
	}
	moved := bl.Shifted(200, 1000)
	var a, b [2]float64
	for _, f := range []float64{0.1, 0.45, 0.5, 0.62, 0.9} {
		bl.Weights(f, a[:])
		moved.Weights(f+0.2, b[:])
		if math.Abs(a[0]-b[0]) > 1e-9 {
			t.Errorf("weights at %v = %v, shifted at %v = %v", f, a, f+0.2, b)
		}
	}
	if same := bl.Shifted(0, 1000); same.seams[0] != bl.seams[0] {
		t.Error("a shift of zero moved the seams")
	}
}
