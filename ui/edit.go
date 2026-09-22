package main

import "github.com/odevine/mimic/engine/card"

// editFields is the editable half of a card: every field the center-pane form
// exposes, all as strings the way the form carries them. Colors is the raw
// WUBRG letters; parseColors turns it into the card's color slice
type editFields struct {
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

// applyEdits returns a copy of base with the edited fields overlaid. base is
// left untouched so the client can reset to it. The art is carried separately,
// so the copied ArtworkURL is only a record and never refetched on a re-render
func applyEdits(base *card.Data, e editFields) *card.Data {
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
