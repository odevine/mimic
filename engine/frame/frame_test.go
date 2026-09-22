package frame

import (
	"testing"

	"github.com/odevine/mimic/engine/card"
)

func wubrg(s ...card.Color) []card.Color { return s }

// Each case names the per-slot keys Derive should produce, verified against
// real Scryfall data for these cards.
var deriveCases = []struct {
	name                        string
	data                        card.Data
	bg, pin, twins, pt, crown   string
	creature, legendary, isLand bool
}{
	{
		name: "mono red instant", data: card.Data{TypeLine: "Instant", ManaCost: "{R}", Colors: wubrg(card.Red)},
		bg: "r", pin: "r", twins: "r", pt: "r", crown: "r",
	},
	{
		name: "gold two-color creature", data: card.Data{TypeLine: "Creature — Wolf", ManaCost: "{G}{W}", Colors: wubrg(card.Green, card.White), Power: "3", Toughness: "3"},
		bg: "gold", pin: "wg", twins: "gold", pt: "gold", crown: "wg", creature: true,
	},
	{
		name: "pure-hybrid creature", data: card.Data{TypeLine: "Creature — Ouphe", ManaCost: "{1}{G/W}{G/W}", Colors: wubrg(card.Green, card.White), Power: "3", Toughness: "2"},
		bg: "wg", pin: "wg", twins: "colorless", pt: "colorless", crown: "wg", creature: true,
	},
	{
		name: "mixed hybrid and pure is gold", data: card.Data{TypeLine: "Creature — Demon", ManaCost: "{1}{B}{B/G}{G}", Colors: wubrg(card.Black, card.Green), Power: "3", Toughness: "3"},
		bg: "gold", pin: "bg", twins: "gold", pt: "gold", crown: "bg", creature: true,
	},
	{
		name: "colored artifact creature", data: card.Data{TypeLine: "Artifact Creature — Bird", ManaCost: "{U}{B}", Colors: wubrg(card.Blue, card.Black), Power: "1", Toughness: "1"},
		bg: "artifact", pin: "ub", twins: "gold", pt: "gold", crown: "ub", creature: true,
	},
	{
		name: "vehicle", data: card.Data{TypeLine: "Artifact — Vehicle", ManaCost: "{2}", Power: "3", Toughness: "3"},
		bg: "vehicle", pin: "artifact", twins: "artifact", pt: "vehicle", crown: "artifact", creature: true,
	},
	{
		name: "colorless eldrazi", data: card.Data{TypeLine: "Creature — Eldrazi", ManaCost: "{4}{C}", Power: "5", Toughness: "5"},
		bg: "colorless", pin: "colorless", twins: "colorless", pt: "colorless", crown: "colorless", creature: true,
	},
	{
		name: "mono legendary land", data: card.Data{TypeLine: "Legendary Land", ProducedMana: wubrg(card.Green)},
		bg: "land", pin: "g", twins: "g", pt: "g", crown: "g", legendary: true, isLand: true,
	},
	{
		name: "land creature", data: card.Data{TypeLine: "Land Creature — Forest Dryad", ProducedMana: wubrg(card.Green), Power: "1", Toughness: "1"},
		bg: "land", pin: "g", twins: "g", pt: "g", crown: "g", creature: true, isLand: true,
	},
	{
		name: "two-color land", data: card.Data{TypeLine: "Land — Forest Island", ProducedMana: wubrg(card.Green, card.Blue)},
		bg: "land", pin: "ug", twins: "gold", pt: "gold", crown: "ug", isLand: true,
	},
	{
		name: "tri land is gold", data: card.Data{TypeLine: "Land — Island Mountain", ProducedMana: wubrg(card.Red, card.Blue, card.White)},
		bg: "gold", pin: "gold", twins: "gold", pt: "gold", crown: "gold", isLand: true,
	},
	{
		name: "any-color land is gold", data: card.Data{TypeLine: "Land", ProducedMana: wubrg(card.Black, card.Green, card.Red, card.Blue, card.White)},
		bg: "gold", pin: "gold", twins: "gold", pt: "gold", crown: "gold", isLand: true,
	},
	{
		name: "wastes plain land", data: card.Data{TypeLine: "Basic Land", ProducedMana: wubrg(card.Colorless)},
		bg: "land", pin: "land", twins: "land", pt: "land", crown: "land", isLand: true,
	},
	// Fetch lands: produced mana empty, read from oracle text.
	{
		name: "typed untapped fetch", data: card.Data{TypeLine: "Land", OracleText: "Sacrifice this land: Search your library for a Plains or Island card, put it onto the battlefield, then shuffle."},
		bg: "land", pin: "wu", twins: "gold", pt: "gold", crown: "wu", isLand: true,
	},
	{
		name: "generic untapped fetch is gold", data: card.Data{TypeLine: "Land", OracleText: "Sacrifice this land: Search your library for a basic land card, put it onto the battlefield, then shuffle."},
		bg: "gold", pin: "gold", twins: "gold", pt: "gold", crown: "gold", isLand: true,
	},
	{
		name: "conditional-untap fetch is gold", data: card.Data{TypeLine: "Land", OracleText: "Search your library for a basic land card, put it onto the battlefield tapped, then if you control four or more lands, untap that land."},
		bg: "gold", pin: "gold", twins: "gold", pt: "gold", crown: "gold", isLand: true,
	},
	{
		name: "always-tapped generic fetch is plain", data: card.Data{TypeLine: "Land", OracleText: "Search your library for a basic land card, put it onto the battlefield tapped, then shuffle."},
		bg: "land", pin: "land", twins: "land", pt: "land", crown: "land", isLand: true,
	},
	{
		name: "always-tapped typed fetch is plain", data: card.Data{TypeLine: "Land", ProducedMana: wubrg(card.Colorless), OracleText: "Search your library for a Plains, Island, or Forest card, put it onto the battlefield tapped, then shuffle."},
		bg: "land", pin: "land", twins: "land", pt: "land", crown: "land", isLand: true,
	},
	{
		name: "ghost quarter is plain", data: card.Data{TypeLine: "Land", ProducedMana: wubrg(card.Colorless), OracleText: "Sacrifice this land: Destroy target land. Its controller may search their library for a basic land card, put it onto the battlefield, then shuffle."},
		bg: "land", pin: "land", twins: "land", pt: "land", crown: "land", isLand: true,
	},
}

