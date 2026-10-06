// Package svgpath reads a color SVG icon, such as the set symbols in the
// mtg-vectors catalog, and draws it with impasto. It understands the subset of
// SVG those icons use: paths and the basic shapes, groups and transforms, flat
// colors and linear and radial gradients, strokes, opacity, class rules from a
// style element, and use. A file that needs more than that, such as a filter, a
// mask, or a clip that actually clips, is reported as unsupported instead of
// drawn wrong
package svgpath

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/odevine/impasto/path"
)

// MaxBytes is the largest file Parse reads. Real icons are a few KB, so the
// limit sits far above them and only turns away a file that is not an icon
const MaxBytes = 256 << 10

const (
	maxDepth  = 32
	maxShapes = 1024
	maxUses   = 256
)

// ErrUnsupported marks a file that uses SVG features outside the supported
// subset. The error that wraps it names the feature
var ErrUnsupported = errors.New("svgpath: unsupported SVG feature")

const svgNS = "http://www.w3.org/2000/svg"

// Icon is an SVG icon, in the file's own coordinates
type Icon struct {
	// ViewBox is the file's drawing box, from its viewBox attribute or, when
	// that is missing, its width and height
	ViewBox Rect
	// Shapes are the file's drawn shapes, in document order
	Shapes []Shape
}

// Rect is an axis-aligned rectangle given by its corner and size
type Rect struct{ X, Y, W, H float32 }

// Shape is one drawn outline with the paint that fills and strokes it. Path is
// the outline in the icon's coordinates, with every transform already applied
type Shape struct {
	Path *path.Path
	Rule path.FillRule

	outline *outline
	ctm     Affine
	fill    *fillUse
	stroke  *strokeUse
}

// fillUse is a shape's fill and the opacity it is drawn at
type fillUse struct {
	src   paintSource
	alpha float64
}

// strokeUse is a shape's stroke
type strokeUse struct {
	src        paintSource
	alpha      float64
	width      float64
	join       path.Join
	cap        path.Cap
	miter      float64
	dash       []float64
	dashOffset float64
}

// Parse reads an SVG icon. It returns an error wrapping ErrUnsupported for a
// feature outside the supported subset, and another error for a file that is
// not well formed SVG
func Parse(data []byte) (*Icon, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("svgpath: file is %d bytes, over the %d byte limit", len(data), MaxBytes)
	}
	doc, err := parseDocument(data)
	if err != nil {
		return nil, err
	}
	icon := &Icon{}
	if err := icon.readViewBox(doc.root.attr); err != nil {
		return nil, err
	}
	b := &builder{doc: doc, icon: icon}
	if err := b.walk(doc.root, defaultStyle(), 0); err != nil {
		return nil, err
	}
	if len(icon.Shapes) == 0 {
		return nil, errors.New("svgpath: file draws no shapes")
	}
	if err := icon.checkExtent(); err != nil {
		return nil, err
	}
	return icon, nil
}

// box is a rectangle given by its edges, in float64 for clip tests
type box struct{ minX, minY, maxX, maxY float64 }

// style is the drawing state an element hands to its children
type style struct {
	fill, stroke  paintRef
	fillOpacity   float64
	strokeOpacity float64
	opacity       float64
	rule          path.FillRule
	strokeWidth   float64
	join          path.Join
	cap           path.Cap
	miter         float64
	dash          []float64
	dashOffset    float64
	color         color.NRGBA
	hidden        bool
	ctm           Affine
	clips         []box
}

// paintRef is a fill or stroke value as the file writes it
type paintRef struct {
	kind int // refNone, refColor or refURL
	col  color.NRGBA
	id   string
}

const (
	refNone = iota
	refColor
	refURL
)

func defaultStyle() style {
	return style{
		fill:          paintRef{kind: refColor, col: color.NRGBA{A: 255}},
		fillOpacity:   1,
		strokeOpacity: 1,
		opacity:       1,
		strokeWidth:   1,
		miter:         4,
		color:         color.NRGBA{A: 255},
		ctm:           Identity,
	}
}

// builder walks the tree and collects shapes
type builder struct {
	doc  *document
	icon *Icon
	uses int
}

