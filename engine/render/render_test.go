package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

// placeholderDir holds one set of placeholder assets that every test reads,
// since encoding their full-size PNGs is slow under the race detector
var placeholderDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "render-test-")
	if err == nil {
		err = WritePlaceholderAssets(dir, "mimic.test")
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

// renderBolt is a request for a synthetic Lightning Bolt, with no network involved
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
		Art: art,
	}
}

// atMinDPI sets req to render at the smallest resolution a template allows, for
// tests that sample pixels rather than check size, and returns the scale that
// maps a native coordinate into the smaller render
func atMinDPI(tmpl *template.Template, req *template.RenderRequest) template.Scale {
	req.DPI = template.MinDPI
	return tmpl.Primary().ScaleForDPI(req.DPI)
}

func TestRenderProducesCorrectlySizedBuffer(t *testing.T) {
	tmpl := placeholderTemplate(t)
	buf, err := Render(context.Background(), tmpl, *renderBolt(t, nil))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	w, h := buf.Bounds()
	if w != placeholderWidth || h != placeholderHeight {
		t.Errorf("buffer is %dx%d, want %dx%d", w, h, placeholderWidth, placeholderHeight)
	}
}

func TestRenderRefusesWhatItCannotDraw(t *testing.T) {
	tmpl := placeholderTemplate(t)
	ctx := context.Background()

	_, err := Render(ctx, tmpl, template.RenderRequest{Card: &card.Data{TypeLine: "Legendary Planeswalker — Jace"}})
	var unsupported *template.UnsupportedError
	want := frame.Shape{Role: frame.RoleSingle, Kind: frame.KindPlaneswalker}
	if !errors.As(err, &unsupported) || unsupported.Shape != want || unsupported.Template != "mimic.test" {
		t.Errorf("a planeswalker should be refused with an UnsupportedError for the template, got %v", err)
	}

	dfc := &card.Data{Layout: "transform", Faces: []card.Face{{TypeLine: "Creature"}, {TypeLine: "Creature"}}}
	if _, err := Render(ctx, tmpl, template.RenderRequest{Card: dfc, Face: 2}); err == nil || errors.As(err, &unsupported) {
		t.Errorf("an out of range face should fail as out of range, got %v", err)
	}
	if _, err := Render(ctx, tmpl, template.RenderRequest{Card: dfc, Face: -1}); err == nil {
		t.Error("a negative face should fail")
	}
	if _, err := Render(ctx, tmpl, template.RenderRequest{}); err == nil {
		t.Error("a request with no card should fail")
	}
	if _, err := Render(ctx, nil, *renderBolt(t, nil)); err == nil {
		t.Error("a render with no template should fail")
	}
}

