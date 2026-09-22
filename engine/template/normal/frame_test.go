package normal

import (
	"testing"

	"github.com/odevine/mimic/engine/frame"
)

func TestConditionMet(t *testing.T) {
	f := frame.Keys{Land: true, Legendary: true, Creature: true}
	yes := []string{"", "land", "legendary", "creature"}
	no := []string{"nonland", "nonlegendary", "nyx", "companion", "fullart", "pt_dark", "color_indicator"}
	for _, c := range yes {
		if !conditionMet(f, c) {
			t.Errorf("conditionMet(%q) = false, want true", c)
		}
	}
	for _, c := range no {
		if conditionMet(f, c) {
			t.Errorf("conditionMet(%q) = true, want false", c)
		}
	}
}

func TestKeyForLayer(t *testing.T) {
	f := frame.Keys{Background: "gold", Pinlines: "wg", Twins: "colorless", PTBox: "colorless", Crown: "wg"}
	cases := map[string]string{
		"background":            "gold",
		"pinlines_textbox":      "wg",
		"land_pinlines_textbox": "wg",
		"legendary_crown":       "wg",
		"name_title_boxes":      "colorless",
		"pt_box":                "colorless",
		"border":                "", // any-only layers fall back
	}
	for layer, want := range cases {
		if got := keyForLayer(f, layer); got != want {
			t.Errorf("keyForLayer(%q) = %q, want %q", layer, got, want)
		}
	}
}
