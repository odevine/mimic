package main

import (
	"os"
	"path/filepath"
	"testing"
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
	bundle, _ := tinyBundle(t)
	dst, err := bundlePath("normal", "0.1.0")
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
