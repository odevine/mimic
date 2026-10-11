package template

import (
	"image"
	"image/color"
	"reflect"
	"sync"
	"testing"

	"github.com/odevine/mimic/engine/manifest"
)

// testLayout compiles a layout from its manifest form, the way a template does
func testLayout(t *testing.T, ml manifest.Layout) *Layout {
	t.Helper()
	l, err := compileLayout("normal", ml, nil)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// nativeLayout is a stand-in for the normal template's authored geometry: a
// 1200 dpi canvas with one text box and an art window
func nativeLayout(t *testing.T) *Layout {
	return testLayout(t, manifest.Layout{
		Canvas: manifest.Canvas{Width: 3264, Height: 4440},
		Art:    []manifest.ArtWindow{{X: 356, Y: 607, Width: 2552, Height: 1856, After: "background"}},
		TextBoxes: map[string]manifest.TextBox{
			"title": {
				Rect: manifest.Rect{X: 386, Y: 510, Width: 2400, Height: 200},
				Text: manifest.TextStyle{FontSize: 156, MinFontSize: 120, PaddingX: 12, PaddingY: 20, LineSpacing: 1.1, Tracking: 125},
				Flow: manifest.Flow{ClearGap: 0.5},
			},
		},
		Symbols: map[string]manifest.Symbol{
			SymbolSet: {X: 400, Y: 600, Width: 200, Height: 100, Scale: 0.9},
		},
	})
}

func TestNativeDPI(t *testing.T) {
	l := nativeLayout(t)
	if got := l.NativeDPI(); got != 1200 {
		t.Errorf("inferred NativeDPI = %d, want 1200", got)
	}
	l.Canvas.DPI = 800
	if got := l.NativeDPI(); got != 800 {
		t.Errorf("stated NativeDPI = %d, want 800", got)
	}
}

func TestNativeDPIOfATurnedLayoutReadsTheDeliveredWidth(t *testing.T) {
	l := testLayout(t, manifest.Layout{Canvas: manifest.Canvas{Width: 4440, Height: 3264, Rotate: 270}})
	if !l.Turned() || l.DeliveredWidth() != 3264 || l.DeliveredHeight() != 4440 {
		t.Errorf("turned layout delivers %dx%d, turned = %v", l.DeliveredWidth(), l.DeliveredHeight(), l.Turned())
	}
	if got := l.NativeDPI(); got != 1200 {
		t.Errorf("NativeDPI = %d, want 1200 from the delivered width", got)
	}
	if got := l.Resolution(300); got != (Resolution{DPI: 300, Width: 816, Height: 1110}) {
		t.Errorf("Resolution(300) = %+v, want the delivered 816x1110", got)
	}
}

func TestClampDPI(t *testing.T) {
	l := nativeLayout(t)
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
			if got := l.ClampDPI(tt.dpi); got != tt.want {
				t.Errorf("ClampDPI(%d) = %d, want %d", tt.dpi, got, tt.want)
			}
		})
	}
}

func TestScaleForDPI(t *testing.T) {
	l := nativeLayout(t)
	if got := l.ScaleForDPI(300); got != 0.25 {
		t.Errorf("ScaleForDPI(300) = %v, want 0.25", got)
	}
	// Native and out-of-range both render at the authored size rather than
	// resampling assets up
	for _, dpi := range []int{0, 1200, 2400} {
		if got := l.ScaleForDPI(dpi); !got.Native() {
			t.Errorf("ScaleForDPI(%d) = %v, want native", dpi, got)
		}
	}
}

func TestResolution(t *testing.T) {
	l := nativeLayout(t)
	got := l.Resolution(300)
	want := Resolution{DPI: 300, Width: 816, Height: 1110}
	if got != want {
		t.Errorf("Resolution(300) = %+v, want %+v", got, want)
	}
	if got := l.Resolution(0); !got.Native || got.Width != 3264 || got.DPI != 1200 {
		t.Errorf("Resolution(0) = %+v, want the native 3264-wide 1200 dpi size", got)
	}
}

func TestPresets(t *testing.T) {
	l := nativeLayout(t)
	got := l.Presets()
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
	l.Canvas.DPI = 400
	got = l.Presets()
	if len(got) != 3 {
		t.Fatalf("Presets() at 400 dpi = %+v, want 150, 300 and the native 400", got)
	}
	if last := got[len(got)-1]; last.DPI != 400 || !last.Native {
		t.Errorf("last preset = %+v, want the native 400 dpi entry", last)
	}
}

