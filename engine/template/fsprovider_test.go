package template

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
}

func TestManifestParsesAndValidates(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `{
		"template": "normal",
		"width": 744,
		"height": 1039,
		"layers": [
			{"name": "background", "colorVariants": {"any": {"path": "layers/bg.png"}}},
			{"name": "pinlines", "condition": "legendary", "colorVariants": {"r": {"path": "layers/r.png"}}, "blend": "multiply"}
		],
		"textBoxes": {"title": {"x": 10, "y": 20, "width": 100, "height": 30, "fontSize": 24, "align": "left", "color": "#000000"}},
		"art": {"x": 5, "y": 5, "width": 200, "height": 150, "after": "background"}
	}`)

	p := NewFSAssetProvider(dir)
	m, err := p.Manifest()
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if m.Template != "normal" || m.Width != 744 || m.Height != 1039 {
		t.Errorf("header = %q %dx%d, want normal 744x1039", m.Template, m.Width, m.Height)
	}
	if len(m.Layers) != 2 || m.Layers[0].Name != "background" {
		t.Fatalf("layers not parsed in order: %+v", m.Layers)
	}
	if m.Layers[1].Condition != "legendary" || m.Layers[1].Blend != "multiply" {
		t.Errorf("layer 1 = %+v, want legendary/multiply", m.Layers[1])
	}
	if got := m.Layers[0].ColorVariants["any"].Path; got != "layers/bg.png" {
		t.Errorf("bg path = %q, want layers/bg.png", got)
	}
	if m.Art.After != "background" {
		t.Errorf("art.after = %q, want background", m.Art.After)
	}
}

func TestManifestRejectsOversizedDimensions(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `{"template":"x","width":50000,"height":10,"layers":[]}`)
	if _, err := NewFSAssetProvider(dir).Manifest(); err == nil {
		t.Fatal("expected error for oversized dimensions, got nil")
	}
}

func TestManifestRejectsNonPositiveDimensions(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `{"template":"x","width":0,"height":10,"layers":[]}`)
	if _, err := NewFSAssetProvider(dir).Manifest(); err == nil {
		t.Fatal("expected error for zero width, got nil")
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
