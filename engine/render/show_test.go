package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/odevine/mimic/engine/card"
	"github.com/odevine/mimic/engine/manifest"
	"github.com/odevine/mimic/engine/template"
)

func TestShowsMatchesAStackEntryByItsName(t *testing.T) {
	req := template.RenderRequest{Show: []string{"layer:crown", "text:title"}}
	if !req.Shows("crown") {
		t.Error("a named layer is not shown")
	}
	// Only a layer can be shown, a longer or shorter name is another entry, and nothing is shown by default
	for _, key := range []string{"title", "crow", "crown2", ""} {
		if req.Shows(key) {
			t.Errorf("Shows(%q) = true", key)
		}
	}
	if (template.RenderRequest{}).Shows("crown") {
		t.Error("a request that shows nothing showed a layer")
	}
}

func TestALayerWhoseConditionFailsIsDrawnWhenShown(t *testing.T) {
	tm := describeTemplate(t, nil)
	crown := image.Pt(200, 90)
	gold := color.NRGBA{0xD0, 0xA0, 0x20, 0xFF}
	frame := color.NRGBA{0xF0, 0xF0, 0xE0, 0xFF}
	// The crown is for legendary cards, and this one is not
	req := template.RenderRequest{Card: creature(card.White)}
	if got := colorAt(rendered(t, tm, req), crown); got != frame {
		t.Fatalf("the crown draws for a card that is not legendary: %v", got)
	}
	req.Show = []string{"layer:crown"}
	if got := colorAt(rendered(t, tm, req), crown); got != gold {
		t.Errorf("a shown crown is %v, want %v", got, gold)
	}
	// Showing one that already draws, or one that is not there, changes nothing
	req.Show = []string{"layer:frame", "layer:nothing-of-this-name"}
	if got := colorAt(rendered(t, tm, req), crown); got != frame {
		t.Errorf("showing what draws already changed the crown to %v", got)
	}
	// What the request hides stays hidden, whatever it shows
	req = template.RenderRequest{Card: creature(card.White), Show: []string{"layer:crown"}, Hide: []string{"layer:crown"}}
	if got := colorAt(rendered(t, tm, req), crown); got != frame {
		t.Errorf("a crown that is hidden and shown draws %v", got)
	}
}

func TestShowingALayerDoesNotChangeTheTemplate(t *testing.T) {
	tm := describeTemplate(t, nil)
	crown := image.Pt(200, 90)
	frame := color.NRGBA{0xF0, 0xF0, 0xE0, 0xFF}
	rendered(t, tm, template.RenderRequest{Card: creature(card.White), Show: []string{"layer:crown"}})
	// The next card, with nothing shown, is as if nothing had been
	if got := colorAt(rendered(t, tm, template.RenderRequest{Card: creature(card.White)}), crown); got != frame {
		t.Errorf("a render that showed the crown left it on the template: %v", got)
	}
}

func TestADescriptionReportsAShownLayerDrawn(t *testing.T) {
	tm := describeTemplate(t, nil)
	req := template.RenderRequest{Card: creature(card.White)}
	if it := items(t, describe(t, tm, req))["layer:crown"]; it.Drawn {
		t.Fatalf("crown = %+v before it is shown", it)
	}
	req.Show = []string{"layer:crown"}
	it := items(t, describe(t, tm, req))["layer:crown"]
	if !it.Drawn || it.Why != "" || it.Authored != crownRect {
		t.Errorf("crown = %+v when shown, want drawn at %v", it, crownRect)
	}
	// A hide is not read by the description, so the layer still reports as it draws
	req.Hide = []string{"layer:crown"}
	if it := items(t, describe(t, tm, req))["layer:crown"]; !it.Drawn {
		t.Errorf("crown = %+v when shown and hidden", it)
	}
}

// A layer that is shown is on the card for the rest of the layout, so text steps around it
func TestTextStepsAroundAShownLayer(t *testing.T) {
	avoiding := describeTemplate(t, func(m *manifest.Manifest) {
		l := m.Layouts["test"]
		title := l.TextBoxes["title"]
		title.Flow.AvoidLayer = "crown"
		title.Rect = manifest.Rect{X: 48, Y: 40, Width: 520, Height: 100}
		title.Text.FontSize = 30
		l.TextBoxes["title"] = title
		m.Layouts["test"] = l
	})
	named := func(c *card.Data) *card.Data {
		c.Name = "A Hidden Gopher Of Very Long Name Indeed"
		return c
	}
	ink := func(req template.RenderRequest) image.Rectangle {
		img := rendered(t, avoiding, req)
		var out image.Rectangle
		for y := 0; y < 200; y++ {
			for x := 0; x < 700; x++ {
				if c := colorAt(img, image.Pt(x, y)); c.R < 0x60 && c.G < 0x60 && c.B < 0x60 {
					out = out.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return out
	}
	steps := ink(template.RenderRequest{Card: named(legendary())})
	room := ink(template.RenderRequest{Card: named(creature(card.White))})
	if steps == room {
		t.Fatalf("the title does not step around the crown for a legendary card, ink %v both ways", steps)
	}
	if shown := ink(template.RenderRequest{Card: named(creature(card.White)), Show: []string{"layer:crown"}}); shown != steps {
		t.Errorf("a shown crown left the text at %v, and a legendary card's is at %v", shown, steps)
	}
}