func TestGeometryScalesEveryMeasurement(t *testing.T) {
	l := nativeLayout(t)
	g := l.Geometry(300)

	if g.Width != 816 || g.Height != 1110 || g.DeliveredWidth != 816 || g.DeliveredHeight != 1110 {
		t.Errorf("scaled canvas = %dx%d delivering %dx%d, want 816x1110 both ways", g.Width, g.Height, g.DeliveredWidth, g.DeliveredHeight)
	}
	if want := (ArtSlot{X: 89, Y: 152, Width: 638, Height: 464, After: "background"}); len(g.Art) != 1 || g.Art[0] != want {
		t.Errorf("scaled art = %+v, want %+v", g.Art, want)
	}

	box := g.Boxes["title"].Base
	if box.X != 97 || box.Y != 128 || box.Width != 600 || box.Height != 50 {
		t.Errorf("scaled box geometry = (%d,%d %dx%d), want (97,128 600x50)", box.X, box.Y, box.Width, box.Height)
	}
	if box.FontSize != 39 || box.MinFontSize != 30 {
		t.Errorf("scaled font sizes = %v/%v, want 39/30", box.FontSize, box.MinFontSize)
	}
	if box.PaddingX == nil || *box.PaddingX != 3 || box.PaddingY == nil || *box.PaddingY != 5 {
		t.Errorf("scaled padding = %v/%v, want 3/5", box.PaddingX, box.PaddingY)
	}
	// Ratios are resolution independent, so they carry over untouched
	if box.LineSpacing != 1.1 || box.Tracking != 125 || box.ClearGap != 0.5 {
		t.Errorf("ratios changed: spacing %v, tracking %v, gap %v", box.LineSpacing, box.Tracking, box.ClearGap)
	}

	sym := g.Symbols[SymbolSet]
	if sym.X != 100 || sym.Y != 150 || sym.Width != 50 || sym.Height != 25 {
		t.Errorf("scaled symbol = %d,%d %dx%d, want 100,150 50x25", sym.X, sym.Y, sym.Width, sym.Height)
	}
	if sym.Scale != 0.9 {
		t.Errorf("symbol Scale = %v, want the fraction untouched", sym.Scale)
	}

	// Scaling leaves the authored layout alone, so one template serves a preview
	// and an export
	if l.Canvas.Width != 3264 || l.Boxes[0].Base.Text.FontSize != 156 {
		t.Error("Geometry changed the layout it was built from")
	}
}

func TestGeometryIsBuiltOncePerResolution(t *testing.T) {
	l := nativeLayout(t)
	if l.Geometry(300) != l.Geometry(300) {
		t.Error("two asks for the same resolution built two geometries")
	}
	if l.Geometry(300) == l.Geometry(600) {
		t.Error("two resolutions shared a geometry")
	}
	// A dpi that clamps to native is the native geometry, however it was asked for
	if l.Geometry(0) != l.Geometry(1200) || l.Geometry(0) != l.Geometry(9999) {
		t.Error("native geometry was built more than once")
	}
}

func TestGeometryIsSafeForConcurrentRenders(t *testing.T) {
	l := nativeLayout(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(dpi int) {
			defer wg.Done()
			if g := l.Geometry(dpi); g == nil || g.Boxes["title"].Base.Width == 0 {
				t.Errorf("no geometry at %d dpi", dpi)
			}
		}(150 * (1 + i%4))
	}
	wg.Wait()
}

func TestNativeGeometryIsAsAuthored(t *testing.T) {
	g := nativeLayout(t).Geometry(0)
	if !g.Scale.Native() || g.Width != 3264 || g.Boxes["title"].Base.FontSize != 156 || g.Boxes["title"].Base.X != 386 {
		t.Errorf("native geometry = %+v", g)
	}
}

func TestScaleBoxScalesTheAvoidRectangle(t *testing.T) {
	got := scaleBox(TextBoxSpec{Avoid: image.Rect(100, 200, 300, 400), FontSize: 100}, 0.25)
	if want := image.Rect(25, 50, 75, 100); got.Avoid != want {
		t.Errorf("scaled avoid = %v, want %v", got.Avoid, want)
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

// A layer cut to its bounds shrinks to the pixels the document-sized layer does
func TestImageAtMatchesTheWholeDocumentScaled(t *testing.T) {
	const docW, docH = 301, 427
	cut := image.Rect(37, 53, 211, 389)
	full := image.NewNRGBA(image.Rect(0, 0, docW, docH))
	for y := cut.Min.Y; y < cut.Max.Y; y++ {
		for x := cut.Min.X; x < cut.Max.X; x++ {
			full.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 3), uint8(x + y), uint8(60 + (x*y)%190)})
		}
	}
	piece := full.SubImage(cut)
	for _, f := range []float64{0.25, 0.37, 0.5, 0.73, 0.99} {
		s := Scale(f)
		want := s.Image(full).(*image.RGBA)
		got, at := s.ImageAt(piece, cut.Min.X, cut.Min.Y, docW, docH)
		g := got.(*image.RGBA)
		for y := 0; y < want.Rect.Dy(); y++ {
			for x := 0; x < want.Rect.Dx(); x++ {
				var have color.RGBA
				if p := image.Pt(x, y).Sub(at); p.In(g.Rect) {
					have = g.RGBAAt(p.X, p.Y)
				}
				if w := want.RGBAAt(x, y); have != w {
					t.Fatalf("scale %v: pixel (%d,%d) = %v, want %v", f, x, y, have, w)
				}
			}
		}
	}
}

func TestImageAtLeavesNativeAlone(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	got, at := Scale(1).ImageAt(img, 7, 9, 100, 100)
	if got != image.Image(img) || at != image.Pt(7, 9) {
		t.Errorf("native ImageAt = %v at %v", got, at)
	}
}
