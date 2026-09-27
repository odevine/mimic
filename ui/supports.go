package main

import (
	"encoding/json"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/template"
)

// shapedCard is a card as the page receives it, with the shape of each image it
// renders to, so the page can check a card against the active template's
// supports without classifying cards itself. Decoding one back into card.Data
// drops the shapes, so a card the page posts back needs no unwrapping
type shapedCard struct {
	*card.Data
	Shapes []template.Shape `json:"shapes"`
}

func shaped(d *card.Data) *shapedCard {
	if d == nil {
		return nil
	}
	return &shapedCard{Data: d, Shapes: template.Classify(d)}
}

// shapedAll wraps every card, keeping a nil list nil so an omitempty field
// stays omitted
func shapedAll(cards []*card.Data) []*shapedCard {
	if cards == nil {
		return nil
	}
	out := make([]*shapedCard, len(cards))
	for i, d := range cards {
		out[i] = shaped(d)
	}
	return out
}

// MarshalJSON sends a resolved row's cards with their shapes. The outer fields
// share the embedded row's JSON names and sit shallower, so they win
func (r resolvedRow) MarshalJSON() ([]byte, error) {
	type plain resolvedRow
	return json.Marshal(struct {
		plain
		Card       *shapedCard   `json:"card,omitempty"`
		Candidates []*shapedCard `json:"candidates,omitempty"`
		Cards      []*shapedCard `json:"cards,omitempty"`
	}{plain(r), shaped(r.Card), shapedAll(r.Candidates), shapedAll(r.Cards)})
}

// supportsOf is the named template's registered supports, empty when this
// build has no template by that name
func supportsOf(name string) template.Supports {
	for _, r := range template.List() {
		if r.Name == name {
			return r.Supports
		}
	}
	return template.Supports{}
}

// supports is what the template renders currently go through can render
func (p *renderPipeline) supports() template.Supports {
	return supportsOf(p.active.Load().name)
}
