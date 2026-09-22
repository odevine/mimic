// Package mana draws the braced codes a Magic card's mana cost and rules text
// carry, like {R} or {T}, as the pips a printed card shows: a colored disc with
// a Mana font icon on it. This is printed-card iconography, the same on any
// frame, not the pixels of a particular template
package mana

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/odevine/mimic/engine/fonts"
	"github.com/odevine/mimic/engine/template"
)

// The Mana font keeps its icons in the private use area rather than at ASCII
// letters. These are the codepoints its cheatsheet documents at
// https://mana.andrewgioia.com/cheatsheet.html
const (
	glyphWhite     = ''
	glyphBlue      = ''
	glyphBlack     = ''
	glyphRed       = ''
	glyphGreen     = ''
	glyphZero      = '' // 1 through 15 run on from here
	glyphX         = ''
	glyphY         = ''
	glyphZ         = ''
	glyphPhyrexian = ''
	glyphSnow      = ''
	glyphTap       = ''
	glyphUntap     = ''
	glyphSixteen   = '' // 17 through 20 run on from here
	glyphColorless = ''
	glyphArtistNib = '' // the nib the artist credit opens with
)

// PipDiameter is a pip's diameter as a fraction of the text size it draws at,
// measured off a Scryfall scan of a printed card. A caller sizing something
// else against a pip, like the cost's drop shadow, scales off this
const PipDiameter = 0.782

// The pip's other proportions. A symbol is centered near the middle of a
// capital, so it dips just below the baseline and reaches about cap height
const (
	// pipGap is the space after a pip, as a fraction of its own diameter, which
	// keeps the symbols of a cost apart. The two together are the advance, which
	// a printed card holds steady while the split between disc and gap varies a
	// little between printings
	pipGap = 0.114
	// pipRise is how far the pip's center sits above the baseline
	pipRise = 0.33
)

// How an icon sits on its disc, in fractions of the disc's diameter
const (
	// pipIconEm is the em the icon draws at. The font's icons fill their em, so
	// this is what insets one from the disc's edge. A printed card leaves the
	// icon close to the rim, a ring of about a fifteenth of the diameter
	pipIconEm = 0.86
	// iconRise is where the Mana font centers an icon above its baseline, in
	// ems, which is the point that has to land on the disc's center
	iconRise = 0.417
	// hybridIconEm and hybridIconOffset shrink the two icons of a hybrid and
	// push them apart along the split, one into each half
	hybridIconEm     = 0.40
	hybridIconOffset = 0.18
	// splitFeather is how many pixels the two halves of a hybrid blend across,
	// so the seam does not stair-step
	splitFeather = 1.5
)

// The pale washes a printed pip uses for its disc, and the near-black its icon
// draws in
var (
	pipColors = map[string]color.NRGBA{
		"W": {R: 0xFF, G: 0xFB, B: 0xD5, A: 0xFF},
		"U": {R: 0xAA, G: 0xE0, B: 0xFA, A: 0xFF},
		"B": {R: 0xCB, G: 0xC2, B: 0xBF, A: 0xFF},
		"R": {R: 0xF9, G: 0xAA, B: 0x8F, A: 0xFF},
		"G": {R: 0x9B, G: 0xD3, B: 0xAE, A: 0xFF},
	}
	// pipGray backs the symbols that carry no color: generic costs, colorless,
	// snow, the variables, and tap and untap
	pipGray = color.NRGBA{R: 0xCC, G: 0xC2, B: 0xC0, A: 0xFF}
	pipInk  = color.NRGBA{R: 0x1A, G: 0x17, B: 0x18, A: 0xFF}
)

// colorIcon maps a color letter to its Mana font icon
var colorIcon = map[string]rune{
	"W": glyphWhite,
	"U": glyphBlue,
	"B": glyphBlack,
	"R": glyphRed,
	"G": glyphGreen,
}

// neutralIcon maps the codes that draw on a gray disc and never appear as half
// of a hybrid to their icon. pipHalf covers the rest
var neutralIcon = map[string]rune{
	"S": glyphSnow,
	"X": glyphX,
	"Y": glyphY,
	"Z": glyphZ,
	"T": glyphTap,
	"Q": glyphUntap,
}