func (b *builder) walk(n *node, parent style, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("svgpath: elements nest deeper than %d", maxDepth)
	}
	switch n.name {
	case "defs", "title", "desc", "metadata", "style", "linearGradient", "radialGradient",
		"clipPath", "foreignObject", "marker", "pattern", "mask", "filter", "symbol":
		// Nothing to draw where these stand. A drawn element that refers to a mask
		// or filter is the one reported
		return nil
	case "svg", "g", "switch", "a", "use", "path", "rect", "circle", "ellipse", "polygon":
	default:
		return fmt.Errorf("%w: <%s> element", ErrUnsupported, n.name)
	}
	if n.name == "svg" && n != b.doc.root {
		return fmt.Errorf("%w: nested <svg>", ErrUnsupported)
	}

	st := parent
	d := b.doc.computed(n)
	if err := st.apply(d); err != nil {
		return err
	}
	if st.hidden && d["display"] == "none" {
		return nil
	}
	if n.name != "svg" {
		if tr := n.attr["transform"]; tr != "" {
			t, err := parseTransform(tr)
			if err != nil {
				return err
			}
			st.ctm = st.ctm.Mul(t)
		}
	}
	if cp := d["clip-path"]; cp != "" && cp != "none" {
		clip, err := b.clipBox(cp, st.ctm)
		if err != nil {
			return err
		}
		st.clips = append(append([]box(nil), st.clips...), clip)
	}

	switch n.name {
	case "path", "rect", "circle", "ellipse", "polygon":
		return b.shape(n, st)
	case "use":
		return b.use(n, st, depth)
	}
	for _, k := range n.kids {
		// An Illustrator export wraps its art in a switch beside an extension
		// element, and the art is the part to draw
		if n.name == "switch" && k.name == "foreignObject" {
			continue
		}
		if err := b.walk(k, st, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// use draws the element a use points at, moved by the use's x and y
func (b *builder) use(n *node, st style, depth int) error {
	if b.uses++; b.uses > maxUses {
		return fmt.Errorf("svgpath: more than %d use elements", maxUses)
	}
	id := n.hrefID()
	target := b.doc.ids[id]
	if id == "" || target == nil {
		return fmt.Errorf("%w: use of %q", ErrUnsupported, n.attr["href"])
	}
	x, err := lengthAttr(n.attr, "x")
	if err != nil {
		return err
	}
	y, err := lengthAttr(n.attr, "y")
	if err != nil {
		return err
	}
	st.ctm = st.ctm.Mul(Translate(x, y))
	return b.walk(target, st, depth+1)
}

// apply folds an element's declared properties into the state
func (st *style) apply(d decls) error {
	// color first, since currentColor in a fill reads it
	if v, ok := d["color"]; ok {
		c, err := parseColor(v)
		if err != nil {
			return err
		}
		st.color = c
	}
	if v, ok := d["fill"]; ok {
		p, err := st.parsePaint(v, st.fill)
		if err != nil {
			return err
		}
		st.fill = p
	}
	if v, ok := d["stroke"]; ok {
		p, err := st.parsePaint(v, st.stroke)
		if err != nil {
			return err
		}
		st.stroke = p
	}
	for name, dst := range map[string]*float64{"fill-opacity": &st.fillOpacity, "stroke-opacity": &st.strokeOpacity} {
		if v, ok := d[name]; ok {
			f, err := parseOpacity(v)
			if err != nil {
				return err
			}
			*dst = f
		}
	}
	if v, ok := d["opacity"]; ok {
		f, err := parseOpacity(v)
		if err != nil {
			return err
		}
		st.opacity *= f
	}
	switch strings.TrimSpace(d["fill-rule"]) {
	case "evenodd":
		st.rule = path.EvenOdd
	case "nonzero":
		st.rule = path.NonZero
	}
	if v, ok := d["stroke-width"]; ok {
		w, err := parseLength(v)
		if err != nil {
			return fmt.Errorf("svgpath: stroke-width: %w", err)
		}
		st.strokeWidth = w
	}
	switch d["stroke-linejoin"] {
	case "round":
		st.join = path.JoinRound
	case "bevel":
		st.join = path.JoinBevel
	case "miter", "miter-clip", "arcs":
		st.join = path.JoinMiter
	}
	switch d["stroke-linecap"] {
	case "round":
		st.cap = path.CapRound
	case "square":
		st.cap = path.CapSquare
	case "butt":
		st.cap = path.CapButt
	}
	if v, ok := d["stroke-miterlimit"]; ok {
		m, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || m < 1 {
			return fmt.Errorf("svgpath: stroke-miterlimit %q is not usable", v)
		}
		st.miter = m
	}
	if v, ok := d["stroke-dasharray"]; ok {
		if strings.TrimSpace(v) == "none" {
			st.dash = nil
		} else {
			list, err := numberList(v)
			if err != nil {
				return fmt.Errorf("svgpath: stroke-dasharray: %w", err)
			}
			st.dash = usableDash(list)
		}
	}
	if v, ok := d["stroke-dashoffset"]; ok {
		o, err := parseLength(v)
		if err != nil {
			return fmt.Errorf("svgpath: stroke-dashoffset: %w", err)
		}
		st.dashOffset = o
	}
	for _, name := range []string{"filter", "mask"} {
		if v := strings.TrimSpace(d[name]); v != "" && v != "none" {
			return fmt.Errorf("%w: %s", ErrUnsupported, name)
		}
	}
	switch strings.TrimSpace(d["display"]) {
	case "none":
		st.hidden = true
	}
	switch strings.TrimSpace(d["visibility"]) {
	case "hidden", "collapse":
		st.hidden = true
	case "visible":
		st.hidden = false
	}
	return nil
}

// parsePaint reads a fill or stroke value. prev is what an inherit keeps
func (st *style) parsePaint(v string, prev paintRef) (paintRef, error) {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "none", "transparent":
		return paintRef{kind: refNone}, nil
	case "inherit":
		return prev, nil
	case "currentcolor":
		return paintRef{kind: refColor, col: st.color}, nil
	}
	if strings.HasPrefix(v, "url(") {
		end := strings.IndexByte(v, ')')
		ref := strings.Trim(strings.TrimSpace(v[4:max(end, 4)]), `'"`)
		if end < 0 || !strings.HasPrefix(ref, "#") {
			return paintRef{}, fmt.Errorf("%w: paint %q", ErrUnsupported, v)
		}
		return paintRef{kind: refURL, id: ref[1:]}, nil
	}
	c, err := parseColor(v)
	if err != nil {
		return paintRef{}, err
	}
	return paintRef{kind: refColor, col: c}, nil
}

// parseOpacity reads a number or percentage, held within 0 to 1
func parseOpacity(v string) (float64, error) {
	v = strings.TrimSpace(v)
	scale := 1.0
	if strings.HasSuffix(v, "%") {
		v, scale = strings.TrimSuffix(v, "%"), 0.01
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) {
		return 0, fmt.Errorf("svgpath: opacity %q is not a number", v)
	}
	return math.Min(1, math.Max(0, f*scale)), nil
}

// clipBox resolves a clip-path to the box it keeps, in icon coordinates. Only a
// plain rectangle is understood, so a clip that is anything else is reported
func (b *builder) clipBox(value string, ctm Affine) (box, error) {
	v := strings.TrimSpace(value)
	if !strings.HasPrefix(v, "url(#") || !strings.HasSuffix(v, ")") {
		return box{}, fmt.Errorf("%w: clip-path %q", ErrUnsupported, value)
	}
	clip := b.doc.ids[strings.Trim(v[5:len(v)-1], `'"`)]
	if clip == nil || clip.name != "clipPath" || len(clip.kids) != 1 {
		return box{}, fmt.Errorf("%w: clip-path %q", ErrUnsupported, value)
	}
	o, err := b.outlineOf(clip.kids[0])
	if err != nil {
		return box{}, err
	}
	if o == nil {
		return box{}, fmt.Errorf("%w: clip-path %q", ErrUnsupported, value)
	}
	if _, ok := o.isAxisRect(); !ok {
		return box{}, fmt.Errorf("%w: a clip-path that is not a rectangle", ErrUnsupported)
	}
	// An axis-aligned rectangle stays one only under a scale and move
	if ctm.B != 0 || ctm.C != 0 {
		return box{}, fmt.Errorf("%w: a rotated clip-path", ErrUnsupported)
	}
	minX, minY, maxX, maxY, _ := o.apply(ctm).bounds()
	return box{float64(minX), float64(minY), float64(maxX), float64(maxY)}, nil
}

// shape builds a drawn shape from a path or basic shape element
func (b *builder) shape(n *node, st style) error {
	if st.hidden {
		return nil
	}
	local, err := b.outlineOf(n)
	if err != nil {
		return err
	}
	if local == nil || local.Empty() {
		return nil
	}
	lx0, ly0, lx1, ly1, ok := local.bounds()
	if !ok {
		return nil
	}
	bounds := Rect{X: lx0, Y: ly0, W: lx1 - lx0, H: ly1 - ly0}

	sh := Shape{Rule: st.rule, ctm: st.ctm}
	if st.fill.kind != refNone {
		src, err := b.source(st.fill, bounds)
		if err != nil {
			return err
		}
		sh.fill = &fillUse{src: src, alpha: st.opacity * st.fillOpacity}
	}
	if st.stroke.kind != refNone && st.strokeWidth > 0 {
		src, err := b.source(st.stroke, bounds)
		if err != nil {
			return err
		}
		sh.stroke = &strokeUse{
			src: src, alpha: st.opacity * st.strokeOpacity, width: st.strokeWidth,
			join: st.join, cap: st.cap, miter: st.miter, dash: st.dash, dashOffset: st.dashOffset,
		}
	}
	if sh.fill == nil && sh.stroke == nil {
		return nil
	}

	sh.outline = local.apply(st.ctm)
	sh.Path = sh.outline.build(1, 1, 0, 0)
	if len(st.clips) > 0 {
		x0, y0, x1, y1, _ := sh.outline.bounds()
		for _, c := range st.clips {
			const eps = 0.01
			if float64(x0) < c.minX-eps || float64(y0) < c.minY-eps || float64(x1) > c.maxX+eps || float64(y1) > c.maxY+eps {
				return fmt.Errorf("%w: a clip-path that cuts into the art", ErrUnsupported)
			}
		}
	}
	if len(b.icon.Shapes) >= maxShapes {
		return fmt.Errorf("svgpath: more than %d shapes", maxShapes)
	}
	b.icon.Shapes = append(b.icon.Shapes, sh)
	return nil
}

// source turns a paint reference into something that can paint. bounds is the
// shape's own box, which a gradient may be laid out against
func (b *builder) source(p paintRef, bounds Rect) (paintSource, error) {
	if p.kind == refColor {
		return solid{c: p.col}, nil
	}
	g, err := b.gradient(p.id, bounds)
	if err != nil {
		return nil, err
	}
	if len(g.stops) == 1 {
		s := g.stops[0]
		c := s.c
		c.A = uint8(math.Round(float64(c.A) * s.opacity))
		return solid{c: c}, nil
	}
	return g, nil
}

// outlineOf builds the outline of a path or basic shape element in its own
// coordinates, nil for one that draws nothing
func (b *builder) outlineOf(n *node) (*outline, error) {
	switch n.name {
	case "path":
		d := n.attr["d"]
		if strings.TrimSpace(d) == "" {
			return nil, nil
		}
		return pathData(d)
	case "rect":
		return rectPath(n.attr)
	case "circle":
		return ellipsePath(n.attr, true)
	case "ellipse":
		return ellipsePath(n.attr, false)
	case "polygon":
		return polygonPath(n.attr)
	}
	return nil, fmt.Errorf("%w: <%s> in a clip-path", ErrUnsupported, n.name)
}

// readViewBox sets the icon's box from the root element's attributes
func (ic *Icon) readViewBox(attr map[string]string) error {
	if viewBox := attr["viewBox"]; viewBox != "" {
		f := strings.FieldsFunc(viewBox, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' })
		if len(f) != 4 {
			return fmt.Errorf("svgpath: viewBox %q does not have four numbers", viewBox)
		}
		var v [4]float64
		for i := range f {
			n, err := parseLength(f[i])
			if err != nil {
				return fmt.Errorf("svgpath: viewBox %q: %w", viewBox, err)
			}
			v[i] = n
		}
		ic.ViewBox = Rect{float32(v[0]), float32(v[1]), float32(v[2]), float32(v[3])}
	} else {
		w, errW := parseLength(attr["width"])
		h, errH := parseLength(attr["height"])
		if errW != nil || errH != nil {
			return errors.New("svgpath: svg has no viewBox and no usable width and height")
		}
		ic.ViewBox = Rect{0, 0, float32(w), float32(h)}
	}
	if ic.ViewBox.W <= 0 || ic.ViewBox.H <= 0 || ic.ViewBox.W > maxCoord || ic.ViewBox.H > maxCoord {
		return fmt.Errorf("svgpath: viewBox size %gx%g is not usable", ic.ViewBox.W, ic.ViewBox.H)
	}
	return nil
}

// parseLength reads a plain number, with an optional px unit
func parseLength(s string) (float64, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "px")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) || math.Abs(v) > maxCoord {
		return 0, fmt.Errorf("%q is not a usable length", s)
	}
	return v, nil
}

