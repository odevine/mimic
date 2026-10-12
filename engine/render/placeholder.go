package render

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"sync"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

// PlaceholderID is the id of the built-in placeholder template
const PlaceholderID = "mimic.placeholder"

// Placeholder canvas size. It is a stand-in at a demo resolution, not a real
// frame's geometry, which is measured and calibrated once real assets are
// extracted
const (
	placeholderWidth  = 744
	placeholderHeight = 1039
)

// placeholderColors gives each color key a flat fill for its background layer.
// The keys are the color keys frame.Derive produces for background
var placeholderColors = map[string]color.NRGBA{
	"w":         {0xF8, 0xF6, 0xD8, 0xFF},
	"u":         {0xB5, 0xD7, 0xE9, 0xFF},
	"b":         {0xB6, 0xAE, 0xA8, 0xFF},
	"r":         {0xE7, 0xA9, 0x8B, 0xFF},
	"g":         {0xA3, 0xC9, 0xA8, 0xFF},
	"gold":      {0xD6, 0xBF, 0x6E, 0xFF},
	"artifact":  {0xB9, 0xC2, 0xC9, 0xFF},
	"colorless": {0xB9, 0xC2, 0xC9, 0xFF},
	"land":      {0xC3, 0xA8, 0x7F, 0xFF},
}

// PlaceholderAssets is the assets of the built-in placeholder template, held in
// memory. It is a coarse frame at demo resolution that serves an ordinary card: a
// color keyed background, a text panel, and a legendary crown. It gives a program a
// template to render with before a real one is installed, and is built the first
// time it is asked for
func PlaceholderAssets() *template.MemoryAssets {
	p := placeholder()
	return template.NewMemoryAssets(p.manifest, p.files)
}

// Placeholder loads the built-in placeholder template
func Placeholder() (*template.Template, error) {
	t, _, err := template.Load(PlaceholderAssets())
	return t, err
}

type placeholderAssets struct {
	manifest []byte
	files    map[string][]byte
}

var placeholder = sync.OnceValue(buildPlaceholder)

func buildPlaceholder() placeholderAssets {
	files := map[string][]byte{}
	bg := make(map[string]manifest.Variant, len(placeholderColors))
	for key, col := range placeholderColors {
		path := "layers/bg_" + key + ".png"
		files[path] = flat(fullRect(), col)
		bg[key] = manifest.Variant{Path: path}
	}

	// A translucent panel behind the rules text, and a crown strip at the top
	files["layers/textbox.png"] = flat(image.Rect(48, 612, 696, 984), color.NRGBA{0xF4, 0xF1, 0xE6, 0xDC})
	// A darker gold than any background so the crown stays visible on the gold
	// color key as well as the lighter ones
	files["layers/crown.png"] = flat(image.Rect(36, 30, 708, 120), color.NRGBA{0xA0, 0x7E, 0x2A, 0xFF})

	text := func(x, y, w, h int, size float64, font, align string) manifest.TextBox {
		return manifest.TextBox{
			Rect: manifest.Rect{X: x, Y: y, Width: w, Height: h},
			Text: manifest.TextStyle{FontSize: size, Font: font, Color: "#111111", Align: align},
		}
	}
	title := text(48, 40, 520, 52, 38, "title", "left")
	title.Flow.ClearOf = "mana"
	mana := text(500, 46, 208, 44, 28, "", "right")
	mana.Text.MinFontSize = 28
	m := manifest.Manifest{
		Contract:    manifest.Contract,
		ID:          PlaceholderID,
		Name:        "Placeholder",
		Description: "Flat colors where a frame's art goes",
		Parts: map[string]manifest.Part{
			"background": {Slot: "background", Variants: bg},
			"textbox":    {Variants: map[string]manifest.Variant{"any": {Path: "layers/textbox.png"}}},
			"crown":      {Variants: map[string]manifest.Variant{"any": {Path: "layers/crown.png"}}},
		},
		Layouts: map[string]manifest.Layout{
			"normal": {
				Serves: []frame.Shape{{Role: frame.RoleSingle, Kind: frame.KindStandard}},
				Canvas: manifest.Canvas{Width: placeholderWidth, Height: placeholderHeight},
				Stack: []manifest.Entry{
					{Part: "background"},
					{Part: "textbox"},
					{Part: "crown", Condition: "legendary"},
				},
				TextBoxes: map[string]manifest.TextBox{
					"title":  title,
					"mana":   mana,
					"type":   text(48, 566, 648, 34, 24, "title", "left"),
					"oracle": text(72, 636, 600, 320, 24, "", "left"),
					"pt":     text(596, 946, 112, 56, 34, "title", "center"),
				},
				Art: []manifest.ArtWindow{{X: 60, Y: 132, Width: 624, Height: 424, After: "background"}},
			},
		},
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		panic(err)
	}
	return placeholderAssets{manifest: raw, files: files}
}

func fullRect() image.Rectangle { return image.Rect(0, 0, placeholderWidth, placeholderHeight) }

// flat is a document-sized transparent PNG with fill painted into rect
func flat(rect image.Rectangle, fill color.NRGBA) []byte {
	img := image.NewNRGBA(fullRect())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