// pip is one symbol's artwork: the disc's color and the icon on it. A hybrid
// carries two of each and splits the disc corner to corner, the way a printed
// hybrid pip divides
type pip struct {
	back   [2]color.NRGBA
	icon   [2]rune
	hybrid bool
}

// solidPip is a one-color disc under one icon, what every symbol but a hybrid
// draws
func solidPip(back color.NRGBA, icon rune) pip {
	return pip{back: [2]color.NRGBA{back, back}, icon: [2]rune{icon, icon}}
}

// lookupPip resolves a Scryfall symbol code to its artwork, reporting false for
// a code this does not draw so the caller can leave the braces literal
func lookupPip(code string) (pip, bool) {
	c := strings.ToUpper(strings.TrimSpace(code))
	if a, b, ok := strings.Cut(c, "/"); ok {
		return compoundPip(a, b)
	}
	if back, icon, ok := pipHalf(c); ok {
		return solidPip(back, icon), true
	}
	if icon, ok := neutralIcon[c]; ok {
		return solidPip(pipGray, icon), true
	}
	return pip{}, false
}

// compoundPip resolves a slashed code. Phyrexian mana prints the phyrexian icon
// on the color's own disc, and everything else is a hybrid: two halves, which
// for a monocolored hybrid like {2/W} means a generic number beside a color
func compoundPip(a, b string) (pip, bool) {
	first, firstIcon, ok := pipHalf(a)
	if !ok {
		return pip{}, false
	}
	if b == "P" {
		return solidPip(first, glyphPhyrexian), true
	}
	second, secondIcon, ok := pipHalf(b)
	if !ok {
		return pip{}, false
	}
	return pip{
		back:   [2]color.NRGBA{first, second},
		icon:   [2]rune{firstIcon, secondIcon},
		hybrid: true,
	}, true
}

// pipHalf resolves a color letter, colorless, or a generic number to the disc
// color and icon it contributes: the pieces a hybrid's two halves are built
// from, and on their own the pips those codes draw alone
func pipHalf(s string) (color.NRGBA, rune, bool) {
	if icon, ok := colorIcon[s]; ok {
		return pipColors[s], icon, true
	}
	if icon, ok := genericIcon(s); ok {
		return pipGray, icon, true
	}
	if s == "C" {
		return pipGray, glyphColorless, true
	}
	return color.NRGBA{}, 0, false
}

// genericIcon returns the icon for a generic cost of 0 through 20, which the
// font lays out as one run of 0 through 15 and a second of 16 through 20
func genericIcon(s string) (rune, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	switch {
	case n >= 0 && n <= 15:
		return glyphZero + rune(n), true
	case n >= 16 && n <= 20:
		return glyphSixteen + rune(n-16), true
	}
	return 0, false
}

// Symbols draws the braced codes a card carries as the pips it prints: a
// colored disc with a Mana font icon on it. It satisfies
// template.SymbolRenderer, so text layout places symbols without knowing any of
// this. Faces and rasterized pips are cached, since a cost repeats symbols and
// a card's rules text repeats them again
type Symbols struct {
	sizer *fonts.Sizer
	mu    sync.Mutex
	faces map[int]font.Face
	pips  map[pipKey]*image.RGBA
}

// pipKey identifies a rasterized pip: a symbol code at a box size
type pipKey struct {
	code string
	box  int
}

// NewSymbols builds the renderer from the mana font in fontDir, or the
// embedded one. It returns nil when no mana font resolves, which leaves braced
// codes as their literal characters rather than drawing them in a face that has
// no icons
func NewSymbols(fontDir string) *Symbols {
	sizer := fonts.ResolveFont(fonts.Mana, fontDir)
	if sizer.Fallback() {
		return nil
	}
	return &Symbols{
		sizer: sizer,
		faces: map[int]font.Face{},
		pips:  map[pipKey]*image.RGBA{},
	}
}