func TestRenderReportsProgress(t *testing.T) {
	tmpl := placeholderTemplate(t)
	req := renderBolt(t, nil)
	atMinDPI(tmpl, req)
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

	if _, err := Render(context.Background(), tmpl, *req); err != nil {
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
	tmpl := placeholderTemplate(t)
	req := renderBolt(t, nil)
	s := atMinDPI(tmpl, req)
	buf, err := Render(context.Background(), tmpl, *req)
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
	tmpl := placeholderTemplate(t)
	req := renderBolt(t, art)
	s := atMinDPI(tmpl, req)
	buf, err := Render(context.Background(), tmpl, *req)
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
	tmpl := placeholderTemplate(t)
	req := renderBolt(t, nil)
	atMinDPI(tmpl, req)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, tmpl, *req); err == nil {
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
	tmpl := placeholderTemplate(t)
	req := renderBolt(t, nil)
	// Half the placeholder's authored resolution, whatever that works out to
	l := tmpl.Primary()
	half := l.Resolution(l.NativeDPI() / 2)
	req.DPI = half.DPI

	buf, err := Render(context.Background(), tmpl, *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	w, h := buf.Bounds()
	if w != half.Width || h != half.Height {
		t.Errorf("buffer is %dx%d, want %dx%d", w, h, half.Width, half.Height)
	}
}

func TestRenderClampsDPIToNative(t *testing.T) {
	// Asking past the authored resolution renders at it rather than resampling
	// every layer up
	req := renderBolt(t, nil)
	req.DPI = 100000

	buf, err := Render(context.Background(), placeholderTemplate(t), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	w, h := buf.Bounds()
	if w != placeholderWidth || h != placeholderHeight {
		t.Errorf("buffer is %dx%d, want the native %dx%d", w, h, placeholderWidth, placeholderHeight)
	}
}

// A frame layer decodes when the compositor reaches it, so a missing PNG fails
// the render rather than the node list, and the error still names the path
func TestRenderReportsAMissingFrameLayer(t *testing.T) {
	base := template.NewFSAssetProvider(placeholderDir)
	m := readManifest(t, base)
	m.Parts["background"] = manifest.Part{Slot: "background", Variants: anyImage("nope/missing.png")}
	tmpl := mustLoad(t, withManifest(t, base, m, nil))

	req := renderBolt(t, nil)
	atMinDPI(tmpl, req)
	_, err := Render(context.Background(), tmpl, *req)
	if err == nil {
		t.Fatal("expected an error for a layer whose PNG is missing")
	}
	if !strings.Contains(err.Error(), "nope/missing.png") {
		t.Errorf("error %q does not name the missing asset", err)
	}
}

// Cancelling while layers are loading stops the render with the context's error
func TestRenderStopsWhenCancelledDuringFrameLoad(t *testing.T) {
	tmpl := placeholderTemplate(t)
	req := renderBolt(t, nil)
	atMinDPI(tmpl, req)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The frame band is reported as layers load, which is after the text is laid
	// out, so cancelling on the first such report lands inside the compositor
	req.Progress = func(step string, frac float64) {
		if step == stepFrame && frac >= fracTextTo && ctx.Err() == nil {
			cancel()
		}
	}
	_, err := Render(ctx, tmpl, *req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Render = %v, want context.Canceled", err)
	}
}

// The nyx frame stands in for the background, and loads where its own part says,
// not where the background's does
func TestRenderPlacesNyxAtItsOwnPosition(t *testing.T) {
	base := template.NewFSAssetProvider(placeholderDir)
	m := readManifest(t, base)
	l := m.Layouts["normal"]
	l.Stack[0].Condition = "nonnyx"
	l.Stack = append(l.Stack, manifest.Entry{Part: "nyx", Condition: "nyx"})
	m.Layouts["normal"] = l
	m.Parts["nyx"] = manifest.Part{
		Slot: "background", Origin: manifest.Point{X: 100, Y: 200},
		Variants: map[string]manifest.Variant{"r": {Path: "nyx/r.png"}},
	}

	blue := color.NRGBA{0, 0, 0xFF, 0xFF}
	patch := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	draw.Draw(patch, patch.Bounds(), image.NewUniform(blue), image.Point{}, draw.Src)
	var png_ bytes.Buffer
	if err := png.Encode(&png_, patch); err != nil {
		t.Fatal(err)
	}
	tmpl := mustLoad(t, withManifest(t, base, m, map[string][]byte{"nyx/r.png": png_.Bytes()}))

	req := renderBolt(t, nil)
	req.Card.TypeLine = "Enchantment"
	buf, err := Render(context.Background(), tmpl, *req)
	if err != nil {
		t.Fatal(err)
	}
	img := buf.ToImage(8)
	if got := color.NRGBAModel.Convert(img.At(110, 210)).(color.NRGBA); !closeColor(got, blue) {
		t.Errorf("inside the nyx patch = %v, want %v", got, blue)
	}
	if _, _, _, a := img.At(10, 10).RGBA(); a != 0 {
		t.Errorf("outside the nyx patch has alpha %d, so the background drew too", a)
	}

	// A card that is not an enchantment keeps the ordinary background
	plain := renderBolt(t, nil)
	buf, err = Render(context.Background(), tmpl, *plain)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(buf.ToImage(8).At(10, 10)).(color.NRGBA); !closeColor(got, placeholderColors["r"]) {
		t.Errorf("a plain card's background = %v, want red", got)
	}
}

// featureTemplate is the placeholder with the background's opacity driven by a
// range feature and a crown entry that a bool feature switches on
func featureTemplate(t *testing.T) *template.Template {
	t.Helper()
	base := template.NewFSAssetProvider(placeholderDir)
	m := readManifest(t, base)
	m.Features = map[string]manifest.Feature{
		"strength": {Type: "range", Min: 0, Max: 1, Step: 0.05},
		"banner":   {Type: "bool"},
	}
	l := m.Layouts["normal"]
	l.Stack[0].OpacityFeature = "strength"
	l.Stack[2].Condition = "feature:banner"
	m.Layouts["normal"] = l
	return mustLoad(t, withManifest(t, base, m, nil))
}

func TestRenderAppliesAnEntrysOpacity(t *testing.T) {
	base := template.NewFSAssetProvider(placeholderDir)
	m := readManifest(t, base)
	half := 0.5
	l := m.Layouts["normal"]
	l.Stack[0].Opacity = &half
	m.Layouts["normal"] = l
	tmpl := mustLoad(t, withManifest(t, base, m, nil))

	req := renderBolt(t, nil)
	s := atMinDPI(tmpl, req)
	buf, err := Render(context.Background(), tmpl, *req)
	if err != nil {
		t.Fatal(err)
	}
	got := color.NRGBAModel.Convert(buf.ToImage(8).At(s.Px(4), s.Px(4))).(color.NRGBA)
	if got.A < 125 || got.A > 131 {
		t.Errorf("a background at half opacity has alpha %d, want about 128", got.A)
	}
}

func TestRenderTakesAnEntrysOpacityFromAFeature(t *testing.T) {
	tmpl := featureTemplate(t)
	alphaAt := func(features frame.Features) uint8 {
		t.Helper()
		req := renderBolt(t, nil)
		req.Features = features
		s := atMinDPI(tmpl, req)
		buf, err := Render(context.Background(), tmpl, *req)
		if err != nil {
			t.Fatal(err)
		}
		return color.NRGBAModel.Convert(buf.ToImage(8).At(s.Px(4), s.Px(4))).(color.NRGBA).A
	}
	if a := alphaAt(frame.Features{"strength": 0.25}); a < 61 || a > 67 {
		t.Errorf("a feature of 0.25 gives alpha %d, want about 64", a)
	}
	if a := alphaAt(nil); a != 255 {
		t.Errorf("a feature with no value gives alpha %d, want opaque", a)
	}
	if a := alphaAt(frame.Features{"strength": 7.0}); a != 255 {
		t.Errorf("a feature past its range gives alpha %d, want it held at opaque", a)
	}
	if a := alphaAt(frame.Features{"strength": 0.0}); a != 0 {
		t.Errorf("a feature of 0 gives alpha %d, want the entry left out", a)
	}
}

func TestRenderDrawsAnEntryWhenItsFeatureIsOn(t *testing.T) {
	tmpl := featureTemplate(t)
	crownAt := func(on bool) bool {
		t.Helper()
		req := renderBolt(t, nil)
		req.Features = frame.Features{"banner": on}
		s := atMinDPI(tmpl, req)
		buf, err := Render(context.Background(), tmpl, *req)
		if err != nil {
			t.Fatal(err)
		}
		// The crown strip sits at the top, in a darker gold than any background
		got := color.NRGBAModel.Convert(buf.ToImage(8).At(s.Px(300), s.Px(70))).(color.NRGBA)
		return closeColor(got, color.NRGBA{0xA0, 0x7E, 0x2A, 0xFF})
	}
	if !crownAt(true) {
		t.Error("the crown did not draw with its feature on")
	}
	if crownAt(false) {
		t.Error("the crown drew with its feature off")
	}
}

// A render through a layer cache composites each layer's image with its index of
// visible pixels, and one through a plain provider composites the bare image.
// Both, on a first render and on one that reuses the cached layers, must give the
// same pixels, at native size and scaled
func TestRenderThroughACacheMatchesAPlainProvider(t *testing.T) {
	plain := placeholderTemplate(t)
	for _, dpi := range []int{0, 150} {
		plainReq := renderBolt(t, nil)
		plainReq.Card.TypeLine = "Legendary Creature — Elf"
		plainReq.Card.Power, plainReq.Card.Toughness = "2", "2"
		plainReq.DPI = dpi
		want, err := Render(context.Background(), plain, *plainReq)
		if err != nil {
			t.Fatal(err)
		}

		cached := template.NewCachedAssets(template.NewFSAssetProvider(placeholderDir), 1<<30)
		defer cached.Close()
		tmpl := mustLoad(t, cached)
		for pass := 1; pass <= 2; pass++ {
			got, err := Render(context.Background(), tmpl, *plainReq)
			if err != nil {
				t.Fatal(err)
			}
			if !got.SameSize(want) {
				t.Fatalf("dpi %d pass %d: %dx%d, want %dx%d", dpi, pass, got.Width, got.Height, want.Width, want.Height)
			}
			for i := range want.Pix {
				if got.Pix[i] != want.Pix[i] {
					t.Fatalf("dpi %d pass %d: value %d (pixel %d, channel %d) = %v, want %v", dpi, pass, i, i/4, i%4, got.Pix[i], want.Pix[i])
				}
			}
		}
		if s := cached.Stats(); s.Hits == 0 {
			t.Errorf("dpi %d: the second render found nothing in the cache: %+v", dpi, s)
		}
	}
}

// Renders of one template from many goroutines, with the template replaced in a
// library while they run, all complete with the template each started with
func TestRendersKeepTheTemplateTheyStartedWith(t *testing.T) {
	lib := template.NewLibrary()
	lib.Add(placeholderTemplate(t))
	errs := make(chan error, 12)
	for i := 0; i < 6; i++ {
		go func() {
			tmpl, ok := lib.Get("mimic.test")
			if !ok {
				errs <- errors.New("template missing from the library")
				return
			}
			req := renderBolt(t, nil)
			req.DPI = template.MinDPI
			_, err := Render(context.Background(), tmpl, *req)
			errs <- err
		}()
	}
	for i := 0; i < 6; i++ {
		go func() {
			lib.Add(placeholderTemplate(t))
			errs <- nil
		}()
	}
	for i := 0; i < 12; i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

// pairTemplate is a frame whose pinlines come as one flat image per color, so a card
// of two colors has none of its own to draw and the engine mixes its two
func pairTemplate(t *testing.T, pair bool) (*template.Template, string) {
	t.Helper()
	dir := t.TempDir()
	red, blue, green := color.NRGBA{0xE0, 0x20, 0x20, 0xFF}, color.NRGBA{0x20, 0x20, 0xE0, 0xFF}, color.NRGBA{0x20, 0xE0, 0x20, 0xFF}
	for name, c := range map[string]color.NRGBA{"r": red, "u": blue, "ur": green} {
		if err := writeFlat(filepath.Join(dir, name+".png"), fullRect(), c); err != nil {
			t.Fatal(err)
		}
	}
	variants := map[string]manifest.Variant{"r": {Path: "r.png"}, "u": {Path: "u.png"}}
	if pair {
		variants["ur"] = manifest.Variant{Path: "ur.png"}
	}
	writeManifestFile(t, dir, testManifest(manifest.Layout{
		Stack: []manifest.Entry{{Part: "pinlines"}},
	}, map[string]manifest.Part{"pinlines": {Slot: "pinlines", Variants: variants}}))
	return mustLoad(t, template.NewFSAssetProvider(dir)), dir
}

func izzet() *card.Data {
	return &card.Data{Name: "Izzet Charm", ManaCost: "{U}{R}", TypeLine: "Instant", Colors: []card.Color{card.Blue, card.Red}}
}

func TestRenderMixesAPairFromItsTwoColors(t *testing.T) {
	tmpl, _ := pairTemplate(t, false)
	buf, err := Render(context.Background(), tmpl, template.RenderRequest{Card: izzet()})
	if err != nil {
		t.Fatal(err)
	}
	img := buf.ToImage(8)
	at := func(x int) color.NRGBA {
		return color.NRGBAModel.Convert(img.At(x, 500)).(color.NRGBA)
	}
	// Blue and red run left to right for this pair, and meet in the middle
	if got := at(20); !closeColor(got, color.NRGBA{0x20, 0x20, 0xE0, 0xFF}) {
		t.Errorf("left = %v, want blue", got)
	}
	if got := at(placeholderWidth - 20); !closeColor(got, color.NRGBA{0xE0, 0x20, 0x20, 0xFF}) {
		t.Errorf("right = %v, want red", got)
	}
	if got := at(placeholderWidth / 2); !closeColor(got, color.NRGBA{0x80, 0x20, 0x80, 0xFF}) {
		t.Errorf("middle = %v, want an even mix", got)
	}
}

func TestRenderPrefersAPairImageToAMix(t *testing.T) {
	tmpl, _ := pairTemplate(t, true)
	buf, err := Render(context.Background(), tmpl, template.RenderRequest{Card: izzet()})
	if err != nil {
		t.Fatal(err)
	}
	green := color.NRGBA{0x20, 0xE0, 0x20, 0xFF}
	for _, x := range []int{20, placeholderWidth / 2, placeholderWidth - 20} {
		if got := color.NRGBAModel.Convert(buf.ToImage(8).At(x, 500)).(color.NRGBA); !closeColor(got, green) {
			t.Errorf("x %d = %v, want the pair image's green", x, got)
		}
	}
}

// A batch of cards of the same two colors blends them once
func TestRenderBlendsAPairOnceForABatch(t *testing.T) {
	_, dir := pairTemplate(t, false)
	cached := template.NewCachedAssets(template.NewFSAssetProvider(dir), 1<<30)
	defer cached.Close()
	tmpl := mustLoad(t, cached)

	if _, err := Render(context.Background(), tmpl, template.RenderRequest{Card: izzet()}); err != nil {
		t.Fatal(err)
	}
	first := cached.Stats()
	for i := 0; i < 3; i++ {
		other := izzet()
		other.Name = fmt.Sprintf("Another %d", i)
		if _, err := Render(context.Background(), tmpl, template.RenderRequest{Card: other}); err != nil {
			t.Fatal(err)
		}
	}
	later := cached.Stats()
	if later.Misses != first.Misses {
		t.Errorf("misses went from %d to %d, so a later card built or decoded again", first.Misses, later.Misses)
	}
	if later.Hits <= first.Hits {
		t.Errorf("hits went from %d to %d, want the later cards to find the blend", first.Hits, later.Hits)
	}
}
