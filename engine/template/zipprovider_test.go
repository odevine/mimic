package template

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

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

const testManifestJSON = `{
	"template": "normal",
	"width": 744,
	"height": 1039,
	"layers": [
		{"name": "background", "colorVariants": {"any": {"path": "background/any.png"}}}
	],
	"art": {"x": 5, "y": 5, "width": 200, "height": 150, "after": "background"}
}`

// goodBundle is a well-formed bundle for the happy-path cases
func goodBundle(t *testing.T, dir string) string {
	return writeBundle(t, dir, []bundleEntry{
		{"bundle.json", zip.Deflate, `{"format":1,"template":"normal","version":"0.1.0","minEngine":"0.3.0"}`},
		{"manifest.json", zip.Deflate, testManifestJSON},
		{"background/any.png", zip.Store, "fake-png-bytes"},
	})
}

func TestZipManifestParsesAndValidates(t *testing.T) {
	p, err := NewZipAssetProvider(goodBundle(t, t.TempDir()))
	if err != nil {
		t.Fatalf("NewZipAssetProvider: %v", err)
	}
	defer p.Close()

	m, err := p.Manifest()
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if m.Template != "normal" || m.Width != 744 || m.Height != 1039 {
		t.Errorf("header = %q %dx%d, want normal 744x1039", m.Template, m.Width, m.Height)
	}
	if len(m.Layers) != 1 || m.Layers[0].ColorVariants["any"].Path != "background/any.png" {
		t.Fatalf("layers not parsed: %+v", m.Layers)
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

func TestZipRejectsNewerFormat(t *testing.T) {
	path := writeBundle(t, t.TempDir(), []bundleEntry{
		{"bundle.json", zip.Deflate, `{"format":2,"template":"normal","version":"0.1.0","minEngine":"0.3.0"}`},
		{"manifest.json", zip.Deflate, testManifestJSON},
	})
	if _, err := NewZipAssetProvider(path); err == nil {
		t.Fatal("expected a newer format to be rejected, got nil error")
	}
}

func TestZipRejectsMinEngineTooHigh(t *testing.T) {
	// A stamped running version older than the bundle requires must be rejected.
	// dev and empty always pass the gate, so the test needs a concrete version
	old := version.Version
	version.Version = "0.3.0"
	defer func() { version.Version = old }()

	path := writeBundle(t, t.TempDir(), []bundleEntry{
		{"bundle.json", zip.Deflate, `{"format":1,"template":"normal","version":"2.0.0","minEngine":"0.4.0"}`},
		{"manifest.json", zip.Deflate, testManifestJSON},
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
	if err := os.WriteFile(filepath.Join(loose, "manifest.json"), []byte(testManifestJSON), 0o644); err != nil {
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

	fsM, err := fs.Manifest()
	if err != nil {
		t.Fatalf("FS Manifest: %v", err)
	}
	zpM, err := zp.Manifest()
	if err != nil {
		t.Fatalf("Zip Manifest: %v", err)
	}
	if fsM.Template != zpM.Template || fsM.Width != zpM.Width || fsM.Height != zpM.Height ||
		len(fsM.Layers) != len(zpM.Layers) {
		t.Errorf("manifests differ: fs=%+v zip=%+v", fsM, zpM)
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
