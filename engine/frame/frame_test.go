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

func TestConditionMet(t *testing.T) {
	k := Keys{Land: true, Legendary: true, Creature: true}
	yes := []string{"", "land", "legendary", "creature"}
	no := []string{"nonland", "nonlegendary", "nyx", "companion", "fullart", "pt_dark", "color_indicator"}
	for _, c := range yes {
		if !k.ConditionMet(c) {
			t.Errorf("ConditionMet(%q) = false, want true", c)
		}
	}
	for _, c := range no {
		if k.ConditionMet(c) {
			t.Errorf("ConditionMet(%q) = true, want false", c)
		}
	}
}

func TestSlot(t *testing.T) {
	k := Keys{Background: "gold", Pinlines: "wg", Twins: "colorless", PTBox: "vehicle", Crown: "wg"}
	cases := map[string]string{
		"background": "gold",
		"pinlines":   "wg",
		"twins":      "colorless",
		"ptBox":      "vehicle",
		"crown":      "wg",
		"":           "",
		"border":     "", // an any-only layer's empty ColorSlot
	}
	for name, want := range cases {
		if got := k.Slot(name); got != want {
			t.Errorf("Slot(%q) = %q, want %q", name, got, want)
		}
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

func TestConditionMetFaceAndCompound(t *testing.T) {
	land := &card.Data{TypeLine: "Land", ProducedMana: wubrg(card.Blue)}
	front, back, single := DeriveFace(land, Front), DeriveFace(land, Back), Derive(land)
	cases := []struct {
		keys      Keys
		condition string
		want      bool
	}{
		{front, "front", true},
		{front, "back", false},
		{back, "back", true},
		{single, "front", false},
		{single, "back", false},
		{back, "back,land", true},
		{back, "back, land", true},
		{front, "back,land", false},
		{back, "back,nonland", false},
		{back, "back,land,nyx", false},
	}
	for _, c := range cases {
		if got := c.keys.ConditionMet(c.condition); got != c.want {
			t.Errorf("ConditionMet(%q) with front=%v back=%v = %v, want %v", c.condition, c.keys.Front, c.keys.Back, got, c.want)
		}
	}
}

func TestIndicatorSlot(t *testing.T) {
	cases := []struct {
		colors []card.Color
		want   string
	}{
		{nil, ""},
		{wubrg(card.Blue), "u"},
		{wubrg(card.Green, card.Red), "rg"},
		{wubrg(card.Green, card.White), "wg"},
		{wubrg(card.Red, card.White), "wr"},
		{wubrg(card.Green, card.Blue), "ug"},
		{wubrg(card.Red, card.Blue, card.Black), "ubr"},
	}
	for _, c := range cases {
		k := Derive(&card.Data{TypeLine: "Creature", ColorIndicator: c.colors})
		if got := k.Slot("indicator"); got != c.want {
			t.Errorf("indicator for %v = %q, want %q", c.colors, got, c.want)
		}
		if got := k.ConditionMet("color_indicator"); got != (c.want != "") {
			t.Errorf("color_indicator for %v = %v", c.colors, got)
		}
	}
}

func TestTransformIconSlot(t *testing.T) {
	cases := []struct {
		effects []string
		want    string
	}{
		{nil, "convertdfc"},
		{[]string{"legendary"}, "convertdfc"},
		{[]string{"compasslanddfc"}, "compasslanddfc"},
		{[]string{"legendary", "originpwdfc"}, "originpwdfc"},
		{[]string{"waxingandwaningmoondfc"}, "sunmoondfc"},
	}
	for _, c := range cases {
		d := &card.Data{TypeLine: "Creature", FrameEffects: c.effects}
		if got := DeriveFace(d, Back).Slot("transform_icon"); got != c.want {
			t.Errorf("icon for %v = %q, want %q", c.effects, got, c.want)
		}
	}
	if got := Derive(&card.Data{FrameEffects: []string{"sunmoondfc"}}).Slot("transform_icon"); got != "" {
		t.Errorf("single-faced icon = %q, want empty", got)
	}
}

func TestTransformIconSide(t *testing.T) {
	cases := []struct {
		effects     []string
		side        Side
		left, right bool
	}{
		{nil, Front, true, false},
		{nil, Back, false, true},
		{[]string{"convertdfc"}, Back, false, true},
		{[]string{"sunmoondfc"}, Back, true, false},
		{[]string{"fandfc"}, Back, true, false},
		{nil, Single, false, false},
	}
	for _, c := range cases {
		k := DeriveFace(&card.Data{TypeLine: "Creature", FrameEffects: c.effects}, c.side)
		if got := k.ConditionMet("icon_left"); got != c.left {
			t.Errorf("icon_left for %v on side %d = %v, want %v", c.effects, c.side, got, c.left)
		}
		if got := k.ConditionMet("icon_right"); got != c.right {
			t.Errorf("icon_right for %v on side %d = %v, want %v", c.effects, c.side, got, c.right)
		}
	}
}

func TestFuseKeys(t *testing.T) {
	fuse := func(a, b string) card.Data {
		return card.Data{Layout: "split", Keywords: []string{"Fuse"}, Faces: []card.Face{
			{ManaCost: a, TypeLine: "Instant"}, {ManaCost: b, TypeLine: "Instant"},
		}}
	}
	cases := []struct {
		name         string
		data         card.Data
		fuse         bool
		key, pinline string
	}{
		{"two single colors", fuse("{1}{R}", "{W}"), true, "rw", "rw"},
		{"order follows the halves", fuse("{W}", "{1}{R}"), true, "wr", "wr"},
		{"the same color twice", fuse("{1}{R}", "{R}{R}"), true, "r", "r"},
		{"a dual half and a single", fuse("{B}{G}", "{R}"), true, "bgr", "bgr"},
		{"a dual half twice", fuse("{B}{G}", "{B}{G}"), true, "bg", "bg"},
		{"a hybrid half runs green to white", fuse("{G/W}{G/W}", "{R}"), true, "gwr", "gwr"},
		{"red and white hybrid", fuse("{R/W}", "{U}"), true, "rwu", "rwu"},
		{"four colors", fuse("{B}{G}", "{W}{U}"), true, "gold", "bgwu"},
		{"a colorless half", fuse("{2}", "{R}"), true, "gold", "gold"},
		{"no keyword", card.Data{Layout: "split", Faces: []card.Face{{ManaCost: "{R}"}, {ManaCost: "{W}"}}}, false, "", ""},
	}
	for _, c := range cases {
		k := Derive(&c.data)
		if k.Fuse != c.fuse || k.FuseKey != c.key || k.FusePinKey != c.pinline {
			t.Errorf("%s: Fuse = %v, keys %q and %q, want %v, %q and %q", c.name, k.Fuse, k.FuseKey, k.FusePinKey, c.fuse, c.key, c.pinline)
		}
		if k.ConditionMet("fuse") != c.fuse || k.Slot("fuse") != c.key || k.Slot("fuse_pinlines") != c.pinline {
			t.Errorf("%s: the fuse condition and slots do not follow the keys", c.name)
		}
	}
}

func TestConditionMetSetSymbol(t *testing.T) {
	with := Keys{SetSymbol: true}
	without := Keys{}
	if !with.ConditionMet("set_symbol") || without.ConditionMet("set_symbol") {
		t.Error("set_symbol should hold exactly when the keys say a symbol draws")
	}
	// It combines with the other conditions, as a type line that gives way to
	// both a color indicator and a symbol needs
	both := Keys{SetSymbol: true, Indicator: "u"}
	if !both.ConditionMet("color_indicator,set_symbol") {
		t.Error("color_indicator,set_symbol should hold when both do")
	}
	if (Keys{Indicator: "u"}).ConditionMet("color_indicator,set_symbol") {
		t.Error("color_indicator,set_symbol held without a symbol")
	}
}
