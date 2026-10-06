package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/catalog/catalogtest"
)

// withEmptyAssetChain points the loose-dir roots at nothing and the cache at a
// fresh temp dir, so a test drives the fallback tiers deterministically
func withEmptyAssetChain(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	oldBases, oldDir := LooseDirBases, catalog.Dir
	LooseDirBases = []string{filepath.Join(tmp, "no-such-assets")}
	catalog.Dir = func() (string, error) { return filepath.Join(tmp, "templates"), nil }
	t.Cleanup(func() { LooseDirBases, catalog.Dir = oldBases, oldDir })
	return tmp
}

func TestResolveFallsBackToPlaceholders(t *testing.T) {
	withEmptyAssetChain(t)

	at, source, err := Resolve("normal")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer at.Close()
	if source != SourcePlaceholder {
		t.Fatalf("source = %d, want SourcePlaceholder", source)
	}
	// Placeholders produce a valid, renderable manifest
	if _, err := at.provider.Manifest(); err != nil {
		t.Fatalf("placeholder Manifest: %v", err)
	}
}

func TestResolvePrefersCachedBundle(t *testing.T) {
	withEmptyAssetChain(t)

	// Seed the cache with a valid bundle so the middle tier wins
	bundle, _ := catalogtest.Bundle(t)
	dst, err := catalog.BundlePath("normal", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, bundle, 0o644); err != nil {
		t.Fatal(err)
	}

	at, source, err := Resolve("normal")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer at.Close()
	if source != SourceBundle {
		t.Fatalf("source = %d, want SourceBundle", source)
	}
	if at.Version != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", at.Version)
	}
	if _, err := at.provider.Manifest(); err != nil {
		t.Fatalf("bundle Manifest: %v", err)
	}
}

// TestResolvePrefersLooseDir confirms a loose developer directory wins the chain
func TestResolvePrefersLooseDir(t *testing.T) {
	tmp := withEmptyAssetChain(t)
	LooseDirBases = []string{filepath.Join(tmp, "assets")}
	dir := filepath.Join(tmp, "assets", "normal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A minimal manifest so the FS provider validates
	manifest := `{"template":"normal","width":744,"height":1039,"layers":[]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	at, source, err := Resolve("normal")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer at.Close()
	if source != SourceLoose {
		t.Fatalf("source = %d, want SourceLoose", source)
	}
	if at.Version != LocalVersion {
		t.Errorf("version = %q, want %q", at.Version, LocalVersion)
	}
	if _, ok := at.provider.(*template.CachedAssets); !ok {
		t.Errorf("provider is a %T, want the layer cache", at.provider)
	}
}

func TestActiveFromVersionDownloadsAndBuilds(t *testing.T) {
	withEmptyAssetChain(t)
	srv := catalogtest.Serve(t)
	oldURL := catalog.IndexURL
	catalog.IndexURL = srv.IndexURL
	t.Cleanup(func() { catalog.IndexURL = oldURL })

	var sawProgress bool
	progress := func(done, total int64) { sawProgress = true }

	at, err := FromVersion(context.Background(), "normal", "0.1.0", progress)
	if err != nil {
		t.Fatalf("FromVersion: %v", err)
	}
	defer at.Close()
	if at.Name != "normal" || at.Version != "0.1.0" {
		t.Errorf("active = %s %s, want normal 0.1.0", at.Name, at.Version)
	}
	if at.template == nil {
		t.Fatal("active template is nil")
	}
	if _, err := at.provider.Manifest(); err != nil {
		t.Fatalf("provider Manifest: %v", err)
	}
	if !sawProgress {
		t.Error("progress callback was never called during a download")
	}
	if !catalog.IsCached("normal", "0.1.0") {
		t.Error("version not cached after FromVersion")
	}
}

// loadAnyLayer decodes the first layer asset a template has through its provider
func loadAnyLayer(t *testing.T, at *Template) {
	t.Helper()
	m, err := at.provider.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range m.Layers {
		for _, v := range l.ColorVariants {
			if _, err := template.LoadImage(at.provider, v.Path); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("the manifest has no layers")
}

func layerStats(at *Template) template.CacheStats {
	return at.provider.(*template.CachedAssets).Stats()
}

// resetLayerCache puts the budget back for the next test
func resetLayerCache(t *testing.T) {
	t.Cleanup(func() { SetLayerCacheBytes(template.DefaultImageCacheBytes) })
}

func TestSetLayerCacheBytesResizesLoadedTemplates(t *testing.T) {
	withEmptyAssetChain(t)
	resetLayerCache(t)
	at, _, err := Resolve("normal")
	if err != nil {
		t.Fatal(err)
	}
	defer at.Close()

	loadAnyLayer(t, at)
	if layerStats(at).Layers != 1 {
		t.Fatalf("stats = %+v, want the layer cached", layerStats(at))
	}
	SetLayerCacheBytes(0)
	if s := layerStats(at); s.Layers != 0 || s.Bytes != 0 {
		t.Errorf("stats after a zero budget = %+v, want empty", s)
	}
	loadAnyLayer(t, at)
	if layerStats(at).Layers != 0 {
		t.Error("a zero budget should keep nothing")
	}
	SetLayerCacheBytes(template.DefaultImageCacheBytes)
	loadAnyLayer(t, at)
	if layerStats(at).Layers != 1 {
		t.Error("raising the budget should cache again")
	}
}

func TestNewTemplatesStartWithTheChosenBudget(t *testing.T) {
	withEmptyAssetChain(t)
	resetLayerCache(t)
	SetLayerCacheBytes(0)
	at, _, err := Resolve("normal")
	if err != nil {
		t.Fatal(err)
	}
	defer at.Close()
	loadAnyLayer(t, at)
	if layerStats(at).Layers != 0 {
		t.Error("a template loaded under a zero budget should cache nothing")
	}
}

func TestInstallEmptiesTheSwappedOutLayerCache(t *testing.T) {
	withEmptyAssetChain(t)
	resetLayerCache(t)
	first, _, err := Resolve("normal")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := Resolve("normal")
	if err != nil {
		t.Fatal(err)
	}
	p := New(nil, nil)
	defer p.Close()
	p.Install(first)
	loadAnyLayer(t, first)
	if layerStats(first).Layers != 1 {
		t.Fatalf("stats = %+v, want the layer cached", layerStats(first))
	}

	p.Install(second)
	if layerStats(first).Layers != 0 {
		t.Error("the swapped-out template kept its layers")
	}
	// Later size changes must not bring the dead cache back
	SetLayerCacheBytes(template.DefaultImageCacheBytes)
	loadAnyLayer(t, first)
	if layerStats(first).Layers != 0 {
		t.Error("a resize revived the swapped-out template's cache")
	}
	p.Install(second)
	loadAnyLayer(t, second)
	if layerStats(second).Layers != 1 {
		t.Error("reinstalling the active template should leave its cache alone")
	}
}
