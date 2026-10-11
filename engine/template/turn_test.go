package template

import (
	"image"
	"image/color"
	"strconv"
	"testing"

	"github.com/odevine/impasto/raster"
	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

// numbered is a w by h buffer whose pixel at (x, y) holds the red value y*w+x
func numbered(w, h int) *raster.Buffer {
	buf := raster.MustNewBuffer(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			buf.Set(x, y, float32(y*w+x), 0, 0, 1)
		}
	}
	return buf
}

func TestRotate(t *testing.T) {
	// The top-left pixel of a 3 by 2 buffer is 0 and its top-right is 2
	src := numbered(3, 2)
	cases := []struct {
		degrees    int
		w, h       int
		x, y       int // where the source's top-left pixel (0) lands
		topRightAt [2]int
	}{
		{90, 2, 3, 1, 0, [2]int{1, 2}},
		{180, 3, 2, 2, 1, [2]int{0, 1}},
		{270, 2, 3, 0, 2, [2]int{0, 0}},
		{-90, 2, 3, 0, 2, [2]int{0, 0}},
		{360, 3, 2, 0, 0, [2]int{2, 0}},
	}
	for _, c := range cases {
		got := Rotate(src, c.degrees)
		if got.Width != c.w || got.Height != c.h {
			t.Errorf("%d degrees: %dx%d, want %dx%d", c.degrees, got.Width, got.Height, c.w, c.h)
			continue
		}
		if r, _, _, _ := got.At(c.x, c.y); r != 0 {
			t.Errorf("%d degrees: the source's pixel 0 is not at (%d,%d)", c.degrees, c.x, c.y)
		}
		if r, _, _, _ := got.At(c.topRightAt[0], c.topRightAt[1]); r != 2 {
			t.Errorf("%d degrees: the source's top-right pixel is not at %v", c.degrees, c.topRightAt)
		}
	}
	if Rotate(src, 0) != src {
		t.Error("no turn should return the buffer itself")
	}
}

func TestHalfOfArt(t *testing.T) {
	art := image.NewNRGBA(image.Rect(0, 0, 20, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			art.SetNRGBA(x, y, color.NRGBA{R: uint8(x), A: 255})
		}
	}
	left, right := HalfOfArt(art, 0, 2), HalfOfArt(art, 1, 2)
	if left.Bounds().Dx() != 10 || right.Bounds().Dx() != 10 || left.Bounds().Dy() != 10 {
		t.Fatalf("halves are %v and %v", left.Bounds(), right.Bounds())
	}
	if c := color.NRGBAModel.Convert(left.At(left.Bounds().Min.X, 0)).(color.NRGBA); c.R != 0 {
		t.Errorf("the left half starts at column %d, want 0", c.R)
	}
	if c := color.NRGBAModel.Convert(right.At(right.Bounds().Min.X, 0)).(color.NRGBA); c.R != 10 {
		t.Errorf("the right half starts at column %d, want 10", c.R)
	}
	if HalfOfArt(art, 0, 1) != image.Image(art) {
		t.Error("one strip should be the whole image")
	}
}

func TestGeometryScalesEveryArtWindow(t *testing.T) {
	l := testLayout(t, manifest.Layout{
		Canvas: manifest.Canvas{Width: 400, Height: 200, DPI: 200},
		Art: []manifest.ArtWindow{
			{X: 10, Y: 20, Width: 100, Height: 50, After: "a"},
			{X: 210, Y: 20, Width: 100, Height: 50, After: "b"},
		},
	})
	art := l.Geometry(100).Art
	want := []ArtSlot{{X: 5, Y: 10, Width: 50, Height: 25, After: "a"}, {X: 105, Y: 10, Width: 50, Height: 25, After: "b"}}
	if len(art) != 2 || art[0] != want[0] || art[1] != want[1] {
		t.Errorf("scaled art = %+v, want %+v", art, want)
	}
}

