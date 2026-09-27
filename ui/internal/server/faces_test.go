package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
	"github.com/odevine/mimic/ui/internal/batch"
	"github.com/odevine/mimic/ui/internal/pipeline"
	"github.com/odevine/mimic/ui/internal/prefs"
)

// facesServer is a run server with transform installed as a loose developer
// folder of placeholder assets beside the active normal template
func facesServer(t *testing.T) *Server {
	t.Helper()
	s := runServer(t)
	assets := filepath.Join(t.TempDir(), "assets")
	if err := pipeline.WritePlaceholders(filepath.Join(assets, "transform"), "transform"); err != nil {
		t.Fatal(err)
	}
	pipeline.LooseDirBases = []string{assets}
	t.Cleanup(s.pipe.Close)
	return s
}

func transformRow(front, back card.Face) batch.Row {
	return batch.Row{Qty: 1, Base: card.Data{
		Name: front.Name, TypeLine: front.TypeLine, Power: front.Power, Toughness: front.Toughness,
		SetCode: "isd", CollectorNumber: "51", Layout: "transform", Faces: []card.Face{front, back},
	}}
}

func TestChooseTemplatePerFace(t *testing.T) {
	s := facesServer(t)
	front := template.Shape{Role: template.RoleTransformFront, Kind: template.KindStandard}
	if c, ok := s.pipe.Choose(pipeline.PrimaryShape); !ok || c.Name != "normal" {
		t.Errorf("single standard chose %+v, %v, want the active normal", c, ok)
	}
	if c, ok := s.pipe.Choose(front); !ok || c.Name != "transform" {
		t.Errorf("transform front chose %+v, %v, want transform", c, ok)
	}
	walker := template.Shape{Role: template.RoleTransformBack, Kind: template.KindPlaneswalker}
	if _, ok := s.pipe.Choose(walker); ok {
		t.Error("a planeswalker back has no template, but one was chosen")
	}
	// A preference for a template that does not render the shape is ignored
	s.prefs.SetFaceTemplate(pipeline.ShapeKey(front), prefs.TemplateChoice{Name: "normal"})
	if c, _ := s.pipe.Choose(front); c.Name != "transform" {
		t.Errorf("an unusable preference chose %+v", c)
	}

	faces := s.pipe.FaceTemplates()
	if faces["transform_back/standard"] != "transform" || faces["single/standard"] != "normal" {
		t.Errorf("faceTemplates = %v", faces)
	}
}

func TestRunRendersEveryFace(t *testing.T) {
	s := facesServer(t)
	dir := t.TempDir()
	rows := []batch.Row{
		transformRow(card.Face{Name: "Delver of Secrets", TypeLine: "Creature — Human Wizard", Power: "1", Toughness: "1"},
			card.Face{Name: "Insectile Aberration", TypeLine: "Creature — Human Insect", Power: "3", Toughness: "2"}),
		transformRow(card.Face{Name: "Nissa, Vastwood Seer", TypeLine: "Legendary Creature — Elf Scout", Power: "2", Toughness: "2"},
			card.Face{Name: "Nissa, Sage Animist", TypeLine: "Legendary Planeswalker — Nissa", ArtworkURL: "http://127.0.0.1:1/never-fetched.jpg"}),
	}
	postRun(t, s, runBody{Rows: rows, OutDir: dir})
	v := waitRun(t, s)

	if len(v.Cards) != 4 {
		t.Fatalf("run has %d cards, want one per face: %+v", len(v.Cards), v.Cards)
	}
	want := []struct {
		name, status, file string
		face               int
	}{
		{"Delver of Secrets", batch.StatusDone, "Delver of Secrets [ISD-51].png", 0},
		{"Insectile Aberration", batch.StatusDone, "Insectile Aberration [ISD-51].png", 1},
		{"Nissa, Vastwood Seer", batch.StatusDone, "Nissa, Vastwood Seer [ISD-51].png", 0},
		{"Nissa, Sage Animist", batch.StatusUnsupported, "", 1},
	}
	for i, w := range want {
		c := v.Cards[i]
		if c.Name != w.name || c.Status != w.status || c.File != w.file || c.Face != w.face {
			t.Errorf("card %d = %+v, want %+v", i, c, w)
		}
		if w.file != "" {
			if _, err := os.Stat(filepath.Join(dir, w.file)); err != nil {
				t.Errorf("card %d: %v", i, err)
			}
		}
	}
}

