package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/odevine/mimic/engine/card"
)

// cardEditor is the center pane: an editable view of the card fields the normal
// template renders. It edits a copy of the fetched card, so the original can be
// restored, and never touches the network itself
type cardEditor struct {
	name      *widget.Entry
	manaCost  *widget.Entry
	colors    *widget.Entry
	typeLine  *widget.Entry
	oracle    *widget.Entry
	flavor    *widget.Entry
	power     *widget.Entry
	toughness *widget.Entry
	loyalty   *widget.Entry
	artist    *widget.Entry

	setCode   *widget.Entry
	collector *widget.Entry
	rarity    *widget.Entry
	released  *widget.Entry
	language  *widget.Entry

	content fyne.CanvasObject
}

// newCardEditor builds the editor widgets and lays them out. onSubmit fires when
// the user presses Enter in a single-line field, a shortcut for the render
// button so a quick tweak previews without reaching for the mouse
func newCardEditor(onSubmit func()) *cardEditor {
	e := &cardEditor{
		name:      widget.NewEntry(),
		manaCost:  widget.NewEntry(),
		colors:    widget.NewEntry(),
		typeLine:  widget.NewEntry(),
		oracle:    widget.NewMultiLineEntry(),
		flavor:    widget.NewMultiLineEntry(),
		power:     widget.NewEntry(),
		toughness: widget.NewEntry(),
		loyalty:   widget.NewEntry(),
		artist:    widget.NewEntry(),
		setCode:   widget.NewEntry(),
		collector: widget.NewEntry(),
		rarity:    widget.NewEntry(),
		released:  widget.NewEntry(),
		language:  widget.NewEntry(),
	}

	e.manaCost.SetPlaceHolder("{2}{U}{U}")
	e.colors.SetPlaceHolder("WUBRG letters, drives the frame")
	e.released.SetPlaceHolder("YYYY-MM-DD")
	for _, m := range []*widget.Entry{e.oracle, e.flavor} {
		m.Wrapping = fyne.TextWrapWord
	}
	e.oracle.SetMinRowsVisible(6)
	e.flavor.SetMinRowsVisible(3)

	// Enter in a single-line field previews, the multiline fields keep Enter for
	// newlines and rely on the render button
	for _, s := range []*widget.Entry{
		e.name, e.manaCost, e.colors, e.typeLine, e.power, e.toughness,
		e.loyalty, e.artist, e.setCode, e.collector, e.rarity, e.released, e.language,
	} {
		s.OnSubmitted = func(string) { onSubmit() }
	}

	cardForm := form(
		"Name", e.name,
		"Mana cost", e.manaCost,
		"Colors", e.colors,
		"Type line", e.typeLine,
		"Rules text", e.oracle,
		"Flavor text", e.flavor,
		"Power", e.power,
		"Toughness", e.toughness,
		"Loyalty", e.loyalty,
		"Artist", e.artist,
	)
	printing := form(
		"Set", e.setCode,
		"Collector #", e.collector,
		"Rarity", e.rarity,
		"Released", e.released,
		"Language", e.language,
	)

	e.content = container.NewVBox(
		cardForm,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Printing", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		printing,
	)
	return e
}

// load fills every field from the card. It runs on the UI goroutine
func (e *cardEditor) load(d *card.Data) {
	e.name.SetText(d.Name)
	e.manaCost.SetText(d.ManaCost)
	e.colors.SetText(colorsToString(d.Colors))
	e.typeLine.SetText(d.TypeLine)
	e.oracle.SetText(d.OracleText)
	e.flavor.SetText(d.FlavorText)
	e.power.SetText(d.Power)
	e.toughness.SetText(d.Toughness)
	e.loyalty.SetText(d.Loyalty)
	e.artist.SetText(d.Artist)
	e.setCode.SetText(d.SetCode)
	e.collector.SetText(d.CollectorNumber)
	e.rarity.SetText(d.Rarity)
	e.released.SetText(d.ReleasedAt)
	e.language.SetText(d.Language)
}

// apply returns a copy of base with the edited fields overlaid. base is left
// untouched so a reset can restore it. The art is carried separately, so the
// copied ArtworkURL is only a record and never refetched on a re-render
func (e *cardEditor) apply(base *card.Data) *card.Data {
	d := *base
	d.Name = e.name.Text
	d.ManaCost = e.manaCost.Text
	d.Colors = parseColors(e.colors.Text)
	d.TypeLine = e.typeLine.Text
	d.OracleText = e.oracle.Text
	d.FlavorText = e.flavor.Text
	d.Power = e.power.Text
	d.Toughness = e.toughness.Text
	d.Loyalty = e.loyalty.Text
	d.Artist = e.artist.Text
	d.SetCode = e.setCode.Text
	d.CollectorNumber = e.collector.Text
	d.Rarity = e.rarity.Text
	d.ReleasedAt = e.released.Text
	d.Language = e.language.Text
	return &d
}

// form pairs labels with fields in a two-column form layout. Arguments alternate
// label string, field widget
func form(pairs ...any) *fyne.Container {
	objs := make([]fyne.CanvasObject, 0, len(pairs))
	for i := 0; i+1 < len(pairs); i += 2 {
		label := widget.NewLabel(pairs[i].(string))
		objs = append(objs, label, pairs[i+1].(fyne.CanvasObject))
	}
	return container.New(layout.NewFormLayout(), objs...)
}

// colorsToString renders a color slice as its bare WUBRG letters
func colorsToString(cs []card.Color) string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString(string(c))
	}
	return b.String()
}

// parseColors reads WUBRG(C) letters into a color slice, ignoring anything else
// and dropping duplicates so the frame logic sees a clean set
func parseColors(s string) []card.Color {
	seen := make(map[card.Color]bool)
	var out []card.Color
	for _, r := range strings.ToUpper(s) {
		c := card.Color(string(r))
		switch c {
		case card.White, card.Blue, card.Black, card.Red, card.Green, card.Colorless:
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}
