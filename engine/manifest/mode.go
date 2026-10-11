package manifest

import (
	"fmt"
	"strings"

	"github.com/odevine/impasto/blend"
)

// modes are the compositing modes a manifest can name
var modes = map[string]blend.Mode{
	"normal":     blend.Normal,
	"multiply":   blend.Multiply,
	"screen":     blend.Screen,
	"overlay":    blend.Overlay,
	"softlight":  blend.SoftLight,
	"hardlight":  blend.HardLight,
	"colordodge": blend.ColorDodge,
	"colorburn":  blend.ColorBurn,
	"darken":     blend.Darken,
	"lighten":    blend.Lighten,
	"difference": blend.Difference,
	"exclusion":  blend.Exclusion,
	"hue":        blend.Hue,
	"saturation": blend.Saturation,
	"color":      blend.Color,
	"luminosity": blend.Luminosity,
}

// ParseMode maps a manifest's compositing mode name to a mode. An empty name is
// normal, and one the engine does not have is an error so a typo is not silently
// composited the wrong way
func ParseMode(name string) (blend.Mode, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return blend.Normal, nil
	}
	if m, ok := modes[n]; ok {
		return m, nil
	}
	return blend.Normal, fmt.Errorf("unknown blend mode %q", name)
}
