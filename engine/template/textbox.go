package template

import "math"

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
