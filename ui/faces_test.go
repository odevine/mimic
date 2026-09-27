package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/render"
	"github.com/odevine/mimic/engine/template"
)

// facesServer is a run server with transform installed as a loose developer
// folder of placeholder assets beside the active normal template
func facesServer(t *testing.T) *server {
	t.Helper()
	s := runServer(t)
	s.pipe.preferences = s.prefs.faceTemplates
	assets := filepath.Join(t.TempDir(), "assets")
	if err := render.WritePlaceholderAssets(filepath.Join(assets, "transform"), "transform"); err != nil {
		t.Fatal(err)
	}
	looseDirBases = []string{assets}
	t.Cleanup(s.pipe.close)
	return s
}

func transformRow(front, back card.Face) runRow {
	return runRow{Qty: 1, Base: card.Data{
		Name: front.Name, TypeLine: front.TypeLine, Power: front.Power, Toughness: front.Toughness,
		SetCode: "isd", CollectorNumber: "51", Layout: "transform", Faces: []card.Face{front, back},
	}}
}

func TestChooseTemplatePerFace(t *testing.T) {
	s := facesServer(t)
	front := template.Shape{Role: template.RoleTransformFront, Kind: template.KindStandard}
	if c, ok := s.pipe.choose(primaryShape); !ok || c.Name != "normal" {
		t.Errorf("single standard chose %+v, %v, want the active normal", c, ok)
	}
	if c, ok := s.pipe.choose(front); !ok || c.Name != "transform" {
		t.Errorf("transform front chose %+v, %v, want transform", c, ok)
	}
	walker := template.Shape{Role: template.RoleTransformBack, Kind: template.KindPlaneswalker}
	if _, ok := s.pipe.choose(walker); ok {
		t.Error("a planeswalker back has no template, but one was chosen")
	}
	// A preference for a template that does not render the shape is ignored
	s.prefs.setFaceTemplate(shapeKey(front), templateChoice{Name: "normal"})
	if c, _ := s.pipe.choose(front); c.Name != "transform" {
		t.Errorf("an unusable preference chose %+v", c)
	}

	faces := s.pipe.faceTemplates()
	if faces["transform_back/standard"] != "transform" || faces["single/standard"] != "normal" {
		t.Errorf("faceTemplates = %v", faces)
	}
}

func TestRunRendersEveryFace(t *testing.T) {
	s := facesServer(t)
	dir := t.TempDir()
	rows := []runRow{
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
		{"Delver of Secrets", cardDone, "Delver of Secrets [ISD-51].png", 0},
		{"Insectile Aberration", cardDone, "Insectile Aberration [ISD-51].png", 1},
		{"Nissa, Vastwood Seer", cardDone, "Nissa, Vastwood Seer [ISD-51].png", 0},
		{"Nissa, Sage Animist", cardUnsupported, "", 1},
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

func TestExpandFacesKeepsOneRowForOneFace(t *testing.T) {
	rows := expandFaces([]runRow{customRunRow("Grizzly Bears", "", ""), {Base: card.Data{Name: "Meld", Layout: "meld"}}})
	if len(rows) != 2 || rows[0].Face != 0 || rows[1].Face != 0 {
		t.Errorf("rows = %+v", rows)
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
	var rows []faceRow
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Key != "transform_back/standard" || rows[0].Using != "transform · local" {
		t.Errorf("face rows = %s", rec.Body)
	}

	if rec := put(faceChoiceBody{Key: "transform_front/standard", Name: "transform", Version: localVersion}); rec.Code != http.StatusOK {
		t.Errorf("setting transform = %d %s", rec.Code, rec.Body)
	}
	if got := s.prefs.faceTemplates()["transform_front/standard"]; got.Name != "transform" {
		t.Errorf("saved %+v", got)
	}
	if rec := put(faceChoiceBody{Key: "transform_front/standard", Name: "normal"}); rec.Code != http.StatusBadRequest {
		t.Errorf("a template that does not render the face = %d", rec.Code)
	}
	if rec := put(faceChoiceBody{Key: "single/planeswalker", Name: "transform"}); rec.Code != http.StatusBadRequest {
		t.Errorf("a face no template renders = %d", rec.Code)
	}
	if rec := put(faceChoiceBody{Key: "transform_front/standard"}); rec.Code != http.StatusOK || len(s.prefs.faceTemplates()) != 0 {
		t.Errorf("clearing = %d, prefs %v", rec.Code, s.prefs.faceTemplates())
	}
}