func TestDeriveNyx(t *testing.T) {
	cases := []struct {
		typeLine string
		want     bool
	}{
		{"Enchantment Creature — God", true},
		{"Legendary Enchantment", true},
		{"Creature — Angel", false},
		{"Artifact", false},
	}
	for _, c := range cases {
		if got := Derive(&card.Data{TypeLine: c.typeLine}).Nyx; got != c.want {
			t.Errorf("%q: nyx = %v, want %v", c.typeLine, got, c.want)
		}
	}
}

func TestDerive(t *testing.T) {
	for _, c := range deriveCases {
		t.Run(c.name, func(t *testing.T) {
			f := Derive(&c.data)
			for _, check := range []struct{ slot, got, want string }{
				{"background", f.Background, c.bg},
				{"pinlines", f.Pinlines, c.pin},
				{"twins", f.Twins, c.twins},
				{"ptBox", f.PTBox, c.pt},
				{"crown", f.Crown, c.crown},
			} {
				if check.got != check.want {
					t.Errorf("%s = %q, want %q", check.slot, check.got, check.want)
				}
			}
			if f.Creature != c.creature {
				t.Errorf("creature = %v, want %v", f.Creature, c.creature)
			}
			if f.Legendary != c.legendary {
				t.Errorf("legendary = %v, want %v", f.Legendary, c.legendary)
			}
			if f.Land != c.isLand {
				t.Errorf("land = %v, want %v", f.Land, c.isLand)
			}
		})
	}
}

func TestAllHybrid(t *testing.T) {
	cases := map[string]bool{
		"{1}{G/W}{G/W}":   true,  // Kitchen Finks
		"{W/U}{W/U}":      true,  // pure hybrid
		"{G}{W}":          false, // pure pips
		"{1}{B}{B/G}{G}":  false, // mixed
		"{G/U}{W}":        false, // hybrid plus a pure pip
		"{2/W}{2/W}{2/W}": false, // monocolor hybrid
		"{B/P}":           false, // Phyrexian
		"{3}":             false, // no colored pip
		"":                false,
	}
	for cost, want := range cases {
		if got := allHybrid(cost); got != want {
			t.Errorf("allHybrid(%q) = %v, want %v", cost, got, want)
		}
	}
}
