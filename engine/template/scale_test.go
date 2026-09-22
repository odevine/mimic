package template

import (
	"image"
	"image/color"
	"reflect"
	"testing"
)

// nativeManifest is a stand-in for the normal template's authored geometry: a
// 1200 dpi canvas with one text box and an art slot
func nativeManifest() *Manifest {
	pad := 12
	return &Manifest{
		Template: "test",
		Width:    3264,
		Height:   4440,
		Art:      ArtSlot{X: 356, Y: 607, Width: 2552, Height: 1856, After: "background"},
		TextBoxes: map[string]TextBoxSpec{
			"title": {
				X: 386, Y: 510, Width: 2400, Height: 200,
				FontSize: 156, MinFontSize: 120, Padding: 20, PaddingX: &pad,
				LineSpacing: 1.1, Tracking: 125, ClearGap: 0.5,
				Avoid: image.Rect(100, 200, 300, 400),
			},
		},
	}
}

func TestNativeDPI(t *testing.T) {
	m := nativeManifest()
	if got := m.NativeDPI(); got != 1200 {
		t.Errorf("inferred NativeDPI = %d, want 1200", got)
	}
	m.DPI = 800
	if got := m.NativeDPI(); got != 800 {
		t.Errorf("stated NativeDPI = %d, want 800", got)
	}
}

func TestClampDPI(t *testing.T) {
	m := nativeManifest()
	tests := []struct {
		name string
		dpi  int
		want int
	}{
		{"zero means native", 0, 1200},
		{"negative means native", -50, 1200},
		{"above native clamps down", 2400, 1200},
		{"below the floor clamps up", 10, MinDPI},
		{"in range passes through", 300, 300},
		{"native passes through", 1200, 1200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.ClampDPI(tt.dpi); got != tt.want {
				t.Errorf("ClampDPI(%d) = %d, want %d", tt.dpi, got, tt.want)
			}
		})
	}
}

func TestScaleForDPI(t *testing.T) {
	m := nativeManifest()
	if got := m.ScaleForDPI(300); got != 0.25 {
		t.Errorf("ScaleForDPI(300) = %v, want 0.25", got)
	}
	// Native and out-of-range both render at the authored size rather than
	// resampling assets up
	for _, dpi := range []int{0, 1200, 2400} {
		if got := m.ScaleForDPI(dpi); !got.Native() {
			t.Errorf("ScaleForDPI(%d) = %v, want native", dpi, got)
		}
	}
}

func TestResolution(t *testing.T) {
	m := nativeManifest()
	got := m.Resolution(300)
	want := Resolution{DPI: 300, Width: 816, Height: 1110}
	if got != want {
		t.Errorf("Resolution(300) = %+v, want %+v", got, want)
	}
	if got := m.Resolution(0); !got.Native || got.Width != 3264 || got.DPI != 1200 {
		t.Errorf("Resolution(0) = %+v, want the native 3264-wide 1200 dpi size", got)
	}
}

