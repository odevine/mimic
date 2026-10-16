package template

import (
	"fmt"
	"image"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/odevine/impasto/blend"

	"github.com/odevine/mimic/engine/frame"
	"github.com/odevine/mimic/engine/manifest"
)

// Part is an art set a layout draws from: an image for each material and the slot
// that picks one
type Part struct {
	Name string
	Slot frame.Slot
	// Variants is the path of the image for each material, by the material's key
	Variants map[string]string
	// Origin is where the images' top left sits in the canvas
	Origin image.Point
	Blend  *manifest.Blend
}

// Entry is one use of a part in a layout's stack
type Entry struct {
	ID        string
	Part      *Part
	Condition frame.Condition
	// Face is which face's data the entry reads, as for a text box
	Face string
	// At places the image in place of the part's origin, and is nil to keep it
	At     *image.Point
	Mirror *EntryMirror
	Mode   blend.Mode
	// Opacity is from 0 to 1, and OpacityFeature names a range feature to use in
	// its place
	Opacity        float64
	OpacityFeature string
}

// Origin is where the entry's image sits in the canvas
func (e *Entry) Origin() image.Point {
	if e.At != nil {
		return *e.At
	}
	return e.Part.Origin
}

// EntryMirror is a left to right flip of a region of an entry, when its condition holds
type EntryMirror struct {
	Condition frame.Condition
	// Region is the part flipped, in the authored canvas, on its left half. The
	// empty rectangle flips the whole image
	Region image.Rectangle
}

// Box is a text box of a layout, with its overrides compiled
type Box struct {
	Key string
	// Draws is what the engine fills into the box, such as "title"
	Draws     string
	Face      string
	Space     string
	Condition frame.Condition
	// Base is the box as authored, and Overrides are the changes made to it when
	// a condition holds, the first that holds taking effect
	Base      manifest.TextBox
	Overrides []Override
}

// Override is a condition and the box that results from merging its changes over
// the base box
type Override struct {
	Condition frame.Condition
	Box       manifest.TextBox
}

// Symbol is where a symbol draws
type Symbol struct {
	Spec      manifest.Symbol
	Condition frame.Condition
}

// Layout is everything needed to draw one card shape, compiled from a manifest so
// a render parses nothing. It is safe for concurrent use
type Layout struct {
	Name   string
	Serves []frame.Shape
	Canvas manifest.Canvas
	Stack  []*Entry
	// Boxes lists the text boxes sorted by key
	Boxes   []*Box
	Art     []manifest.ArtWindow
	Symbols map[string]*Symbol
	// Sections lists the regions whose rules text splits into rows, sorted by key
	Sections []*Section

	entries map[string]*Entry
	boxes   map[string]*Box
	parts   map[string]*Part
	geoMu   sync.Mutex
	geo     map[int]*Geometry
}

// Entry returns the stack entry with the id
func (l *Layout) Entry(id string) (*Entry, bool) {
	e, ok := l.entries[id]
	return e, ok
}

// Part returns the part with the name, whether or not the stack uses it
func (l *Layout) Part(name string) (*Part, bool) {
	p, ok := l.parts[name]
	return p, ok
}

// Box returns the text box with the key
func (l *Layout) Box(key string) (*Box, bool) {
	b, ok := l.boxes[key]
	return b, ok
}

