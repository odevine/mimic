package cardlist

import (
	"encoding/json"
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
}

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
	return &d
}

// editsOf reads a card back into its editable fields, the inverse of ApplyEdits
func editsOf(d *card.Data) Edits {
	var colors strings.Builder
	for _, c := range d.Colors {
		colors.WriteString(string(c))
	}
	return Edits{
		Name:      d.Name,
		ManaCost:  d.ManaCost,
		Colors:    colors.String(),
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
