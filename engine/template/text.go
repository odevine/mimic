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

// FaceSource yields a font face at a point size. Shrink-to-fit asks for several
// sizes, so a caller passes a source rather than a single face. A source that
// ignores size, such as a fixed bitmap face, disables shrinking on its own
type FaceSource interface {
	Face(size float64) (font.Face, error)
}

// RenderTextBox lays text out and draws it into a document-sized transparent
// image, positioned and aligned within box, and returns it to be composited
// like any other layer. An area box whose text overflows its height shrinks the
// font toward the box's floor to fit, while a baseline-anchored box keeps its
// size.
//
// src supplies the font at whatever size the fit needs. This module bundles
// none. There is no shaping beyond what a face provides, and a braced mana
// symbol in rules text renders as its literal characters
func RenderTextBox(box TextBoxSpec, text string, docW, docH int, src FaceSource) (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, docW, docH))
	if strings.TrimSpace(text) == "" {
		return img, nil
	}
	lay, err := fitLayout(box, text, src)
	if err != nil {
		return nil, err
	}
	drawLayout(img, box, lay)
	return img, nil
}

// fitLayout returns text laid out at the largest size that fits box's height,
// from box.FontSize down to the floor. A baseline-anchored box is single-line
// point text and is not height-shrunk, so it lays out at box.FontSize. When
// even the floor overflows, the floor's layout is returned for drawLayout to
// clip
func fitLayout(box TextBoxSpec, text string, src FaceSource) (textLayout, error) {
	max := box.FontSize
	lay, fits, err := layoutAt(box, text, src, max)
	if err != nil {
		return textLayout{}, err
	}
	min := minFontSize(box)
	if box.VAlign == "baseline" || fits || max <= min {
		return lay, nil
	}
	// max overflows, so search (min, max) for the largest size that fits,
	// keeping the floor's layout as the fallback when nothing does
	best, _, err := layoutAt(box, text, src, min)
	if err != nil {
		return textLayout{}, err
	}
	lo, hi := min, max
	for i := 0; i < 8 && hi-lo > 0.5; i++ {
		mid := (lo + hi) / 2
		l, ok, err := layoutAt(box, text, src, mid)
		if err != nil {
			return textLayout{}, err
		}
		if ok {
			best, lo = l, mid
		} else {
			hi = mid
		}
	}
	return best, nil
}

// layoutAt lays text out at one em size and reports whether the block fits box's
// height
func layoutAt(box TextBoxSpec, text string, src FaceSource, size float64) (textLayout, bool, error) {
	face, err := src.Face(size)
	if err != nil {
		return textLayout{}, false, err
	}
	sized := box
	sized.FontSize = size
	lay := layoutText(sized, text, face)
	return lay, blockHeight(lay, sized) <= box.Height, nil
}

// blockHeight is the total height of a laid-out block, its line count times the
// line height at its size
func blockHeight(lay textLayout, box TextBoxSpec) int {
	if len(lay.lines) == 0 {
		return 0
	}
	m := faceOf(lay.lines[0]).Metrics()
	return len(lay.lines) * lineHeightPx(m, lay.size, box.LineSpacing)
}

// minFontSize is the shrink-to-fit floor, a box's own MinFontSize or, when it
// sets none, a little over half the font size
func minFontSize(box TextBoxSpec) float64 {
	if box.MinFontSize > 0 {
		return box.MinFontSize
	}
	return box.FontSize * 0.55
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
	lineHeight := lineHeightPx(metrics, lay.size, box.LineSpacing)

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
