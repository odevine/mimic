// Package card holds the printed information of a Magic card and the client
// that fetches it. It is the boundary between an external data source and the
// rest of the engine. Template code depends on *card.Data, never on a data
// source's JSON shape, so a second source can plug in without any template
// changing
package card

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
