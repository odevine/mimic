package svgpath

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// namedColors are the color keywords the icons use or plausibly would. A
// keyword outside this list is reported as unsupported instead of guessed
var namedColors = map[string]color.NRGBA{
	"black":   {0, 0, 0, 255},
	"white":   {255, 255, 255, 255},
	"red":     {255, 0, 0, 255},
	"green":   {0, 128, 0, 255},
	"blue":    {0, 0, 255, 255},
	"yellow":  {255, 255, 0, 255},
	"orange":  {255, 165, 0, 255},
	"gray":    {128, 128, 128, 255},
	"grey":    {128, 128, 128, 255},
	"silver":  {192, 192, 192, 255},
	"gold":    {255, 215, 0, 255},
	"purple":  {128, 0, 128, 255},
	"brown":   {165, 42, 42, 255},
	"maroon":  {128, 0, 0, 255},
	"navy":    {0, 0, 128, 255},
	"teal":    {0, 128, 128, 255},
	"lime":    {0, 255, 0, 255},
	"aqua":    {0, 255, 255, 255},
	"cyan":    {0, 255, 255, 255},
	"fuchsia": {255, 0, 255, 255},
	"magenta": {255, 0, 255, 255},
}

// parseColor reads a CSS color: #rgb, #rrggbb, #rgba, #rrggbbaa, rgb(), rgba(), or
// one of the keywords above
func parseColor(s string) (color.NRGBA, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := namedColors[s]; ok {
		return c, nil
	}
	if strings.HasPrefix(s, "#") {
		return parseHexColor(s[1:])
	}
	if strings.HasPrefix(s, "rgb") {
		return parseFuncColor(s)
	}
	return color.NRGBA{}, fmt.Errorf("%w: color %q", ErrUnsupported, s)
}

func parseHexColor(h string) (color.NRGBA, error) {
	switch len(h) {
	case 3, 4:
		var e strings.Builder
		for _, r := range h {
			e.WriteRune(r)
			e.WriteRune(r)
		}
		h = e.String()
	case 6, 8:
	default:
		return color.NRGBA{}, fmt.Errorf("svgpath: color #%s is not a hex color", h)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("svgpath: color #%s is not a hex color", h)
	}
	if len(h) == 6 {
		return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
	}
	return color.NRGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}, nil
}

func parseFuncColor(s string) (color.NRGBA, error) {
	open, closeIdx := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || closeIdx < open {
		return color.NRGBA{}, fmt.Errorf("svgpath: color %q is malformed", s)
	}
	parts := strings.FieldsFunc(s[open+1:closeIdx], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
	if len(parts) != 3 && len(parts) != 4 {
		return color.NRGBA{}, fmt.Errorf("svgpath: color %q is malformed", s)
	}
	var v [4]float64
	v[3] = 1
	for i, p := range parts {
		pct := strings.HasSuffix(p, "%")
		f, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
		if err != nil {
			return color.NRGBA{}, fmt.Errorf("svgpath: color %q is malformed", s)
		}
		switch {
		case i == 3 && pct:
			v[i] = f / 100
		case i == 3:
			v[i] = f
		case pct:
			v[i] = f * 255 / 100
		default:
			v[i] = f
		}
	}
	return color.NRGBA{clamp8(v[0]), clamp8(v[1]), clamp8(v[2]), clamp8(v[3] * 255)}, nil
}

func clamp8(f float64) uint8 {
	switch {
	case f < 0:
		return 0
	case f > 255:
		return 255
	}
	return uint8(f + 0.5)
}
