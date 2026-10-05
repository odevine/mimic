package template

import (
	"image"
	"image/color"
	"testing"

	"github.com/odevine/impasto/raster"
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

func TestTurnedManifestSizes(t *testing.T) {
	m := &Manifest{Width: 4440, Height: 3264, DPI: 1200, Rotate: 270}
	if m.DeliveredWidth() != 3264 || m.DeliveredHeight() != 4440 {
		t.Errorf("delivered %dx%d, want 3264x4440", m.DeliveredWidth(), m.DeliveredHeight())
	}
	got := m.Resolution(300)
	if got.DPI != 300 || got.Width != 816 || got.Height != 1110 {
		t.Errorf("Resolution(300) = %+v, want 816x1110 at 300 dpi", got)
	}
	if native := (&Manifest{Width: 4440, Height: 3264, Rotate: 270}).NativeDPI(); native != 1200 {
		t.Errorf("an unstated dpi infers %d from the delivered width, want 1200", native)
	}

	scaled := (&Manifest{Width: 400, Height: 200, Arts: []ArtSlot{{X: 10, Y: 20, Width: 100, Height: 50, After: "bg"}}}).Scaled(0.5)
	if len(scaled.Arts) != 1 || scaled.Arts[0] != (ArtSlot{X: 5, Y: 10, Width: 50, Height: 25, After: "bg"}) {
		t.Errorf("scaled arts = %+v", scaled.Arts)
	}
}

func TestValidateManifestSplitFields(t *testing.T) {
	ok := Manifest{Width: 10, Height: 10}
	if err := validateManifest(&ok); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Manifest{
		"rotate":     {Width: 10, Height: 10, Rotate: 45},
		"negative":   {Width: 10, Height: 10, Rotate: -90},
		"layer half": {Width: 10, Height: 10, Layers: []LayerSpec{{Name: "l", Half: 3}}},
		"box half":   {Width: 10, Height: 10, TextBoxes: map[string]TextBoxSpec{"b": {Half: -1}}},
		"box space":  {Width: 10, Height: 10, TextBoxes: map[string]TextBoxSpec{"b": {Space: "sideways"}}},
	}
	for name, m := range bad {
		if err := validateManifest(&m); err == nil {
			t.Errorf("%s should be rejected", name)
		}
	}
	good := Manifest{Width: 10, Height: 10, Rotate: 270, TextBoxes: map[string]TextBoxSpec{"b": {Half: 2, Space: SpaceOutput}}}
	if err := validateManifest(&good); err != nil {
		t.Errorf("a split manifest was rejected: %v", err)
	}
}

func TestBlendWeights(t *testing.T) {
	w := make([]float64, 3)
	cases := []struct {
		n    int
		t    float64
		want []float64
	}{
		{2, 0.20, []float64{1, 0}},
		{2, 0.52, []float64{0.5, 0.5}},
		{2, 0.80, []float64{0, 1}},
		{3, 0.10, []float64{1, 0, 0}},
		{3, 0.50, []float64{0, 1, 0}},
		{3, 0.735, []float64{0, 0.5, 0.5}},
		{3, 0.95, []float64{0, 0, 1}},
	}
	for _, c := range cases {
		blendWeights(BlendStops(c.n), c.n, c.t, w[:c.n])
		for i, want := range c.want {
			if got := w[i]; got < want-1e-9 || got > want+1e-9 {
				t.Errorf("%d colors at %.3f: weights %v, want %v", c.n, c.t, w[:c.n], c.want)
				break
			}
		}
	}
	if BlendStops(1) != nil || BlendStops(5) != nil {
		t.Error("only two to four colors have seams")
	}
}

func TestBlendedPaths(t *testing.T) {
	v := map[string]LayerAsset{"r": {Path: "r.png"}, "w": {Path: "w.png"}, "gold": {Path: "g.png"}}
	if got := BlendedPaths(v, "rw"); len(got) != 2 || got[0] != "r.png" || got[1] != "w.png" {
		t.Errorf("rw blends %v", got)
	}
	for _, key := range []string{"", "r", "ru", "gold"} {
		if got := BlendedPaths(v, key); got != nil {
			t.Errorf("%q blends %v, want nothing", key, got)
		}
	}
}
