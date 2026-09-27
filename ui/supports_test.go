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
