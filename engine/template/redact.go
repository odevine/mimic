package template

import (
	"image"
	"image/draw"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// redactMark opens and closes a redacted span in a box's text, so
// "You ~~lose the game~~." prints a marker bar where "lose the game" would be.
// No card's own text uses a doubled tilde, and it reads as a strikethrough in
// the markdown people already type
const redactMark = "~~"

// redactSeg is one piece of a word split at its redaction marks, the text as
// written without the marks and whether it falls inside a redaction
type redactSeg struct {
	text   string
	redact bool
}

// splitRedactions splits word at each ~~, flipping between plain and redacted
// at every mark, and drops the marks themselves. on is whether the word opens
// inside a redaction, and the second result is whether it closes inside one, so
// a span can run across words. Empty pieces are left out, which makes a bare ~~
// split to nothing
func splitRedactions(word string, on bool) ([]redactSeg, bool) {
	var out []redactSeg
	for {
		i := strings.Index(word, redactMark)
		if i < 0 {
			break
		}
		if i > 0 {
			out = append(out, redactSeg{text: word[:i], redact: on})
		}
		on = !on
		word = word[i+len(redactMark):]
	}
	if word != "" {
		out = append(out, redactSeg{text: word, redact: on})
	}
	return out, on
}

// redactBar is a redacted stretch of one line being collected for painting: the
// pen x it starts at, where its last run ends, and the face that sizes the
// stroke. The zero value is an empty bar
type redactBar struct {
	left, right fixed.Int26_6
	face        font.Face
	open        bool
}

// extend adds a redacted run drawn at pen x to the bar, opening it at x when it
// is empty. The bar reaches the end of the run's ink, so a symbol's trailing gap
// is left out the way it is at the end of a line
func (b *redactBar) extend(run glyphRun, x fixed.Int26_6) {
	if !b.open {
		*b = redactBar{left: x, face: run.face, open: true}
	}
	b.right = x + run.advance
	if run.sym != "" {
		b.right = x + fixed.I(run.metr.Box)
	}
}

// paint draws the bar, if one is open, onto the given baseline and empties it.
// lineHeight is the block's baseline-to-baseline distance, which caps the
// stroke's thickness
func (b *redactBar) paint(img *image.RGBA, src image.Image, baseline, lineHeight int) {
	if b.open {
		paintRedaction(img, src, b.left, b.right, baseline, lineHeight, b.face)
	}
	*b = redactBar{}
}

// paintRedaction draws a marker stroke over the span from left to right on a
// line. It covers the face's capitals and most of its descenders, though never
// more than most of a line so strokes on adjacent lines stay apart under tight
// leading. It overhangs the span a little at each end and rounds both ends, and
// its edges wander a few percent of its height so it reads as drawn by hand. The
// wander is seeded from where the stroke sits, so the same card always renders
// the same strokes
func paintRedaction(img *image.RGBA, src image.Image, left, right fixed.Int26_6, baseline, lineHeight int, face font.Face) {
	m := face.Metrics()
	capH := fixedFloat(m.CapHeight)
	if capH <= 0 {
		capH = 0.7 * fixedFloat(m.Ascent)
	}
	top := float64(baseline) - capH*1.15
	bottom := float64(baseline) + fixedFloat(m.Descent)*0.6
	if limit := float64(lineHeight) * 0.8; lineHeight > 0 && bottom-top > limit {
		trim := (bottom - top - limit) / 2
		top, bottom = top+trim, bottom-trim
	}
	h := bottom - top
	if h <= 0 {
		return
	}
	r := h / 2
	l, rt := fixedFloat(left)-h*0.12, fixedFloat(right)+h*0.12
	// A span narrower than the stroke is thick, like a lone comma, draws as a dot
	if rt-l < h {
		c := (l + rt) / 2
		l, rt = c-r, c+r
	}
	mid := (top + bottom) / 2
	seed := float64(baseline)*0.37 + l*0.11
	wander := func(x, phase float64) float64 {
		return r * (0.06*math.Sin(x/(h*1.7)+phase) + 0.035*math.Sin(x/(h*0.6)+phase*2.3))
	}

	rect := image.Rect(
		int(math.Floor(l)), int(math.Floor(top-r*0.2)),
		int(math.Ceil(rt)), int(math.Ceil(bottom+r*0.2)),
	).Intersect(img.Bounds())
	if rect.Empty() {
		return
	}
	mask := image.NewAlpha(rect)
	cover := make([]float64, rect.Dy())
	// Each column is sampled at a few x positions so the rounded ends come out
	// smooth, and each sample adds the share of every row its edges overlap
	const samples = 4
	for px := rect.Min.X; px < rect.Max.X; px++ {
		clear(cover)
		for k := 0; k < samples; k++ {
			x := float64(px) + (float64(k)+0.5)/samples
			if x < l || x > rt {
				continue
			}
			var d float64
			switch {
			case x < l+r:
				d = l + r - x
			case x > rt-r:
				d = x - (rt - r)
			}
			s := math.Sqrt(max(0, 1-(d/r)*(d/r)))
			y0 := mid - (r+wander(x, seed))*s
			y1 := mid + (r+wander(x, seed+5))*s
			for py := rect.Min.Y; py < rect.Max.Y; py++ {
				if o := min(float64(py+1), y1) - max(float64(py), y0); o > 0 {
					cover[py-rect.Min.Y] += o / samples
				}
			}
		}
		for i, c := range cover {
			mask.Pix[mask.PixOffset(px, rect.Min.Y+i)] = uint8(math.Round(min(c, 1) * 0xFF))
		}
	}
	draw.DrawMask(img, rect, src, image.Point{}, mask, rect.Min, draw.Over)
}

// fixedFloat is a 26.6 fixed-point value as a float in whole pixels
func fixedFloat(v fixed.Int26_6) float64 {
	return float64(v) / 64
}