func TestPresets(t *testing.T) {
	m := nativeManifest()
	got := m.Presets()
	want := []Resolution{
		{DPI: 150, Width: 408, Height: 555},
		{DPI: 300, Width: 816, Height: 1110},
		{DPI: 600, Width: 1632, Height: 2220},
		{DPI: 1200, Width: 3264, Height: 4440, Native: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Presets() = %+v, want %+v", got, want)
	}

	// A template authored below a preset drops that preset rather than
	// offering an upscale, and always ends with its own resolution
	m.DPI = 400
	got = m.Presets()
	if len(got) != 3 {
		t.Fatalf("Presets() at 400 dpi = %+v, want 150, 300 and the native 400", got)
	}
	if last := got[len(got)-1]; last.DPI != 400 || !last.Native {
		t.Errorf("last preset = %+v, want the native 400 dpi entry", last)
	}
}

func TestScaledManifest(t *testing.T) {
	m := nativeManifest()
	s := m.ScaleForDPI(300)
	out := m.Scaled(s)

	if out.Width != 816 || out.Height != 1110 {
		t.Errorf("scaled canvas = %dx%d, want 816x1110", out.Width, out.Height)
	}
	if out.DPI != 300 {
		t.Errorf("scaled DPI = %d, want 300", out.DPI)
	}
	if want := (ArtSlot{X: 89, Y: 152, Width: 638, Height: 464, After: "background"}); out.Art != want {
		t.Errorf("scaled art slot = %+v, want %+v", out.Art, want)
	}

	box := out.TextBoxes["title"]
	if box.X != 97 || box.Y != 128 || box.Width != 600 || box.Height != 50 {
		t.Errorf("scaled box geometry = (%d,%d %dx%d), want (97,128 600x50)", box.X, box.Y, box.Width, box.Height)
	}
	if box.FontSize != 39 || box.MinFontSize != 30 {
		t.Errorf("scaled font sizes = %v/%v, want 39/30", box.FontSize, box.MinFontSize)
	}
	if box.Padding != 5 || box.PaddingX == nil || *box.PaddingX != 3 {
		t.Errorf("scaled padding = %d/%v, want 5/3", box.Padding, box.PaddingX)
	}
	if want := image.Rect(25, 50, 75, 100); box.Avoid != want {
		t.Errorf("scaled avoid = %v, want %v", box.Avoid, want)
	}
	// Ratios are resolution independent, so they carry over untouched
	if box.LineSpacing != 1.1 || box.Tracking != 125 || box.ClearGap != 0.5 {
		t.Errorf("ratios changed: spacing %v, tracking %v, gap %v", box.LineSpacing, box.Tracking, box.ClearGap)
	}

	// Scaling leaves the original alone, so one manifest serves a preview and an
	// export from the same provider
	if m.Width != 3264 || m.TextBoxes["title"].FontSize != 156 {
		t.Error("Scaled mutated the manifest it was called on")
	}
	// The padding pointer is a copy, not the original's
	if box.PaddingX == m.TextBoxes["title"].PaddingX {
		t.Error("scaled box shares its PaddingX pointer with the original")
	}
}

func TestScaledNativeIsFree(t *testing.T) {
	m := nativeManifest()
	if out := m.Scaled(1); out != m {
		t.Error("Scaled(1) copied the manifest, want the original back untouched")
	}
	if out := m.Scaled(0); out != m {
		t.Error("Scaled(0) copied the manifest, want the original back untouched")
	}
}

func TestScaleImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := range 40 {
		for x := range 80 {
			src.Set(x, y, color.RGBA{R: 200, A: 255})
		}
	}
	out := Scale(0.25).Image(src)
	if got := out.Bounds(); got != image.Rect(0, 0, 20, 10) {
		t.Errorf("scaled bounds = %v, want 0,0-20,10", got)
	}
	if same := Scale(1).Image(src); same != image.Image(src) {
		t.Error("Image at native scale resampled, want the source back")
	}
	// A scale small enough to round a dimension to zero still yields a pixel
	if got := Scale(0.001).Image(src).Bounds(); got.Dx() < 1 || got.Dy() < 1 {
		t.Errorf("tiny scale produced %v, want at least one pixel each way", got)
	}
}

func TestBoxScaleAveragesRatherThanSamples(t *testing.T) {
	// A checkerboard of opaque black and white averages to mid grey. A sampling
	// filter would land on one square or the other and return an extreme
	src := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			v := uint8(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			src.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	out := Scale(0.125).Image(src)
	if got := out.Bounds(); got != image.Rect(0, 0, 8, 8) {
		t.Fatalf("bounds = %v, want 0,0-8,8", got)
	}
	r, _, _, a := out.At(3, 3).RGBA()
	if a>>8 != 255 {
		t.Errorf("alpha = %d, want 255", a>>8)
	}
	if grey := r >> 8; grey < 120 || grey > 136 {
		t.Errorf("averaged value = %d, want mid grey", grey)
	}
}

func TestBoxScaleKeepsTransparentColorOut(t *testing.T) {
	// Half opaque red, half fully transparent. Averaging in premultiplied alpha
	// halves both the red and the alpha; averaging the raw bytes instead would
	// leave whatever color the transparent pixels carry showing through
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			if x < 32 {
				src.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
			} else {
				// A transparent pixel still carrying blue, the way an exported
				// layer's untouched areas often do
				src.SetNRGBA(x, y, color.NRGBA{B: 255, A: 0})
			}
		}
	}
	out := Scale(0.03125).Image(src) // 64 wide down to 2, so each pixel spans a half
	r, g, b, a := out.At(0, 0).RGBA()
	if a>>8 != 255 || r>>8 != 255 || g != 0 || b != 0 {
		t.Errorf("opaque half = rgba(%d,%d,%d,%d), want solid red", r>>8, g>>8, b>>8, a>>8)
	}
	r, _, b, a = out.At(1, 0).RGBA()
	if a>>8 != 0 {
		t.Errorf("transparent half alpha = %d, want 0", a>>8)
	}
	if r != 0 || b != 0 {
		t.Errorf("transparent half carried color rgb(%d,_,%d), want none", r>>8, b>>8)
	}
}
