package card

import (
	"encoding/json"
	"fmt"
)

// scryfallCard is the subset of Scryfall's card JSON this project reads.
// Nothing outside this file depends on this shape. toData is the only bridge
// between it and *Data
type scryfallCard struct {
	Name            string    `json:"name"`
	ManaCost        string    `json:"mana_cost"`
	TypeLine        string    `json:"type_line"`
	OracleText      string    `json:"oracle_text"`
	FlavorText      string    `json:"flavor_text"`
	Power           string    `json:"power"`
	Toughness       string    `json:"toughness"`
	Loyalty         string    `json:"loyalty"`
	Colors          []string  `json:"colors"`
	ColorIdentity   []string  `json:"color_identity"`
	ProducedMana    []string  `json:"produced_mana"`
	Rarity          string    `json:"rarity"`
	CollectorNumber string    `json:"collector_number"`
	Set             string    `json:"set"`
	Lang            string    `json:"lang"`
	ReleasedAt      string    `json:"released_at"`
	Artist          string    `json:"artist"`
	ImageURIs       imageURIs `json:"image_uris"`
	CardFaces       []face    `json:"card_faces"`
}

// scryfallList is the subset of Scryfall's list response the search endpoint
// returns. Only the first page is read, so has_more and next_page are ignored
type scryfallList struct {
	Data []scryfallCard `json:"data"`
}

type imageURIs struct {
	ArtCrop string `json:"art_crop"`
}

// face is one entry of a double-faced card's card_faces array. It carries the
// per-face fields that live at the top level on a single-faced card
type face struct {
	Name       string    `json:"name"`
	ManaCost   string    `json:"mana_cost"`
	TypeLine   string    `json:"type_line"`
	OracleText string    `json:"oracle_text"`
	FlavorText string    `json:"flavor_text"`
	Power      string    `json:"power"`
	Toughness  string    `json:"toughness"`
	Loyalty    string    `json:"loyalty"`
	Colors     []string  `json:"colors"`
	Artist     string    `json:"artist"`
	ImageURIs  imageURIs `json:"image_uris"`
}

// toData maps a Scryfall card into *Data.
//
// Modal double-faced and transform cards carry their real per-face fields in
// card_faces, with the top-level fields empty, so this falls back to the front
// face. Full double-faced support needs its own template concept, a back face
// rendered as a second pass, which v1 does not cover
func (sc *scryfallCard) toData() *Data {
	d := &Data{
		Name:            sc.Name,
		ManaCost:        sc.ManaCost,
		TypeLine:        sc.TypeLine,
		OracleText:      sc.OracleText,
		FlavorText:      sc.FlavorText,
		Power:           sc.Power,
		Toughness:       sc.Toughness,
		Loyalty:         sc.Loyalty,
		Colors:          toColors(sc.Colors),
		ColorIdentity:   toColors(sc.ColorIdentity),
		ProducedMana:    toColors(sc.ProducedMana),
		Rarity:          sc.Rarity,
		CollectorNumber: sc.CollectorNumber,
		SetCode:         sc.Set,
		Language:        sc.Lang,
		ReleasedAt:      sc.ReleasedAt,
		Artist:          sc.Artist,
		ArtworkURL:      sc.ImageURIs.ArtCrop,
	}

	// An empty type line with faces present marks a DFC whose per-face fields
	// live one level down. ColorIdentity, rarity, set, and collector number
	// stay shared at the top level
	if sc.TypeLine == "" && len(sc.CardFaces) > 0 {
		f := sc.CardFaces[0]
		d.Name = f.Name
		d.ManaCost = f.ManaCost
		d.TypeLine = f.TypeLine
		d.OracleText = f.OracleText
		d.FlavorText = f.FlavorText
		d.Power = f.Power
		d.Toughness = f.Toughness
		d.Loyalty = f.Loyalty
		d.Colors = toColors(f.Colors)
		d.Artist = f.Artist
		d.ArtworkURL = f.ImageURIs.ArtCrop
	}
	return d
}

func toColors(in []string) []Color {
	if len(in) == 0 {
		return nil
	}
	out := make([]Color, len(in))
	for i, s := range in {
		out[i] = Color(s)
	}
	return out
}

// FromScryfallJSON maps one Scryfall card object into *Data. It reads the card
// JSON the API returns and each line of a bulk data file alike, so a caller
// holding Scryfall data from any source maps it the same way a lookup does
func FromScryfallJSON(raw []byte) (*Data, error) {
	var sc scryfallCard
	if err := json.Unmarshal(raw, &sc); err != nil {
		return nil, fmt.Errorf("scryfall: decoding card: %w", err)
	}
	return sc.toData(), nil
}

// artHost is Scryfall's image host, which carries no API rate limit
const artHost = "https://cards.scryfall.io"

// ArtCropURL is the art crop of a card's front face on Scryfall's image host,
// built from the card's Scryfall id. The host files each image under the first
// two characters of the id, as in art_crop/front/0/2/02645651-....jpg. It
// returns "" for an id too short to file
func ArtCropURL(id string) string {
	if len(id) < 2 {
		return ""
	}
	return fmt.Sprintf("%s/art_crop/front/%c/%c/%s.jpg", artHost, id[0], id[1], id)
}