// Symbol reports the room code takes at a text size. It is arithmetic over a
// table lookup, with nothing rasterized, so the shrink-to-fit search can measure
// a box at as many sizes as it likes
func (s *Symbols) Symbol(code string, size float64) (template.SymbolMetrics, bool) {
	if _, ok := lookupPip(code); !ok {
		return template.SymbolMetrics{}, false
	}
	box := int(math.Round(size * PipDiameter))
	if box < 2 {
		return template.SymbolMetrics{}, false
	}
	return template.SymbolMetrics{
		Advance: f26(size * PipDiameter * (1 + pipGap)),
		Box:     box,
		Ascent:  int(math.Round(size*pipRise)) + box/2,
	}, true
}

// DrawSymbol paints code into dst, filling at
func (s *Symbols) DrawSymbol(dst *image.RGBA, code string, at image.Rectangle) error {
	img, err := s.pipImage(code, at.Dx())
	if err != nil || img == nil {
		return err
	}
	draw.Draw(dst, at, img, image.Point{}, draw.Over)
	return nil
}

// pipImage returns the rasterized pip for code at a box size, building it on
// first use. It returns a nil image for a code with no artwork, which
// DrawSymbol treats as nothing to paint
func (s *Symbols) pipImage(code string, box int) (*image.RGBA, error) {
	p, ok := lookupPip(code)
	if !ok || box < 2 {
		return nil, nil
	}
	key := pipKey{code: code, box: box}
	s.mu.Lock()
	defer s.mu.Unlock()
	if img, ok := s.pips[key]; ok {
		return img, nil
	}
	img, err := s.drawPip(p, box)
	if err != nil {
		return nil, err
	}
	s.pips[key] = img
	return img, nil
}

// drawPip rasterizes one symbol at a box size: the disc through a circle mask,
// then the icon over it. A hybrid draws both icons smaller, one in each half.
// The caller holds the lock, so this may reach the face cache directly
func (s *Symbols) drawPip(p pip, box int) (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, box, box))
	paintDisc(img, discMask(box), p)
	center := float64(box) / 2
	if !p.hybrid {
		return img, s.drawIcon(img, p.icon[0], math.Round(float64(box)*pipIconEm), center, center, pipInk)
	}
	em := math.Round(float64(box) * hybridIconEm)
	off := float64(box) * hybridIconOffset
	if err := s.drawIcon(img, p.icon[0], em, center-off, center-off, pipInk); err != nil {
		return nil, err
	}
	return img, s.drawIcon(img, p.icon[1], em, center+off, center+off, pipInk)
}

// drawIcon paints one Mana font icon centered on (cx, cy) at an em size. Every
// icon in the font is one em wide and centered iconRise above its baseline, so
// the pen follows from the center alone
func (s *Symbols) drawIcon(img *image.RGBA, r rune, em, cx, cy float64, ink color.Color) error {
	face, err := s.face(em)
	if err != nil {
		return err
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: face}
	d.Dot = fixed.Point26_6{X: f26(cx - em/2), Y: f26(cy + iconRise*em)}
	d.DrawString(string(r))
	return nil
}

// face returns the mana face at a whole-pixel em, building it on first use. The
// caller holds the lock
func (s *Symbols) face(em float64) (font.Face, error) {
	size := int(math.Round(em))
	if size < 1 {
		size = 1
	}
	if f, ok := s.faces[size]; ok {
		return f, nil
	}
	f, err := s.sizer.Face(float64(size))
	if err != nil {
		return nil, err
	}
	s.faces[size] = f
	return f, nil
}

