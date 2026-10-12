package templates

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/catalog"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/templatetest"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
)

// The ids of the templates the tests install beside the placeholder
const (
	transformID = "test.transform"
	standardID  = "test.standard"
)

func TestTemplateSource(t *testing.T) {
	if got := templateSource(render.PlaceholderID, ""); got != "placeholder" {
		t.Errorf("empty version: %q, want placeholder", got)
	}
	if got := templateSource("test.frame", pipeline.LocalVersion); got != "local" {
		t.Errorf("local version: %q, want local", got)
	}
}

func TestActiveReportsTheShapesItRenders(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	got := svc.Active()
	if !slices.Contains(got.Shapes, pipeline.PrimaryShape) {
		t.Errorf("active template = %+v", got)
	}
}

func TestChooseTemplatePerFace(t *testing.T) {
	ws := workspacetest.NewWithTransform(t)
	front := template.Shape{Role: template.RoleTransformFront, Kind: template.KindStandard}
	if c, ok := ws.Pipe.Choose(pipeline.PrimaryShape); !ok || c.Name != render.PlaceholderID {
		t.Errorf("single standard chose %+v, %v, want the active template", c, ok)
	}
	if c, ok := ws.Pipe.Choose(front); !ok || c.Name != transformID {
		t.Errorf("transform front chose %+v, %v, want transform", c, ok)
	}
	walker := template.Shape{Role: template.RoleTransformBack, Kind: template.KindPlaneswalker}
	if _, ok := ws.Pipe.Choose(walker); ok {
		t.Error("a planeswalker back has no template, but one was chosen")
	}
	// A preference for a template that does not render the shape is ignored
	ws.Prefs.SetFaceTemplate(pipeline.ShapeKey(front), prefs.TemplateChoice{Name: render.PlaceholderID})
	if c, _ := ws.Pipe.Choose(front); c.Name != transformID {
		t.Errorf("an unusable preference chose %+v", c)
	}

	faces := ws.Pipe.FaceTemplates()
	if faces["transform_back/standard"] != transformID || faces["single/standard"] != render.PlaceholderID {
		t.Errorf("faceTemplates = %v", faces)
	}
}

func TestSetFace(t *testing.T) {
	ws := workspacetest.NewWithTransform(t)
	svc := New(ws)

	rows := svc.Faces()
	if len(rows) != 3 || !rows[0].Primary || rows[0].Key != "single/standard" || rows[1].Key != "transform_back/standard" || rows[1].Using != transformID+" · local" {
		t.Errorf("face rows = %+v", rows)
	}

	if _, err := svc.SetFace(FaceChoice{Key: "transform_front/standard", Name: transformID, Version: pipeline.LocalVersion}); err != nil {
		t.Errorf("setting transform: %v", err)
	}
	if got := ws.Prefs.FaceTemplates()["transform_front/standard"]; got.Name != transformID {
		t.Errorf("saved %+v", got)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "transform_front/standard", Name: standardID}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a template that does not render the face: %v", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "single/planeswalker", Name: transformID}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a face no template renders: %v", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "transform_front/standard"}); err != nil || len(ws.Prefs.FaceTemplates()) != 0 {
		t.Errorf("clearing: %v, prefs %v", err, ws.Prefs.FaceTemplates())
	}
}

func TestSetStandardTemplateSwitchesActive(t *testing.T) {
	ws := workspacetest.NewWithTransform(t)
	svc := New(ws)
	// A second loose template that renders standard cards to switch to
	templatetest.Spec{ID: standardID}.WriteLoose(t, filepath.Join(pipeline.LooseDirBases[0], "standard"))

	if _, err := svc.SetFace(FaceChoice{Key: "single/standard", Name: transformID}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a template that does not render standard cards: %v", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "single/standard"}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("clearing the standard template: %v", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "single/standard", Name: standardID}); err != nil {
		t.Fatalf("switching to the standard template: %v", err)
	}
	if name, version := ws.Active(); name != standardID || version != pipeline.LocalVersion {
		t.Errorf("active = %s %s, want the standard template local", name, version)
	}
	if len(ws.Prefs.FaceTemplates()) != 0 {
		t.Errorf("the standard choice was saved as a face preference: %v", ws.Prefs.FaceTemplates())
	}
}

func TestInstallOnlyLeavesTheActiveTemplate(t *testing.T) {
	ws := workspacetest.NewWithTransform(t)
	svc := New(ws)
	selectAndWait := func(req SelectRequest) {
		t.Helper()
		id, err := svc.Select(req)
		if err != nil || id == "" {
			t.Fatalf("select: %q, %v", id, err)
		}
		j, _ := ws.Jobs.Lookup(id)
		for range 500 {
			if _, done := j.Backlog(0); done {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("select job did not finish")
	}

	selectAndWait(SelectRequest{Name: transformID, Version: pipeline.LocalVersion, Install: true})
	if name := ws.Pipe.Active().Name; name != render.PlaceholderID {
		t.Errorf("installing transform made %s active", name)
	}
	selectAndWait(SelectRequest{Name: transformID, Version: pipeline.LocalVersion})
	if name := ws.Pipe.Active().Name; name != transformID {
		t.Errorf("selecting transform left %s active", name)
	}
}

func TestChangesRefusedMidRun(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	ws.Run.Set(batch.New(nil, batch.Options{ID: "run-9"}), nil)
	svc := New(ws)
	if _, err := svc.Select(SelectRequest{Name: standardID, Version: "1.0.0"}); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("template switch mid-run: %v, want a conflict", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "single/standard", Name: standardID}); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("face template mid-run: %v, want a conflict", err)
	}
}