// lengthAttr reads an optional length attribute, zero when it is absent
func lengthAttr(attrs map[string]string, name string) (float64, error) {
	v, ok := attrs[name]
	if !ok {
		return 0, nil
	}
	n, err := parseLength(v)
	if err != nil {
		return 0, fmt.Errorf("svgpath: %s: %w", name, err)
	}
	return n, nil
}

func rectPath(attrs map[string]string) (*outline, error) {
	if attrs["rx"] != "" && attrs["rx"] != "0" || attrs["ry"] != "" && attrs["ry"] != "0" {
		return nil, fmt.Errorf("%w: rounded <rect>", ErrUnsupported)
	}
	var v [4]float64
	for i, n := range []string{"x", "y", "width", "height"} {
		var err error
		if v[i], err = lengthAttr(attrs, n); err != nil {
			return nil, err
		}
	}
	if v[2] <= 0 || v[3] <= 0 {
		return nil, nil
	}
	return (&outline{}).Rect(float32(v[0]), float32(v[1]), float32(v[0]+v[2]), float32(v[1]+v[3])), nil
}

func ellipsePath(attrs map[string]string, circle bool) (*outline, error) {
	var v [4]float64
	names := []string{"cx", "cy", "rx", "ry"}
	if circle {
		names = []string{"cx", "cy", "r", "r"}
	}
	for i, n := range names {
		var err error
		if v[i], err = lengthAttr(attrs, n); err != nil {
			return nil, err
		}
	}
	if v[2] <= 0 || v[3] <= 0 {
		return nil, nil
	}
	return (&outline{}).Ellipse(float32(v[0]), float32(v[1]), float32(v[2]), float32(v[3])), nil
}

