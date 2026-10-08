package templates

import (
	"testing"
	"time"

	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
	"github.com/odevine/mimic/ui/internal/workspace/workspacetest"
	"path/filepath"
)

func TestTemplateSource(t *testing.T) {
	if got := templateSource("normal", ""); got != "placeholder" {
		t.Errorf("empty version: %q, want placeholder", got)
	}
	if got := templateSource("normal", pipeline.LocalVersion); got != "local" {
		t.Errorf("local version: %q, want local", got)
	}
}

func TestActiveReportsSupports(t *testing.T) {
	svc := New(workspacetest.NewSmall(t))
	got := svc.Active()
	if len(got.Supports.Roles) == 0 || len(got.Supports.Kinds) == 0 {
		t.Errorf("active template = %+v", got)
	}
}

func TestChooseTemplatePerFace(t *testing.T) {
	ws := workspacetest.NewWithTransform(t)
	front := template.Shape{Role: template.RoleTransformFront, Kind: template.KindStandard}
	if c, ok := ws.Pipe.Choose(pipeline.PrimaryShape); !ok || c.Name != "normal" {
		t.Errorf("single standard chose %+v, %v, want the active normal", c, ok)
	}
	if c, ok := ws.Pipe.Choose(front); !ok || c.Name != "transform" {
		t.Errorf("transform front chose %+v, %v, want transform", c, ok)
	}
	walker := template.Shape{Role: template.RoleTransformBack, Kind: template.KindPlaneswalker}
	if _, ok := ws.Pipe.Choose(walker); ok {
		t.Error("a planeswalker back has no template, but one was chosen")
	}
	// A preference for a template that does not render the shape is ignored
	ws.Prefs.SetFaceTemplate(pipeline.ShapeKey(front), prefs.TemplateChoice{Name: "normal"})
	if c, _ := ws.Pipe.Choose(front); c.Name != "transform" {
		t.Errorf("an unusable preference chose %+v", c)
	}

	faces := ws.Pipe.FaceTemplates()
	if faces["transform_back/standard"] != "transform" || faces["single/standard"] != "normal" {
		t.Errorf("faceTemplates = %v", faces)
	}
}

func TestSetFace(t *testing.T) {
	ws := workspacetest.NewWithTransform(t)
	svc := New(ws)

	rows := svc.Faces()
	if len(rows) != 3 || !rows[0].Primary || rows[0].Key != "single/standard" || rows[1].Key != "transform_back/standard" || rows[1].Using != "transform · local" {
		t.Errorf("face rows = %+v", rows)
	}

	if _, err := svc.SetFace(FaceChoice{Key: "transform_front/standard", Name: "transform", Version: pipeline.LocalVersion}); err != nil {
		t.Errorf("setting transform: %v", err)
	}
	if got := ws.Prefs.FaceTemplates()["transform_front/standard"]; got.Name != "transform" {
		t.Errorf("saved %+v", got)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "transform_front/standard", Name: "normal"}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a template that does not render the face: %v", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "single/planeswalker", Name: "transform"}); apierr.KindOf(err) != apierr.BadRequest {
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
	if err := pipeline.WritePlaceholders(filepath.Join(pipeline.LooseDirBases[0], "normal"), "normal"); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.SetFace(FaceChoice{Key: "single/standard", Name: "transform"}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("a template that does not render standard cards: %v", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "single/standard"}); apierr.KindOf(err) != apierr.BadRequest {
		t.Errorf("clearing the standard template: %v", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "single/standard", Name: "normal"}); err != nil {
		t.Fatalf("switching to normal: %v", err)
	}
	if name, version := ws.Active(); name != "normal" || version != pipeline.LocalVersion {
		t.Errorf("active = %s %s, want normal local", name, version)
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

	selectAndWait(SelectRequest{Name: "transform", Version: pipeline.LocalVersion, Install: true})
	if name := ws.Pipe.Active().Name; name != "normal" {
		t.Errorf("installing transform made %s active", name)
	}
	selectAndWait(SelectRequest{Name: "transform", Version: pipeline.LocalVersion})
	if name := ws.Pipe.Active().Name; name != "transform" {
		t.Errorf("selecting transform left %s active", name)
	}
}

func TestChangesRefusedMidRun(t *testing.T) {
	ws := workspacetest.NewSmall(t)
	ws.Run.Set(batch.New(nil, batch.Options{ID: "run-9"}), nil)
	svc := New(ws)
	if _, err := svc.Select(SelectRequest{Name: "normal", Version: "1.0.0"}); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("template switch mid-run: %v, want a conflict", err)
	}
	if _, err := svc.SetFace(FaceChoice{Key: "single/standard", Name: "normal"}); apierr.KindOf(err) != apierr.Conflict {
		t.Errorf("face template mid-run: %v, want a conflict", err)
	}
}
