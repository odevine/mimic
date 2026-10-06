package render

import (
	"context"
	"testing"

	"github.com/odevine/impasto/canvas"
	"github.com/odevine/impasto/raster"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/card/svgpath"
	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/template"
)

func mustSymbol(t *testing.T, svg string) *card.SetSymbol {
	t.Helper()
	icon, err := svgpath.Parse([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	return &card.SetSymbol{Code: "tst", Icon: icon}
}

// A 40 by 20 rectangle inside a larger box, so the fit has room to show
const wideSymbol = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><path d="M30 40h40v20H30z"/></svg>`

func layerOf(t *testing.T, n canvas.Node) *raster.Buffer {
	t.Helper()
	l, ok := n.(*canvas.Layer)
	if !ok {
		t.Fatalf("node is %T, want a layer", n)
	}
	return l.Content
}

// pixel returns straight linear color and alpha at x, y
func pixel(b *raster.Buffer, x, y int) (r, g, bl, a float32) {
	r, g, bl, a = b.At(x, y)
	if a > 0 {
		r, g, bl = r/a, g/a, bl/a
	}
	return
}

func opaque(b *raster.Buffer, x, y int) bool {
	_, _, _, a := b.At(x, y)
	return a > 0.99
}

func clear(b *raster.Buffer, x, y int) bool {
	_, _, _, a := b.At(x, y)
	return a < 0.01
}

func TestSymbolNodeFitsAndAligns(t *testing.T) {
	sym := mustSymbol(t, wideSymbol)
	// The ink is 40 by 20, so a 200 by 200 box scales it 5x, to the full width
	// and half the height
	spec := template.SymbolSpec{X: 100, Y: 300, Width: 200, Height: 200}
	node, err := symbolNode(sym, spec, "common", 600, 800)
	if err != nil || node == nil {
		t.Fatalf("symbolNode = %v, %v", node, err)
	}
	b := layerOf(t, node)
	if w, h := b.Bounds(); w != 600 || h != 800 {
		t.Fatalf("layer is %dx%d, want the document's 600x800", w, h)
	}
	// Full width, centered on the box's vertical middle, 100 tall
	if !opaque(b, 150, 400) || !opaque(b, 290, 400) {
		t.Error("symbol does not cover the middle of its box")
	}
	if !clear(b, 150, 340) || !clear(b, 150, 460) {
		t.Error("symbol spills above or below its fitted height")
	}
	if !clear(b, 90, 400) || !clear(b, 310, 400) {
		t.Error("symbol spills past its box")
	}

	// A taller box than the symbol's aspect, so the height is spare
	tall := template.SymbolSpec{X: 100, Y: 300, Width: 100, Height: 200}
	for name, c := range map[string]struct {
		vAlign  string
		in, out int
	}{
		"top":    {"top", 320, 380},
		"center": {"center", 400, 320},
		"bottom": {"bottom", 480, 320},
	} {
		tall.VAlign = c.vAlign
		n, err := symbolNode(sym, tall, "common", 600, 800)
		if err != nil {
			t.Fatal(err)
		}
		tb := layerOf(t, n)
		if !opaque(tb, 150, c.in) || !clear(tb, 150, c.out) {
			t.Errorf("%s: want ink at y=%d and none at y=%d", name, c.in, c.out)
		}
	}
}

func TestSymbolNodeAlignsAcrossTheBox(t *testing.T) {
	// A square icon in a wide box leaves spare width to place
	sym := mustSymbol(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10H0z"/></svg>`)
	spec := template.SymbolSpec{X: 100, Y: 100, Width: 300, Height: 100}
	for _, c := range []struct {
		align   string
		in, out int
	}{
		{"left", 150, 250},
		{"center", 250, 150},
		{"", 350, 250},
		{"right", 350, 150},
	} {
		spec.Align = c.align
		n, err := symbolNode(sym, spec, "common", 600, 400)
		if err != nil {
			t.Fatal(err)
		}
		b := layerOf(t, n)
		if !opaque(b, c.in, 150) || !clear(b, c.out, 150) {
			t.Errorf("align %q: want ink at x=%d and none at x=%d", c.align, c.in, c.out)
		}
	}
}

