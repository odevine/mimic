package cardlist

import (
	"encoding/json"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// ShapedCard is a card as the page receives it, with the shape of each image it
// renders to, so the page can check a card against the active template's
// supports without classifying cards itself. Decoding one back into card.Data
// drops the shapes, so a card the page posts back needs no unwrapping
type ShapedCard struct {
	*card.Data
	Shapes []template.Shape `json:"shapes"`
}

// Shaped wraps a card with its shapes, keeping nil nil
func Shaped(d *card.Data) *ShapedCard {
	if d == nil {
		return nil
	}
	return &ShapedCard{Data: d, Shapes: template.Classify(d)}
}

// ShapedAll wraps every card, keeping a nil list nil so an omitempty field
// stays omitted
func ShapedAll(cards []*card.Data) []*ShapedCard {
	if cards == nil {
		return nil
	}
	out := make([]*ShapedCard, len(cards))
	for i, d := range cards {
		out[i] = Shaped(d)
	}
	return out
}

// MarshalJSON sends a resolved row's cards with their shapes. The outer fields
// share the embedded row's JSON names and sit shallower, so they win
func (r Resolved) MarshalJSON() ([]byte, error) {
	type plain Resolved
	return json.Marshal(struct {
		plain
		Card       *ShapedCard   `json:"card,omitempty"`
		Candidates []*ShapedCard `json:"candidates,omitempty"`
		Cards      []*ShapedCard `json:"cards,omitempty"`
	}{plain(r), Shaped(r.Card), ShapedAll(r.Candidates), ShapedAll(r.Cards)})
}
