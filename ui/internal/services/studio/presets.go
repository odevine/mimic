package studio

import (
	"context"
	"fmt"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/apierr"
	"github.com/odevine/mimic/ui/internal/cardlist"
	"github.com/odevine/mimic/ui/internal/testcards"
	"github.com/odevine/mimic/ui/internal/workspace"
)

// Preset is a real card chosen because it exercises one thing a template decides, such
// as how many colors it has or whether it is a land, named by its set and collector
// number so the same printing is found each time
type Preset struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Name  string `json:"name"`
	Set   string `json:"set"`
	// Number is the printing's collector number in its set
	Number string `json:"number"`
	// User is whether a person saved the card, which can then be removed
	User bool `json:"user,omitempty"`
}

// Presets are the test cards the editor offers. They are looked up with ResolvePreset
// when one is chosen, so the card the template draws is the card as Scryfall has it
func (s *Service) Presets() []Preset {
	out := presets()
	saved, err := s.cards.List()
	if err != nil {
		// The built-in cards are still there to use, and saving says what is wrong
		return out
	}
	for _, c := range saved {
		out = append(out, Preset{ID: c.ID, Label: c.Label, Name: c.Card.Name, Set: strings.ToLower(c.Card.SetCode), Number: c.Card.CollectorNumber, User: true})
	}
	return out
}

func presets() []Preset {
	return []Preset{
		{ID: "monocolored", Label: "Monocolored", Name: "Mirran Spy", Set: "mbs", Number: "26"},
		{ID: "two-colors", Label: "Two Colors", Name: "Consuming Aberration", Set: "gtc", Number: "152"},
		{ID: "three-colors", Label: "Three Colors", Name: "Eerie Ultimatum", Set: "iko", Number: "184"},
		{ID: "legendary", Label: "Legendary Creature", Name: "Norin the Wary", Set: "tsp", Number: "171"},
		{ID: "artifact", Label: "Artifact", Name: "Sensei's Divining Top", Set: "2x2", Number: "314"},
		{ID: "land", Label: "Land", Name: "Flooded Strand", Set: "ktk", Number: "233"},
	}
}

// resolver is how a preset's card is looked up, from the local card data when it is
// chosen in settings and loaded and from Scryfall otherwise, as a list's rows are
func (s *Service) resolver() cardlist.Resolver {
	if s.resolve != nil {
		return s.resolve()
	}
	if s.ws.UseLocalCards() {
		return cardlist.LocalResolver(s.ws.Cards, s.ws.Pipe.Client())
	}
	return cardlist.APIResolver(s.ws.Pipe.Client())
}

// ResolvePreset looks up the card a preset names. A lookup that succeeds is kept for the
// session, so choosing a preset again does not ask again, and one that fails is not
func (s *Service) ResolvePreset(id string) (*cardlist.ShapedCard, error) {
	if testcards.IsID(id) {
		saved, err := s.cards.Get(id)
		if err != nil {
			return nil, err
		}
		c := saved.Card
		return cardlist.Shaped(&c), nil
	}
	var p *Preset
	for _, c := range presets() {
		if c.ID == id {
			p = &c
			break
		}
	}
	if p == nil {
		return nil, apierr.Newf(apierr.NotFound, "%q is not one of the test cards", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), workspace.NetTimeout)
	defer cancel()
	row := cardlist.Row{Line: 1, Text: p.Name, Qty: 1, Name: p.Name, Set: p.Set, Number: p.Number}
	var got cardlist.Resolved
	if err := cardlist.ResolveAll(ctx, s.resolver(), &s.presetCache, []cardlist.Row{row}, func(_ int, r cardlist.Resolved) { got = r }); err != nil {
		return nil, apierr.Wrap(apierr.BadGateway, fmt.Sprintf("Could not look up %s", p.Name), err)
	}
	if got.Status != cardlist.StatusMatched || got.Card == nil {
		why := got.Note
		if why == "" {
			why = "Scryfall has no such card"
		}
		return nil, apierr.Newf(apierr.BadGateway, "Could not look up %s (%s %s): %s", p.Name, strings.ToUpper(p.Set), p.Number, why)
	}
	return cardlist.Shaped(got.Card), nil
}

// SavePresetRequest is a card to keep as a test card under a name. Edits, when given,
// are laid over the card as the form has them, and the card that is kept is the result
type SavePresetRequest struct {
	Label string          `json:"label"`
	Card  card.Data       `json:"card"`
	Edits *cardlist.Edits `json:"edits,omitempty"`
	Face  int             `json:"face"`
}

// SavePreset keeps a card as a test card, for the templates to be tried with in any
// draft, and returns it as the list now has it
func (s *Service) SavePreset(req SavePresetRequest) (Preset, error) {
	c := withEdits(req.Card, req.Edits)
	if strings.TrimSpace(c.Name) == "" {
		return Preset{}, apierr.New(apierr.BadRequest, "A test card needs a card with a name")
	}
	saved, err := s.cards.Add(req.Label, c, req.Face)
	if err != nil {
		return Preset{}, err
	}
	return Preset{ID: saved.ID, Label: saved.Label, Name: c.Name, Set: strings.ToLower(c.SetCode), Number: c.CollectorNumber, User: true}, nil
}

// DeletePreset forgets a test card that was saved. The built-in ones cannot be removed
func (s *Service) DeletePreset(id string) error {
	if !testcards.IsID(id) {
		return apierr.New(apierr.BadRequest, "Only a test card you saved can be removed")
	}
	return s.cards.Remove(id)
}