func TestSymbolNodeScale(t *testing.T) {
	sym := mustSymbol(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10H0z"/></svg>`)
	spec := template.SymbolSpec{X: 100, Y: 100, Width: 100, Height: 100, Scale: 0.5, Align: "center"}
	n, err := symbolNode(sym, spec, "common", 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	b := layerOf(t, n)
	if !opaque(b, 150, 150) || !opaque(b, 130, 130) || !clear(b, 120, 150) || !clear(b, 150, 120) {
		t.Error("a half scale symbol should fill the middle half of its box")
	}
}

func TestSymbolNodeRarityPaint(t *testing.T) {
	sym := mustSymbol(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10H0z"/></svg>`)
	spec := template.SymbolSpec{
		X: 100, Y: 100, Width: 100, Height: 100,
		Rarity: map[string]template.SymbolPaint{
			"common": {Stops: []template.SymbolStop{{Color: "#000000", At: 0}}},
			"rare":   {Stops: []template.SymbolStop{{Color: "#ff0000", At: 0}}},
			"mythic": {Stops: []template.SymbolStop{{Color: "#0000ff", At: 0}}},
		},
	}
	color := func(rarity string) (r, g, b float32) {
		n, err := symbolNode(sym, spec, rarity, 300, 300)
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, _ = pixel(layerOf(t, n), 150, 150)
		return
	}
	if r, g, b := color("rare"); r < 0.9 || g > 0.01 || b > 0.01 {
		t.Errorf("rare = %v %v %v, want red", r, g, b)
	}
	if r, g, b := color("Mythic"); b < 0.9 || r > 0.01 || g > 0.01 {
		t.Errorf("Mythic = %v %v %v, want blue", r, g, b)
	}
	// Bonus prints as mythic
	if _, _, b := color("bonus"); b < 0.9 {
		t.Error("bonus should take mythic's paint")
	}
	// A rarity the spec lacks takes common's, and so does an empty one
	for _, rarity := range []string{"uncommon", "special", ""} {
		if r, g, b := color(rarity); r > 0.01 || g > 0.01 || b > 0.01 {
			t.Errorf("%q = %v %v %v, want common's black", rarity, r, g, b)
		}
	}
	// With no paint listed at all the symbol is black
	spec.Rarity = nil
	if r, g, b := color("rare"); r > 0.01 || g > 0.01 || b > 0.01 {
		t.Errorf("no paint = %v %v %v, want black", r, g, b)
	}
}

func TestSymbolNodeGradient(t *testing.T) {
	sym := mustSymbol(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10H0z"/></svg>`)
	spec := template.SymbolSpec{
		X: 100, Y: 100, Width: 100, Height: 100,
		Rarity: map[string]template.SymbolPaint{"rare": {
			Stops: []template.SymbolStop{{Color: "#000000", At: 0}, {Color: "#ffffff", At: 1}},
		}},
	}
	lum := func(b *raster.Buffer, x, y int) float32 { r, _, _, _ := pixel(b, x, y); return r }

	// Angle 0 runs left to right
	n, _ := symbolNode(sym, spec, "rare", 300, 300)
	b := layerOf(t, n)
	if lum(b, 105, 150) > lum(b, 195, 150) || lum(b, 195, 150)-lum(b, 105, 150) < 0.5 {
		t.Errorf("angle 0 should brighten left to right: %v then %v", lum(b, 105, 150), lum(b, 195, 150))
	}
	if d := lum(b, 150, 105) - lum(b, 150, 195); d > 0.05 || d < -0.05 {
		t.Errorf("angle 0 should not vary down the symbol, got %v", d)
	}

	// Angle 90 runs top to bottom
	p := spec.Rarity["rare"]
	p.Angle = 90
	spec.Rarity["rare"] = p
	n, _ = symbolNode(sym, spec, "rare", 300, 300)
	b = layerOf(t, n)
	if lum(b, 150, 195)-lum(b, 150, 105) < 0.5 {
		t.Errorf("angle 90 should brighten top to bottom: %v then %v", lum(b, 150, 105), lum(b, 150, 195))
	}
}

func TestSymbolNodeOutline(t *testing.T) {
	sym := mustSymbol(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10H0z"/></svg>`)
	spec := template.SymbolSpec{
		X: 100, Y: 100, Width: 100, Height: 100, Scale: 0.8, Align: "center",
		Outline: &template.SymbolOutline{Width: 6, Color: "#ff0000"},
		Rarity:  map[string]template.SymbolPaint{"common": {Stops: []template.SymbolStop{{Color: "#000000", At: 0}}}},
	}
	n, err := symbolNode(sym, spec, "common", 300, 300)
	if err != nil {
		t.Fatal(err)
	}
	b := layerOf(t, n)
	// The fill covers 110..190, and the outline sits outside it, 104..110
	if r, _, _, _ := pixel(b, 150, 150); r > 0.01 {
		t.Errorf("center is red %v, the fill should cover the stroke's inner half", r)
	}
	if r, g, _, a := pixel(b, 107, 150); a < 0.99 || r < 0.9 || g > 0.01 {
		t.Errorf("outline pixel = r%v g%v a%v, want opaque red", r, g, a)
	}
	if !clear(b, 100, 150) {
		t.Error("the outline reaches further than its width")
	}
}

func TestSymbolNodeNothingToDraw(t *testing.T) {
	// An icon whose shapes have no extent has nothing to place
	icon := &svgpath.Icon{ViewBox: svgpath.Rect{W: 10, H: 10}}
	n, err := symbolNode(&card.SetSymbol{Icon: icon}, template.SymbolSpec{Width: 10, Height: 10}, "common", 100, 100)
	if n != nil || err != nil {
		t.Errorf("symbolNode = %v, %v, want nil and no error", n, err)
	}
}

func TestSetSymbolSpec(t *testing.T) {
	sym := mustSymbol(t, wideSymbol)
	box := template.SymbolSpec{Width: 10, Height: 10}
	manifest := func(spec template.SymbolSpec) *template.Manifest {
		return &template.Manifest{Symbols: map[string]template.SymbolSpec{template.SymbolSet: spec}}
	}
	tests := []struct {
		name string
		req  template.RenderRequest
		m    *template.Manifest
		f    frame.Keys
		want bool
	}{
		{"draws", template.RenderRequest{SetSymbol: sym}, manifest(box), frame.Keys{}, true},
		{"no symbol", template.RenderRequest{}, manifest(box), frame.Keys{}, false},
		{"symbol with no icon", template.RenderRequest{SetSymbol: &card.SetSymbol{}}, manifest(box), frame.Keys{}, false},
		{"manifest has no place", template.RenderRequest{SetSymbol: sym}, &template.Manifest{}, frame.Keys{}, false},
		{"empty box", template.RenderRequest{SetSymbol: sym}, manifest(template.SymbolSpec{}), frame.Keys{}, false},
		{"condition holds", template.RenderRequest{SetSymbol: sym}, manifest(template.SymbolSpec{Width: 10, Height: 10, Condition: "land"}), frame.Keys{Land: true}, true},
		{"condition fails", template.RenderRequest{SetSymbol: sym}, manifest(template.SymbolSpec{Width: 10, Height: 10, Condition: "land"}), frame.Keys{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, got := setSymbolSpec(tc.req, tc.m, tc.f); got != tc.want {
				t.Errorf("draws = %v, want %v", got, tc.want)
			}
		})
	}
}

// symbolAssets is the placeholder frame with a place for the set symbol and a
// type line that gives way to it
type symbolAssets struct{ template.AssetProvider }

func (s symbolAssets) Manifest() (*template.Manifest, error) {
	m, err := s.AssetProvider.Manifest()
	if err != nil {
		return nil, err
	}
	m.Symbols = map[string]template.SymbolSpec{
		template.SymbolSet: {X: 620, Y: 560, Width: 70, Height: 44, Align: "right"},
	}
	narrow := m.TextBoxes["type"]
	narrow.Condition = "set_symbol"
	narrow.Width = 540
	full := m.TextBoxes["type"]
	full.Box = "type"
	m.TextBoxes["type"] = narrow
	m.TextBoxes["type_full"] = full
	return m, nil
}

func TestRenderDrawsSetSymbol(t *testing.T) {
	req := renderBolt(t, nil)
	req.Assets = symbolAssets{req.Assets}
	scale := atMinDPI(t, req)

	without, err := New("test").Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	req.SetSymbol = mustSymbol(t, wideSymbol)
	with, err := New("test").Render(context.Background(), *req)
	if err != nil {
		t.Fatalf("Render with symbol: %v", err)
	}

	// The symbol's box is at the right end of the type row. Inside it the render
	// changes, and far from it, in the title row, nothing does
	x, y := scale.Px(660), scale.Px(582)
	if !differ(without, with, x, y) {
		t.Error("the symbol did not change the pixels inside its box")
	}
	if differ(without, with, scale.Px(60), scale.Px(60)) {
		t.Error("the symbol changed pixels outside its box")
	}
}

func differ(a, b *raster.Buffer, x, y int) bool {
	ar, ag, ab, aa := a.At(x, y)
	br, bg, bb, ba := b.At(x, y)
	return ar != br || ag != bg || ab != bb || aa != ba
}