func polygonPath(attrs map[string]string) (*outline, error) {
	s := &scanner{s: attrs["points"]}
	p := &outline{}
	n := 0
	for {
		s.skipSpace()
		if s.eof() {
			break
		}
		x, err := s.number()
		if err != nil {
			return nil, err
		}
		y, err := s.number()
		if err != nil {
			return nil, err
		}
		if math.Abs(x) > maxCoord || math.Abs(y) > maxCoord {
			return nil, s.errorf("coordinate is out of range")
		}
		if n == 0 {
			p.MoveTo(float32(x), float32(y))
		} else {
			p.LineTo(float32(x), float32(y))
		}
		n++
	}
	if n < 3 {
		return nil, nil
	}
	return p.Close(), nil
}

// extentFactor is how many viewBox widths or heights a shape may stray past the
// box on each side. Icons sit inside their box, and the margin allows for ones
// that overshoot a little. A shape far outside it would be scaled into a
// rasterizer workload that is out of proportion to the icon
const extentFactor = 4

// checkExtent rejects a file whose shapes lie far outside its viewBox
func (ic *Icon) checkExtent() error {
	vb := ic.ViewBox
	loX, hiX := vb.X-extentFactor*vb.W, vb.X+(extentFactor+1)*vb.W
	loY, hiY := vb.Y-extentFactor*vb.H, vb.Y+(extentFactor+1)*vb.H
	for _, s := range ic.Shapes {
		minX, minY, maxX, maxY := s.Path.Bounds()
		if minX < loX || maxX > hiX || minY < loY || maxY > hiY {
			return errors.New("svgpath: a shape lies far outside the viewBox")
		}
	}
	return nil
}

