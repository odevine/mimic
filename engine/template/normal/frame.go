package normal

import "github.com/odevine/mimic/engine/frame"

// conditionMet reports whether a layer's condition applies. Conditions the
// engine does not drive yet (nyx, companion, hollow_crown, fullart,
// color_indicator, divider, pt_dark) render off through the default case.
func conditionMet(f frame.Keys, condition string) bool {
	switch condition {
	case "":
		return true
	case "land":
		return f.Land
	case "nonland":
		return !f.Land
	case "legendary":
		return f.Legendary
	case "nonlegendary":
		return !f.Legendary
	case "creature":
		return f.Creature
	default:
		return false
	}
}

// keyForLayer returns the variant key a named layer looks up, mapping a Normal
// PSD layer name onto the frame's generic slots. Layers with only an "any"
// variant (border, shadows, divider) get "" and fall back to it.
func keyForLayer(f frame.Keys, name string) string {
	switch name {
	case "background":
		return f.Background
	case "pinlines_textbox", "land_pinlines_textbox":
		return f.Pinlines
	case "legendary_crown":
		return f.Crown
	case "name_title_boxes":
		return f.Twins
	case "pt_box", "pt_box_dark":
		return f.PTBox
	default:
		return ""
	}
}
