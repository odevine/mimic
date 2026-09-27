package cardlist

import (
	"encoding/json"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

func TestResolvedRowSendsShapes(t *testing.T) {
	jace := &card.Data{Name: "Jace", Layout: "normal", TypeLine: "Legendary Planeswalker — Jace"}
	delver := &card.Data{Name: "Delver", Layout: "transform", Faces: []card.Face{{TypeLine: "Creature"}, {TypeLine: "Creature"}}}
	raw, err := json.Marshal(Resolved{Status: StatusMatched, Card: jace, Candidates: []*card.Data{jace, delver}})
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
	if got.Status != StatusMatched || got.Card.Name != "Jace" || got.Cards != nil {
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
	if err := json.Unmarshal(mustJSON(t, Shaped(jace)), &back); err != nil || back.TypeLine != jace.TypeLine {
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
