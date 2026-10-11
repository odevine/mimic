// Package frame derives the WUBRG-driven classification a Magic card's frame
// reads: which colors its border, pinlines, name box, and crown want, and the
// type-line predicates (land, legendary, creature, nyx) a layer's condition
// checks against. This is Magic's own frame rules, the same for any template
// that draws a WUBRG-colored border, not the pixels of a particular one. A
// template maps these keys onto its own layer names and conditions
package frame

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/odevine/mimic/engine/card"
)

// Keys holds the per-slot color keys and the boolean signals a WUBRG frame's
// layers are chosen by. The slots are kept independent because a two-color card
// can want dual pinlines while its name box stays gold, and an artifact can want
// an artifact background while its pinlines stay colored
type Keys struct {
	Background Material // frame body and edge: mono, dual, gold, artifact, vehicle, colorless, or land
	Pinlines   Material // pinlines and rules textbox: mono, dual, gold, artifact, colorless, or land
	Twins      Material // name and type boxes: mono, gold, artifact, colorless, or land, never dual
	PTBox      Material // twins plus a vehicle variant
	Crown      Material // legendary crown: mono, dual, gold, artifact, colorless, or land
	// Indicator is the color indicator's art, named for its colors in WUBRG order
	// such as "u" or "ubr", or none for a face with no indicator
	Indicator Material
	// TransformIcon names the icon a double-faced face prints in its corner, such as
	// "sunmoondfc", or is none for a single-faced card
	TransformIcon Material
	Land          bool
	Legendary     bool
	Creature      bool // has printed power and toughness, which covers Vehicles
	Nyx           bool // enchantment, so the nyx frame stands in for the background
	Companion     bool // printed with the companion frame
	// Front and Back mark which face of a double-faced card is rendering. A
	// single-faced card is neither
	Front, Back bool
	// SetSymbol marks a card whose set symbol draws, which the renderer sets once
	// it knows the symbol and the manifest's place for it. The type line reads it
	// to give up the room the symbol takes
	SetSymbol bool
	// Fuse marks a fuse split card, which prints a bar across both halves. The bar
	// runs the halves' pinline colors, each half's colors in order, so a red half
	// beside a white one is red then white. FuseKey colors its textbox, gold when
	// the run has more than three colors, and FusePinKey colors its pinline, which
	// runs up to four. Both are none for any other card
	Fuse       bool
	FuseKey    Material
	FusePinKey Material
}

// iconRight reports whether the transform icon sits at the right end of the
// title bar, which a triangle back face prints in place of the left corner
func (k Keys) iconRight() bool {
	return k.Back && k.TransformIcon == Named(triangleIcon)
}

// Side is which face of a card is rendering, which a double-faced frame reads
// to pick its front or back art
type Side int

const (
	Single Side = iota
	Front
	Back
)

// Derive reads a single-faced card once into the keys and signals a
// template's render loop needs
func Derive(d *card.Data) Keys { return DeriveFace(d, Single) }

// DeriveFace reads the face d prints into the keys a template's render loop
// needs, with side marking whether it is the front or back of a double-faced
// card
func DeriveFace(d *card.Data, side Side) Keys {
	tl := strings.ToLower(d.TypeLine)
	isLand := strings.Contains(tl, "land")
	isVehicle := strings.Contains(tl, "vehicle")
	isArtifact := strings.Contains(tl, "artifact")

	colors := frameColors(d, isLand)
	pureHybrid := len(colors) == 2 && allHybrid(d.ManaCost)

	k := Keys{
		Background: backgroundMaterial(isLand, isVehicle, isArtifact, colors, pureHybrid),
		Pinlines:   pinlineMaterial(isLand, isArtifact, colors),
		Twins:      twinsMaterial(isLand, isVehicle, isArtifact, colors, pureHybrid, false),
		PTBox:      twinsMaterial(isLand, isVehicle, isArtifact, colors, pureHybrid, true),
		Crown:      crownMaterial(isLand, isArtifact, colors),
		Indicator:  ColorSet(wubrgOnly(d.ColorIndicator)),
		Land:       isLand,
		Legendary:  strings.Contains(tl, "legendary"),
		Creature:   d.Power != "" && d.Toughness != "",
		Nyx:        strings.Contains(tl, "enchantment"),
		Companion:  slices.Contains(d.FrameEffects, "companion"),
		Front:      side == Front,
		Back:       side == Back,
		Fuse:       d.HasFuse(),
	}
	if k.Fuse {
		k.FuseKey, k.FusePinKey = fuseKeys(d)
	}
	if side != Single {
		k.TransformIcon = Named(transformIcon(d.FrameEffects))
	}
	return k
}