// blenderFor is a blender with a seam at each of the fractions of a 1000 wide document
func blenderFor(t *testing.T, shape string, end float64, seams ...[2]float64) Blender {
	t.Helper()
	var ms []manifest.Seam
	for _, s := range seams {
		ms = append(ms, manifest.Seam{Center: (s[0]+s[1])/2*1000 - 500, Width: (s[1] - s[0]) * 1000})
	}
	b := &manifest.Blend{Shape: shape, End: end, Seams: map[string][]manifest.Seam{strconv.Itoa(len(seams) + 1): ms}}
	bl, ok := NewBlender(len(seams)+1, b, 1000)
	if !ok {
		t.Fatalf("no blender for %d seams", len(seams))
	}
	return bl
}

// The linear blender with the old fixed seams mixes the way the engine always has
func TestBlenderWeightsWithTheFuseBarSeams(t *testing.T) {
	two := blenderFor(t, "linear", 0, [2]float64{.50, .54})
	three := blenderFor(t, "linear", 0, [2]float64{.28, .33}, [2]float64{.71, .76})
	cases := []struct {
		bl   Blender
		n    int
		f    float64
		want []float64
	}{
		{two, 2, 0.20, []float64{1, 0}},
		{two, 2, 0.52, []float64{0.5, 0.5}},
		{two, 2, 0.80, []float64{0, 1}},
		{three, 3, 0.10, []float64{1, 0, 0}},
		{three, 3, 0.50, []float64{0, 1, 0}},
		{three, 3, 0.735, []float64{0, 0.5, 0.5}},
		{three, 3, 0.95, []float64{0, 0, 1}},
	}
	for _, c := range cases {
		w := make([]float64, c.n)
		c.bl.Weights(c.f, w)
		for i, want := range c.want {
			if w[i] < want-1e-9 || w[i] > want+1e-9 {
				t.Errorf("%d colors at %.3f: weights %v, want %v", c.n, c.f, w, c.want)
				break
			}
		}
	}
}

func TestBlenderWeightsAlwaysSumToOneAndNeverGoNegative(t *testing.T) {
	blenders := map[string]Blender{
		"smooth pair":  blenderFor(t, "smooth", 0, [2]float64{.33, .66}),
		"linear pair":  blenderFor(t, "linear", 0, [2]float64{.33, .66}),
		"short end":    blenderFor(t, "smooth", 0.94, [2]float64{.33, .66}),
		"three colors": blenderFor(t, "smooth", 0.8, [2]float64{.2, .3}, [2]float64{.6, .8}),
		"four colors":  blenderFor(t, "linear", 0, [2]float64{.2, .3}, [2]float64{.45, .55}, [2]float64{.7, .8}),
	}
	counts := map[string]int{"smooth pair": 2, "linear pair": 2, "short end": 2, "three colors": 3, "four colors": 4}
	for name, bl := range blenders {
		w := make([]float64, counts[name])
		for f := 0.0; f <= 1.0001; f += 0.01 {
			bl.Weights(f, w)
			sum := 0.0
			for _, v := range w {
				if v < -1e-12 {
					t.Fatalf("%s at %.2f: negative weight in %v", name, f, w)
				}
				sum += v
			}
			if sum < 1-1e-9 || sum > 1+1e-9 {
				t.Fatalf("%s at %.2f: weights %v sum to %v", name, f, w, sum)
			}
		}
	}
}

func TestBlenderSmoothRampMeetsInTheMiddleAndEasesAtTheEnds(t *testing.T) {
	smooth := blenderFor(t, "smooth", 0, [2]float64{.4, .6})
	linear := blenderFor(t, "linear", 0, [2]float64{.4, .6})
	w := make([]float64, 2)
	smooth.Weights(0.5, w)
	if w[0] < 0.5-1e-9 || w[0] > 0.5+1e-9 {
		t.Errorf("a smooth ramp at its center = %v, want an even mix", w)
	}
	// A quarter of the way in, smoothstep has come less far than a line has
	smooth.Weights(0.45, w)
	s := w[1]
	linear.Weights(0.45, w)
	if !(s < w[1]) {
		t.Errorf("smooth ramp %v a quarter in, linear %v, want smooth behind", s, w[1])
	}
}

