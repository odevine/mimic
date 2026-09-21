package card

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
	Artist          string    `json:"artist"`
	ImageURIs       imageURIs `json:"image_uris"`
	CardFaces       []face    `json:"card_faces"`
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
