// Package svgpath reads the outlines of a single-colour SVG icon, such as the
// set symbols Scryfall publishes, into impasto paths. It understands a small
// subset of SVG: the viewBox, path, rect, circle, ellipse and polygon shapes,
// plain g nesting, and fill-rule. Colours in the file are ignored, since the
// caller supplies the paint. A file that needs more than that, such as one with
// a transform or a stroke, is reported as unsupported instead of drawn wrong
package svgpath

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/odevine/impasto/path"
)

// MaxBytes is the largest file Parse reads. Real set icons are a few KB, so the
// limit sits far above them and only turns away a file that is not an icon
const MaxBytes = 256 << 10

const (
	maxDepth  = 32
	maxShapes = 1024
)

// ErrUnsupported marks a file that uses SVG features outside the subset above.
// The error that wraps it names the feature
var ErrUnsupported = errors.New("svgpath: unsupported SVG feature")

// Icon is the outlines of one SVG file, in the file's own coordinates
type Icon struct {
	// ViewBox is the file's drawing box, from its viewBox attribute or, when
	// that is missing, its width and height
	ViewBox Rect
	// Shapes are the file's filled shapes, in document order
	Shapes []Shape
}

// Rect is an axis-aligned rectangle given by its corner and size
type Rect struct{ X, Y, W, H float32 }

// Shape is one filled outline and the rule that resolves its overlaps
type Shape struct {
	Path *path.Path
	Rule path.FillRule
}

// Parse reads an SVG icon. It returns an error wrapping ErrUnsupported for a
// feature outside the supported subset, and another error for a file that is
// not well formed SVG
func Parse(data []byte) (*Icon, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("svgpath: file is %d bytes, over the %d byte limit", len(data), MaxBytes)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	// A file that declares a charset other than UTF-8 is read as UTF-8 anyway,
	// since the path data is ASCII and decoding the rest does not matter here
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }

	icon := &Icon{}
	var stack []state
	cur := state{}
	sawRoot := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svgpath: reading SVG: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if !sawRoot {
				if name != "svg" {
					return nil, fmt.Errorf("svgpath: root element is %q, not svg", name)
				}
				sawRoot = true
				if err := icon.readViewBox(t); err != nil {
					return nil, err
				}
				stack = append(stack, cur)
				continue
			}
			if t.Name.Space != "" && t.Name.Space != svgNS {
				// Editor metadata in another namespace draws nothing
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("svgpath: reading SVG: %w", err)
				}
				continue
			}
			if len(stack) >= maxDepth {
				return nil, fmt.Errorf("svgpath: elements nest deeper than %d", maxDepth)
			}
			switch name {
			case "title", "desc", "metadata", "defs":
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("svgpath: reading SVG: %w", err)
				}
			case "g":
				next, err := cur.inherit(t, name)
				if err != nil {
					return nil, err
				}
				stack = append(stack, cur)
				cur = next
			case "path", "rect", "circle", "ellipse", "polygon":
				sh, err := cur.shape(t, name)
				if err != nil {
					return nil, err
				}
				if sh != nil {
					if len(icon.Shapes) >= maxShapes {
						return nil, fmt.Errorf("svgpath: more than %d shapes", maxShapes)
					}
					icon.Shapes = append(icon.Shapes, *sh)
				}
				stack = append(stack, cur)
			default:
				return nil, fmt.Errorf("%w: <%s> element", ErrUnsupported, name)
			}
		case xml.EndElement:
			if len(stack) > 0 {
				cur = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		}
	}
	if !sawRoot {
		return nil, errors.New("svgpath: no svg element")
	}
	if len(icon.Shapes) == 0 {
		return nil, errors.New("svgpath: file draws no shapes")
	}
	if err := icon.checkExtent(); err != nil {
		return nil, err
	}
	return icon, nil
}

const svgNS = "http://www.w3.org/2000/svg"

// state is what a g element passes down to its children
type state struct {
	rule   path.FillRule
	noFill bool // fill is none here and in children that do not set their own
	hidden bool // display is none, so the subtree draws nothing
}

// inherit reads the attributes a group or shape can set, returning the state
// its children see
func (s state) inherit(el xml.StartElement, name string) (state, error) {
	for _, a := range el.Attr {
		if a.Name.Space != "" {
			continue
		}
		switch a.Name.Local {
		case "transform", "style", "clip-path", "mask", "filter":
			return s, fmt.Errorf("%w: %s on <%s>", ErrUnsupported, a.Name.Local, name)
		case "stroke":
			if v := strings.TrimSpace(a.Value); v != "none" && v != "" {
				return s, fmt.Errorf("%w: stroke on <%s>", ErrUnsupported, name)
			}
		case "fill":
			s.noFill = strings.TrimSpace(a.Value) == "none"
		case "display":
			if strings.TrimSpace(a.Value) == "none" {
				s.hidden = true
			}
		case "fill-rule":
			switch strings.TrimSpace(a.Value) {
			case "evenodd":
				s.rule = path.EvenOdd
			case "nonzero":
				s.rule = path.NonZero
			}
		}
	}
	return s, nil
}

// shape builds the outline of one drawing element, or nil for one that draws
// nothing
func (s state) shape(el xml.StartElement, name string) (*Shape, error) {
	s, err := s.inherit(el, name)
	if err != nil {
		return nil, err
	}
	if s.hidden || s.noFill {
		return nil, nil
	}
	attrs := make(map[string]string, len(el.Attr))
	for _, a := range el.Attr {
		if a.Name.Space == "" {
			attrs[a.Name.Local] = a.Value
		}
	}
	var p *path.Path
	switch name {
	case "path":
		d, ok := attrs["d"]
		if !ok || strings.TrimSpace(d) == "" {
			return nil, nil
		}
		if p, err = pathData(d); err != nil {
			return nil, err
		}
	case "rect":
		p, err = rectPath(attrs)
	case "circle":
		p, err = ellipsePath(attrs, true)
	case "ellipse":
		p, err = ellipsePath(attrs, false)
	case "polygon":
		p, err = polygonPath(attrs)
	}
	if err != nil {
		return nil, err
	}
	if p == nil || p.Empty() {
		return nil, nil
	}
	return &Shape{Path: p, Rule: s.rule}, nil
}

// readViewBox sets the icon's box from the root element
func (ic *Icon) readViewBox(el xml.StartElement) error {
	var viewBox, width, height string
	for _, a := range el.Attr {
		if a.Name.Space != "" {
			continue
		}
		switch a.Name.Local {
		case "viewBox":
			viewBox = a.Value
		case "width":
			width = a.Value
		case "height":
			height = a.Value
		}
	}
	if viewBox != "" {
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
		w, errW := parseLength(width)
		h, errH := parseLength(height)
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

func rectPath(attrs map[string]string) (*path.Path, error) {
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
	return path.New().Rect(float32(v[0]), float32(v[1]), float32(v[0]+v[2]), float32(v[1]+v[3])), nil
}

func ellipsePath(attrs map[string]string, circle bool) (*path.Path, error) {
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
	return path.New().Ellipse(float32(v[0]), float32(v[1]), float32(v[2]), float32(v[3])), nil
}

func polygonPath(attrs map[string]string) (*path.Path, error) {
	s := &scanner{s: attrs["points"]}
	p := path.New()
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
