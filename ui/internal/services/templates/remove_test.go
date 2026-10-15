package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/templatetest"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

func install(t *testing.T, svc *Service, id, version string) {
	t.Helper()
	if _, err := svc.Import(ImportRequest{Path: bundleFile(t, templatetest.Spec{ID: id, Version: version})}); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveDeletesAVersionAndTheFolderOfItsLast(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	install(t, svc, "jane.pinlines", "1.0.0")
	install(t, svc, "jane.pinlines", "1.1.0")
	res, err := svc.Remove(RemoveRequest{Template: "jane.pinlines", Version: "1.0.0"})
	if err != nil || res.Active || res.Template != "jane.pinlines" || res.Version != "1.0.0" {
		t.Fatalf("Remove = %+v, %v", res, err)
	}
	if catalog.IsCached("jane.pinlines", "1.0.0") || !catalog.IsCached("jane.pinlines", "1.1.0") {
		t.Fatal("the wrong version was taken out of the cache")
	}
	if _, err := svc.Remove(RemoveRequest{Template: "jane.pinlines", Version: "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	dest, _ := catalog.BundlePath("jane.pinlines", "1.1.0")
	if _, err := os.Stat(filepath.Dir(dest)); !os.IsNotExist(err) {
		t.Errorf("the template's folder is still there after its last version: %v", err)
	}
	for _, tv := range svc.List(t.Context()) {
		if tv.Name == "jane.pinlines" {
			t.Errorf("a template with no version left is still listed: %+v", tv)
		}
	}
}

func TestRemoveOfTheActiveVersionSwitchesToAnotherOfTheSameTemplate(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	install(t, svc, "jane.pinlines", "1.0.0")
	install(t, svc, "jane.pinlines", "1.1.0")
	at, err := pipeline.FromVersion(t.Context(), "jane.pinlines", "1.1.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetActiveTemplate(at)
	res, err := svc.Remove(RemoveRequest{Template: "jane.pinlines", Version: "1.1.0"})
	if err != nil || !res.Active || res.Now != "jane.pinlines" {
		t.Fatalf("Remove = %+v, %v", res, err)
	}
	if name, ver := ws.Active(); name != "jane.pinlines" || ver != "1.0.0" {
		t.Errorf("the app draws with %s %s, want the version that is left", name, ver)
	}
}

func TestRemoveOfTheOnlyActiveVersionFallsBackToWhatTheAppStartsWith(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	install(t, svc, "jane.pinlines", "1.0.0")
	at, err := pipeline.FromVersion(t.Context(), "jane.pinlines", "1.0.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetActiveTemplate(at)
	res, err := svc.Remove(RemoveRequest{Template: "jane.pinlines", Version: "1.0.0"})
	if err != nil || !res.Active || res.Now == "jane.pinlines" || res.Now == "" {
		t.Fatalf("Remove = %+v, %v, want the app to have switched to another template", res, err)
	}
	if name, _ := ws.Active(); name == "jane.pinlines" {
		t.Error("the app still draws with the template that was removed")
	}
}

func TestRemoveOfTheStartingTemplateWithNothingElseLeavesThePlaceholder(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	install(t, svc, pipeline.DefaultTemplate, "1.0.0")
	at, err := pipeline.FromVersion(t.Context(), pipeline.DefaultTemplate, "1.0.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetActiveTemplate(at)
	if _, err := svc.Remove(RemoveRequest{Template: pipeline.DefaultTemplate, Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if name, _ := ws.Active(); name == pipeline.DefaultTemplate {
		t.Error("the app still draws with the bundle that was deleted")
	}
	if got := svc.Active(); got.Source != "placeholder" {
		t.Errorf("source = %q, want placeholder", got.Source)
	}
}

func TestRemoveListsWhatCanBeRemovedAndRestored(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	install(t, svc, "jane.pinlines", "1.0.0")
	var views []VersionView
	for _, tv := range svc.List(t.Context()) {
		if tv.Name == "jane.pinlines" {
			views = tv.Versions
		}
	}
	if len(views) != 1 || !views[0].Removable || views[0].Restorable {
		t.Fatalf("an imported version = %+v, want it removable and not restorable", views)
	}
	// The developer folder the small workspace starts on is not the app's to delete
	for _, tv := range svc.List(t.Context()) {
		for _, v := range tv.Versions {
			if v.Version == pipeline.LocalVersion && v.Removable {
				t.Errorf("%s: a developer folder is offered for removal", tv.Name)
			}
		}
	}
}

func TestRemoveRefusesWhatIsNotAnInstalledVersion(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	svc := New(ws)
	install(t, svc, "jane.pinlines", "1.0.0")
	tests := []struct {
		name string
		req  RemoveRequest
		kind apierr.Kind
	}{
		{"not an id", RemoveRequest{Template: "../jane", Version: "1.0.0"}, apierr.BadRequest},
		{"the developer folder", RemoveRequest{Template: "jane.pinlines", Version: pipeline.LocalVersion}, apierr.BadRequest},
		{"a path as the version", RemoveRequest{Template: "jane.pinlines", Version: "../1.0.0"}, apierr.BadRequest},
		{"no version", RemoveRequest{Template: "jane.pinlines"}, apierr.BadRequest},
		{"a version that is not installed", RemoveRequest{Template: "jane.pinlines", Version: "9.9.9"}, apierr.NotFound},
		{"a template that is not installed", RemoveRequest{Template: "jane.other", Version: "1.0.0"}, apierr.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.Remove(tt.req); apierr.KindOf(err) != tt.kind {
				t.Errorf("Remove(%+v) = %v, want kind %v", tt.req, err, tt.kind)
			}
		})
	}
	if !catalog.IsCached("jane.pinlines", "1.0.0") {
		t.Error("a refused removal deleted the bundle")
	}
	ws.Run.Set(batch.New(nil, batch.Options{ID: "run-1"}), nil)
	if _, err := svc.Remove(RemoveRequest{Template: "jane.pinlines", Version: "1.0.0"}); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("a removal mid-run: %v, want a conflict", err)
	}
	if !catalog.IsCached("jane.pinlines", "1.0.0") {
		t.Error("a removal mid-run deleted the bundle")
	}
}
