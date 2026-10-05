// Package card holds the printed information of a Magic card and the client
// that fetches it. It is the boundary between an external data source and the
// rest of the engine. Template code depends on *card.Data, never on a data
// source's JSON shape, so a second source can plug in without any template
// changing
package card

import "strings"

// Color is a single WUBRG color as Scryfall spells it, plus Colorless, which
// only appears in ProducedMana
type Color string

const (
	White     Color = "W"
	Blue      Color = "U"
	Black     Color = "B"
	Red       Color = "R"
	Green     Color = "G"
	Colorless Color = "C"
)

// Data is a card's printed information. It is a plain struct, not a rules
// engine. This project renders what a card says, it does not interpret it
type Data struct {
	Name, ManaCost, TypeLine, OracleText, FlavorText string
	Power, Toughness, Loyalty                        string
	Colors, ColorIdentity                            []Color // WUBRG
	// ProducedMana is the mana a land or other source can add, as Scryfall
	// reports it. It may include Colorless and is the signal a land's frame
	// colors come from, since a land's own Colors are empty
	ProducedMana                             []Color
	Rarity, CollectorNumber, SetCode, Artist string
	// Language is Scryfall's two-letter printing language, "en" for English. It
	// feeds the set line at the card bottom
	Language string
	// ReleasedAt is when this printing came out, as Scryfall writes it,
	// "YYYY-MM-DD". Its year is the one the copyright line carries
	ReleasedAt string
	ArtworkURL string // fetched separately
	// Layout is Scryfall's name for how the card is printed, such as "normal",
	// "transform", "split", or "saga". It is empty for a card typed in by hand,
	// which classifies from its type line alone
	Layout string
	// FrameEffects are Scryfall's frame_effects for the printing, such as
	// "legendary" or "sunmoondfc", which name the transform icon a double-faced
	// card prints
	FrameEffects []string
	// Keywords are Scryfall's keyword abilities for the whole card, such as Fuse
	// or Aftermath, which a layout alone does not tell apart
	Keywords []string
	// ColorIndicator is the colored dot a card whose colors its mana cost does
	// not show prints beside its type line, in WUBRG as Scryfall lists them. A
	// double-faced card's is its front face's
	ColorIndicator []Color
	// Faces holds each face's own printed fields, in Scryfall's order, for a
	// card printed with more than one: double-faced, split, adventure, and flip
	// cards. It is empty for a single-faced card. The top-level fields above
	// are the front face, and are the ones an edit changes
	Faces []Face
	// FaceIndex is which of Faces the top-level fields print, 0 for the front.
	// Face sets it on the copy it returns
	FaceIndex int
}

// Face is the printed information one face of a multi-faced card carries on
// its own. Everything else, such as rarity, set, and color identity, is shared
// by the whole card and lives on Data
type Face struct {
	Name, ManaCost, TypeLine, OracleText, FlavorText string
	Power, Toughness, Loyalty                        string
	Colors                                           []Color
	ColorIndicator                                   []Color
	Artist, ArtworkURL                               string
}

// Face returns the card as face i prints it: the shared fields with face i's
// own laid over them. Face 0, a single-faced card, and an index out of range
// return d itself, since the top-level fields already hold the front face
func (d *Data) Face(i int) *Data {
	if i <= 0 || i >= len(d.Faces) {
		return d
	}
	return d.overlay(i)
}

// overlay is the card with face i's own fields laid over the shared ones
func (d *Data) overlay(i int) *Data {
	f := d.Faces[i]
	out := *d
	out.Name = f.Name
	out.ManaCost = f.ManaCost
	out.TypeLine = f.TypeLine
	out.OracleText = f.OracleText
	out.FlavorText = f.FlavorText
	out.Power = f.Power
	out.Toughness = f.Toughness
	out.Loyalty = f.Loyalty
	out.Colors = f.Colors
	out.ColorIndicator = f.ColorIndicator
	if f.Artist != "" {
		out.Artist = f.Artist
	}
	out.ArtworkURL = f.ArtworkURL
	out.FaceIndex = i
	return &out
}

// Year is the year this printing came out, or empty when the date is missing or
// not the four-digit year Scryfall writes
func (d *Data) Year() string {
	if len(d.ReleasedAt) < 4 {
		return ""
	}
	y := d.ReleasedAt[:4]
	for _, r := range y {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return y
}

// Half returns the card as half i of a split card prints it. Face leaves index 0
// as the whole card because a double-faced card's top level is its front, but a
// split card's top level joins both halves, so every index reads from Faces. A
// half Scryfall gives no colors takes them from its mana cost, and on a fuse
// card the Fuse reminder is left out, since the card prints it once across both
// halves. An index out of range returns d itself
func (d *Data) Half(i int) *Data {
	if i < 0 || i >= len(d.Faces) {
		return d
	}
	out := d.overlay(i)
	if len(out.Colors) == 0 {
		out.Colors = ColorsFromManaCost(out.ManaCost)
	}
	if d.HasFuse() {
		out.OracleText = stripFuse(out.OracleText)
	}
	return out
}

// HasFuse reports whether the card is a fuse split card, by its keywords or, for
// a card with none, by a face's Fuse reminder line
func (d *Data) HasFuse() bool {
	for _, k := range d.Keywords {
		if strings.EqualFold(k, "fuse") {
			return true
		}
	}
	return d.FuseText() != ""
}

// FuseText is the Fuse reminder line a fuse card prints once across its bottom,
// or empty for a card without one
func (d *Data) FuseText() string {
	for _, f := range d.Faces {
		for _, line := range strings.Split(f.OracleText, "\n") {
			if strings.HasPrefix(line, fusePrefix) {
				return line
			}
		}
	}
	return ""
}

const fusePrefix = "Fuse ("

// stripFuse drops the Fuse reminder line from a face's rules text
func stripFuse(oracle string) string {
	var kept []string
	for _, line := range strings.Split(oracle, "\n") {
		if !strings.HasPrefix(line, fusePrefix) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// ColorsFromManaCost reads the colors a mana cost shows, in WUBRG order. A
// hybrid symbol counts toward both its colors and a Phyrexian one toward its
// own, while generic, colorless, snow, and X costs add none
func ColorsFromManaCost(cost string) []Color {
	seen := map[Color]bool{}
	for _, sym := range strings.Split(cost, "{") {
		sym, _, _ = strings.Cut(sym, "}")
		for _, part := range strings.Split(sym, "/") {
			seen[Color(strings.ToUpper(part))] = true
		}
	}
	var out []Color
	for _, c := range []Color{White, Blue, Black, Red, Green} {
		if seen[c] {
			out = append(out, c)
		}
	}
	return out
}
