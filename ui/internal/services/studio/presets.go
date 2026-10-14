package studio

import (
	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/ui/internal/cardlist"
)

// Preset is a card made to exercise one thing a template decides, so a frame can
// be checked without searching for a card that has it
type Preset struct {
	ID    string               `json:"id"`
	Label string               `json:"label"`
	Card  *cardlist.ShapedCard `json:"card"`
}

// Presets are the test cards the editor offers. Each is a standard single face with
// no art, and they differ in what a frame's conditions and colors read: how many
// colors it has, its types, and how much text it carries
func (s *Service) Presets() []Preset { return presets() }

func presets() []Preset {
	c := func(id, label string, d card.Data) Preset {
		d.Layout, d.Language, d.ReleasedAt, d.SetCode, d.CollectorNumber, d.Artist = "normal", "en", "2026-01-01", "tst", "1", "A. Artist"
		if d.ColorIdentity == nil {
			d.ColorIdentity = d.Colors
		}
		return Preset{ID: id, Label: label, Card: cardlist.Shaped(&d)}
	}
	W, U, B, R, G := card.White, card.Blue, card.Black, card.Red, card.Green
	return []Preset{
		c("white-creature", "White creature", card.Data{
			Name: "Dawnwatch Sentinel", ManaCost: "{1}{W}", TypeLine: "Creature - Human Soldier", Rarity: "common",
			OracleText: "Vigilance\nWhen Dawnwatch Sentinel enters, you gain 2 life.", FlavorText: "\"The first light finds us ready.\"",
			Power: "2", Toughness: "2", Colors: []card.Color{W},
		}),
		c("two-color", "Two colors", card.Data{
			Name: "Tidecaller Savant", ManaCost: "{1}{W}{U}", TypeLine: "Creature - Merfolk Wizard", Rarity: "uncommon",
			OracleText: "Flying\nWhenever you cast a spell with mana value 3 or less, scry 1.",
			Power:      "2", Toughness: "3", Colors: []card.Color{W, U},
		}),
		c("three-color", "Three colors", card.Data{
			Name: "Convergence Drake", ManaCost: "{W}{U}{B}", TypeLine: "Creature - Drake", Rarity: "rare",
			OracleText: "Flying, lifelink\nWhen Convergence Drake dies, each opponent loses 2 life.",
			Power:      "3", Toughness: "3", Colors: []card.Color{W, U, B},
		}),
		c("five-color", "Five colors", card.Data{
			Name: "Prism Warden", ManaCost: "{W}{U}{B}{R}{G}", TypeLine: "Legendary Creature - Elemental", Rarity: "mythic",
			OracleText: "Protection from monocolored\nAt the beginning of your upkeep, add one mana of any color.",
			Power:      "5", Toughness: "5", Colors: []card.Color{W, U, B, R, G},
		}),
		c("legendary", "Legendary creature", card.Data{
			Name: "Ilvara, the Hollow King", ManaCost: "{2}{B}{R}", TypeLine: "Legendary Creature - Vampire Noble", Rarity: "rare",
			OracleText: "Menace\nWhenever Ilvara attacks, each opponent sacrifices a creature with the least power.",
			Power:      "4", Toughness: "3", Colors: []card.Color{B, R},
		}),
		c("artifact", "Colorless artifact", card.Data{
			Name: "Brass Compass", ManaCost: "{2}", TypeLine: "Artifact", Rarity: "common",
			OracleText: "{T}: Add one mana of any color.\n{2}, {T}, Sacrifice Brass Compass: Draw a card.",
		}),
		c("land", "Land", card.Data{
			Name: "Mistral Shore", TypeLine: "Land", Rarity: "rare",
			OracleText:   "Mistral Shore enters tapped unless you control two or more other lands.\n{T}: Add {W} or {U}.",
			ProducedMana: []card.Color{W, U},
		}),
		c("instant", "Instant", card.Data{
			Name: "Stormbound Reply", ManaCost: "{U}{R}", TypeLine: "Instant", Rarity: "uncommon",
			OracleText: "Counter target spell unless its controller pays {3}. If they do not, Stormbound Reply deals 2 damage to any target.",
			Colors:     []card.Color{U, R},
		}),
		c("long-text", "Long title and text", card.Data{
			Name: "The Unreasonably Long-Titled Archivist of Forgotten Spellbooks", ManaCost: "{4}{W}{W}{U}{U}", TypeLine: "Legendary Creature - Human Wizard Advisor Scribe", Rarity: "mythic",
			OracleText: "Flying, vigilance, ward {2}\nWhen The Unreasonably Long-Titled Archivist enters, look at the top seven cards of your library. You may reveal an instant or sorcery card from among them and put it into your hand. Put the rest on the bottom of your library in a random order.\nWhenever you cast an instant or sorcery spell, you may pay {1}{U}. If you do, copy that spell and you may choose new targets for the copy. Activate only once each turn.\n{3}{W}{U}, {T}: Return target card from your graveyard to your hand, then exile this creature.",
			FlavorText: "\"Every book is a door, and she has read every door in the world.\"",
			Power:      "6", Toughness: "7", Colors: []card.Color{W, U},
		}),
	}
}