// discMask rasterizes an antialiased filled circle filling an n by n square,
// the coverage the disc's colors paint through
func discMask(n int) *image.Alpha {
	r := vector.NewRasterizer(n, n)
	addCircle(r, float32(n)/2, float32(n)/2, float32(n)/2)
	m := image.NewAlpha(image.Rect(0, 0, n, n))
	r.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// addCircle traces a circle onto r as the four cubic arcs of the usual kappa
// approximation
func addCircle(r *vector.Rasterizer, cx, cy, rad float32) {
	const kappa = 0.5522847
	o := rad * kappa
	r.MoveTo(cx+rad, cy)
	r.CubeTo(cx+rad, cy+o, cx+o, cy+rad, cx, cy+rad)
	r.CubeTo(cx-o, cy+rad, cx-rad, cy+o, cx-rad, cy)
	r.CubeTo(cx-rad, cy-o, cx-o, cy-rad, cx, cy-rad)
	r.CubeTo(cx+o, cy-rad, cx+rad, cy-o, cx+rad, cy)
	r.ClosePath()
}

// paintDisc colors img through mask: one color throughout, or, for a hybrid,
// the two colors either side of the diagonal a printed hybrid splits on
func paintDisc(img *image.RGBA, mask *image.Alpha, p pip) {
	n := img.Bounds().Dx()
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			a := mask.AlphaAt(x, y).A
			if a == 0 {
				continue
			}
			c := p.back[0]
			if p.hybrid {
				c = mixColor(p.back[0], p.back[1], splitShare(float64(x)+0.5, float64(y)+0.5, float64(n)))
			}
			img.SetRGBA(x, y, premul(c, a))
		}
	}
}

// splitShare is how much of a hybrid's first color a point takes: 1 in the
// upper left half, 0 in the lower right, and a short ramp across the seam that
// runs corner to corner between them
func splitShare(x, y, n float64) float64 {
	dist := (n - x - y) / math.Sqrt2
	switch share := dist/splitFeather + 0.5; {
	case share < 0:
		return 0
	case share > 1:
		return 1
	default:
		return share
	}
}

// mixColor blends b toward a by share, an opaque mix of two opaque colors
func mixColor(a, b color.NRGBA, share float64) color.NRGBA {
	mix := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x)*share + float64(y)*(1-share)))
	}
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 0xFF}
}

// premul scales an opaque color by a coverage value into the premultiplied form
// an RGBA image stores
func premul(c color.NRGBA, a uint8) color.RGBA {
	return color.RGBA{
		R: uint8(int(c.R) * int(a) / 0xFF),
		G: uint8(int(c.G) * int(a) / 0xFF),
		B: uint8(int(c.B) * int(a) / 0xFF),
		A: a,
	}
}

// f26 converts a pixel distance to the fixed-point units a font pen uses
func f26(v float64) fixed.Int26_6 {
	return fixed.Int26_6(math.Round(v * 64))
}

// NibCode is the braced code an artist credit opens with to draw the nib.
// Scryfall has no code for the nib, since it is part of a card's printing
// rather than its rules, so this one is the engine's own
const NibCode = "ARTIST"

// The nib's proportions, measured off a printed card. Its em runs a little over
// the text size, the glyph inks that whole em across, and the gap after it is
// what holds the artist's name off it
const (
	nibEm  = 1.02
	nibGap = 0.2
	// nibRise is where the nib centers above the baseline, in ems of the nib. A
	// printed card centers it on the capitals beside it, which this is measured
	// to land on for the small-caps face the credit draws in
	nibRise = 0.306
)

// ArtistNib draws the nib an artist credit opens with. It is flat ink in the
// line's own color rather than a pip, so it reads as part of the credit, and it
// borrows the mana renderer's font and caches
type ArtistNib struct {
	Sym *Symbols
	Ink color.Color
}

// Symbol reports the room the nib takes at a text size, and nothing for any
// other code, so an artist name that happens to hold braces keeps them
func (n ArtistNib) Symbol(code string, size float64) (template.SymbolMetrics, bool) {
	if code != NibCode {
		return template.SymbolMetrics{}, false
	}
	em := size * nibEm
	box := int(math.Round(em))
	if box < 2 {
		return template.SymbolMetrics{}, false
	}
	return template.SymbolMetrics{
		Advance: f26(em * (1 + nibGap)),
		Box:     box,
		Ascent:  int(math.Round(em*nibRise)) + box/2,
	}, true
}

// DrawSymbol paints the nib into dst, centered on at
func (n ArtistNib) DrawSymbol(dst *image.RGBA, code string, at image.Rectangle) error {
	if code != NibCode || at.Dx() < 2 {
		return nil
	}
	n.Sym.mu.Lock()
	defer n.Sym.mu.Unlock()
	em := float64(at.Dx())
	return n.Sym.drawIcon(dst, glyphArtistNib, em, float64(at.Min.X)+em/2, float64(at.Min.Y)+em/2, n.Ink)
}
