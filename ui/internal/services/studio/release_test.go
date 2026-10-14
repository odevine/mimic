package studio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
)

func TestReleaseNamesWhatTheBundleWouldBeAndSaysWhenItIsReady(t *testing.T) {
	svc, _, _ := newService(t)
	sum, err := svc.Create(CreateRequest{Name: "My frame"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.Release(sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.TemplateID != "user.my-frame" || r.Name != "My frame" || r.Version != "0.1.0" || r.Filename != "user.my-frame-0.1.0.mimic" {
		t.Fatalf("release = %+v", r)
	}
	if !r.Ready || r.Installed || r.Active || r.Credits == nil {
		t.Fatalf("a new template is ready, not yet in use, and has an empty list of credits: %+v", r)
	}
}

func TestReleaseOfATemplateThatDoesNotBuildIsNotReady(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "Broken"})
	if err := os.WriteFile(filepath.Join(dir, sum.ID, "template.json"), []byte(`{"id":"user.broken"`), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Release(sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ready || len(r.Diagnostics) == 0 {
		t.Fatalf("release = %+v", r)
	}
	if _, err := svc.Use(sum.ID); err == nil || apierr.KindOf(err) != apierr.BadRequest {
		t.Fatalf("using it = %v", err)
	}
	if _, err := svc.Export(ExportRequest{ID: sum.ID, Path: filepath.Join(t.TempDir(), "x.mimic")}); err == nil {
		t.Fatal("a template that does not build was exported")
	}
}

func TestUseMakesTheDraftAnInstalledTemplateAtTheDraftVersion(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "My frame"})
	res, err := svc.Use(sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Template != "user.my-frame" || res.Version != pipeline.DraftVersion || res.Replaced || res.Active {
		t.Fatalf("result = %+v", res)
	}
	if !catalog.IsCached("user.my-frame", "draft") {
		t.Fatal("the bundle is not where installed templates are read from")
	}
	in, ok := pipeline.InstalledInfo("user.my-frame")
	if !ok || in.Name != "My frame" {
		t.Fatalf("installed = %+v, %v", in, ok)
	}
	if v := pipeline.InstalledVersions("user.my-frame"); len(v) != 1 || v[0] != "draft" {
		t.Fatalf("versions = %v", v)
	}
	at, err := pipeline.FromVersion(t.Context(), "user.my-frame", "draft", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer at.Close()
	if at.Name != "user.my-frame" || at.Version != "draft" {
		t.Fatalf("opened %s %s", at.Name, at.Version)
	}
	if r, _ := svc.Release(sum.ID); !r.Installed {
		t.Fatal("release does not say it is in use")
	}
}

func TestUsingADraftAgainReplacesTheBundleAndReopensTheTemplateInUse(t *testing.T) {
	svc, ws, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "My frame"})
	if _, err := svc.Use(sum.ID); err != nil {
		t.Fatal(err)
	}
	at, err := pipeline.FromVersion(t.Context(), "user.my-frame", "draft", nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetActiveTemplate(at)
	if _, err := svc.Rename(RenameRequest{ID: sum.ID, Name: "My better frame"}); err != nil {
		t.Fatal(err)
	}
	if got := ws.Pipe.Title("user.my-frame"); got != "My frame" {
		t.Fatalf("the template in use changed before it was used again: %q", got)
	}
	res, err := svc.Use(sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Replaced || !res.Active {
		t.Fatalf("result = %+v", res)
	}
	if got := ws.Pipe.Title("user.my-frame"); got != "My better frame" {
		t.Fatalf("the template in use is still %q", got)
	}
	if r, _ := svc.Release(sum.ID); !r.Active {
		t.Fatal("release does not say it is the template in use")
	}
}

func TestUseIsRefusedWhileARunIsInProgress(t *testing.T) {
	svc, ws, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "My frame"})
	ws.Run.Set(batch.New(nil, batch.Options{ID: "run-1"}), nil)
	if _, err := svc.Use(sum.ID); err == nil || apierr.KindOf(err) != apierr.Conflict {
		t.Fatalf("using it = %v", err)
	}
}

func TestDeletingADraftTakesBackTheTemplateItGave(t *testing.T) {
	svc, ws, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "My frame"})
	if _, err := svc.Use(sum.ID); err != nil {
		t.Fatal(err)
	}
	at, _ := pipeline.FromVersion(t.Context(), "user.my-frame", "draft", nil)
	ws.SetActiveTemplate(at)
	if err := svc.Delete(sum.ID); err != nil {
		t.Fatal(err)
	}
	if catalog.IsCached("user.my-frame", "draft") {
		t.Fatal("the bundle was left in the cache")
	}
	if _, ok := pipeline.InstalledInfo("user.my-frame"); ok {
		t.Fatal("the template is still installed")
	}
	if name, _ := ws.Active(); name == "user.my-frame" {
		t.Fatal("the deleted template is still the one in use")
	}
}

func TestExportWritesABundleTheAppCanOpenAndGivesItsExtension(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "My frame"})
	out := filepath.Join(t.TempDir(), "mine")
	res, err := svc.Export(ExportRequest{ID: sum.ID, Path: out})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != out+".mimic" || res.Bytes <= 0 {
		t.Fatalf("result = %+v", res)
	}
	zp, err := template.NewZipAssetProvider(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer zp.Close()
	tm, _, err := template.Load(zp)
	if err != nil {
		t.Fatal(err)
	}
	if tm.ID() != "user.my-frame" || zp.Header().Version != "0.1.0" {
		t.Fatalf("bundle is %s %s", tm.ID(), zp.Header().Version)
	}
	entries, _ := os.ReadDir(filepath.Dir(res.Path))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".bundle-") {
			t.Fatalf("a temporary file was left: %s", e.Name())
		}
	}
}

func TestExportRefusesAFolderAndNoPath(t *testing.T) {
	svc, _, _ := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "My frame"})
	if _, err := svc.Export(ExportRequest{ID: sum.ID, Path: ""}); err == nil {
		t.Fatal("no path was accepted")
	}
	dir := filepath.Join(t.TempDir(), "x.mimic")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Export(ExportRequest{ID: sum.ID, Path: dir}); err == nil {
		t.Fatal("a folder was accepted")
	}
}

func TestShowFolderOpensTheDraftsOwnFolderAndNothingElse(t *testing.T) {
	svc, _, dir := newService(t)
	sum, _ := svc.Create(CreateRequest{Name: "My frame"})
	var got string
	svc.show = func(p string) error { got = p; return nil }
	if err := svc.ShowFolder(sum.ID); err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, sum.ID) {
		t.Fatalf("opened %q", got)
	}
	if err := svc.ShowFolder("../elsewhere"); err == nil {
		t.Fatal("a path outside the drafts was opened")
	}
}