// compileLayout builds a layout from its manifest form. A manifest that decoded
// without an error always compiles, so a failure here is a bug in the checks
func compileLayout(name string, ml manifest.Layout, parts map[string]*Part) (*Layout, error) {
	l := &Layout{
		Name: name, Serves: ml.Serves, Canvas: ml.Canvas, Art: ml.Art,
		entries: map[string]*Entry{}, boxes: map[string]*Box{}, Symbols: map[string]*Symbol{},
		parts: parts, geo: map[int]*Geometry{},
	}
	for i, me := range ml.Stack {
		e, err := compileEntry(me, parts)
		if err != nil {
			return nil, fmt.Errorf("layout %q stack[%d]: %w", name, i, err)
		}
		l.Stack = append(l.Stack, e)
		l.entries[e.ID] = e
	}
	keys := make([]string, 0, len(ml.TextBoxes))
	for k := range ml.TextBoxes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b, err := compileBox(k, ml.TextBoxes[k])
		if err != nil {
			return nil, fmt.Errorf("layout %q text box %q: %w", name, k, err)
		}
		l.Boxes = append(l.Boxes, b)
		l.boxes[k] = b
	}
	secs, err := compileSections(ml.Sections)
	if err != nil {
		return nil, fmt.Errorf("layout %q sections: %w", name, err)
	}
	l.Sections = secs
	for sn, ms := range ml.Symbols {
		c, err := frame.Compile(ms.Condition)
		if err != nil {
			return nil, fmt.Errorf("layout %q symbol %q: %w", name, sn, err)
		}
		l.Symbols[sn] = &Symbol{Spec: ms, Condition: c}
	}
	return l, nil
}

func compileEntry(me manifest.Entry, parts map[string]*Part) (*Entry, error) {
	part, ok := parts[me.Part]
	if !ok {
		return nil, fmt.Errorf("no part %q", me.Part)
	}
	cond, err := frame.Compile(me.Condition)
	if err != nil {
		return nil, err
	}
	mode, err := manifest.ParseMode(me.Mode)
	if err != nil {
		return nil, err
	}
	e := &Entry{ID: me.Key(), Part: part, Condition: cond, Face: me.Face, Mode: mode, Opacity: 1, OpacityFeature: me.OpacityFeature}
	if me.Opacity != nil {
		e.Opacity = *me.Opacity
	}
	if me.At != nil {
		e.At = &image.Point{X: me.At.X, Y: me.At.Y}
	}
	if me.Mirror != nil {
		mc, err := frame.Compile(me.Mirror.Condition)
		if err != nil {
			return nil, err
		}
		e.Mirror = &EntryMirror{Condition: mc}
		if me.Mirror.Width > 0 && me.Mirror.Height > 0 {
			e.Mirror.Region = image.Rect(me.Mirror.X, me.Mirror.Y, me.Mirror.X+me.Mirror.Width, me.Mirror.Y+me.Mirror.Height)
		}
	}
	return e, nil
}

func compileBox(key string, mb manifest.TextBox) (*Box, error) {
	cond, err := frame.Compile(mb.Condition)
	if err != nil {
		return nil, err
	}
	b := &Box{Key: key, Draws: mb.Content(key), Face: mb.Face, Space: mb.Space, Condition: cond, Base: mb}
	for _, o := range mb.When {
		oc, err := frame.Compile(o.Condition)
		if err != nil {
			return nil, err
		}
		if o.Box == nil {
			return nil, fmt.Errorf("an override was not merged, so the manifest was not decoded")
		}
		b.Overrides = append(b.Overrides, Override{Condition: oc, Box: *o.Box})
	}
	// The override that names the most conditions is tried first, and overrides that
	// name the same number keep the order they were written in
	sort.SliceStable(b.Overrides, func(i, j int) bool {
		return conditionTerms(b.Overrides[i].Condition) > conditionTerms(b.Overrides[j].Condition)
	})
	return b, nil
}

// conditionTerms is how many terms a condition names
func conditionTerms(c frame.Condition) int {
	s := strings.TrimSpace(c.String())
	if s == "" {
		return 0
	}
	return strings.Count(s, ",") + 1
}

// Pick finds the first override whose condition holds, which is the one that takes
// effect, and reports that none does when the base box stands
func (b *Box) Pick(holds func(frame.Condition) bool) (index int, ok bool) {
	for i, o := range b.Overrides {
		if holds(o.Condition) {
			return i, true
		}
	}
	return -1, false
}

// Turned reports whether the layout's rotation swaps its width and height
func (l *Layout) Turned() bool { return l.Canvas.Rotate == 90 || l.Canvas.Rotate == 270 }

// DeliveredWidth is the width of the finished image, which is the authored
// height when the layout rotates a quarter turn
func (l *Layout) DeliveredWidth() int {
	if l.Turned() {
		return l.Canvas.Height
	}
	return l.Canvas.Width
}

