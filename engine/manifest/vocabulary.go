package manifest

import (
	"slices"

	"github.com/odevine/mimic/engine/frame"
)

// VariantKeys lists the keys that can name an image of a part with the slot, in the
// order an editor shows them: "any" first, then single colors, pairs, and the other
// materials. A part with no slot has only "any"
func VariantKeys(slot frame.Slot) []string {
	switch slot {
	case frame.SlotNone:
		return []string{"any"}
	case frame.SlotTransformIcon:
		return append([]string{"any"}, frame.TransformIconNames()...)
	case frame.SlotIndicator:
		return append([]string{"any"}, colorSets()...)
	}
	keys := []string{"any"}
	const letters = "wubrg"
	for i := range letters {
		keys = append(keys, letters[i:i+1])
	}
	for i := range letters {
		for j := i + 1; j < len(letters); j++ {
			keys = append(keys, letters[i:i+1]+letters[j:j+1])
		}
	}
	return append(keys, "gold", "artifact", "colorless", "land", "vehicle")
}

// colorSets lists every non-empty set of the letters wubrg in order, shortest first
func colorSets() []string {
	const letters = "wubrg"
	var out []string
	for mask := 1; mask < 1<<len(letters); mask++ {
		var s []byte
		for i := range letters {
			if mask&(1<<i) != 0 {
				s = append(s, letters[i])
			}
		}
		out = append(out, string(s))
	}
	slices.SortStableFunc(out, func(a, b string) int { return len(a) - len(b) })
	return out
}
