package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
