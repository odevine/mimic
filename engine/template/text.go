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

// glyphRun is a span drawn as one thing. A laid-out line is a sequence of runs.
// Rules and flavor use different faces, and a braced symbol is a run of its own,
// so neither changes how a line is measured or drawn
type glyphRun struct {
	text    string
	face    font.Face
	advance fixed.Int26_6
	// sym, when set, makes this a symbol run: it holds the braced code and the
	// run draws through ren instead of drawing text in face. The face is kept
	// so a line of nothing but symbols still has metrics to lay out against
	sym  string
	metr SymbolMetrics
	ren  SymbolRenderer
}

// token is an unbreakable unit of one or more runs with no space between them.
// Line breaking happens between tokens, never inside one. A plain word is one
// text run, and a mana cost like {1}{U} is several symbol runs in one token
type token struct {
	runs    []glyphRun
	advance fixed.Int26_6
}

// SymbolMetrics is the space one symbol takes on a line. A symbol draws into a
// square box whose left edge sits at the pen: Box is that square's side, Ascent
// is how far its top is above the baseline, and Advance is what the pen moves,
// which is a little more than Box so adjacent symbols do not touch
type SymbolMetrics struct {
	Advance fixed.Int26_6
	Box     int
	Ascent  int
}

// SymbolRenderer draws the braced codes a card's text carries, like {R} or {T}.
// Layout parses the braces and asks the renderer how much room a code needs and
// then to paint it, which keeps this package free of any knowledge of what a
// symbol looks like, the way it knows a FaceSource but no particular font
type SymbolRenderer interface {
	// Symbol reports the room code takes at a text size. It reports false for a
	// code it does not draw, which leaves the braced text literal
	Symbol(code string, size float64) (SymbolMetrics, bool)
	// DrawSymbol paints code into dst, filling the square at
	DrawSymbol(dst *image.RGBA, code string, at image.Rectangle) error
}

// line is one laid-out line. A text line carries its tokens and their combined
// advance including the single space between adjacent tokens. A divider line
// carries no text and stands for the separator rule between two parts
type line struct {
	tokens  []token
	width   fixed.Int26_6
	divider bool
	// paraStart marks the first line of a paragraph that follows another, so
	// drawing adds a little space before it to set the paragraphs apart
	paraStart bool
}

// textLayout is the result of fitting a box's parts: the lines to draw, text
// and divider, and the point size they were laid out at
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

// TextPart is one styled run of text for a box: the text and the source that
// yields its face. RenderTextBox stacks a box's parts top to bottom with a
// divider between consecutive non-empty parts, which is how rules text and
// flavor share the one box
type TextPart struct {
	Text string
	Src  FaceSource
	// Emph, when set, renders parenthesized reminder text in this face while the
	// rest of the part stays in Src. Nil leaves the whole part in Src
	Emph FaceSource
	// EmphLead also renders in Emph a leading word a paragraph opens with when it
	// is in this set and an em-dash follows, the way an ability word italicizes.
	// The keys are lowercased. Nil disables it
	EmphLead map[string]bool
	// Sym draws the braced symbol codes in this part, so "{T}: Add {G}." prints
	// the two pips. Nil leaves every code as its literal characters
	Sym SymbolRenderer
}

// partStyle is one part resolved at one size: the faces its words draw in, the
// renderer its braced codes draw through, and the size and letter spacing they
// were resolved at. Tokenizing needs all of it, so it travels as one value
type partStyle struct {
	face     font.Face
	emph     font.Face
	emphLead map[string]bool
	sym      SymbolRenderer
	size     float64
	track    fixed.Int26_6
}

// TextBoxResult is what RenderTextBox produces: the drawn image, and where a
// flavor divider belongs. The text package does not load assets, so it reports
// the divider's document center Y and leaves the caller to place the divider
// art there
type TextBoxResult struct {
	Image *image.RGBA
	// DividerY is the document Y of the divider's center, valid only when
	// HasDivider is true
	DividerY   int
	HasDivider bool
}

