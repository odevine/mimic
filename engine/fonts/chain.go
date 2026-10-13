package fonts

import (
	"image"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// chainFace draws each rune from the first of its fonts that has a glyph for it.
// The first font sets the line's metrics and does the kerning among its own
// runes, so a line is as tall as it would be with that font alone, and a rune
// from a later font sits on the same baseline at the same size. A rune none of
// them has is left to the first font, which reports it missing
type chainFace struct {
	size  float64
	fonts []*opentype.Font
	// faces holds the sized face of each font. A later one is made the first
	// time a rune needs it, since most text never does
	faces  []font.Face
	failed []bool
	// pick remembers which font drew each rune
	pick map[rune]int
}

// newChainFace sizes the first font now and the others when a rune calls for them
func newChainFace(fonts []*opentype.Font, size float64) (font.Face, error) {
	first, err := newFace(fonts[0], size)
	if err != nil {
		return nil, err
	}
	c := &chainFace{
		size:   size,
		fonts:  fonts,
		faces:  make([]font.Face, len(fonts)),
		failed: make([]bool, len(fonts)),
		pick:   map[rune]int{},
	}
	c.faces[0] = first
	return c, nil
}

// face returns the sized face of font i, or nil when it cannot be made
func (c *chainFace) face(i int) font.Face {
	if c.faces[i] == nil && !c.failed[i] {
		f, err := newFace(c.fonts[i], c.size)
		if err != nil {
			c.failed[i] = true
			return nil
		}
		c.faces[i] = f
	}
	return c.faces[i]
}

// fontFor is the index of the first font with a glyph for r, or 0 when none has
func (c *chainFace) fontFor(r rune) int {
	if i, ok := c.pick[r]; ok {
		return i
	}
	chosen := 0
	for i := range c.fonts {
		f := c.face(i)
		if f == nil {
			continue
		}
		if _, ok := f.GlyphAdvance(r); ok {
			chosen = i
			break
		}
	}
	c.pick[r] = chosen
	return chosen
}

func (c *chainFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	return c.faces[c.fontFor(r)].Glyph(dot, r)
}

func (c *chainFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	return c.faces[c.fontFor(r)].GlyphBounds(r)
}

func (c *chainFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	return c.faces[c.fontFor(r)].GlyphAdvance(r)
}

// Kern kerns two runes drawn from the same font and leaves a pair split across
// fonts unkerned, since neither font knows the other's glyphs
func (c *chainFace) Kern(r0, r1 rune) fixed.Int26_6 {
	i := c.fontFor(r0)
	if i != c.fontFor(r1) {
		return 0
	}
	return c.faces[i].Kern(r0, r1)
}

// Metrics are the first font's
func (c *chainFace) Metrics() font.Metrics { return c.faces[0].Metrics() }

func (c *chainFace) Close() error {
	var first error
	for _, f := range c.faces {
		if f == nil {
			continue
		}
		if err := f.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
