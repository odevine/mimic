package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

func TestResolvedRowSendsShapes(t *testing.T) {
	jace := &card.Data{Name: "Jace", Layout: "normal", TypeLine: "Legendary Planeswalker — Jace"}
	delver := &card.Data{Name: "Delver", Layout: "transform", Faces: []card.Face{{TypeLine: "Creature"}, {TypeLine: "Creature"}}}
	raw, err := json.Marshal(resolvedRow{Status: rowMatched, Card: jace, Candidates: []*card.Data{jace, delver}})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Status string
		Card   struct {
			Name   string
			Shapes []template.Shape
		}
		Candidates []struct{ Shapes []template.Shape }
		Cards      []any
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != rowMatched || got.Card.Name != "Jace" || got.Cards != nil {
		t.Errorf("row = %s", raw)
	}
	if len(got.Card.Shapes) != 1 || got.Card.Shapes[0] != (template.Shape{Role: template.RoleSingle, Kind: template.KindPlaneswalker}) {
		t.Errorf("card shapes = %v", got.Card.Shapes)
	}
	if len(got.Candidates) != 2 || len(got.Candidates[1].Shapes) != 2 {
		t.Errorf("candidates = %s", raw)
	}

	// A shaped card posted back decodes as plain card data
	var back card.Data
	if err := json.Unmarshal(mustJSON(t, shaped(jace)), &back); err != nil || back.TypeLine != jace.TypeLine {
		t.Errorf("round trip = %+v, %v", back, err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestActiveTemplateReportsSupports(t *testing.T) {
	s := runServer(t)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/template/active", nil))
	var got struct{ Supports template.Supports }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Supports.Roles) == 0 || len(got.Supports.Kinds) == 0 {
		t.Errorf("active template = %s", rec.Body)
	}
}

func TestRunMarksUnsupportedCards(t *testing.T) {
	s := runServer(t)
	dir := t.TempDir()
	rows := []runRow{
		customRunRow("Grizzly Bears", "", ""),
		{Qty: 1, Base: card.Data{Name: "Jace", TypeLine: "Legendary Planeswalker — Jace", ArtworkURL: "http://127.0.0.1:1/never-fetched.jpg"}},
	}
	postRun(t, s, runBody{Rows: rows, OutDir: dir})
	v := waitRun(t, s)

	if v.Cards[0].Status != cardDone {
		t.Errorf("bears = %+v", v.Cards[0])
	}
	// Refused before the art fetch, which would otherwise fail on the bad URL
	if c := v.Cards[1]; c.Status != cardUnsupported || !strings.Contains(c.Err, "planeswalker") {
		t.Errorf("jace = %+v", c)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "Jace*.png")); len(matches) != 0 {
		t.Errorf("wrote %v for an unsupported card", matches)
	}
}