func TestBlenderEndLeavesTheRestOnTheLeftColor(t *testing.T) {
	bl := blenderFor(t, "smooth", 0.94, [2]float64{.4, .6})
	w := make([]float64, 2)
	bl.Weights(1, w)
	if w[1] < 0.94-1e-9 || w[1] > 0.94+1e-9 || w[0] < 0.06-1e-9 || w[0] > 0.06+1e-9 {
		t.Errorf("weights at the far right = %v, want 0.06 and 0.94", w)
	}
	bl.Weights(0, w)
	if w[0] != 1 || w[1] != 0 {
		t.Errorf("weights at the far left = %v, want pure left color", w)
	}
}

func TestBlenderDefaultsForAPairAndRefusesWhatItHasNoSeamsFor(t *testing.T) {
	bl, ok := NewBlender(2, nil, 3264)
	if !ok {
		t.Fatal("a pair with no blend has no blender, want the default seam")
	}
	w := make([]float64, 2)
	bl.Weights(0.5, w)
	if w[0] < 0.5-1e-9 || w[0] > 0.5+1e-9 {
		t.Errorf("the default seam's center = %v, want an even mix in the middle", w)
	}
	// 650 pixels wide and centered, so it ends 325 pixels each side of the middle
	bl.Weights(0.5-325.0/3264-0.001, w)
	if w[1] != 0 {
		t.Errorf("just left of the default seam = %v, want pure left color", w)
	}
	bl.Weights(0.5+325.0/3264+0.001, w)
	if w[0] != 0 {
		t.Errorf("just right of the default seam = %v, want pure right color", w)
	}
	for _, n := range []int{0, 1, 3, 4, 5} {
		if _, ok := NewBlender(n, nil, 3264); ok {
			t.Errorf("a run of %d colors with no seams got a blender", n)
		}
	}
	if _, ok := NewBlender(2, nil, 0); ok {
		t.Error("a blender was made for a document with no width")
	}
	given := &manifest.Blend{Seams: map[string][]manifest.Seam{"3": {{Center: -300, Width: 100}, {Center: 300, Width: 100}}}}
	if _, ok := NewBlender(3, given, 3264); !ok {
		t.Error("a run with seams of its own length got no blender")
	}
	if _, ok := NewBlender(2, given, 3264); !ok {
		t.Error("a pair with seams for another length should still take the default")
	}
	short := &manifest.Blend{Seams: map[string][]manifest.Seam{"3": {{Center: 0, Width: 100}}}}
	if _, ok := NewBlender(3, short, 3264); ok {
		t.Error("a run with too few seams got a blender")
	}
}

func TestBlendedPaths(t *testing.T) {
	v := map[string]string{"r": "r.png", "w": "w.png", "gold": "g.png"}
	runs := map[string]frame.Material{
		"red then white":     frame.Run(card.Red, card.White),
		"red and white dual": frame.Dual([]card.Color{card.White, card.Red}),
	}
	for name, m := range runs {
		if got := BlendedPaths(v, m); len(got) != 2 || got[0] != "r.png" || got[1] != "w.png" {
			t.Errorf("%s blends %v, want r.png then w.png", name, got)
		}
	}
	none := map[string]frame.Material{
		"none":              {},
		"one color":         frame.Mono(card.Red),
		"a missing variant": frame.Run(card.Red, card.Blue),
		"gold":              frame.Gold,
	}
	for name, m := range none {
		if got := BlendedPaths(v, m); got != nil {
			t.Errorf("%s blends %v, want nothing", name, got)
		}
	}
}
