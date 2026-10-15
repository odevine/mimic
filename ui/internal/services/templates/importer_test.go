package templates

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/templatetest"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

// bundleFile writes a built template as a file the person could have chosen
func bundleFile(t *testing.T, spec templatetest.Spec) string {
	t.Helper()
	data, _ := spec.Bundle(t)
	path := filepath.Join(t.TempDir(), "chosen.mimic")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// headed is a bundle's bytes with the id and version in its header changed, for the ones the
// engine would not have built
func headed(t *testing.T, data []byte, id, version string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(r)
		r.Close()
		if f.Name == "bundle.json" {
			var h map[string]any
			if err := json.Unmarshal(raw, &h); err != nil {
				t.Fatal(err)
			}
			h["id"], h["version"] = id, version
			raw, _ = json.Marshal(h)
		}
		if _, err := w.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "odd.mimic")
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportInstallsABundleBesideTheOthers(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	path := bundleFile(t, templatetest.Spec{ID: "jane.pinlines", Version: "1.2.0", Name: "Jane's pinlines"})
	res, err := svc.Import(ImportRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if res.Template != "jane.pinlines" || res.Version != "1.2.0" || res.Title != "Jane's pinlines" || res.Same || res.Replaced || res.Active {
		t.Errorf("result = %+v", res)
	}
	if !catalog.IsCached("jane.pinlines", "1.2.0") {
		t.Fatal("the bundle is not where installed templates are read from")
	}
	// It is listed in the library with the version cached and nothing active
	var found *TemplateView
	views := svc.List(t.Context())
	for i := range views {
		if views[i].Name == "jane.pinlines" {
			found = &views[i]
		}
	}
	if found == nil || len(found.Versions) != 1 || !found.Versions[0].Cached || found.Versions[0].Active {
		t.Fatalf("library row = %+v", found)
	}
	// The file chosen is left where it was, and no half-made copy is left in the cache
	if _, err := os.Stat(path); err != nil {
		t.Error("the chosen file is gone")
	}
	dest, _ := catalog.BundlePath("jane.pinlines", "1.2.0")
	entries, _ := os.ReadDir(filepath.Dir(dest))
	if len(entries) != 1 {
		t.Errorf("the template's folder holds %d files, want the bundle alone", len(entries))
	}
}

func TestImportOfTheSameFileAgainWritesNothing(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	path := bundleFile(t, templatetest.Spec{ID: "jane.pinlines", Version: "1.2.0"})
	if _, err := svc.Import(ImportRequest{Path: path}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Import(ImportRequest{Path: path})
	if err != nil || !res.Same || res.Replaced {
		t.Errorf("second import = %+v, %v, want the same file said so", res, err)
	}
}

func TestImportOfADifferentBundleOfAnInstalledVersionAsksBeforeReplacing(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	first := bundleFile(t, templatetest.Spec{ID: "jane.pinlines", Version: "1.2.0", Name: "First"})
	second := bundleFile(t, templatetest.Spec{ID: "jane.pinlines", Version: "1.2.0", Name: "Second"})
	if _, err := svc.Import(ImportRequest{Path: first}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Import(ImportRequest{Path: second})
	if err != nil || !res.Exists || res.Replaced || res.Title != "Second" {
		t.Fatalf("import of a different bundle = %+v, %v, want it said to exist and left alone", res, err)
	}
	dest, _ := catalog.BundlePath("jane.pinlines", "1.2.0")
	before, _ := os.ReadFile(first)
	if now, _ := os.ReadFile(dest); string(now) != string(before) {
		t.Error("the installed bundle was changed without being asked")
	}
	res, err = svc.Import(ImportRequest{Path: second, Replace: true})
	if err != nil || !res.Replaced || res.Exists || res.Title != "Second" {
		t.Errorf("replace = %+v, %v", res, err)
	}
	if now, _ := os.ReadFile(dest); string(now) == string(before) {
		t.Error("the bundle was not replaced")
	}
}

func TestImportRefusesWhatCannotBeKept(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	notZip := filepath.Join(t.TempDir(), "notes.mimic")
	if err := os.WriteFile(notZip, []byte("not a bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := templatetest.Spec{ID: "jane.pinlines", Version: "1.0.0"}.Bundle(t)
	odd := func(id, version string) string { return headed(t, data, id, version) }
	tests := []struct {
		name string
		req  ImportRequest
		want string
	}{
		{"no file", ImportRequest{}, "Choose a template file"},
		{"a missing file", ImportRequest{Path: filepath.Join(t.TempDir(), "gone.mimic")}, "not a template this version"},
		{"a file that is not a bundle", ImportRequest{Path: notZip}, "not a template this version"},
		{"an id with no owner", ImportRequest{Path: odd("pinlines", "1.0.0")}, "is not one the app can keep"},
		{"an id that leaves the cache", ImportRequest{Path: odd("../x.y", "1.0.0")}, "is not one the app can keep"},
		{"a version the cache cannot name", ImportRequest{Path: odd("jane.pinlines", "../1")}, "is not one the app can keep"},
		{"the draft version", ImportRequest{Path: odd("jane.pinlines", "draft")}, "is not one the app can keep"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Import(tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one saying %q", err, tt.want)
			}
			if apierr.KindOf(err) != apierr.BadRequest {
				t.Errorf("kind = %v, want a bad request", apierr.KindOf(err))
			}
		})
	}
}

func TestImportOfTheActiveTemplateOpensItAgain(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	first := bundleFile(t, templatetest.Spec{ID: "jane.pinlines", Version: "1.2.0", Name: "First"})
	if _, err := svc.Import(ImportRequest{Path: first}); err != nil {
		t.Fatal(err)
	}
	at, err := pipeline.FromVersion(t.Context(), "jane.pinlines", "1.2.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetActiveTemplate(at)
	second := bundleFile(t, templatetest.Spec{ID: "jane.pinlines", Version: "1.2.0", Name: "Second"})
	res, err := svc.Import(ImportRequest{Path: second, Replace: true})
	if err != nil || !res.Active || !res.Replaced {
		t.Fatalf("replace of the active template = %+v, %v", res, err)
	}
	if name, ver := ws.Active(); name != "jane.pinlines" || ver != "1.2.0" {
		t.Errorf("active = %s %s after the replace", name, ver)
	}
}
