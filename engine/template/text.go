package template

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// glyphRun is a span of text drawn in one face. A laid-out line is a sequence
// of runs, all in the box face today. Inline symbols later add runs in a
// symbol face without changing how a line is measured or drawn
type glyphRun struct {
	text    string
	face    font.Face
	advance fixed.Int26_6
}

// token is an unbreakable unit of one or more runs with no space between them.
// Line breaking happens between tokens, never inside one. A plain word is one
// text run today, and a mana cost like {1}{U} becomes several runs in one
// token once inline symbols land
type token struct {
	runs    []glyphRun
	advance fixed.Int26_6
}

// line is one laid-out line: its tokens in order and their combined advance,
// including the single space between adjacent tokens
type line struct {
	tokens []token
	width  fixed.Int26_6
}

// textLayout is the result of fitting text into a box: the lines to draw and
// the point size they were laid out at. The size is the box's font size today
// and becomes meaningful once a shrink-to-fit pass chooses it
type textLayout struct {
	lines []line
	size  float64
}

// RenderTextBox draws greedily word-wrapped text into a document-sized
// transparent image, positioned and aligned within box, and returns it to be
// composited like any other layer.
//
// The face parameter is where a caller supplies a font it has obtained itself.
// This module bundles none. The current handling has no shaping beyond what
// the face provides and no shrink-to-fit: text that overflows the box height
// is dropped rather than resized, and a braced mana symbol in rules text
// renders as its literal characters
func RenderTextBox(box TextBoxSpec, text string, docW, docH int, face font.Face) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, docW, docH))
	if strings.TrimSpace(text) == "" {
		return img
	}
	drawLayout(img, box, layoutText(box, text, face))
	return img
}

// layoutText wraps text into box using face, greedily breaking between tokens
// and keeping paragraph breaks on explicit newlines. It measures with the face
// and does no shrink to fit yet, so the returned size is box.FontSize
func layoutText(box TextBoxSpec, text string, face font.Face) textLayout {
	space := spaceAdvance(face)
	maxW := fixed.I(box.Width)
	var lines []line
	for _, para := range strings.Split(text, "\n") {
		var cur line
		for _, tk := range tokenize(para, face) {
			switch {
			case len(cur.tokens) == 0:
				cur = line{tokens: []token{tk}, width: tk.advance}
			case cur.width+space+tk.advance <= maxW:
				cur.tokens = append(cur.tokens, tk)
				cur.width += space + tk.advance
			default:
				lines = append(lines, cur)
				cur = line{tokens: []token{tk}, width: tk.advance}
			}
		}
		if len(cur.tokens) > 0 {
			lines = append(lines, cur)
		}
	}
	return textLayout{lines: lines, size: box.FontSize}
}

// tokenize splits a paragraph into whitespace-delimited tokens, each a single
// text run in face. When inline symbol support lands, this is where a word
// splits into text and symbol runs
func tokenize(para string, face font.Face) []token {
	drawer := &font.Drawer{Face: face}
	var toks []token
	for _, word := range strings.Fields(para) {
		adv := drawer.MeasureString(word)
		toks = append(toks, token{
			runs:    []glyphRun{{text: word, face: face, advance: adv}},
			advance: adv,
		})
	}
	return toks
}

// drawLayout paints a laid-out block into img, aligning each line horizontally
// per box.Align and the block vertically per box.VAlign, clipping lines that
// fall past the box bottom
func drawLayout(img *image.RGBA, box TextBoxSpec, lay textLayout) {
	if len(lay.lines) == 0 {
		return
	}
	face := faceOf(lay.lines[0])
	metrics := face.Metrics()
	src := image.NewUniform(parseHexColor(box.Color))
	space := spaceAdvance(face)
	lineHeight := lineHeightPx(metrics, box.FontSize, box.LineSpacing)

	baseline := firstBaseline(box, metrics, len(lay.lines), lineHeight)
	bottom := box.Y + box.Height
	for _, ln := range lay.lines {
		if baseline > bottom {
			break
		}
		x := lineStartX(box, ln.width)
		for _, tk := range ln.tokens {
			for _, run := range tk.runs {
				drawer := &font.Drawer{
					Dst:  img,
					Src:  src,
					Face: run.face,
					Dot:  fixed.Point26_6{X: x, Y: fixed.I(baseline)},
				}
				drawer.DrawString(run.text)
				x += run.advance
			}
			x += space
		}
		baseline += lineHeight
	}
}

// faceOf returns the face of a line's first run, the face the whole line is
// measured and spaced against today
func faceOf(ln line) font.Face { return ln.tokens[0].runs[0].face }

// spaceAdvance is the width of a single space in face, the gap kept between
// adjacent tokens
func spaceAdvance(face font.Face) fixed.Int26_6 {
	return (&font.Drawer{Face: face}).MeasureString(" ")
}

// lineStartX is the pen x for a line of the given width under box.Align
func lineStartX(box TextBoxSpec, width fixed.Int26_6) fixed.Int26_6 {
	left := fixed.I(box.X)
	switch box.Align {
	case "center":
		return left + (fixed.I(box.Width)-width)/2
	case "right":
		return left + fixed.I(box.Width) - width
	default:
		return left
	}
}

// firstBaseline is the y of the first line's baseline. A "baseline" vertical
// anchor puts it at box.Y directly, matching point text whose stored position
// is its baseline and so is font-independent. Any other anchor places the
// block by its top and drops to the first baseline through the face ascent
func firstBaseline(box TextBoxSpec, m font.Metrics, nLines, lineHeight int) int {
	if box.VAlign == "baseline" {
		return box.Y
	}
	return blockTop(box, nLines, lineHeight) + m.Ascent.Ceil()
}

// blockTop is the y of the top of an nLines block within box under box.VAlign.
// center and bottom anchor the block inside the box, anything else the top
func blockTop(box TextBoxSpec, nLines, lineHeight int) int {
	switch box.VAlign {
	case "center":
		return box.Y + (box.Height-nLines*lineHeight)/2
	case "bottom":
		return box.Y + box.Height - nLines*lineHeight
	default:
		return box.Y
	}
}

// lineHeightPx is the baseline-to-baseline distance for a line. A positive
// spacing sets it to that multiple of the em size, the way a PSD states
// leading, so 1.0 is solid and 1.2 is the usual auto. A non-positive spacing
// uses the face's natural height, falling back to ascent plus descent when the
// face reports none
func lineHeightPx(m font.Metrics, emPx, spacing float64) int {
	if spacing > 0 {
		return int(math.Round(emPx * spacing))
	}
	h := m.Height.Ceil()
	if h <= 0 {
		h = m.Ascent.Ceil() + m.Descent.Ceil()
	}
	return h
}

// parseHexColor reads a "#RRGGBB" string, returning opaque black when it cannot
func parseHexColor(s string) color.Color {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.Black
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.Black
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
}