// DeliveredHeight is the height of the finished image
func (l *Layout) DeliveredHeight() int {
	if l.Turned() {
		return l.Canvas.Width
	}
	return l.Canvas.Height
}

// NativeDPI is the resolution the layout's canvas was authored at. One that states
// its own DPI reports that, and one that does not infers it from its width and the
// physical width of a card with bleed
func (l *Layout) NativeDPI() int {
	if l.Canvas.DPI > 0 {
		return l.Canvas.DPI
	}
	return int(math.Round(float64(l.DeliveredWidth()) / CardWidthInches))
}

// ClampDPI holds dpi within the range the layout renders at: no less than MinDPI,
// and no more than the authored resolution. A non-positive dpi means native
func (l *Layout) ClampDPI(dpi int) int {
	native := l.NativeDPI()
	switch {
	case dpi <= 0 || dpi > native:
		return native
	case dpi < MinDPI:
		return min(MinDPI, native)
	default:
		return dpi
	}
}

// ScaleForDPI is the factor that renders the layout at dpi, which is clamped
// first, so a caller can pass a saved preference through without checking it
// against the template that happens to be active
func (l *Layout) ScaleForDPI(dpi int) Scale {
	native := l.NativeDPI()
	if native <= 0 {
		return 1
	}
	dpi = l.ClampDPI(dpi)
	if dpi == native {
		return 1
	}
	return Scale(float64(dpi) / float64(native))
}

// Resolution is one render size: the pixel dimensions it produces and the dpi
// those dimensions print at
type Resolution struct {
	DPI    int  `json:"dpi"`
	Width  int  `json:"width"`
	Height int  `json:"height"`
	Native bool `json:"native,omitempty"`
}

// Resolution reports the pixel size the layout renders at for dpi, clamped the way
// ScaleForDPI clamps it
func (l *Layout) Resolution(dpi int) Resolution {
	dpi = l.ClampDPI(dpi)
	g := l.Geometry(dpi)
	return Resolution{DPI: dpi, Width: g.DeliveredWidth, Height: g.DeliveredHeight, Native: dpi == l.NativeDPI()}
}

// Presets lists the resolutions a ui offers for the layout, smallest first and
// ending with its own
func (l *Layout) Presets() []Resolution {
	native := l.NativeDPI()
	out := make([]Resolution, 0, len(PresetDPIs)+1)
	for _, dpi := range PresetDPIs {
		if dpi >= native || dpi < MinDPI {
			continue
		}
		out = append(out, l.Resolution(dpi))
	}
	return append(out, l.Resolution(native))
}

// Geometry is a layout's measurements at one resolution. It is built once for each
// resolution and shared, so a batch of cards does not scale the manifest for each
type Geometry struct {
	Scale  Scale
	Width  int
	Height int
	// DeliveredWidth and DeliveredHeight are the finished image's size, after any
	// rotation
	DeliveredWidth  int
	DeliveredHeight int
	// Boxes is each text box at this scale, by key
	Boxes   map[string]ScaledBox
	Art     []ArtSlot
	Symbols map[string]SymbolSpec
}

// ScaledBox is a text box at one scale, as the flat spec the text layout reads
type ScaledBox struct {
	Base TextBoxSpec
	// Overrides parallels Box.Overrides, each the box an override makes
	Overrides []TextBoxSpec
}

// Geometry returns the layout's measurements at dpi, which is clamped first
func (l *Layout) Geometry(dpi int) *Geometry {
	dpi = l.ClampDPI(dpi)
	l.geoMu.Lock()
	defer l.geoMu.Unlock()
	if g, ok := l.geo[dpi]; ok {
		return g
	}
	g := l.buildGeometry(l.ScaleForDPI(dpi))
	l.geo[dpi] = g
	return g
}

