package cardlist

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/odevine/mimic/engine/card"
)

// Edits is the editable half of a card: every field the center-pane form
// exposes, all as strings the way the form carries them. Colors is the raw
// WUBRG letters; parseColors turns it into the card's color slice
type Edits struct {
	Name      string `json:"name"`
	ManaCost  string `json:"manaCost"`
	Colors    string `json:"colors"`
	TypeLine  string `json:"typeLine"`
	Oracle    string `json:"oracle"`
	Flavor    string `json:"flavor"`
	Power     string `json:"power"`
	Toughness string `json:"toughness"`
	Loyalty   string `json:"loyalty"`
	Artist    string `json:"artist"`
	SetCode   string `json:"setCode"`
	Collector string `json:"collector"`
	Rarity    string `json:"rarity"`
	Released  string `json:"released"`
	Language  string `json:"language"`

	// The fields of each half of a split card, which prints its two halves from
	// its faces. They mean nothing to a card that is not a split card, and the
	// top-level name, cost, colors, type, rules, and flavor mean nothing to a
	// split card, which draws only its halves
	Half1Name     string `json:"half1Name"`
	Half1ManaCost string `json:"half1ManaCost"`
	Half1Colors   string `json:"half1Colors"`
	Half1TypeLine string `json:"half1TypeLine"`
	Half1Oracle   string `json:"half1Oracle"`
	Half1Flavor   string `json:"half1Flavor"`
	Half2Name     string `json:"half2Name"`
	Half2ManaCost string `json:"half2ManaCost"`
	Half2Colors   string `json:"half2Colors"`
	Half2TypeLine string `json:"half2TypeLine"`
	Half2Oracle   string `json:"half2Oracle"`
	Half2Flavor   string `json:"half2Flavor"`
}

// halfFields are the editable fields of one half, as pointers into an Edits
type halfFields struct {
	name, manaCost, colors, typeLine, oracle, flavor *string
}

// half is the fields of half i of a split card, 0 for the first
func (e *Edits) half(i int) halfFields {
	if i == 0 {
		return halfFields{&e.Half1Name, &e.Half1ManaCost, &e.Half1Colors, &e.Half1TypeLine, &e.Half1Oracle, &e.Half1Flavor}
	}
	return halfFields{&e.Half2Name, &e.Half2ManaCost, &e.Half2Colors, &e.Half2TypeLine, &e.Half2Oracle, &e.Half2Flavor}
}

// isSplit reports whether the card prints its two halves from its faces
func isSplit(d *card.Data) bool { return d.Layout == "split" && len(d.Faces) >= 2 }

// ApplyEdits returns a copy of base with the edited fields overlaid. base is
// left untouched so the client can reset to it. The art is carried separately,
// so the copied ArtworkURL is only a record and never refetched on a re-render
func ApplyEdits(base *card.Data, e Edits) *card.Data {
	d := *base
	d.Name = e.Name
	d.ManaCost = e.ManaCost
	d.Colors = parseColors(e.Colors)
	d.TypeLine = e.TypeLine
	d.OracleText = e.Oracle
	d.FlavorText = e.Flavor
	d.Power = e.Power
	d.Toughness = e.Toughness
	d.Loyalty = e.Loyalty
	d.Artist = e.Artist
	d.SetCode = e.SetCode
	d.CollectorNumber = e.Collector
	d.Rarity = e.Rarity
	d.ReleasedAt = e.Released
	d.Language = e.Language
	if isSplit(base) {
		d.Faces = slices.Clone(base.Faces)
		for i := 0; i < 2; i++ {
			h := e.half(i)
			// An edit that names nothing of a half, as from a client that predates
			// the half fields, leaves that half as fetched
			if h.empty() {
				continue
			}
			d.Faces[i].Name = *h.name
			d.Faces[i].ManaCost = *h.manaCost
			d.Faces[i].Colors = parseColors(*h.colors)
			d.Faces[i].TypeLine = *h.typeLine
			d.Faces[i].OracleText = *h.oracle
			d.Faces[i].FlavorText = *h.flavor
		}
	}
	return &d
}

// editsOf reads a card back into its editable fields, the inverse of ApplyEdits
func editsOf(d *card.Data) Edits {
	e := Edits{
		Name:      d.Name,
		ManaCost:  d.ManaCost,
		Colors:    colorLetters(d.Colors),
		TypeLine:  d.TypeLine,
		Oracle:    d.OracleText,
		Flavor:    d.FlavorText,
		Power:     d.Power,
		Toughness: d.Toughness,
		Loyalty:   d.Loyalty,
		Artist:    d.Artist,
		SetCode:   d.SetCode,
		Collector: d.CollectorNumber,
		Rarity:    d.Rarity,
		Released:  d.ReleasedAt,
		Language:  d.Language,
	}
	if isSplit(d) {
		for i := 0; i < 2; i++ {
			f, h := d.Faces[i], e.half(i)
			*h.name, *h.manaCost, *h.typeLine = f.Name, f.ManaCost, f.TypeLine
			*h.oracle, *h.flavor = f.OracleText, f.FlavorText
			*h.colors = colorLetters(d.Half(i).Colors)
		}
	}
	return e
}

// Overlay returns a copy of base with the named edit fields replaced, keyed
// the way Edits marshals. Keys it does not know are ignored, so a partial
// set from a CSV row or the review table changes only what it names
func Overlay(base *card.Data, fields map[string]string) *card.Data {
	if len(fields) == 0 {
		d := *base
		return &d
	}
	raw, _ := json.Marshal(editsOf(base))
	var m map[string]string
	_ = json.Unmarshal(raw, &m)
	for k, v := range fields {
		if _, ok := m[k]; ok {
			m[k] = v
		}
	}
	raw, _ = json.Marshal(m)
	var e Edits
	_ = json.Unmarshal(raw, &e)
	return ApplyEdits(base, e)
}

// colorLetters is the raw WUBRG letters Edits carries a color slice as
func colorLetters(colors []card.Color) string {
	var b strings.Builder
	for _, c := range colors {
		b.WriteString(string(c))
	}
	return b.String()
}

// empty reports whether none of the half's fields is set
func (h halfFields) empty() bool {
	return *h.name == "" && *h.manaCost == "" && *h.colors == "" && *h.typeLine == "" && *h.oracle == "" && *h.flavor == ""
}
