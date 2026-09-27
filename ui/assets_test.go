package main

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
	oldBases, oldCfg := looseDirBases, userConfigDir
	looseDirBases = []string{filepath.Join(tmp, "no-such-assets")}
	userConfigDir = func() (string, error) { return filepath.Join(tmp, "config"), nil }
	t.Cleanup(func() { looseDirBases, userConfigDir = oldBases, oldCfg })
	return tmp
}

func TestResolveFallsBackToPlaceholders(t *testing.T) {
	withEmptyAssetChain(t)

	at, source, err := resolveActiveTemplate("normal")
	if err != nil {
		t.Fatalf("resolveActiveTemplate: %v", err)
	}
	defer at.cleanup()
	if source != sourcePlaceholder {
		t.Fatalf("source = %d, want sourcePlaceholder", source)
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

	at, source, err := resolveActiveTemplate("normal")
	if err != nil {
		t.Fatalf("resolveActiveTemplate: %v", err)
	}
	defer at.cleanup()
	if source != sourceBundle {
		t.Fatalf("source = %d, want sourceBundle", source)
	}
	if at.version != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", at.version)
	}
	if _, err := at.provider.Manifest(); err != nil {
		t.Fatalf("bundle Manifest: %v", err)
	}
}

// TestResolvePrefersLooseDir confirms a loose developer directory wins the chain
func TestResolvePrefersLooseDir(t *testing.T) {
	tmp := withEmptyAssetChain(t)
	looseDirBases = []string{filepath.Join(tmp, "assets")}
	dir := filepath.Join(tmp, "assets", "normal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A minimal manifest so the FS provider validates
	manifest := `{"template":"normal","width":744,"height":1039,"layers":[]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	at, source, err := resolveActiveTemplate("normal")
	if err != nil {
		t.Fatalf("resolveActiveTemplate: %v", err)
	}
	defer at.cleanup()
	if source != sourceLoose {
		t.Fatalf("source = %d, want sourceLoose", source)
	}
	if at.version != localVersion {
		t.Errorf("version = %q, want %q", at.version, localVersion)
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

	at, err := activeFromVersion(context.Background(), "normal", "0.1.0", progress)
	if err != nil {
		t.Fatalf("activeFromVersion: %v", err)
	}
	defer at.cleanup()
	if at.name != "normal" || at.version != "0.1.0" {
		t.Errorf("active = %s %s, want normal 0.1.0", at.name, at.version)
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
		t.Error("version not cached after activeFromVersion")
	}
}