// Bounds is the tight box around everything the icon draws, strokes included, in
// the file's own coordinates. It differs from the ViewBox when the art does not
// fill its box, which is what a caller fitting the symbol to a space wants to
// measure
func (ic *Icon) Bounds() Rect {
	minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY := float32(math.Inf(-1)), float32(math.Inf(-1))
	for _, s := range ic.Shapes {
		x0, y0, x1, y1, ok := s.outline.bounds()
		if !ok {
			continue
		}
		if s.stroke != nil {
			grow := float32(s.stroke.width / 2 * s.ctm.Scale())
			x0, y0, x1, y1 = x0-grow, y0-grow, x1+grow, y1+grow
		}
		minX, minY = min(minX, x0), min(minY, y0)
		maxX, maxY = max(maxX, x1), max(maxY, y1)
	}
	if maxX < minX || maxY < minY {
		return ic.ViewBox
	}
	return Rect{minX, minY, maxX - minX, maxY - minY}
}

// usableDash returns a dash array the stroker can follow, nil to draw a solid
// line. SVG draws a solid line for an array that holds a negative number or sums
// to zero, and a very long array is not one an icon has
func usableDash(list []float64) []float64 {
	var sum float64
	for _, v := range list {
		if v < 0 || math.IsNaN(v) {
			return nil
		}
		sum += v
	}
	if sum <= 0 || len(list) > 16 {
		return nil
	}
	if len(list)%2 == 1 {
		list = append(list[:len(list):len(list)], list...)
	}
	return list
}
