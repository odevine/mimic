// Package normal renders a card with the Normal frame. It owns the logic that
// is specific to this frame: how a card's color identity and type line pick a
// layer variant, and which layer conditions apply. A second template is free to
// define its own color-key vocabulary without this package constraining it.
package normal

import (
	"strings"

	"github.com/odevine/mimic/engine/card"
)

// ResolveColorKey picks the color key a card's layer variants are looked up by.
// Land and artifact resolve by type line before color, because Magic draws
// those as their own frame styles regardless of color identity
func ResolveColorKey(d *card.Data) string {
	typeLine := strings.ToLower(d.TypeLine)
	switch {
	case strings.Contains(typeLine, "land"):
		return "land"
	case len(d.ColorIdentity) == 0:
		if strings.Contains(typeLine, "artifact") {
			return "artifact"
		}
		return "colorless"
	case len(d.ColorIdentity) == 1:
		return strings.ToLower(string(d.ColorIdentity[0]))
	default:
		return "gold"
	}
}

// IsLegendary reports whether the type line marks the card legendary
func IsLegendary(d *card.Data) bool {
	return strings.Contains(strings.ToLower(d.TypeLine), "legendary")
}

// conditionMet reports whether a layer's condition applies to this card. The
// default case compares the condition against the color key, which is the
// escape hatch that lets a manifest extend the vocabulary without code changes
func conditionMet(condition, colorKey string, legendary bool) bool {
	switch condition {
	case "":
		return true
	case "legendary":
		return legendary
	case "nonlegendary":
		return !legendary
	case "land":
		return colorKey == "land"
	case "nonland":
		return colorKey != "land"
	default:
		return condition == colorKey
	}
}
