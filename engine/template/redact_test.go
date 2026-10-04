package template

import (
	"image"
	"reflect"
	"testing"

	"golang.org/x/image/font/basicfont"
)

func TestSplitRedactions(t *testing.T) {
	cases := []struct {
		word    string
		on      bool
		want    []redactSeg
		wantOut bool
	}{
		{"plain", false, []redactSeg{{"plain", false}}, false},
		{"~~win", false, []redactSeg{{"win", true}}, true},
		{"game~~.", true, []redactSeg{{"game", true}, {".", false}}, false},
		{"a~~b~~c", false, []redactSeg{{"a", false}, {"b", true}, {"c", false}}, false},
		// A bare mark only flips the state and leaves nothing to draw
		{"~~", false, nil, true},
		{"~~", true, nil, false},
		// A single tilde is just a character
		{"~x", false, []redactSeg{{"~x", false}}, false},
	}
	for _, c := range cases {
		got, out := splitRedactions(c.word, c.on)
		if !reflect.DeepEqual(got, c.want) || out != c.wantOut {
			t.Errorf("splitRedactions(%q, %v) = %v, %v; want %v, %v", c.word, c.on, got, out, c.want, c.wantOut)
		}
	}
}

// redactions lists, line by line, whether each token's runs are redacted, "+"
// for a redacted run and "-" for a plain one, so "-+" is a word half redacted.
func redactions(lay textLayout) [][]string {
	var out [][]string
	for _, ln := range lay.lines {
		var toks []string
		for _, tk := range ln.tokens {
			s := ""
			for _, run := range tk.runs {
				if run.redact {
					s += "+"
				} else {
					s += "-"
				}
			}
			toks = append(toks, s)
		}
		out = append(out, toks)
	}
	return out
}

func TestTokenizeRedactsAcrossWords(t *testing.T) {
	// A span runs across words, a mark can sit inside a word, and a bare mark
	// opens a span without a token of its own.
	box := TextBoxSpec{Width: 1000, Height: 1000}
	lay := layoutText(box, "You ~~can't lose~~ win the~~ game~~. ~~ gone ~~", basicfont.Face7x13)
	if got, want := lineTexts(lay), []string{"You can't lose win the game. gone"}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q with the marks dropped", got, want)
	}
	want := [][]string{{"-", "+", "+", "-", "-", "+-", "+"}}
	if got := redactions(lay); !reflect.DeepEqual(got, want) {
		t.Errorf("redactions = %v, want %v", got, want)
	}
}

func TestRedactionEndsWithItsParagraph(t *testing.T) {
	// An unclosed span stops at the line break rather than eating the rest of
	// the card.
	box := TextBoxSpec{Width: 1000, Height: 1000}
	lay := layoutText(box, "~~hidden\nshown", basicfont.Face7x13)
	want := [][]string{{"+"}, {"-"}}
	if got := redactions(lay); !reflect.DeepEqual(got, want) {
		t.Errorf("redactions = %v, want %v", got, want)
	}
}

func TestRedactionKeepsTheTextsWidth(t *testing.T) {
	// The bar takes the room the words would have, so the card wraps exactly
	// as the uncensored one does.
	box := TextBoxSpec{Width: 1000, Height: 1000}
	plain := layoutText(box, "a bb c", basicfont.Face7x13)
	marked := layoutText(box, "a ~~bb~~ c", basicfont.Face7x13)
	if plain.lines[0].width != marked.lines[0].width {
		t.Errorf("redacted width = %v, want the plain %v", marked.lines[0].width, plain.lines[0].width)
	}
}

// inkedRows counts the rows of column x that carry any ink.
func inkedRows(img *image.RGBA, x int) int {
	n := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		if img.RGBAAt(x, y).A > 0 {
			n++
		}
	}
	return n
}

func TestRedactionPaintsOneBarOverTheSpan(t *testing.T) {
	// "ab cd ef gh" in 7px glyphs puts the space between cd and ef at x 35-42
	// and the one between ab and cd at 14-21. Redacting "cd ef" inks the space
	// inside the span, since the bar runs straight across it, and leaves the
	// space before the span clear.
	box := TextBoxSpec{Width: 1000, Height: 200, FontSize: 13, VAlign: "top"}
	render := func(text string) *image.RGBA {
		res, err := RenderTextBox(box, 200, 200, TextPart{Text: text, Src: fixedSource{}})
		if err != nil {
			t.Fatalf("RenderTextBox: %v", err)
		}
		return res.Image
	}
	plain := render("ab cd ef gh")
	marked := render("ab ~~cd ef~~ gh")

	if n := inkedRows(plain, 38); n != 0 {
		t.Fatalf("plain text inks %d rows of the gap between words", n)
	}
	if n := inkedRows(marked, 38); n < 6 {
		t.Errorf("redacted gap inks %d rows, want the bar across it", n)
	}
	if n := inkedRows(marked, 17); n != 0 {
		t.Errorf("the gap before the span inks %d rows, want it clear", n)
	}

	again := render("ab ~~cd ef~~ gh")
	if !reflect.DeepEqual(marked.Pix, again.Pix) {
		t.Error("the same text drew a different bar the second time")
	}
}