// RenderTextBox lays a box's parts out and draws them into a document-sized
// transparent image, then returns it to be composited like any other layer. An
// area box whose text overflows its height shrinks the font toward the box's
// floor to fit, while a baseline-anchored box keeps its size. Parts are stacked
// with a divider slot between them, so rules and flavor separate on their own.
//
// A part's source supplies the font at whatever size the fit needs and its
// symbol renderer the braced codes. This module bundles neither, and there is no
// shaping beyond what a face provides
func RenderTextBox(box TextBoxSpec, docW, docH int, parts ...TextPart) (TextBoxResult, error) {
	img := image.NewRGBA(image.Rect(0, 0, docW, docH))
	inner := insetBox(box)
	lay, err := fitLayout(inner, parts)
	if err != nil {
		return TextBoxResult{}, err
	}
	dividerY, hasDivider, err := drawLayout(img, inner, lay)
	if err != nil {
		return TextBoxResult{}, err
	}
	return TextBoxResult{Image: img, DividerY: dividerY, HasDivider: hasDivider}, nil
}

// TextSpan is the document X range a laid-out box's text covers, from the
// leftmost line's start to the rightmost line's end
type TextSpan struct {
	Left, Right int
}

// Empty reports whether the span covers nothing, which is what a box with no
// text measures to
func (s TextSpan) Empty() bool { return s.Right <= s.Left }

// MeasureTextSpan lays the parts out in box the way RenderTextBox would, then
// reports the X range they cover without drawing any of it. It lets one box be
// sized around another, which is how the card name gives way to the mana cost
func MeasureTextSpan(box TextBoxSpec, parts ...TextPart) (TextSpan, error) {
	inner := insetBox(box)
	lay, err := fitLayout(inner, parts)
	if err != nil {
		return TextSpan{}, err
	}
	span := TextSpan{Left: inner.X + inner.Width, Right: inner.X}
	found := false
	for _, ln := range lay.lines {
		if ln.divider || len(ln.tokens) == 0 {
			continue
		}
		found = true
		start := lineStartX(inner, ln.width)
		if left := start.Floor(); left < span.Left {
			span.Left = left
		}
		if right := (start + ln.width).Ceil(); right > span.Right {
			span.Right = right
		}
	}
	if !found {
		return TextSpan{}, nil
	}
	return span, nil
}

// insetBox shrinks box by its padding on every side, giving the rectangle the
// text occupies. All layout and drawing use this inner rectangle, so padding
// governs the wrap width, the horizontal anchor, and the vertical fit at once.
// A zero padding leaves the box unchanged
func insetBox(box TextBoxSpec) TextBoxSpec {
	if box.Padding <= 0 {
		return box
	}
	box.X += box.Padding
	box.Y += box.Padding
	if box.Width -= 2 * box.Padding; box.Width < 0 {
		box.Width = 0
	}
	if box.Height -= 2 * box.Padding; box.Height < 0 {
		box.Height = 0
	}
	return box
}

// fitLayout returns the parts laid out at the largest size that fits box, from
// box.FontSize down to the floor. A baseline-anchored box is single-line point
// text, so it shrinks to fit box's width on one line. An area box shrinks to fit
// box's height. When even the floor overflows, the floor's layout is returned
// for drawLayout to clip
func fitLayout(box TextBoxSpec, parts []TextPart) (textLayout, error) {
	fits := func(lay textLayout) bool {
		if box.VAlign == "baseline" {
			return oneLineFits(lay, box)
		}
		return blockHeight(lay, box) <= box.Height
	}
	max := box.FontSize
	lay, err := layoutParts(box, parts, max)
	if err != nil {
		return textLayout{}, err
	}
	min := minFontSize(box)
	if fits(lay) || max <= min {
		return lay, nil
	}
	// max overflows, so search (min, max) for the largest size that fits,
	// keeping the floor's layout as the fallback when nothing does
	best, err := layoutParts(box, parts, min)
	if err != nil {
		return textLayout{}, err
	}
	lo, hi := min, max
	for i := 0; i < 8 && hi-lo > 0.5; i++ {
		mid := (lo + hi) / 2
		l, err := layoutParts(box, parts, mid)
		if err != nil {
			return textLayout{}, err
		}
		if fits(l) {
			best, lo = l, mid
		} else {
			hi = mid
		}
	}
	return best, nil
}

// oneLineFits reports whether a laid-out block is a single text line no wider
// than box, the fit test for a baseline-anchored point-text box
func oneLineFits(lay textLayout, box TextBoxSpec) bool {
	count := 0
	var w fixed.Int26_6
	for _, ln := range lay.lines {
		if ln.divider {
			continue
		}
		count++
		if ln.width > w {
			w = ln.width
		}
	}
	return count <= 1 && w <= fixed.I(box.Width)
}

