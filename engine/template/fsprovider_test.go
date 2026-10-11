package template

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/manifest"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
}

func TestReadManifestReturnsTheFile(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, minimalManifestJSON)
	got, err := NewFSAssetProvider(dir).ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if string(got) != minimalManifestJSON {
		t.Errorf("ReadManifest = %q, want the file's bytes", got)
	}
}

func TestReadManifestReportsAMissingFile(t *testing.T) {
	if _, err := NewFSAssetProvider(t.TempDir()).ReadManifest(); err == nil {
		t.Fatal("expected an error for a folder with no manifest, got nil")
	}
}

func TestReadManifestRefusesAFileOverTheLimit(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, strings.Repeat(" ", manifest.MaxBytes+1))
	if _, err := NewFSAssetProvider(dir).ReadManifest(); err == nil {
		t.Fatal("expected an error for an oversized manifest, got nil")
	}
	writeManifest(t, dir, strings.Repeat(" ", manifest.MaxBytes))
	if _, err := NewFSAssetProvider(dir).ReadManifest(); err != nil {
		t.Errorf("a manifest at the limit should be read: %v", err)
	}
}

func TestOpenReadsLayer(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "layers"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("fake-png-bytes")
	if err := os.WriteFile(filepath.Join(dir, "layers", "bg.png"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	rc, err := NewFSAssetProvider(dir).Open("layers/bg.png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != string(want) {
		t.Errorf("read %q, want %q", got, want)
	}
}

func TestOpenRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	// A secret sitting beside the asset root, reachable only via traversal
	secret := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(secret) })

	p := NewFSAssetProvider(dir)
	if _, err := p.Open("../secret.txt"); err == nil {
		t.Fatal("expected traversal to be rejected, got nil error")
	}
}
