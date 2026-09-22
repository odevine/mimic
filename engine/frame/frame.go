// Package frame derives the WUBRG-driven classification a Magic card's frame
// reads: which colors its border, pinlines, name box, and crown want, and the
// type-line predicates (land, legendary, creature, nyx) a layer's condition
// checks against. This is Magic's own frame rules, the same for any template
// that draws a WUBRG-colored border, not the pixels of a particular one. A
// template maps these keys onto its own layer names and conditions
package frame

import (
	"regexp"
	"sort"
	"strings"

	"github.com/odevine/mimic/engine/card"
)

// Keys holds the per-slot color keys and the boolean signals a WUBRG frame's
// layers are chosen by. The slots are kept independent because a two-color card
// can want dual pinlines while its name box stays gold, and an artifact can want
// an artifact background while its pinlines stay colored
type Keys struct {
	Background string // frame body and edge: mono/dual/gold/artifact/vehicle/colorless/land
	Pinlines   string // pinlines and rules textbox: mono/dual/gold/artifact/colorless/land
	Twins      string // name and type boxes: mono/gold/artifact/colorless/land (never dual)
	PTBox      string // twins plus a vehicle variant
	Crown      string // legendary crown: mono/dual/gold/artifact/colorless/land

	Land      bool
	Legendary bool
	Creature  bool // has printed power and toughness, which covers Vehicles
	Nyx       bool // enchantment, so the nyx frame stands in for the background
}

// Derive reads a card once into the keys and signals a template's render loop
// needs
func Derive(d *card.Data) Keys {
	tl := strings.ToLower(d.TypeLine)
	isLand := strings.Contains(tl, "land")
	isVehicle := strings.Contains(tl, "vehicle")
	isArtifact := strings.Contains(tl, "artifact")

	colors := frameColors(d, isLand)
	pureHybrid := len(colors) == 2 && allHybrid(d.ManaCost)

	return Keys{
		Background: backgroundKey(isLand, isVehicle, isArtifact, colors, pureHybrid),
		Pinlines:   pinlineKey(isLand, isArtifact, colors),
		Twins:      twinsKey(isLand, isVehicle, isArtifact, colors, pureHybrid, false),
		PTBox:      twinsKey(isLand, isVehicle, isArtifact, colors, pureHybrid, true),
		Crown:      crownKey(isLand, isArtifact, colors),
		Land:       isLand,
		Legendary:  strings.Contains(tl, "legendary"),
		Creature:   d.Power != "" && d.Toughness != "",
		Nyx:        strings.Contains(tl, "enchantment"),
	}
}

// frameColors is the WUBRG set that drives the colored slots: a land's produced
// mana (or its fetch targets), otherwise the card's own colors.
func frameColors(d *card.Data, isLand bool) []card.Color {
	if isLand {
		return landColors(d)
	}
	return canonical(wubrgOnly(d.Colors))
}

// landColors reads a land's frame colors from its produced mana, dropping
// Colorless. A land that produces no colored mana is a fetch, read from oracle.
func landColors(d *card.Data) []card.Color {
	if produced := canonical(wubrgOnly(d.ProducedMana)); len(produced) > 0 {
		return produced
	}
	return fetchColors(d.OracleText)
}

// fetchColors reads the colors a fetch land commits to. A land that always
// enters tapped is a plain colorless land. Otherwise named basic types give
// their colors and a generic "basic land" gives all five.
func fetchColors(oracle string) []card.Color {
	o := strings.ToLower(oracle)
	// Guard against lands that mention basics without self-ramping, like Ghost
	// Quarter, whose controller searches "their library", not yours.
	if !strings.Contains(o, "search your library") {
		return nil
	}
	if strings.Contains(o, "onto the battlefield tapped") && !strings.Contains(o, "untap") {
		return nil
	}
	if named := namedBasicColors(o); len(named) > 0 {
		return canonical(named)
	}
	if strings.Contains(o, "basic land") {
		return []card.Color{card.White, card.Blue, card.Black, card.Red, card.Green}
	}
	return nil
}

var basicTypeColor = map[string]card.Color{
	"plains": card.White, "island": card.Blue, "swamp": card.Black,
	"mountain": card.Red, "forest": card.Green,
}

func namedBasicColors(lowerOracle string) []card.Color {
	var out []card.Color
	for name, c := range basicTypeColor {
		if regexp.MustCompile(`\b` + name + `\b`).MatchString(lowerOracle) {
			out = append(out, c)
		}
	}
	return out
}