// layoutParts wraps each part at size in its own face and stacks them, adding a
// divider line between consecutive parts that both have content
func layoutParts(box TextBoxSpec, parts []TextPart, size float64) (textLayout, error) {
	track := trackingPx(box.Tracking, size)
	var lines []line
	for _, p := range parts {
		face, err := p.Src.Face(size)
		if err != nil {
			return textLayout{}, err
		}
		var emph font.Face
		if p.Emph != nil {
			if emph, err = p.Emph.Face(size); err != nil {
				return textLayout{}, err
			}
		}
		st := partStyle{face: face, emph: emph, emphLead: p.EmphLead, sym: p.Sym, size: size, track: track}
		wrapped := wrapPart(box, p.Text, st)
		if len(wrapped) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, line{divider: true})
		}
		lines = append(lines, wrapped...)
	}
	return textLayout{lines: lines, size: size}, nil
}

// wrapPart greedily word-wraps one part's text to box.Width in its style's face,
// keeping paragraph breaks on explicit newlines, and returns its text lines
func wrapPart(box TextBoxSpec, text string, st partStyle) []line {
	space := spaceAdvance(st.face)
	maxW := fixed.I(box.Width)
	var lines []line
	for pi, para := range strings.Split(text, "\n") {
		start := len(lines)
		var cur line
		for _, tk := range tokenize(para, st) {
			switch {
			case len(cur.tokens) == 0:
				cur = line{tokens: []token{tk}, width: tk.advance}
			case cur.width+space+tk.advance <= maxW:
				cur.tokens = append(cur.tokens, tk)
				cur.width += space + tk.advance
			default:
				lines = append(lines, trimTrailingGap(cur))
				cur = line{tokens: []token{tk}, width: tk.advance}
			}
		}
		if len(cur.tokens) > 0 {
			lines = append(lines, trimTrailingGap(cur))
		}
		// A paragraph after the first opens with extra space, unless it wrapped
		// to nothing, in which case the next real paragraph carries the break
		if pi > 0 && len(lines) > start {
			lines[start].paraStart = true
		}
	}
	return lines
}

// trimTrailingGap takes the gap back off a line whose last run is a symbol. A
// symbol's advance carries the space to whatever follows it, and at the end of a
// line nothing does, so leaving it in would hold a right-aligned cost that far
// inside its box
func trimTrailingGap(ln line) line {
	if len(ln.tokens) == 0 {
		return ln
	}
	last := ln.tokens[len(ln.tokens)-1]
	if len(last.runs) == 0 {
		return ln
	}
	if run := last.runs[len(last.runs)-1]; run.sym != "" {
		ln.width -= run.advance - fixed.I(run.metr.Box)
	}
	return ln
}

// layoutText wraps a single face's text into box, a convenience for one-part
// layouts and tests
func layoutText(box TextBoxSpec, text string, face font.Face) textLayout {
	st := partStyle{face: face, size: box.FontSize, track: trackingPx(box.Tracking, box.FontSize)}
	return textLayout{lines: wrapPart(box, text, st), size: box.FontSize}
}

// tokenize splits a paragraph into whitespace-delimited tokens. A word draws in
// the style's face, or in its emph face when it falls inside parentheses or is
// part of a leading ability word from emphLead, so reminder text and ability
// words italicize
func tokenize(para string, st partStyle) []token {
	words := strings.Fields(para)
	leadEnd := -1
	if st.emph != nil {
		leadEnd = leadAbilityEnd(words, st.emphLead)
	}
	var toks []token
	italic := false
	for i, word := range words {
		f := st.face
		if st.emph != nil && (italic || strings.Contains(word, "(") || i <= leadEnd) {
			f = st.emph
		}
		if strings.Contains(word, "(") {
			italic = true
		}
		if strings.Contains(word, ")") {
			italic = false
		}
		toks = append(toks, buildToken(word, f, st))
	}
	return toks
}

// buildToken turns one word into a token, a symbol run for each braced code the
// renderer draws and a text run for everything between them. A word with no
// braces is the single text run it has always been
func buildToken(word string, face font.Face, st partStyle) token {
	var tk token
	for _, piece := range splitSymbols(word) {
		if piece.code != "" && st.sym != nil {
			if m, ok := st.sym.Symbol(piece.code, st.size); ok {
				tk.runs = append(tk.runs, glyphRun{face: face, advance: m.Advance, sym: piece.code, metr: m, ren: st.sym})
				tk.advance += m.Advance
				continue
			}
		}
		adv := (&font.Drawer{Face: face}).MeasureString(piece.text)
		if st.track != 0 {
			adv += st.track * fixed.Int26_6(len([]rune(piece.text)))
		}
		tk.runs = append(tk.runs, glyphRun{text: piece.text, face: face, advance: adv})
		tk.advance += adv
	}
	return tk
}

