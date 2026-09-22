package card

import (
	"fmt"
	"strconv"
	"strings"
)

// TextFor returns the printed text for a named card-anatomy box: title, mana,
// type, pt, artist, collector, or set. It is the standard vocabulary a WUBRG
// frame's boxes draw from, shared by any template rather than reimplemented
// per frame. Unknown names return empty so a manifest can define boxes this
// mapping does not fill.
//
// The oracle box is rules text layered with flavor text as separate styled
// parts by whatever builds a card's text parts, so this direct lookup returns
// the rules text alone
func TextFor(name string, d *Data) string {
	switch name {
	case "title":
		return d.Name
	case "mana":
		return d.ManaCost
	case "type":
		return d.TypeLine
	case "oracle":
		return d.OracleText
	case "pt":
		switch {
		case d.Loyalty != "":
			return d.Loyalty
		case d.Power != "" || d.Toughness != "":
			return d.Power + "/" + d.Toughness
		default:
			return ""
		}
	case "artist":
		return d.Artist
	case "collector":
		return CollectorLine(d)
	case "set":
		return SetLine(d)
	default:
		return ""
	}
}

// CollectorLine formats the rarity and collector number, as "R 0177". Real
// cards print the set total after the number ("0177/302"), which Scryfall's
// card data does not carry, so it is left out
func CollectorLine(d *Data) string {
	num := padCollectorNumber(d.CollectorNumber)
	if num == "" {
		return ""
	}
	if letter := RarityLetter(d.Rarity); letter != "" {
		return letter + " " + num
	}
	return num
}

// padCollectorNumber left-pads a purely numeric collector number to four
// digits, so 177 reads as 0177. A number carrying a non-digit part is left as
// is
func padCollectorNumber(n string) string {
	if v, err := strconv.Atoi(n); err == nil {
		return fmt.Sprintf("%04d", v)
	}
	return n
}

// RarityLetter is the single-letter rarity code the collector line prints
func RarityLetter(rarity string) string {
	switch strings.ToLower(rarity) {
	case "common":
		return "C"
	case "uncommon":
		return "U"
	case "rare":
		return "R"
	case "mythic":
		return "M"
	case "special":
		return "S"
	case "bonus":
		return "B"
	case "":
		return ""
	default:
		return strings.ToUpper(rarity[:1])
	}
}

// SetLine formats the set code and printing language, as "A25 • EN",
// defaulting to English when the card carries no language
func SetLine(d *Data) string {
	if d.SetCode == "" {
		return ""
	}
	lang := strings.ToUpper(d.Language)
	if lang == "" {
		lang = "EN"
	}
	return strings.ToUpper(d.SetCode) + " • " + lang
}

// The two halves of the printed copyright line, with the printing's year
// between them
const (
	copyrightMarks  = "™ & ©"
	copyrightHolder = "Wizards of the Coast"
)

// CopyrightLine is the boilerplate line a printed card carries at the bottom:
// the printing's year between the symbols and the holder. override, when set,
// stands in whole, and a printing with no date drops the year rather than
// guessing one
func CopyrightLine(override string, d *Data) string {
	if override != "" {
		return override
	}
	if year := d.Year(); year != "" {
		return copyrightMarks + " " + year + " " + copyrightHolder
	}
	return copyrightMarks + " " + copyrightHolder
}
