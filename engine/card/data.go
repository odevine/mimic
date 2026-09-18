// Package card holds the printed information of a Magic card and the client
// that fetches it. It is the boundary between an external data source and the
// rest of the engine. Template code depends on *card.Data, never on a data
// source's JSON shape, so a second source can plug in without any template
// changing
package card

// Color is a single WUBRG color as Scryfall spells it
type Color string

const (
	White Color = "W"
	Blue  Color = "U"
	Black Color = "B"
	Red   Color = "R"
	Green Color = "G"
)

// Data is a card's printed information. It is a plain struct, not a rules
// engine. This project renders what a card says, it does not interpret it
type Data struct {
	Name, ManaCost, TypeLine, OracleText, FlavorText string
	Power, Toughness, Loyalty                        string
	Colors, ColorIdentity                            []Color // WUBRG
	Rarity, CollectorNumber, SetCode, Artist         string
	ArtworkURL                                       string // fetched separately
}
