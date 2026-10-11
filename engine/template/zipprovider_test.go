package template

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/version"
)

// bundleEntry is one file to place in a test .mimic
type bundleEntry struct {
	name   string
	method uint16
	body   string
}

// writeBundle builds a .mimic in dir from entries and returns its path. It
// mirrors the producer layout: JSON deflated, PNGs stored
func writeBundle(t *testing.T, dir string, entries []bundleEntry) string {
	t.Helper()
	path := filepath.Join(dir, "test.mimic")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating bundle: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: e.method})
		if err != nil {
			t.Fatalf("adding %q: %v", e.name, err)
		}
		if _, err := io.WriteString(w, e.body); err != nil {
			t.Fatalf("writing %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing bundle: %v", err)
	}
	return path
}

// minimalManifestJSON is the smallest template that loads: one part, one layout
const minimalManifestJSON = `{
	"contract": "1.0",
	"id": "mimic.minimal",
	"name": "Minimal",
	"description": "The least a template needs",
	"parts": {"background": {"variants": {"any": {"path": "background/any.png"}}}},
	"layouts": {"normal": {
		"serves": [{"role": "single", "kind": "standard"}],
		"canvas": {"width": 744, "height": 1039},
		"stack": [{"part": "background"}],
		"textBoxes": {},
		"art": [{"x": 5, "y": 5, "width": 200, "height": 150, "after": "background"}]
	}}
}`

// goodBundle is a well-formed bundle for the happy-path cases
func goodBundle(t *testing.T, dir string) string {
	return writeBundle(t, dir, []bundleEntry{
		{"bundle.json", zip.Deflate, `{"format":2,"id":"mimic.minimal","version":"0.1.0","minEngine":"0.3.0"}`},
		{"manifest.json", zip.Deflate, minimalManifestJSON},
		{"background/any.png", zip.Store, "fake-png-bytes"},
	})
}

func TestZipReadManifestLoads(t *testing.T) {
	p, err := NewZipAssetProvider(goodBundle(t, t.TempDir()))
	if err != nil {
		t.Fatalf("NewZipAssetProvider: %v", err)
	}
	defer p.Close()

	raw, err := p.ReadManifest()
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if string(raw) != minimalManifestJSON {
		t.Errorf("ReadManifest = %q, want the bundle's manifest", raw)
	}
	tmpl, ds, err := Load(p)
	if err != nil || len(ds) != 0 {
		t.Fatalf("Load = %v, %v", ds, err)
	}
	if tmpl.ID() != "mimic.minimal" {
		t.Errorf("id = %q, want mimic.minimal", tmpl.ID())
	}
}

func TestZipRefusesAManifestOverTheLimit(t *testing.T) {
	path := writeBundle(t, t.TempDir(), []bundleEntry{
		{"bundle.json", zip.Deflate, `{"format":2,"id":"mimic.minimal","version":"0.1.0","minEngine":"0.3.0"}`},
		// Spaces deflate to almost nothing, so the file is small and what it claims to hold is not
		{"manifest.json", zip.Deflate, strings.Repeat(" ", manifest.MaxBytes+10)},
	})
	p, err := NewZipAssetProvider(path)
	if err != nil {
		t.Fatalf("NewZipAssetProvider: %v", err)
	}
	defer p.Close()
	if _, err := p.ReadManifest(); err == nil {
		t.Fatal("expected an oversized manifest to be refused, got nil error")
	}
}

func TestZipOpenReadsLayer(t *testing.T) {
	p, err := NewZipAssetProvider(goodBundle(t, t.TempDir()))
	if err != nil {
		t.Fatalf("NewZipAssetProvider: %v", err)
	}
	defer p.Close()

	rc, err := p.Open("background/any.png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "fake-png-bytes" {
		t.Errorf("read %q, want fake-png-bytes", got)
	}
}

func TestZipOpenRejectsTraversal(t *testing.T) {
	p, err := NewZipAssetProvider(goodBundle(t, t.TempDir()))
	if err != nil {
		t.Fatalf("NewZipAssetProvider: %v", err)
	}
	defer p.Close()

	if _, err := p.Open("../secret.txt"); err == nil {
		t.Fatal("expected traversal to be rejected, got nil error")
	}
}

func TestZipRejectsAFormatItDoesNotRead(t *testing.T) {
	for _, format := range []int{1, 3} {
		path := writeBundle(t, t.TempDir(), []bundleEntry{
			{"bundle.json", zip.Deflate, fmt.Sprintf(`{"format":%d,"id":"mimic.minimal","version":"0.1.0","minEngine":"0.3.0"}`, format)},
			{"manifest.json", zip.Deflate, minimalManifestJSON},
		})
		if _, err := NewZipAssetProvider(path); err == nil {
			t.Errorf("format %d was accepted", format)
		}
	}
}

func TestZipRejectsMinEngineTooHigh(t *testing.T) {
	// A stamped running version older than the bundle requires must be rejected.
	// dev and empty always pass the gate, so the test needs a concrete version
	old := version.Version
	version.Version = "0.3.0"
	defer func() { version.Version = old }()

	path := writeBundle(t, t.TempDir(), []bundleEntry{
		{"bundle.json", zip.Deflate, `{"format":2,"id":"mimic.minimal","version":"2.0.0","minEngine":"0.4.0"}`},
		{"manifest.json", zip.Deflate, minimalManifestJSON},
	})
	if _, err := NewZipAssetProvider(path); err == nil {
		t.Fatal("expected minEngine gate to reject, got nil error")
	}
}

// TestZipMatchesFSProvider asserts a bundle and a loose directory built from the
// same manifest and layer bytes return identical results, so the two providers
// cannot drift. Full render parity is verified against the live normal.mimic
func TestZipMatchesFSProvider(t *testing.T) {
	dir := t.TempDir()

	// Loose directory
	loose := filepath.Join(dir, "loose")
	if err := os.MkdirAll(filepath.Join(loose, "background"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loose, "manifest.json"), []byte(minimalManifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loose, "background", "any.png"), []byte("fake-png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs := NewFSAssetProvider(loose)
	zp, err := NewZipAssetProvider(goodBundle(t, dir))
	if err != nil {
		t.Fatalf("NewZipAssetProvider: %v", err)
	}
	defer zp.Close()

	fsM, err := fs.ReadManifest()
	if err != nil {
		t.Fatalf("FS ReadManifest: %v", err)
	}
	zpM, err := zp.ReadManifest()
	if err != nil {
		t.Fatalf("Zip ReadManifest: %v", err)
	}
	if !bytes.Equal(fsM, zpM) {
		t.Errorf("manifests differ: fs=%q zip=%q", fsM, zpM)
	}

	fsBytes := readAll(t, fs, "background/any.png")
	zpBytes := readAll(t, zp, "background/any.png")
	if !bytes.Equal(fsBytes, zpBytes) {
		t.Errorf("layer bytes differ: fs=%q zip=%q", fsBytes, zpBytes)
	}
}

func readAll(t *testing.T, p AssetProvider, relPath string) []byte {
	t.Helper()
	rc, err := p.Open(relPath)
	if err != nil {
		t.Fatalf("Open %q: %v", relPath, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read %q: %v", relPath, err)
	}
	return b
}
