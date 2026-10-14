package author

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing/fstest"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

const (
	// StarterDefaultDPI is the resolution a starter is made at when none is asked for
	StarterDefaultDPI = 800
	// StarterMaxDPI is the highest resolution Starter makes a canvas for. A canvas
	// at this size is already larger than the frame layers most templates ship
	StarterMaxDPI = 1200

	// starterHeightInches is the height of a card canvas including the bleed, which
	// pairs with template.CardWidthInches
	starterHeightInches = 3.7
)

// StarterOptions names a new template and the resolution its canvas is made at
type StarterOptions struct {
	// ID is the template's id, such as "me.my-frame"
	ID   string
	Name string
	// DPI sets the canvas size, a standard card with its bleed at that resolution.
	// Zero is StarterDefaultDPI
	DPI int
}

// Starter writes a minimal source folder that builds and renders, and returns each
// file by its slash separated path. It has one layout for standard single-faced
// cards, a plain background, an art window above it, the standard text boxes, and a
// place for the set symbol, all using the engine's built-in fonts. A new template
// begins here so that its first render already shows a card
func Starter(opt StarterOptions) (map[string][]byte, error) {
	dpi := opt.DPI
	if dpi == 0 {
		dpi = StarterDefaultDPI
	}
	if dpi < template.MinDPI || dpi > StarterMaxDPI {
		return nil, fmt.Errorf("author: a starter canvas is made at %d to %d dpi, not %d", template.MinDPI, StarterMaxDPI, dpi)
	}
	w := int(math.Round(template.CardWidthInches * float64(dpi)))
	h := int(math.Round(starterHeightInches * float64(dpi)))

	background, err := solidPNG(w, h, color.NRGBA{R: 0xE9, G: 0xE2, B: 0xD3, A: 0xFF})
	if err != nil {
		return nil, err
	}
	src := Source{
		Contract: manifest.Contract, ID: opt.ID, Version: "0.1.0", Name: opt.Name,
		Description: "A starting point with a plain background and the standard text boxes",
		Parts:       map[string]Part{"background": {Dir: "parts/background"}},
	}
	layout := starterLayout(w, h, dpi)

	// The files are checked the way a build would check them, so a bad id or name
	// is reported here and not on the first render
	files := map[string][]byte{"parts/background/any.png": background}
	for name, v := range map[string]any{sourceFile: src, "layouts/normal.json": layout} {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, err
		}
		files[name] = append(data, '\n')
	}
	if ds := Validate(folder(files), Options{}); ds.HasErrors() {
		return nil, fmt.Errorf("author: %w", ds.Err())
	}
	return files, nil
}

// starterRef is the canvas the starter's boxes are laid out on, which they are
// scaled from
const starterRefW, starterRefH = 3264, 4440

func starterLayout(w, h, dpi int) manifest.Layout {
	sx, sy := float64(w)/starterRefW, float64(h)/starterRefH
	px := func(v int) int { return int(math.Round(float64(v) * sx)) }
	py := func(v int) int { return int(math.Round(float64(v) * sy)) }
	rect := func(x, y, rw, rh int) manifest.Rect {
		return manifest.Rect{X: px(x), Y: py(y), Width: px(rw), Height: py(rh)}
	}
	text := func(size float64, font, align, vAlign string) manifest.TextStyle {
		return manifest.TextStyle{FontSize: math.Round(size * sx), Font: font, Color: "#000000", Align: align, VAlign: vAlign}
	}
	title := manifest.TextBox{Rect: rect(386, 510, 2400, 200), Text: text(156, "title", "left", "baseline"), Flow: manifest.Flow{ClearOf: "mana"}}
	mana := manifest.TextBox{Rect: rect(2047, 506, 866, 200), Text: text(179, "", "right", "baseline")}
	mana.Text.MinFontSize = math.Round(179 * sx)
	mana.Text.Shadow = &manifest.Shadow{Distance: 0.0907, Angle: 25}
	oracle := manifest.TextBox{Rect: rect(347, 2750, 2569, 1244), Text: text(150, "", "left", "center")}
	oracle.Text.LineSpacing = 1
	oracle.Text.PaddingX, oracle.Text.PaddingY = px(40), py(40)
	return manifest.Layout{
		Serves: []frame.Shape{{Role: frame.RoleSingle, Kind: frame.KindStandard}},
		Canvas: manifest.Canvas{Width: w, Height: h, DPI: dpi},
		Stack:  []manifest.Entry{{ID: "background", Part: "background"}},
		TextBoxes: map[string]manifest.TextBox{
			"title":     title,
			"mana":      mana,
			"type_full": {Draws: "type", Rect: rect(385, 2650, 2531, 200), Text: text(132, "title", "left", "baseline")},
			"oracle":    oracle,
			"pt":        {Rect: rect(2490, 4030, 474, 200), Text: text(159, "title", "center", "baseline")},
			"artist":    {Rect: rect(730, 4176, 2100, 120), Text: text(77, "small-caps", "left", "baseline")},
			"collector": {Rect: rect(327, 4102, 1100, 120), Text: text(68, "info", "left", "baseline")},
			"set":       {Rect: rect(329, 4176, 400, 120), Text: text(68, "info", "left", "baseline")},
			"copyright": {Rect: rect(1500, 4102, 1437, 120), Text: text(68, "", "right", "baseline")},
		},
		Art:     []manifest.ArtWindow{{X: px(356), Y: py(607), Width: px(2552), Height: py(1856), After: "background"}},
		Symbols: map[string]manifest.Symbol{"set": {X: px(2540), Y: py(2536), Width: px(360), Height: py(150), Align: "right", VAlign: "center", Scale: 0.9}},
	}
}

// solidPNG is a w by h image of one color
func solidPNG(w, h int, c color.NRGBA) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// folder holds files as a source folder to check
func folder(files map[string][]byte) fstest.MapFS {
	out := make(fstest.MapFS, len(files))
	for name, data := range files {
		out[name] = &fstest.MapFile{Data: data}
	}
	return out
}
