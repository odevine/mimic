package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

// overlay serves a manifest and some files of its own over another provider's,
// which may be nil
type overlay struct {
	template.AssetProvider
	manifest []byte
	files    map[string][]byte
}

func (o overlay) ReadManifest() ([]byte, error) {
	if o.manifest != nil {
		return o.manifest, nil
	}
	if o.AssetProvider == nil {
		return nil, errors.New("overlay has no manifest")
	}
	return o.AssetProvider.ReadManifest()
}

func (o overlay) Open(rel string) (io.ReadCloser, error) {
	if b, ok := o.files[rel]; ok {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	if o.AssetProvider == nil {
		return nil, errors.New("overlay has no file " + rel)
	}
	return o.AssetProvider.Open(rel)
}

// readManifest reads a provider's manifest back into the structure a test edits
func readManifest(t *testing.T, p template.AssetProvider) manifest.Manifest {
	t.Helper()
	raw, err := p.ReadManifest()
	if err != nil {
		t.Fatal(err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func marshal(t *testing.T, m manifest.Manifest) []byte {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// withManifest is a provider that serves m over base's files
func withManifest(t *testing.T, base template.AssetProvider, m manifest.Manifest, files map[string][]byte) template.AssetProvider {
	t.Helper()
	return overlay{AssetProvider: base, manifest: marshal(t, m), files: files}
}

// mustLoad loads the template a provider holds, and fails the test on an error
func mustLoad(t *testing.T, p template.AssetProvider) *template.Template {
	t.Helper()
	tmpl, _, err := template.Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return tmpl
}

// placeholderTemplate is the generated placeholder frame every test reads
func placeholderTemplate(t *testing.T) *template.Template {
	t.Helper()
	return mustLoad(t, PlaceholderAssets())
}

// testManifest wraps a layout and its parts into a template that serves the
// shapes, on the placeholder's canvas unless the layout gives its own
func testManifest(layout manifest.Layout, parts map[string]manifest.Part, serves ...frame.Shape) manifest.Manifest {
	if layout.Canvas.Width == 0 {
		layout.Canvas = manifest.Canvas{Width: placeholderWidth, Height: placeholderHeight}
	}
	if len(serves) == 0 {
		serves = []frame.Shape{{Role: frame.RoleSingle, Kind: frame.KindStandard}}
	}
	layout.Serves = serves
	if layout.TextBoxes == nil {
		layout.TextBoxes = map[string]manifest.TextBox{}
	}
	return manifest.Manifest{
		Contract: manifest.Contract, ID: "mimic.test", Name: "Test", Description: "A test frame",
		Parts:   parts,
		Layouts: map[string]manifest.Layout{"test": layout},
	}
}

// writeManifestFile writes m as the manifest of a template folder
func writeManifestFile(t *testing.T, dir string, m manifest.Manifest) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), marshal(t, m), 0o644); err != nil {
		t.Fatal(err)
	}
}

// any is the one image of a part that does not depend on the card
func anyImage(path string) map[string]manifest.Variant {
	return map[string]manifest.Variant{"any": {Path: path}}
}

// box is a text box of the given size drawn in black
func box(x, y, w, h int, size float64) manifest.TextBox {
	return manifest.TextBox{
		Rect: manifest.Rect{X: x, Y: y, Width: w, Height: h},
		Text: manifest.TextStyle{FontSize: size, Color: "#000000"},
	}
}

// when builds a text box override from the JSON of the parts it changes
func when(cond, patch string) manifest.Override {
	return manifest.Override{Condition: cond, Patch: json.RawMessage(patch)}
}

// writeFlat writes a document-sized PNG with fill painted into rect
func writeFlat(path string, rect image.Rectangle, fill color.NRGBA) error {
	return os.WriteFile(path, flat(rect, fill), 0o644)
}