// symbolPiece is one piece of a split word. code is the braced symbol's code
// without its braces, empty for a literal piece, and text is always the
// characters as written, so an undrawn code can fall back to them
type symbolPiece struct {
	text string
	code string
}

// splitSymbols splits a word into its literal and braced-symbol pieces, so
// "{T}:" yields the symbol T and then ":". Symbols sit flush against text, which
// is why this splits inside a word rather than at spaces. An unclosed brace and
// an empty "{}" are literal
func splitSymbols(word string) []symbolPiece {
	var out []symbolPiece
	for {
		open := strings.IndexByte(word, '{')
		if open < 0 {
			break
		}
		end := strings.IndexByte(word[open:], '}')
		if end < 0 {
			break
		}
		end += open
		if end == open+1 {
			out = append(out, symbolPiece{text: word[:end+1]})
			word = word[end+1:]
			continue
		}
		if open > 0 {
			out = append(out, symbolPiece{text: word[:open]})
		}
		out = append(out, symbolPiece{text: word[open : end+1], code: word[open+1 : end]})
		word = word[end+1:]
	}
	if word != "" {
		out = append(out, symbolPiece{text: word})
	}
	return out
}

// leadAbilityEnd returns the index of the em-dash token when a paragraph opens
// with a phrase from set followed by " — ", so those tokens italicize. The
// phrase may be several words, since flavor words run long. It returns -1 when
// there is no match
func leadAbilityEnd(words []string, set map[string]bool) int {
	if len(set) == 0 {
		return -1
	}
	for i, w := range words {
		if w == "—" {
			if i == 0 {
				return -1
			}
			if set[strings.ToLower(strings.Join(words[:i], " "))] {
				return i
			}
			// A numbered ability word prints its count, like "Descend 8", while
			// the set holds the bare word, so match the first word when the rest
			// before the dash are digits
			if set[strings.ToLower(words[0])] && allDigits(words[1:i]) {
				return i
			}
			return -1
		}
		if i >= 8 {
			break
		}
	}
	return -1
}