// installFeatured writes a template with a feature of each type beside the active
// one, with the catalog out of reach so the list is what is installed
func installFeatured(t *testing.T) *Service {
	t.Helper()
	ws := workspacetest.NewSmall(t)
	assets := filepath.Join(t.TempDir(), "assets")
	templatetest.Spec{ID: featuredID, Name: "Featured", Features: templatetest.Features}.WriteLoose(t, filepath.Join(assets, "featured"))
	pipeline.LooseDirBases = []string{assets}
	oldURL := catalog.IndexURL
	catalog.IndexURL = "http://127.0.0.1:1/index.json"
	t.Cleanup(func() { catalog.IndexURL = oldURL })
	return New(ws)
}

const featuredID = "test.featured"

func featuredView(t *testing.T, svc *Service) *DetailsView {
	t.Helper()
	for _, tv := range svc.List(context.Background()) {
		if tv.Name == featuredID {
			if tv.Details == nil {
				t.Fatal("the installed template has no details")
			}
			return tv.Details
		}
	}
	t.Fatalf("%s is not listed", featuredID)
	return nil
}

func TestListShowsAnInstalledTemplatesDetailsAndFeatures(t *testing.T) {
	d := featuredView(t, installFeatured(t))
	if d.Title != "Featured" {
		t.Errorf("title = %q", d.Title)
	}
	got := map[string]FeatureView{}
	for _, f := range d.Features {
		got[f.Name] = f
	}
	if len(got) != 3 {
		t.Fatalf("features = %+v, want one of each type", d.Features)
	}
	if f := got["banner"]; f.Type != "bool" || f.Value != false || f.Label != "Banner" {
		t.Errorf("banner = %+v", f)
	}
	if f := got["tone"]; f.Type != "choice" || f.Value != "light" || !slices.Equal(f.Values, []string{"light", "dark"}) {
		t.Errorf("tone = %+v", f)
	}
	if f := got["strength"]; f.Type != "range" || f.Value != 0.5 || f.Min != 0 || f.Max != 1 || f.Step != 0.25 {
		t.Errorf("strength = %+v", f)
	}
}

func TestSetFeatureKeepsAValueHeldToWhatTheFeatureAllows(t *testing.T) {
	svc := installFeatured(t)
	set := func(name string, value any) (*DetailsView, error) {
		return svc.SetFeature(FeatureChoice{Template: featuredID, Name: name, Value: value})
	}
	value := func(d *DetailsView, name string) any {
		for _, f := range d.Features {
			if f.Name == name {
				return f.Value
			}
		}
		return nil
	}

	if d, err := set("banner", true); err != nil || value(d, "banner") != true {
		t.Errorf("banner on = %v, %v", d, err)
	}
	if d, err := set("tone", "dark"); err != nil || value(d, "tone") != "dark" {
		t.Errorf("tone dark = %v, %v", d, err)
	}
	// A number lands on the nearest step
	if d, err := set("strength", 0.6); err != nil || value(d, "strength") != 0.5 {
		t.Errorf("strength 0.6 = %v, %v", d, err)
	}
	if got := svc.ws.Prefs.Features(featuredID); got["banner"] != true || got["tone"] != "dark" || got["strength"] != 0.5 {
		t.Errorf("kept %v", got)
	}
	if v := value(featuredView(t, svc), "banner"); v != true {
		t.Errorf("the list shows banner %v", v)
	}
	// Nil forgets the choice, and the default is back
	if d, err := set("tone", nil); err != nil || value(d, "tone") != "light" {
		t.Errorf("forgetting tone = %v, %v", d, err)
	}

	for _, bad := range []struct {
		name  string
		value any
	}{
		{"banner", "yes"}, {"tone", "neon"}, {"tone", true}, {"strength", 2.0}, {"strength", "high"}, {"nope", true},
	} {
		if _, err := set(bad.name, bad.value); apierr.KindOf(err) != apierr.BadRequest {
			t.Errorf("%s = %v gave %v, want a bad request", bad.name, bad.value, err)
		}
	}
	if _, err := svc.SetFeature(FeatureChoice{Template: "test.absent", Name: "banner", Value: true}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a template that is not installed: %v", err)
	}

	svc.ws.Run.Set(batch.New(nil, batch.Options{ID: "run-1"}), nil)
	if _, err := set("banner", false); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("a change mid-run: %v, want a conflict", err)
	}
}
