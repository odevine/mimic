package author

import (
	"bytes"
	"testing"
	"testing/fstest"

	"github.com/odevine/mimic/engine/template"
)

// asFS holds extracted files as a folder
func asFS(files map[string][]byte) fstest.MapFS {
	out := fstest.MapFS{}
	for name, data := range files {
		out[name] = &fstest.MapFile{Data: data}
	}
	return out
}

func extractAndRebuild(t *testing.T, fsys fstest.MapFS, opt Options, eo ExtractOptions) (first, second *Built, files map[string][]byte) {
	t.Helper()
	first, ds := Build(fsys, opt)
	if first == nil {
		t.Fatal(ds)
	}
	files, ds, err := Extract(template.NewMemoryAssets(first.ManifestJSON, first.Assets), eo)
	if err != nil {
		t.Fatalf("%v: %v", err, ds)
	}
	second, ds = Build(asFS(files), opt)
	if second == nil {
		t.Fatalf("the extracted folder does not build: %v", ds)
	}
	return first, second, files
}

func TestExtractBuildsBackToTheSameTemplate(t *testing.T) {
	first, second, files := extractAndRebuild(t, smallFS(t), Options{}, ExtractOptions{Version: "1.2.3"})
	if !bytes.Equal(first.ManifestJSON, second.ManifestJSON) {
		t.Errorf("manifest differs\nfirst:  %s\nsecond: %s", first.ManifestJSON, second.ManifestJSON)
	}
	if len(first.Assets) != len(second.Assets) {
		t.Fatalf("%d assets, want %d", len(second.Assets), len(first.Assets))
	}
	for name, data := range first.Assets {
		if !bytes.Equal(second.Assets[name], data) {
			t.Errorf("asset %s differs", name)
		}
	}
	for _, want := range []string{"template.json", "layouts/normal.json", "parts/bg/w.png", "parts/bg/u.png", "parts/mark/any.png"} {
		if _, ok := files[want]; !ok {
			t.Errorf("no file %s in %v", want, keys(files))
		}
	}
}

func TestExtractKeepsFontsAndRoles(t *testing.T) {
	fsys := withFonts(t, `"script": {"file": "font/Script.otf"}`, sagaRole)
	fsys["font/Script.otf"] = &fstest.MapFile{Data: gothamBytes(t)}
	first, second, files := extractAndRebuild(t, fsys, Options{}, ExtractOptions{Version: "1.2.3", MinEngine: "0.1.0"})
	if !bytes.Equal(first.ManifestJSON, second.ManifestJSON) {
		t.Errorf("manifest differs\nfirst:  %s\nsecond: %s", first.ManifestJSON, second.ManifestJSON)
	}
	if !bytes.Equal(files["font/Script.otf"], gothamBytes(t)) {
		t.Error("the font was not extracted at its path")
	}
}

func TestExtractedFolderPassesStrictValidation(t *testing.T) {
	_, _, files := extractAndRebuild(t, smallFS(t), Options{}, ExtractOptions{Version: "1.2.3"})
	if ds := Validate(asFS(files), Options{}); ds.HasErrors() {
		t.Fatal(ds)
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