// allDigits reports whether every word is a run of digits. An empty list is
// true, the plain no-count case
func allDigits(words []string) bool {
	for _, w := range words {
		if w == "" {
			return false
		}
		for _, r := range w {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// drawLayout paints a laid-out block into img, aligning each text line
// horizontally per box.Align and the block vertically per box.VAlign, and
// clipping lines that fall past the box bottom. A divider line paints nothing,
// its center Y is returned so the caller can place the divider art there. Text
// lines share one line height since a box's parts are the same size
func drawLayout(img *image.RGBA, box TextBoxSpec, lay textLayout) (int, bool, error) {
	face := firstTextFace(lay)
	if face == nil {
		return 0, false, nil
	}
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	lineHeight := lineHeightPx(metrics, lay.size, box.LineSpacing)
	divHeight := dividerHeight(lay.size)
	track := trackingPx(box.Tracking, lay.size)
	space := spaceAdvance(face)
	src := image.NewUniform(ParseHexColor(box.Color))
	bottom := box.Y + box.Height

	dividerY, hasDivider := 0, false

	// y tracks the top of the current line's slot. A baseline anchor puts the
	// first text line's baseline at box.Y, otherwise the block is placed by top
	var y int
	if box.VAlign == "baseline" {
		y = box.Y - ascent
	} else {
		y = blockTop(box, blockHeight(lay, box))
	}
	for _, ln := range lay.lines {
		if ln.divider {
			if !hasDivider {
				dividerY, hasDivider = y+divHeight/2, true
			}
			// The gap falls below the divider, so its distance from the rules
			// above is unchanged and the flavor below gets extra breathing room
			y += divHeight + dividerGap
			continue
		}
		if ln.paraStart {
			y += paragraphGap(lay.size)
		}
		baseline := y + ascent
		if baseline > bottom {
			break
		}
		x := lineStartX(box, ln.width)
		for _, tk := range ln.tokens {
			for _, run := range tk.runs {
				next, err := drawRun(img, src, run, x, baseline, track)
				if err != nil {
					return 0, false, err
				}
				x = next
			}
			x += space
		}
		y += lineHeight
	}
	return dividerY, hasDivider, nil
}

// drawRun paints one run at pen x and the given baseline and returns the pen x
// after it. A symbol run hands its square box to the part's renderer and carries
// no tracking, since the gap between symbols is already in its advance. With no
// tracking a text run draws in one call, otherwise it draws glyph by glyph so
// track can widen the gap after each one
func drawRun(img *image.RGBA, src image.Image, run glyphRun, x fixed.Int26_6, baseline int, track fixed.Int26_6) (fixed.Int26_6, error) {
	if run.sym != "" {
		left, top := x.Round(), baseline-run.metr.Ascent
		at := image.Rect(left, top, left+run.metr.Box, top+run.metr.Box)
		return x + run.advance, run.ren.DrawSymbol(img, run.sym, at)
	}
	drawer := &font.Drawer{Dst: img, Src: src, Face: run.face}
	if track == 0 {
		drawer.Dot = fixed.Point26_6{X: x, Y: fixed.I(baseline)}
		drawer.DrawString(run.text)
		return x + run.advance, nil
	}
	for _, r := range run.text {
		drawer.Dot = fixed.Point26_6{X: x, Y: fixed.I(baseline)}
		drawer.DrawString(string(r))
		adv, ok := run.face.GlyphAdvance(r)
		if !ok {
			adv = drawer.Dot.X - x
		}
		x += adv + track
	}
	return x, nil
}

// blockHeight is the total height of a laid-out block: its text lines at the
// line height plus its dividers at the divider height
func blockHeight(lay textLayout, box TextBoxSpec) int {
	face := firstTextFace(lay)
	if face == nil {
		return 0
	}
	lineHeight := lineHeightPx(face.Metrics(), lay.size, box.LineSpacing)
	divHeight := dividerHeight(lay.size)
	total := 0
	for _, ln := range lay.lines {
		switch {
		case ln.divider:
			total += divHeight + dividerGap
		case ln.paraStart:
			total += paragraphGap(lay.size) + lineHeight
		default:
			total += lineHeight
		}
	}
	return total
}

// blockTop is the y of the top of a block of total height within box, for the
// center and bottom anchors, defaulting to the box top
func blockTop(box TextBoxSpec, total int) int {
	switch box.VAlign {
	case "center":
		return box.Y + (box.Height-total)/2
	case "bottom":
		return box.Y + box.Height - total
	default:
		return box.Y
	}
}

// firstTextFace returns the face of the first text line, the face a block's
// line height and spacing are measured against, or nil when there is no text
func firstTextFace(lay textLayout) font.Face {
	for _, ln := range lay.lines {
		if !ln.divider && len(ln.tokens) > 0 {
			return ln.tokens[0].runs[0].face
		}
	}
	return nil
}

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

// dividerHeight is the vertical space a divider slot reserves between two parts,
// a little less than a text line so the divider art has room above and below
func dividerHeight(size float64) int {
	return int(math.Round(size * 0.8))
}

// dividerGap is extra space added below a divider, in document pixels, so the
// flavor text is not crowded against the rule. It counts toward the block height
// so vertical centering still accounts for it
const dividerGap = 10

// paragraphGap is the extra leading before a paragraph that follows another, a
// fraction of the em so the break reads as more than a plain line break
func paragraphGap(size float64) int {
	return int(math.Round(size * 0.3))
}

// trackingPx is the letter spacing a Photoshop tracking value adds between two
// glyphs at size, its thousandths of an em scaled to the point size. Zero
// tracking yields zero, the plain no-spacing case
func trackingPx(tracking, size float64) fixed.Int26_6 {
	if tracking == 0 {
		return 0
	}
	return fixed.Int26_6(math.Round(size * tracking / 1000 * 64))
}

// minFontSize is the shrink-to-fit floor, a box's own MinFontSize or, when it
// sets none, a little over half the font size
func minFontSize(box TextBoxSpec) float64 {
	if box.MinFontSize > 0 {
		return box.MinFontSize
	}
	return box.FontSize * 0.55
}

// ParseHexColor reads a "#RRGGBB" string, returning opaque black when it cannot.
// A caller drawing its own ink into a box reads the box color the same way
func ParseHexColor(s string) color.Color {
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