func backgroundKey(isLand, isVehicle, isArtifact bool, colors []card.Color, pureHybrid bool) string {
	switch {
	case isLand:
		if len(colors) >= 3 {
			return "gold"
		}
		return "land"
	case isVehicle:
		return "vehicle"
	case isArtifact:
		return "artifact"
	}
	switch len(colors) {
	case 0:
		return "colorless"
	case 1:
		return monoKey(colors[0])
	case 2:
		if pureHybrid {
			return dualKey(colors)
		}
		return "gold"
	default:
		return "gold"
	}
}

// pinlineKey colors the pinlines and rules textbox, which are dual for any
// two-color card, hybrid or gold. Only a colorless card falls to artifact or
// colorless.
func pinlineKey(isLand, isArtifact bool, colors []card.Color) string {
	switch len(colors) {
	case 0:
		switch {
		case isLand:
			return "land"
		case isArtifact:
			return "artifact"
		default:
			return "colorless"
		}
	case 1:
		return monoKey(colors[0])
	case 2:
		return dualKey(colors)
	default:
		return "gold"
	}
}

// twinsKey colors the name, type, and P/T boxes, which have no dual variant. A
// pure-hybrid two-color card reads as colorless here, a gold two-color card as
// gold. ptBox is true for the P/T box, which alone has a vehicle variant.
func twinsKey(isLand, isVehicle, isArtifact bool, colors []card.Color, pureHybrid, ptBox bool) string {
	if isVehicle {
		if ptBox {
			return "vehicle"
		}
		return "artifact"
	}
	switch len(colors) {
	case 0:
		switch {
		case isLand:
			return "land"
		case isArtifact:
			return "artifact"
		default:
			return "colorless"
		}
	case 1:
		return monoKey(colors[0])
	case 2:
		if !isLand && pureHybrid {
			return "colorless"
		}
		return "gold"
	default:
		return "gold"
	}
}

// crownKey colors the legendary crown, which has dual variants, so a two-color
// legendary gets a dual crown rather than gold.
func crownKey(isLand, isArtifact bool, colors []card.Color) string {
	switch len(colors) {
	case 0:
		switch {
		case isArtifact:
			return "artifact"
		case isLand:
			return "land"
		default:
			return "colorless"
		}
	case 1:
		return monoKey(colors[0])
	case 2:
		return dualKey(colors)
	default:
		return "gold"
	}
}

var symbolRe = regexp.MustCompile(`\{([^}]+)\}`)

// allHybrid reports whether every colored pip in a mana cost is a two-color
// hybrid symbol like {G/W}. A single pure pip ({B}), monocolor hybrid ({2/W}),
// or Phyrexian pip ({B/P}) makes it false, so only cards like Kitchen Finks
// take the hybrid frame treatment.
func allHybrid(manaCost string) bool {
	sawHybrid := false
	for _, m := range symbolRe.FindAllStringSubmatch(manaCost, -1) {
		sym := strings.ToUpper(m[1])
		if !strings.ContainsAny(sym, "WUBRG") {
			continue
		}
		if twoColorHybrid(sym) {
			sawHybrid = true
			continue
		}
		return false
	}
	return sawHybrid
}

func twoColorHybrid(sym string) bool {
	parts := strings.Split(sym, "/")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if len(p) != 1 || !strings.ContainsAny(p, "WUBRG") {
			return false
		}
	}
	return true
}

var wubrgOrder = map[card.Color]int{
	card.White: 0, card.Blue: 1, card.Black: 2, card.Red: 3, card.Green: 4,
}

func wubrgOnly(colors []card.Color) []card.Color {
	var out []card.Color
	for _, c := range colors {
		if _, ok := wubrgOrder[c]; ok {
			out = append(out, c)
		}
	}
	return out
}

func canonical(colors []card.Color) []card.Color {
	out := append([]card.Color(nil), colors...)
	sort.SliceStable(out, func(i, j int) bool { return wubrgOrder[out[i]] < wubrgOrder[out[j]] })
	return out
}

func monoKey(c card.Color) string { return strings.ToLower(string(c)) }

func dualKey(colors []card.Color) string {
	var b strings.Builder
	for _, c := range canonical(colors) {
		b.WriteString(strings.ToLower(string(c)))
	}
	return b.String()
}
