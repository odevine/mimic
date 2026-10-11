package render

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// benchTemplateEnv names a directory holding a template's manifest and layers.
// Without it the benchmarks read ../../assets/m15, the developer's local copy, and
// skip when neither exists or it is not a template this engine reads
const benchTemplateEnv = "MIMIC_BENCH_TEMPLATE"

// benchRequests are the cards the benchmarks render in turn, with synthetic art
// and a warm layer cache, so a figure is the engine's own cost with no network
// and no disk
// benchTemplateDir is the folder the benchmarks read a template from, and skips the
// benchmark when there is none
func benchTemplateDir(b *testing.B) string {
	b.Helper()
	if testing.Short() {
		b.Skip("renders at the template's full size")
	}
	dir := os.Getenv(benchTemplateEnv)
	if dir == "" {
		dir = filepath.Join("..", "..", "assets", "m15")
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		b.Skipf("no template at %s, set %s", dir, benchTemplateEnv)
	}
	return dir
}

// loadBench opens a template behind a layer cache, and skips the benchmark when the
// folder is not one this engine reads
func loadBench(b *testing.B, dir string, budget int64) (*template.Template, *template.CachedAssets) {
	b.Helper()
	assets := template.NewCachedAssets(template.NewFSAssetProvider(dir), budget)
	b.Cleanup(func() { assets.Close() })
	tmpl, _, err := template.Load(assets)
	if err != nil {
		b.Skipf("%s is not a template this engine reads: %v", dir, err)
	}
	return tmpl, assets
}

func benchArt() image.Image {
	art := image.NewRGBA(image.Rect(0, 0, 626, 457))
	for y := 0; y < 457; y++ {
		for x := 0; x < 626; x++ {
			art.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	return art
}

// BenchmarkRenderPair renders cards of two colors, which a template made of one image
// per color has the engine mix. Cold is the first card of a batch, with nothing cached,
// and warm is every card after it. Point MIMIC_BENCH_TEMPLATE at a template with pair
// images and one without to see what the mix costs
func BenchmarkRenderPair(b *testing.B) {
	dir := benchTemplateDir(b)
	pair := func(name, cost string, colors ...card.Color) template.RenderRequest {
		return template.RenderRequest{Art: benchArt(), Card: &card.Data{
			Name: name, ManaCost: cost, TypeLine: "Instant", Colors: colors,
			OracleText: "Deals 3 damage to any target and you gain 3 life.",
			Rarity:     "uncommon", CollectorNumber: "12", SetCode: "tst", ReleasedAt: "2024-01-01",
		}}
	}
	reqs := []template.RenderRequest{
		pair("Lightning Helix", "{R}{W}", card.Red, card.White),
		pair("Izzet Charm", "{U}{R}", card.Blue, card.Red),
	}
	b.Run("cold", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			tmpl, _ := loadBench(b, dir, 2<<30)
			b.StartTimer()
			if _, err := Render(context.Background(), tmpl, reqs[0]); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("warm", func(b *testing.B) {
		tmpl, cache := loadBench(b, dir, 2<<30)
		for _, req := range reqs {
			if _, err := Render(context.Background(), tmpl, req); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := Render(context.Background(), tmpl, reqs[i%len(reqs)]); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(cache.Stats().Bytes)/(1<<20), "cacheMB")
	})
}

func benchRequests(b *testing.B) (*template.Template, []template.RenderRequest) {
	b.Helper()
	tmpl, _ := loadBench(b, benchTemplateDir(b), 2<<30)
	art := benchArt()
	var reqs []template.RenderRequest
	for _, name := range []string{"lightning_bolt", "produced_land"} {
		raw, err := os.ReadFile(filepath.Join("..", "card", "testdata", name+".json"))
		if err != nil {
			b.Fatal(err)
		}
		d, err := card.FromScryfallJSON(raw)
		if err != nil {
			b.Fatal(err)
		}
		reqs = append(reqs, template.RenderRequest{Card: d, Art: art})
	}
	// Decode every layer these cards use before timing starts
	for _, req := range reqs {
		if _, err := Render(context.Background(), tmpl, req); err != nil {
			b.Fatal(err)
		}
	}
	return tmpl, reqs
}

// BenchmarkRenderNormal renders cards from one goroutine, which is one card's
// cost end to end. With YCbCr it also converts the result to the planes a JPEG
// stores, as a batch writing JPEGs does
func BenchmarkRenderNormal(b *testing.B) {
	for _, withYCbCr := range []bool{false, true} {
		name := "render"
		if withYCbCr {
			name = "render+ycbcr"
		}
		b.Run(name, func(b *testing.B) {
			tmpl, reqs := benchRequests(b)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf, err := Render(context.Background(), tmpl, reqs[i%len(reqs)])
				if err != nil {
					b.Fatal(err)
				}
				if withYCbCr {
					buf.ToYCbCr()
				}
			}
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "cards/s")
		})
	}
}

// BenchmarkRenderNormalParallel renders from GOMAXPROCS goroutines at once, the
// way a batch does, so it measures throughput when every core is busy. Run it
// with -cpu to vary the worker count
func BenchmarkRenderNormalParallel(b *testing.B) {
	tmpl, reqs := benchRequests(b)
	b.ReportAllocs()
	b.ResetTimer()
	var next atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf, err := Render(context.Background(), tmpl, reqs[int(next.Add(1))%len(reqs)])
			if err != nil {
				b.Error(err)
				return
			}
			buf.ToYCbCr()
		}
	})
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "cards/s")
}
