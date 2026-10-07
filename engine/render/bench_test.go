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
// Without it the benchmarks read ../../assets/normal, the developer's local
// copy, and skip when neither exists
const benchTemplateEnv = "MIMIC_BENCH_TEMPLATE"

// benchRequests are the cards the benchmarks render in turn, with synthetic art
// and a warm layer cache, so a figure is the engine's own cost with no network
// and no disk
func benchRequests(b *testing.B) []template.RenderRequest {
	b.Helper()
	if testing.Short() {
		b.Skip("renders at the template's full size")
	}
	dir := os.Getenv(benchTemplateEnv)
	if dir == "" {
		dir = filepath.Join("..", "..", "assets", "normal")
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		b.Skipf("no template at %s, set %s", dir, benchTemplateEnv)
	}
	assets := template.NewCachedAssets(template.NewFSAssetProvider(dir), 2<<30)
	b.Cleanup(func() { assets.Close() })

	art := image.NewRGBA(image.Rect(0, 0, 626, 457))
	for y := 0; y < 457; y++ {
		for x := 0; x < 626; x++ {
			art.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
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
		reqs = append(reqs, template.RenderRequest{Card: d, Art: art, Assets: assets})
	}
	// Decode every layer these cards use before timing starts
	for _, req := range reqs {
		if _, err := New("normal").Render(context.Background(), req); err != nil {
			b.Fatal(err)
		}
	}
	return reqs
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
			reqs := benchRequests(b)
			tmpl := New("normal")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf, err := tmpl.Render(context.Background(), reqs[i%len(reqs)])
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
	reqs := benchRequests(b)
	tmpl := New("normal")
	b.ReportAllocs()
	b.ResetTimer()
	var next atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf, err := tmpl.Render(context.Background(), reqs[int(next.Add(1))%len(reqs)])
			if err != nil {
				b.Error(err)
				return
			}
			buf.ToYCbCr()
		}
	})
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "cards/s")
}
