package render

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// placeholderDir holds one set of placeholder assets that every test reads,
// since encoding their full-size PNGs is slow under the race detector
var placeholderDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "render-test-")
	if err == nil {
		err = WritePlaceholderAssets(dir, "test")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "writing placeholder assets:", err)
		os.Exit(1)
	}
	placeholderDir = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// renderBolt renders a synthetic Lightning Bolt against generated placeholder
// assets, with no network involved.
func renderBolt(t *testing.T, art image.Image) *template.RenderRequest {
	t.Helper()
	return &template.RenderRequest{
		Card: &card.Data{
			Name:          "Lightning Bolt",
			ManaCost:      "{R}",
			TypeLine:      "Instant",
			OracleText:    "Lightning Bolt deals 3 damage to any target.",
			Colors:        []card.Color{card.Red},
			ColorIdentity: []card.Color{card.Red},
		},
		Art:    art,
		Assets: template.NewFSAssetProvider(placeholderDir),
	}
}

// atMinDPI sets req to render at the smallest resolution a template allows, for
// tests that sample pixels rather than check size, and returns the scale that
// maps a native coordinate into the smaller render
func atMinDPI(t *testing.T, req *template.RenderRequest) template.Scale {
	t.Helper()
	m, err := req.Assets.Manifest()
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	req.DPI = template.MinDPI
	return m.ScaleForDPI(req.DPI)
}

func TestRenderProducesCorrectlySizedBuffer(t *testing.T) {
	req := renderBolt(t, nil)
	tmpl := New("test")
	buf, err := tmpl.Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	w, h := buf.Bounds()
	if w != placeholderWidth || h != placeholderHeight {
		t.Errorf("buffer is %dx%d, want %dx%d", w, h, placeholderWidth, placeholderHeight)
	}
}

func TestRenderReportsProgress(t *testing.T) {
	req := renderBolt(t, nil)
	atMinDPI(t, req)
	var steps []string
	last := -1.0
	req.Progress = func(step string, frac float64) {
		if frac < 0 || frac > 1 {
			t.Errorf("fraction %v out of [0,1] at step %q", frac, step)
		}
		if frac < last {
			t.Errorf("fraction went backward: %v after %v at step %q", frac, last, step)
		}
		last = frac
		if len(steps) == 0 || steps[len(steps)-1] != step {
			steps = append(steps, step)
		}
	}

	if _, err := New("test").Render(context.Background(), *req); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("Progress was never called")
	}
	for _, want := range []string{stepManifest, stepFrame, stepText, stepFinalize} {
		if !containsStep(steps, want) {
			t.Errorf("step %q not reported; got %v", want, steps)
		}
	}
}

func containsStep(steps []string, want string) bool {
	for _, s := range steps {
		if s == want {
			return true
		}
	}
	return false
}

func TestRenderPicksColorKeyedBackground(t *testing.T) {
	// The top-left pixel is bare background, so a red card must show the red
	// background fill there rather than another color's.
	req := renderBolt(t, nil)
	s := atMinDPI(t, req)
	buf, err := New("test").Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := buf.ToImage(8)
	got := color.NRGBAModel.Convert(img.At(s.Px(4), s.Px(4))).(color.NRGBA)
	want := placeholderColors["r"]
	if !closeColor(got, want) {
		t.Errorf("background at (4,4) = %v, want red key %v", got, want)
	}
}

func TestRenderPlacesArt(t *testing.T) {
	// A solid magenta art buffer must appear at the art slot origin.
	art := image.NewNRGBA(image.Rect(0, 0, 624, 456))
	magenta := color.NRGBA{0xFF, 0x00, 0xFF, 0xFF}
	for y := 0; y < 456; y++ {
		for x := 0; x < 624; x++ {
			art.SetNRGBA(x, y, magenta)
		}
	}
	req := renderBolt(t, art)
	s := atMinDPI(t, req)
	buf, err := New("test").Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := buf.ToImage(8)
	// Art slot origin is (60,132); sample well inside it.
	got := color.NRGBAModel.Convert(img.At(s.Px(200), s.Px(300))).(color.NRGBA)
	if !closeColor(got, magenta) {
		t.Errorf("art region = %v, want magenta %v", got, magenta)
	}
}

func TestRenderCancelledContext(t *testing.T) {
	req := renderBolt(t, nil)
	atMinDPI(t, req)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New("test").Render(ctx, *req); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// closeColor allows a small per-channel tolerance for the linear/8-bit round
// trip through impasto.
func closeColor(a, b color.NRGBA) bool {
	const tol = 6
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol && d(a.A, b.A) <= tol
}

func TestRenderAtReducedDPI(t *testing.T) {
	req := renderBolt(t, nil)
	m, err := req.Assets.Manifest()
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	// Half the placeholder's authored resolution, whatever that works out to
	half := m.Resolution(m.NativeDPI() / 2)
	req.DPI = half.DPI

	buf, err := New("test").Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	w, h := buf.Bounds()
	if w != half.Width || h != half.Height {
		t.Errorf("buffer is %dx%d, want %dx%d", w, h, half.Width, half.Height)
	}
}

func TestRenderClampsDPIToNative(t *testing.T) {
	req := renderBolt(t, nil)
	// Asking past the authored resolution renders at it rather than resampling
	// every layer up
	req.DPI = 100000

	buf, err := New("test").Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	w, h := buf.Bounds()
	if w != placeholderWidth || h != placeholderHeight {
		t.Errorf("buffer is %dx%d, want the native %dx%d", w, h, placeholderWidth, placeholderHeight)
	}
}
