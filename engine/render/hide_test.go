package render

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

func TestHidesMatchesAWholeNameOfTheKind(t *testing.T) {
	req := template.RenderRequest{Hide: []string{"layer:frame", "text:title", "art:0", "symbol:set"}}
	for _, c := range []struct {
		kind, key string
		want      bool
	}{
		{"layer", "frame", true},
		{"text", "title", true},
		{"art", "0", true},
		{"symbol", "set", true},
		// The same key of another kind, a longer or shorter key and the empty one are not it
		{"text", "frame", false},
		{"layer", "title", false},
		{"layer", "frame2", false},
		{"layer", "fram", false},
		{"layer", "", false},
		{"art", "01", false},
	} {
		if got := req.Hides(c.kind, c.key); got != c.want {
			t.Errorf("Hides(%q, %q) = %v, want %v", c.kind, c.key, got, c.want)
		}
	}
	if (template.RenderRequest{}).Hides("layer", "frame") {
		t.Error("a request that hides nothing hid a layer")
	}
}

func rendered(t *testing.T, tm *template.Template, req template.RenderRequest) image.Image {
	t.Helper()
	buf, err := Render(context.Background(), tm, req)
	if err != nil {
		t.Fatal(err)
	}
	return buf.ToImage(8)
}

func colorAt(img image.Image, p image.Point) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(p.X, p.Y)).(color.NRGBA)
}

func legendary() *card.Data {
	c := creature(card.White)
	c.TypeLine = "Legendary Creature - Human"
	c.Name = "Hidden Gopher"
	return c
}

func TestAHiddenLayerIsNotDrawnAndTheFrameBelowShows(t *testing.T) {
	tm := describeTemplate(t, nil)
	crown := image.Pt(200, 90)
	gold := color.NRGBA{0xD0, 0xA0, 0x20, 0xFF}
	frame := color.NRGBA{0xF0, 0xF0, 0xE0, 0xFF}
	req := template.RenderRequest{Card: legendary()}
	if got := colorAt(rendered(t, tm, req), crown); got != gold {
		t.Fatalf("the crown is %v before it is hidden, want %v", got, gold)
	}
	req.Hide = []string{"layer:crown"}
	if got := colorAt(rendered(t, tm, req), crown); got != frame {
		t.Errorf("a hidden crown leaves %v, want the frame's %v", got, frame)
	}
	// Hiding a layer the card does not draw changes nothing
	req.Hide = []string{"layer:badge", "layer:nothing-of-this-name"}
	plain := rendered(t, tm, template.RenderRequest{Card: legendary()})
	if got := colorAt(rendered(t, tm, req), crown); got != gold || colorAt(plain, crown) != gold {
		t.Errorf("hiding what is not there changed the crown to %v", got)
	}
}

func TestAHiddenTextBoxIsNotDrawn(t *testing.T) {
	tm := describeTemplate(t, nil)
	area := image.Rect(40, 30, 580, 100)
	frame := color.NRGBA{0xF0, 0xF0, 0xE0, 0xFF}
	req := template.RenderRequest{Card: creature(card.White)}
	if ink := inkBounds(rendered(t, tm, req), area, frame); ink.Empty() {
		t.Fatal("the title draws nothing before it is hidden")
	}
	req.Hide = []string{"text:title"}
	if ink := inkBounds(rendered(t, tm, req), area, frame); !ink.Empty() {
		t.Errorf("a hidden title left ink at %v", ink)
	}
}

func TestHiddenArtAndSymbolAreNotDrawn(t *testing.T) {
	tm := describeTemplate(t, nil)
	art := image.NewNRGBA(image.Rect(0, 0, 60, 40))
	red := color.NRGBA{0xC0, 0x10, 0x10, 0xFF}
	for i := 0; i < len(art.Pix); i += 4 {
		copy(art.Pix[i:], []byte{red.R, red.G, red.B, 0xFF})
	}
	frame := color.NRGBA{0xF0, 0xF0, 0xE0, 0xFF}
	req := template.RenderRequest{Card: creature(card.White), Art: art, SetSymbol: mustSymbol(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`)}
	inArt, inSymbol := image.Pt(360, 380), image.Pt(630, 620)
	shown := rendered(t, tm, req)
	if colorAt(shown, inArt) == frame || colorAt(shown, inSymbol) == frame {
		t.Fatalf("art %v and symbol %v do not draw before they are hidden", colorAt(shown, inArt), colorAt(shown, inSymbol))
	}
	req.Hide = []string{"art:0"}
	hidden := rendered(t, tm, req)
	if got := colorAt(hidden, inArt); got != frame {
		t.Errorf("hidden art leaves %v, want %v", got, frame)
	}
	if colorAt(hidden, inSymbol) == frame {
		t.Error("hiding the art hid the symbol")
	}
	req.Hide = []string{"symbol:set"}
	hidden = rendered(t, tm, req)
	if got := colorAt(hidden, inSymbol); got != frame {
		t.Errorf("a hidden symbol leaves %v, want %v", got, frame)
	}
	if colorAt(hidden, inArt) == frame {
		t.Error("hiding the symbol hid the art")
	}
}

// Hiding a layer takes nothing from the layout, so text that steps around it keeps its
// room, and the preview shows what the layout does with the layer there
func TestHidingALayerDoesNotChangeHowTextStepsAroundIt(t *testing.T) {
	avoiding := func(layer string) *template.Template {
		return describeTemplate(t, func(m *manifest.Manifest) {
			l := m.Layouts["test"]
			title := l.TextBoxes["title"]
			title.Flow.AvoidLayer = layer
			title.Rect = manifest.Rect{X: 48, Y: 40, Width: 520, Height: 100}
			title.Text.FontSize = 30
			l.TextBoxes["title"] = title
			m.Layouts["test"] = l
		})
	}
	area := image.Rect(0, 0, 700, 200)
	// The title's ink is the dark pixels, which neither the frame nor the crown has
	card := legendary()
	card.Name = "A Hidden Gopher Of Very Long Name Indeed"
	ink := func(tm *template.Template, hide ...string) image.Rectangle {
		img := rendered(t, tm, template.RenderRequest{Card: card, Hide: hide})
		var out image.Rectangle
		for y := area.Min.Y; y < area.Max.Y; y++ {
			for x := area.Min.X; x < area.Max.X; x++ {
				if c := colorAt(img, image.Pt(x, y)); c.R < 0x60 && c.G < 0x60 && c.B < 0x60 {
					out = out.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return out
	}
	plain, avoid := ink(avoiding("")), ink(avoiding("crown"))
	if plain == avoid {
		t.Fatalf("the title did not step around the crown at all, ink %v both ways", plain)
	}
	if hidden := ink(avoiding("crown"), "layer:crown"); hidden != avoid {
		t.Errorf("hiding the crown moved the text: ink %v, and %v while it shows", hidden, avoid)
	}
}

func TestDescribeIgnoresWhatARenderHides(t *testing.T) {
	tm := describeTemplate(t, nil)
	req := template.RenderRequest{Card: legendary()}
	want := items(t, describe(t, tm, req))
	req.Hide = []string{"layer:crown", "text:title", "art:0"}
	got := items(t, describe(t, tm, req))
	for _, key := range []string{"layer:crown", "text:title", "art:0", "layer:frame"} {
		if got[key].Drawn != want[key].Drawn || got[key].Delivered != want[key].Delivered {
			t.Errorf("%s = %+v hidden, %+v shown", key, got[key], want[key])
		}
	}
	if !got["layer:crown"].Drawn {
		t.Error("a layer a render hides is not drawn for the description")
	}
}
