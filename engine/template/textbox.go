package template

import (
	"math"
	"sort"
	"strings"

	"github.com/odevine/mimic/engine/frame"
)

// ClearOf narrows box so it stops short of span, the way a card's name gives
// way to its mana cost. box.ClearGap sets the gap as a fraction of box's own
// FontSize, defaulting to 0.5 when zero or less. It only ever narrows, so a
// box whose span already clears it keeps the width the manifest gave it
func ClearOf(box TextBoxSpec, span TextSpan) TextBoxSpec {
	if span.Empty() {
		return box
	}
	gap := box.ClearGap
	if gap <= 0 {
		gap = 0.5
	}
	room := span.Left - int(math.Round(box.FontSize*gap)) - box.X
	if room < box.Width {
		if box.Width = room; box.Width < 0 {
			box.Width = 0
		}
	}
	return box
}

// MoveToRow sets box's Y to another named text box's Y, so two boxes that
// belong on the same row can be realigned, such as a metadata line ducking
// under a stat box that only some cards draw. Text boxes and their names are
// the standard vocabulary every template's manifest shares, so this is not
// specific to any one frame. It leaves box alone when the manifest carries no
// box by that name, so a template missing the row renders box where the
// manifest put it
func MoveToRow(box TextBoxSpec, m *Manifest, name string) TextBoxSpec {
	if other, ok := m.TextBoxes[name]; ok {
		box.Y = other.Y
	}
	return box
}

// ResolveTextBoxes picks, for each logical box the manifest's specs fill, the
// first spec whose Condition holds for this card, and returns them keyed by
// logical box name. Specs naming more conditions are tried first, then by
// sorted key, so "back,color_indicator" beats "back", and an unconditional
// spec is the fallback its overrides replace. A spec's logical box is its Box,
// or its own key when Box is empty, so a manifest with no Box or Condition
// resolves to itself
func ResolveTextBoxes(boxes map[string]TextBoxSpec, f frame.Keys) map[string]TextBoxSpec {
	keys := make([]string, 0, len(boxes))
	for k := range boxes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ci, cj := conditionCount(boxes[keys[i]].Condition), conditionCount(boxes[keys[j]].Condition)
		if ci != cj {
			return ci > cj
		}
		return keys[i] < keys[j]
	})
	out := make(map[string]TextBoxSpec, len(boxes))
	for _, k := range keys {
		spec := boxes[k]
		name := spec.Box
		if name == "" {
			name = k
		}
		if _, taken := out[name]; taken || !f.ConditionMet(spec.Condition) {
			continue
		}
		out[name] = spec
	}
	return out
}

// conditionCount is how many conditions a comma-separated condition names
func conditionCount(condition string) int {
	if strings.TrimSpace(condition) == "" {
		return 0
	}
	return strings.Count(condition, ",") + 1
}
