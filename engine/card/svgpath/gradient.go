package svgpath

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const maxHrefChain = 16

// gradient resolves the linear or radial gradient with the given id, following
// href links for the attributes and stops it inherits. bounds is the box of the
// shape being painted, used when the gradient is laid out against it
func (b *builder) gradient(id string, bounds Rect) (*gradient, error) {
	start := b.doc.ids[id]
	if start == nil || (start.name != "linearGradient" && start.name != "radialGradient") {
		return nil, fmt.Errorf("%w: paint server %q", ErrUnsupported, id)
	}
	// chain is the gradient and each one it links to, nearest first
	chain := []*node{start}
	for cur := start; len(chain) <= maxHrefChain; {
		next := b.doc.ids[cur.hrefID()]
		if next == nil || (next.name != "linearGradient" && next.name != "radialGradient") {
			break
		}
		for _, seen := range chain {
			if seen == next {
				return nil, fmt.Errorf("svgpath: gradient %q links to itself", id)
			}
		}
		chain = append(chain, next)
		cur = next
	}
	attr := func(name string) (string, bool) {
		for _, n := range chain {
			if v, ok := n.attr[name]; ok {
				return v, true
			}
		}
		return "", false
	}

	g := &gradient{radial: start.name == "radialGradient", bounds: bounds, transform: Identity}
	units, _ := attr("gradientUnits")
	g.bbox = strings.TrimSpace(units) != "userSpaceOnUse"
	if v, ok := attr("gradientTransform"); ok {
		t, err := parseTransform(v)
		if err != nil {
			return nil, err
		}
		g.transform = t
	}
	switch v, _ := attr("spreadMethod"); strings.TrimSpace(v) {
	case "reflect":
		g.spread = spreadReflect
	case "repeat":
		g.spread = spreadRepeat
	}

	// coord reads a coordinate, a number or a percentage, with a default
	// for one the chain never sets. Percentages are of the box in bounding box
	// units and of the viewBox otherwise
	coord := func(name string, def float64, vertical bool) (float64, error) {
		v, ok := attr(name)
		if !ok {
			return def, nil
		}
		v = strings.TrimSpace(v)
		if pct, isPct := strings.CutSuffix(v, "%"); isPct {
			f, err := strconv.ParseFloat(pct, 64)
			if err != nil {
				return 0, fmt.Errorf("svgpath: gradient %s %q is not a number", name, v)
			}
			if g.bbox {
				return f / 100, nil
			}
			if vertical {
				return f / 100 * float64(b.icon.ViewBox.H), nil
			}
			return f / 100 * float64(b.icon.ViewBox.W), nil
		}
		f, err := parseLength(v)
		if err != nil {
			return 0, fmt.Errorf("svgpath: gradient %s: %w", name, err)
		}
		return f, nil
	}
	full := func(vertical bool) float64 {
		if g.bbox {
			return 1
		}
		if vertical {
			return float64(b.icon.ViewBox.H)
		}
		return float64(b.icon.ViewBox.W)
	}

	var err error
	if g.radial {
		half := full(false) / 2
		if g.cx, err = coord("cx", half, false); err != nil {
			return nil, err
		}
		if g.cy, err = coord("cy", full(true)/2, true); err != nil {
			return nil, err
		}
		if g.r, err = coord("r", half, false); err != nil {
			return nil, err
		}
		if g.fx, err = coord("fx", g.cx, false); err != nil {
			return nil, err
		}
		if g.fy, err = coord("fy", g.cy, true); err != nil {
			return nil, err
		}
	} else {
		if g.x1, err = coord("x1", 0, false); err != nil {
			return nil, err
		}
		if g.y1, err = coord("y1", 0, true); err != nil {
			return nil, err
		}
		if g.x2, err = coord("x2", full(false), false); err != nil {
			return nil, err
		}
		if g.y2, err = coord("y2", 0, true); err != nil {
			return nil, err
		}
	}

	for _, n := range chain {
		stops, err := b.stops(n)
		if err != nil {
			return nil, err
		}
		if len(stops) > 0 {
			g.stops = stops
			break
		}
	}
	if len(g.stops) == 0 {
		return nil, fmt.Errorf("%w: gradient %q has no stops", ErrUnsupported, id)
	}
	return g, nil
}

// stops reads a gradient's own stop elements, each offset held at or past the
// one before it
func (b *builder) stops(n *node) ([]stop, error) {
	var out []stop
	prev := 0.0
	for _, k := range n.kids {
		if k.name != "stop" {
			continue
		}
		off, err := parseOffset(k.attr["offset"])
		if err != nil {
			return nil, err
		}
		off = math.Min(1, math.Max(prev, off))
		prev = off
		s := stop{offset: off, opacity: 1}
		d := b.doc.computed(k)
		if v, ok := d["stop-color"]; ok {
			if strings.EqualFold(strings.TrimSpace(v), "currentcolor") {
				v = "black"
			}
			if s.c, err = parseColor(v); err != nil {
				return nil, err
			}
		} else {
			s.c.A = 255
		}
		if v, ok := d["stop-opacity"]; ok {
			if s.opacity, err = parseOpacity(v); err != nil {
				return nil, err
			}
		}
		out = append(out, s)
	}
	if len(out) > 64 {
		return nil, fmt.Errorf("svgpath: gradient has %d stops", len(out))
	}
	return out, nil
}

// parseOffset reads a stop offset, a number or a percentage, zero when absent
func parseOffset(v string) (float64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	scale := 1.0
	if strings.HasSuffix(v, "%") {
		v, scale = strings.TrimSuffix(v, "%"), 0.01
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("svgpath: stop offset %q is not a number", v)
	}
	return f * scale, nil
}