// fuseKeys are the colors of a fuse bar, the pinline colors of the two halves in
// order. A half with no pinline of colors, such as a colorless one, makes the bar
// gold, and so does a textbox run of more than three colors
func fuseKeys(d *card.Data) (textbox, pinlines Material) {
	var merged string
	for i := 0; i < 2; i++ {
		h := d.Half(i)
		tl := strings.ToLower(h.TypeLine)
		m := pinlineMaterial(strings.Contains(tl, "land"), strings.Contains(tl, "artifact"), frameColors(h, strings.Contains(tl, "land")))
		if k := m.Kind(); k != KindMono && k != KindRun {
			return Gold, Gold
		}
		if run := m.String(); i == 0 || run != merged {
			merged += run
		}
	}
	if len(merged) > 4 {
		return Gold, Gold
	}
	if len(merged) > 3 {
		return Gold, fromLetters(merged)
	}
	return fromLetters(merged), fromLetters(merged)
}

// transformIcons maps the Scryfall frame effects that name a double-faced
// card's corner icon to the name of that icon's art. waxingandwaningmoondfc
// prints much the same sun and moon, so it shares that art
var transformIcons = map[string]string{
	"sunmoondfc": "sunmoondfc", "compasslanddfc": "compasslanddfc", "originpwdfc": "originpwdfc",
	"mooneldrazidfc": "mooneldrazidfc", "convertdfc": "convertdfc", "upsidedowndfc": "upsidedowndfc",
	"fandfc": "fandfc", "waxingandwaningmoondfc": "sunmoondfc",
}

// triangleIcon is the up and down triangle, filed under convertdfc. It stands
// in for a card whose frame effects name no icon, since Scryfall lists none
// for the transform cards printed since March of the Machine, which all print it
const triangleIcon = "convertdfc"

// transformIcon picks the first frame effect naming an icon, since a card can
// list it beside unrelated effects such as legendary
func transformIcon(effects []string) string {
	for _, e := range effects {
		if icon, ok := transformIcons[e]; ok {
			return icon
		}
	}
	return triangleIcon
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

func backgroundMaterial(isLand, isVehicle, isArtifact bool, colors []card.Color, pureHybrid bool) Material {
	switch {
	case isLand:
		// The land frame body stays a land at any color count, and the pinlines
		// and name box carry the gold of a land that makes three or more colors
		return Land
	case isVehicle:
		return Vehicle
	case isArtifact:
		return Artifact
	}
	switch len(colors) {
	case 0:
		return Colorless
	case 1:
		return Mono(colors[0])
	case 2:
		if pureHybrid {
			return Dual(colors)
		}
		return Gold
	default:
		return Gold
	}
}

// pinlineMaterial colors the pinlines and rules textbox, which are dual for any
// two-color card, hybrid or gold. Only a colorless card falls to artifact or
// colorless
func pinlineMaterial(isLand, isArtifact bool, colors []card.Color) Material {
	switch len(colors) {
	case 0:
		switch {
		case isLand:
			return Land
		case isArtifact:
			return Artifact
		default:
			return Colorless
		}
	case 1:
		return Mono(colors[0])
	case 2:
		return Dual(colors)
	default:
		return Gold
	}
}

// twinsMaterial colors the name, type, and P/T boxes, which have no dual variant.
// A pure-hybrid two-color card reads as colorless here, a gold two-color card as
// gold, and a two-color land as a land. ptBox is true for the P/T box, which
// alone has a vehicle variant
func twinsMaterial(isLand, isVehicle, isArtifact bool, colors []card.Color, pureHybrid, ptBox bool) Material {
	if isVehicle {
		if ptBox {
			return Vehicle
		}
		return Artifact
	}
	switch len(colors) {
	case 0:
		switch {
		case isLand:
			return Land
		case isArtifact:
			return Artifact
		default:
			return Colorless
		}
	case 1:
		return Mono(colors[0])
	case 2:
		switch {
		case isLand:
			return Land
		case pureHybrid:
			return Colorless
		}
		return Gold
	default:
		return Gold
	}
}

// crownMaterial colors the legendary crown, which has dual variants, so a
// two-color legendary gets a dual crown rather than gold
func crownMaterial(isLand, isArtifact bool, colors []card.Color) Material {
	switch len(colors) {
	case 0:
		switch {
		case isArtifact:
			return Artifact
		case isLand:
			return Land
		default:
			return Colorless
		}
	case 1:
		return Mono(colors[0])
	case 2:
		return Dual(colors)
	default:
		return Gold
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