func (l *Layout) buildGeometry(s Scale) *Geometry {
	g := &Geometry{
		Scale: s, Width: s.Px(l.Canvas.Width), Height: s.Px(l.Canvas.Height),
		Boxes: make(map[string]ScaledBox, len(l.Boxes)), Symbols: make(map[string]SymbolSpec, len(l.Symbols)),
	}
	if l.Turned() {
		g.DeliveredWidth, g.DeliveredHeight = g.Height, g.Width
	} else {
		g.DeliveredWidth, g.DeliveredHeight = g.Width, g.Height
	}
	for _, b := range l.Boxes {
		sb := ScaledBox{Base: scaleBox(SpecOf(b.Draws, b.Base), s)}
		for _, o := range b.Overrides {
			sb.Overrides = append(sb.Overrides, scaleBox(SpecOf(b.Draws, o.Box), s))
		}
		g.Boxes[b.Key] = sb
	}
	for _, a := range l.Art {
		g.Art = append(g.Art, ArtSlot{X: s.Px(a.X), Y: s.Px(a.Y), Width: s.Px(a.Width), Height: s.Px(a.Height), After: a.After, Rotate: a.Rotate})
	}
	for name, sym := range l.Symbols {
		g.Symbols[name] = scaleSymbol(SymbolOf(sym.Spec), s)
	}
	return g
}

// SpecOf is the flat spec the text layout reads for a manifest text box
func SpecOf(draws string, b manifest.TextBox) TextBoxSpec {
	px, py := b.Text.PaddingX, b.Text.PaddingY
	spec := TextBoxSpec{
		X: b.Rect.X, Y: b.Rect.Y, Width: b.Rect.Width, Height: b.Rect.Height,
		FontSize: b.Text.FontSize, Align: b.Text.Align, Color: b.Text.Color,
		VAlign: b.Text.VAlign, LineSpacing: b.Text.LineSpacing, MinFontSize: b.Text.MinFontSize,
		PaddingX: &px, PaddingY: &py,
		Tracking: b.Text.Tracking, Font: b.Text.Font, Prefix: b.Text.Prefix,
		ClearOf: b.Flow.ClearOf, ClearGap: b.Flow.ClearGap, AvoidLayer: b.Flow.AvoidLayer,
		Space: b.Space, Rotate: b.Rotate,
	}
	if d := b.Flow.Duck; d != nil {
		spec.DuckLayer, spec.DuckRow = d.Layer, d.Row
	}
	if s := b.Text.Shadow; s != nil {
		spec.Shadow = &ShadowSpec{Distance: s.Distance, Angle: s.Angle, Opacity: s.Opacity, Color: s.Color}
	}
	return spec
}

// SymbolOf is the flat spec the symbol drawing reads for a manifest symbol
func SymbolOf(s manifest.Symbol) SymbolSpec {
	return SymbolSpec{X: s.X, Y: s.Y, Width: s.Width, Height: s.Height, Align: s.Align, VAlign: s.VAlign, Scale: s.Scale}
}

// scaleBox multiplies one text box's pixel geometry. Tracking is in thousandths
// of an em, line spacing and the clear gap are multiples of the font size, and
// a shadow's distance is a fraction of it, so all four scale with the font on
// their own
func scaleBox(box TextBoxSpec, s Scale) TextBoxSpec {
	box.X, box.Y = s.Px(box.X), s.Px(box.Y)
	box.Width, box.Height = s.Px(box.Width), s.Px(box.Height)
	box.FontSize = s.F(box.FontSize)
	box.MinFontSize = s.F(box.MinFontSize)
	box.Padding = s.Px(box.Padding)
	box.PaddingX = scalePad(box.PaddingX, s)
	box.PaddingY = scalePad(box.PaddingY, s)
	box.Avoid = s.Rect(box.Avoid)
	return box
}

// scalePad scales an axis padding override, keeping nil as nil so the box still
// falls back to its all-round padding
func scalePad(p *int, s Scale) *int {
	if p == nil {
		return nil
	}
	v := s.Px(*p)
	return &v
}

// scaleSymbol multiplies a symbol's box. Its scale is a fraction, so it is already
// resolution independent
func scaleSymbol(sym SymbolSpec, s Scale) SymbolSpec {
	sym.X, sym.Y = s.Px(sym.X), s.Px(sym.Y)
	sym.Width, sym.Height = s.Px(sym.Width), s.Px(sym.Height)
	return sym
}