func TestSetFaceTemplate(t *testing.T) {
	s := facesServer(t)
	put := func(body faceChoiceBody) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/template/faces", bytes.NewReader(raw)))
		return rec
	}

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/template/faces", nil))
	var rows []pipeline.FaceRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || !rows[0].Primary || rows[0].Key != "single/standard" || rows[1].Key != "transform_back/standard" || rows[1].Using != "transform · local" {
		t.Errorf("face rows = %s", rec.Body)
	}

	if rec := put(faceChoiceBody{Key: "transform_front/standard", Name: "transform", Version: pipeline.LocalVersion}); rec.Code != http.StatusOK {
		t.Errorf("setting transform = %d %s", rec.Code, rec.Body)
	}
	if got := s.prefs.FaceTemplates()["transform_front/standard"]; got.Name != "transform" {
		t.Errorf("saved %+v", got)
	}
	if rec := put(faceChoiceBody{Key: "transform_front/standard", Name: "normal"}); rec.Code != http.StatusBadRequest {
		t.Errorf("a template that does not render the face = %d", rec.Code)
	}
	if rec := put(faceChoiceBody{Key: "single/planeswalker", Name: "transform"}); rec.Code != http.StatusBadRequest {
		t.Errorf("a face no template renders = %d", rec.Code)
	}
	if rec := put(faceChoiceBody{Key: "transform_front/standard"}); rec.Code != http.StatusOK || len(s.prefs.FaceTemplates()) != 0 {
		t.Errorf("clearing = %d, prefs %v", rec.Code, s.prefs.FaceTemplates())
	}
}

func TestSetStandardTemplateSwitchesActive(t *testing.T) {
	s := facesServer(t)
	// A second loose template that renders standard cards to switch to
	if err := pipeline.WritePlaceholders(filepath.Join(pipeline.LooseDirBases[0], "normal"), "normal"); err != nil {
		t.Fatal(err)
	}
	put := func(body faceChoiceBody) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/template/faces", bytes.NewReader(raw)))
		return rec
	}

	if rec := put(faceChoiceBody{Key: "single/standard", Name: "transform"}); rec.Code != http.StatusBadRequest {
		t.Errorf("a template that does not render standard cards = %d", rec.Code)
	}
	if rec := put(faceChoiceBody{Key: "single/standard"}); rec.Code != http.StatusBadRequest {
		t.Errorf("clearing the standard template = %d", rec.Code)
	}
	if rec := put(faceChoiceBody{Key: "single/standard", Name: "normal"}); rec.Code != http.StatusOK {
		t.Fatalf("switching to normal = %d %s", rec.Code, rec.Body)
	}
	if name, version := s.active(); name != "normal" || version != pipeline.LocalVersion {
		t.Errorf("active = %s %s, want normal local", name, version)
	}
	if len(s.prefs.FaceTemplates()) != 0 {
		t.Errorf("the standard choice was saved as a face preference: %v", s.prefs.FaceTemplates())
	}
}

func TestInstallOnlyLeavesTheActiveTemplate(t *testing.T) {
	s := facesServer(t)
	selectAndWait := func(body selectBody) {
		t.Helper()
		raw, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/template/select", bytes.NewReader(raw)))
		var got struct{ JobID string }
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.JobID == "" {
			t.Fatalf("select = %d %s", rec.Code, rec.Body)
		}
		j, _ := s.lookupJob(got.JobID)
		for range 500 {
			j.mu.Lock()
			done := j.finished
			j.mu.Unlock()
			if done {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("select job did not finish")
	}

	selectAndWait(selectBody{Name: "transform", Version: pipeline.LocalVersion, Install: true})
	if name := s.pipe.Active().Name; name != "normal" {
		t.Errorf("installing transform made %s active", name)
	}
	selectAndWait(selectBody{Name: "transform", Version: pipeline.LocalVersion})
	if name := s.pipe.Active().Name; name != "transform" {
		t.Errorf("selecting transform left %s active", name)
	}
}
