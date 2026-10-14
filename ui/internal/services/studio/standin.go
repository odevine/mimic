package studio

import (
	"context"
	"fmt"
	"strings"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/cardlist"
)

// standInCards are the cards the presets stand for, written out so the launch check, which
// has no network, and the tests have cards that look like them. They carry the set and
// collector number of the printing each preset names
func standInCards() map[string]card.Data {
	W, U, B, R, G := card.White, card.Blue, card.Black, card.Red, card.Green
	c := func(p Preset, d card.Data) card.Data {
		d.Name, d.SetCode, d.CollectorNumber = p.Name, p.Set, p.Number
		d.Layout, d.Language, d.ReleasedAt, d.Artist = "normal", "en", "2026-01-01", "A. Artist"
		if d.ColorIdentity == nil {
			d.ColorIdentity = d.Colors
		}
		return d
	}
	by := map[string]Preset{}
	for _, p := range presets() {
		by[p.ID] = p
	}
	return map[string]card.Data{
		"monocolored": c(by["monocolored"], card.Data{
			ManaCost: "{1}{U}", TypeLine: "Creature - Human Rogue", Rarity: "common",
			OracleText: "Flying\nWhenever you cast an artifact spell, you may untap target creature.", FlavorText: "\"Nothing moves unseen.\"",
			Power: "1", Toughness: "1", Colors: []card.Color{U},
		}),
		"two-colors": c(by["two-colors"], card.Data{
			ManaCost: "{3}{U}{B}", TypeLine: "Creature - Horror", Rarity: "rare",
			OracleText: "Consuming Aberration's power and toughness are each equal to the number of cards in your opponents' graveyards.\nWhenever you cast a spell, each opponent reveals cards from the top of their library until they reveal a land card.",
			Power:      "*", Toughness: "*", Colors: []card.Color{U, B},
		}),
		"three-colors": c(by["three-colors"], card.Data{
			ManaCost: "{W}{W}{B}{B}{G}{G}", TypeLine: "Sorcery", Rarity: "rare",
			OracleText: "Return any number of permanent cards with different names from your graveyard to the battlefield.",
			Colors:     []card.Color{W, B, G},
		}),
		"legendary": c(by["legendary"], card.Data{
			ManaCost: "{R}", TypeLine: "Legendary Creature - Human Warrior", Rarity: "rare",
			OracleText: "When a player casts a spell or a creature attacks, exile Norin the Wary. Return it to the battlefield under its owner's control at the beginning of the next end step.",
			Power:      "1", Toughness: "1", Colors: []card.Color{R},
		}),
		"artifact": c(by["artifact"], card.Data{
			ManaCost: "{1}", TypeLine: "Artifact", Rarity: "rare",
			OracleText: "{1}: Look at the top three cards of your library, then put them back in any order.\n{T}: Draw a card, then put Sensei's Divining Top on top of its owner's library.",
		}),
		"land": c(by["land"], card.Data{
			TypeLine: "Land", Rarity: "rare",
			OracleText: "{T}, Pay 1 life, Sacrifice Flooded Strand: Search your library for a Plains or Island card, put it onto the battlefield, then shuffle.",
		}),
	}
}

// standInSource answers a lookup of a preset's printing from standInCards
type standInSource struct{}

func (standInSource) Search(_ context.Context, q string) ([]*card.Data, error) {
	for _, p := range presets() {
		if strings.Contains(q, fmt.Sprintf("set:%s", p.Set)) && strings.Contains(q, "cn:"+p.Number) {
			d := standInCards()[p.ID]
			return []*card.Data{&d}, nil
		}
	}
	return nil, nil
}

func (standInSource) FetchByName(context.Context, string) (*card.Data, error) { return nil, nil }

// UseStandIn makes the presets look up made-up cards, for the launch check, which has no
// network to ask
func (s *Service) UseStandIn() {
	s.resolve = func() cardlist.Resolver { return cardlist.APIResolver(standInSource{}) }
}
