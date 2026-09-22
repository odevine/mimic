package normal

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// renderBolt renders a synthetic Lightning Bolt against generated placeholder
// assets, with no network involved.
func renderBolt(t *testing.T, art image.Image) *template.RenderRequest {
	t.Helper()
	dir := t.TempDir()
	if err := WritePlaceholderAssets(dir); err != nil {
		t.Fatalf("WritePlaceholderAssets: %v", err)
	}
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
		Assets: template.NewFSAssetProvider(dir),
	}
}

func TestRenderProducesCorrectlySizedBuffer(t *testing.T) {
	req := renderBolt(t, nil)
	tmpl := &Template{}
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

	if _, err := (&Template{}).Render(context.Background(), *req); err != nil {
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
	buf, err := (&Template{}).Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := buf.ToImage(8)
	got := color.NRGBAModel.Convert(img.At(4, 4)).(color.NRGBA)
	want := placeholderColors["r"]
	if !closeColor(got, want) {
		t.Errorf("background at (4,4) = %v, want red key %v", got, want)
	}
}

func TestRenderRegisteredViaRegistry(t *testing.T) {
	tmpl, err := template.Get("normal")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	req := renderBolt(t, nil)
	if _, err := tmpl.Render(context.Background(), *req); err != nil {
		t.Fatalf("Render via registry: %v", err)
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
	buf, err := (&Template{}).Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := buf.ToImage(8)
	// Art slot origin is (60,132); sample well inside it.
	got := color.NRGBAModel.Convert(img.At(200, 300)).(color.NRGBA)
	if !closeColor(got, magenta) {
		t.Errorf("art region = %v, want magenta %v", got, magenta)
	}
}

func TestRenderCancelledContext(t *testing.T) {
	req := renderBolt(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Template{}).Render(ctx, *req); err == nil {
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
